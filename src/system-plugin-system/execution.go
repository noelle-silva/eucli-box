package systemplugin

import (
	"context"
	"strings"
	"sync"

	"eucli-box/pkg/systemplugin"
	"eucli-box/pkg/types"
)

// placeholderSourceGroup 是一次定向派发的分组：同一插件的被引用接口合并成一次调用。
type placeholderSourceGroup struct {
	pluginID string
	sources  []types.SystemPluginPlaceholderSource
}

// ResolvePlaceholderValues 按来源点名取值：只联系被引用占位符的所属插件与接口，
// 未引用的插件不做任何调用。不同插件之间并行，单插件失败不阻断其他插件。
func (s *system) ResolvePlaceholderValues(ctx context.Context, sources []types.SystemPluginPlaceholderSource) ([]types.SystemPluginPlaceholderValue, []types.PlaceholderProblem) {
	index, err := s.discover(ctx)
	if err != nil {
		return nil, nil
	}
	groups := groupPlaceholderSources(sources)
	type groupResult struct {
		position int
		values   []types.SystemPluginPlaceholderValue
		problems []types.PlaceholderProblem
	}
	results := make(chan groupResult, len(groups))
	var wait sync.WaitGroup
	for position, group := range groups {
		wait.Add(1)
		go func(position int, group placeholderSourceGroup) {
			defer wait.Done()
			values, problems := s.resolveGroup(ctx, index, group)
			results <- groupResult{position: position, values: values, problems: problems}
		}(position, group)
	}
	wait.Wait()
	close(results)
	ordered := make([]groupResult, len(groups))
	for result := range results {
		ordered[result.position] = result
	}
	values := []types.SystemPluginPlaceholderValue{}
	problems := []types.PlaceholderProblem{}
	for _, result := range ordered {
		values = append(values, result.values...)
		problems = append(problems, result.problems...)
	}
	return values, problems
}

// PlaceholderSourceProblems 只做静态事实核对：所属缺失、接口未声明、插件停用或不可用；
// 不启动任何插件进程，不产生真实调用。
func (s *system) PlaceholderSourceProblems(ctx context.Context, sources []types.SystemPluginPlaceholderSource) []types.PlaceholderProblem {
	index, err := s.discover(ctx)
	if err != nil {
		return nil
	}
	problems := []types.PlaceholderProblem{}
	for _, source := range sources {
		pluginID := strings.TrimSpace(source.PluginID)
		interfaceID := strings.TrimSpace(source.InterfaceID)
		if pluginID == "" || interfaceID == "" {
			continue
		}
		record, ok := index.find(pluginID)
		if !ok {
			problems = append(problems, types.PlaceholderProblem{Name: source.Name, Type: types.PlaceholderProblemPluginFailed})
			continue
		}
		item, declared := record.declaredPlaceholderInterface(interfaceID)
		if !declared {
			problems = append(problems, types.PlaceholderProblem{Name: source.Name, Type: types.PlaceholderProblemPluginFailed})
			continue
		}
		switch {
		case !record.enabled:
			problems = append(problems, types.PlaceholderProblem{Name: record.effectiveName(item), Type: types.PlaceholderProblemPluginDisabled})
		case record.status != types.SystemPluginStatusActive:
			problems = append(problems, types.PlaceholderProblem{Name: record.effectiveName(item), Type: types.PlaceholderProblemPluginFailed})
		}
	}
	return problems
}

func groupPlaceholderSources(sources []types.SystemPluginPlaceholderSource) []placeholderSourceGroup {
	groupPositions := map[string]int{}
	groups := []placeholderSourceGroup{}
	seen := map[string]struct{}{}
	for _, source := range sources {
		pluginID := strings.TrimSpace(source.PluginID)
		interfaceID := strings.TrimSpace(source.InterfaceID)
		if pluginID == "" || interfaceID == "" {
			continue
		}
		key := placeholderOwnerKey(pluginID, interfaceID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		position, ok := groupPositions[pluginID]
		if !ok {
			position = len(groups)
			groupPositions[pluginID] = position
			groups = append(groups, placeholderSourceGroup{pluginID: pluginID})
		}
		groups[position].sources = append(groups[position].sources, types.SystemPluginPlaceholderSource{PluginID: pluginID, InterfaceID: interfaceID, Name: strings.TrimSpace(source.Name)})
	}
	return groups
}

func (s *system) resolveGroup(ctx context.Context, index *pluginIndex, group placeholderSourceGroup) ([]types.SystemPluginPlaceholderValue, []types.PlaceholderProblem) {
	record, ok := index.find(group.pluginID)
	if !ok {
		return nil, failedProblemsForSources(group.sources)
	}
	requested := make([]types.SystemPluginPlaceholderSource, 0, len(group.sources))
	problems := []types.PlaceholderProblem{}
	for _, source := range group.sources {
		if _, declared := record.declaredPlaceholderInterface(source.InterfaceID); !declared {
			problems = append(problems, types.PlaceholderProblem{Name: source.Name, Type: types.PlaceholderProblemPluginFailed})
			continue
		}
		requested = append(requested, source)
	}
	if len(requested) == 0 {
		return nil, problems
	}
	disabled, blocked := s.acquireServing(record)
	if disabled {
		return nil, append(problems, problemsForSources(record, requested, types.PlaceholderProblemPluginDisabled)...)
	}
	if blocked != "" {
		s.setFailure(record.manifest.ID, blocked)
		return nil, append(problems, problemsForSources(record, requested, types.PlaceholderProblemPluginFailed)...)
	}
	defer s.releaseServing(record.manifest.ID)

	interfaceIDs := make([]string, 0, len(requested))
	for _, source := range requested {
		interfaceIDs = append(interfaceIDs, source.InterfaceID)
	}
	message, err := s.invokeCapability(ctx, record, systemplugin.CapabilityPlaceholderValues, interfaceIDs)
	if err != nil {
		s.setFailure(record.manifest.ID, err.Error())
		return nil, append(problems, problemsForSources(record, requested, types.PlaceholderProblemPluginFailed)...)
	}
	if message.Status != systemplugin.StatusSuccess {
		s.setFailure(record.manifest.ID, nonEmpty(message.Error, "系统插件返回失败状态"))
		return nil, append(problems, problemsForSources(record, requested, types.PlaceholderProblemPluginFailed)...)
	}
	s.setFailure(record.manifest.ID, "")
	values := make([]types.SystemPluginPlaceholderValue, 0, len(requested))
	for _, source := range requested {
		value, ok := message.Values[source.InterfaceID]
		if !ok {
			continue
		}
		item, _ := record.declaredPlaceholderInterface(source.InterfaceID)
		values = append(values, types.SystemPluginPlaceholderValue{
			PluginID:    record.manifest.ID,
			InterfaceID: source.InterfaceID,
			Name:        record.effectiveName(item),
			Value:       value,
		})
	}
	return values, problems
}

func problemsForSources(record pluginRecord, sources []types.SystemPluginPlaceholderSource, problemType string) []types.PlaceholderProblem {
	out := make([]types.PlaceholderProblem, 0, len(sources))
	for _, source := range sources {
		name := strings.TrimSpace(source.Name)
		if item, ok := record.declaredPlaceholderInterface(source.InterfaceID); ok {
			name = record.effectiveName(item)
		}
		out = append(out, types.PlaceholderProblem{Name: name, Type: problemType})
	}
	return out
}

func failedProblemsForSources(sources []types.SystemPluginPlaceholderSource) []types.PlaceholderProblem {
	out := make([]types.PlaceholderProblem, 0, len(sources))
	for _, source := range sources {
		out = append(out, types.PlaceholderProblem{Name: strings.TrimSpace(source.Name), Type: types.PlaceholderProblemPluginFailed})
	}
	return out
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
