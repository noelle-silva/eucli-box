package placeholder

import (
	"context"
	"path/filepath"
	"strings"

	"eucli-box/pkg/datapaths"
	"eucli-box/pkg/types"
)

type System interface {
	LoadPlaceholderLibrary(ctx context.Context) (types.PlaceholderLibrary, error)
	SavePlaceholderLibrary(ctx context.Context, library types.PlaceholderLibrary) (types.PlaceholderLibrary, error)
	ResolveText(ctx context.Context, text string) (types.PlaceholderResolveResult, error)
	ResolvePromptMessages(ctx context.Context, messages []types.PromptMessage) ([]types.PromptMessage, error)
	Problems(ctx context.Context) ([]types.PlaceholderProblem, error)
	DependencyTree(ctx context.Context, name string) (types.PlaceholderDependencyNode, error)
}

// SystemPluginSystem 是占位符系统对系统插件的窄视图：按来源点名取值与静态核对。
type SystemPluginSystem interface {
	ResolvePlaceholderValues(ctx context.Context, sources []types.SystemPluginPlaceholderSource) ([]types.SystemPluginPlaceholderValue, []types.PlaceholderProblem)
	PlaceholderSourceProblems(ctx context.Context, sources []types.SystemPluginPlaceholderSource) []types.PlaceholderProblem
}

type Config struct {
	RootDir       string
	SystemPlugins SystemPluginSystem
}

type system struct {
	libraryFile   string
	systemPlugins SystemPluginSystem
}

func NewSystem(config Config) (System, error) {
	root := strings.TrimSpace(config.RootDir)
	if root == "" {
		root = "data"
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, placeholderInvalid("failed to resolve root directory", err)
	}
	return &system{libraryFile: datapaths.PlaceholdersFile(filepath.Clean(abs)), systemPlugins: config.SystemPlugins}, nil
}

func (s *system) ResolveText(ctx context.Context, text string) (types.PlaceholderResolveResult, error) {
	library, err := s.LoadPlaceholderLibrary(ctx)
	if err != nil {
		return types.PlaceholderResolveResult{}, err
	}
	library, problems := s.withDynamicValues(ctx, library, NamesInText(text))
	result := Resolve(text, library)
	result.Problems = append(result.Problems, problems...)
	return result, nil
}

func (s *system) ResolvePromptMessages(ctx context.Context, messages []types.PromptMessage) ([]types.PromptMessage, error) {
	library, err := s.LoadPlaceholderLibrary(ctx)
	if err != nil {
		return nil, err
	}
	seeds := []string{}
	for _, message := range messages {
		seeds = append(seeds, NamesInText(message.Content)...)
	}
	library, _ = s.withDynamicValues(ctx, library, seeds)
	out := make([]types.PromptMessage, len(messages))
	copy(out, messages)
	for index := range out {
		out[index].Content = Resolve(out[index].Content, library).Text
	}
	return out, nil
}

func (s *system) Problems(ctx context.Context) ([]types.PlaceholderProblem, error) {
	library, err := s.LoadPlaceholderLibrary(ctx)
	if err != nil {
		return nil, err
	}
	problems := Problems(library)
	if s.systemPlugins != nil {
		problems = append(problems, s.systemPlugins.PlaceholderSourceProblems(ctx, pluginSourcesInLibrary(library))...)
	}
	return problems, nil
}

func (s *system) DependencyTree(ctx context.Context, name string) (types.PlaceholderDependencyNode, error) {
	library, err := s.LoadPlaceholderLibrary(ctx)
	if err != nil {
		return types.PlaceholderDependencyNode{}, err
	}
	return DependencyTree(name, library), nil
}

// withDynamicValues 按来源点名取得插件值并注入库副本。
// 只请求被引用来源；若插件返回值又引入新的占位符引用，下一轮继续点名，最多 maxDynamicValueRounds 轮。
func (s *system) withDynamicValues(ctx context.Context, library types.PlaceholderLibrary, seedNames []string) (types.PlaceholderLibrary, []types.PlaceholderProblem) {
	if s.systemPlugins == nil {
		return library, nil
	}
	fetched := map[string]struct{}{}
	problems := []types.PlaceholderProblem{}
	for round := 0; round < maxDynamicValueRounds; round++ {
		pending := []types.SystemPluginPlaceholderSource{}
		for _, source := range referencedPluginSources(library, seedNames) {
			key := pluginSourceKey(source.PluginID, source.InterfaceID)
			if _, done := fetched[key]; done {
				continue
			}
			fetched[key] = struct{}{}
			pending = append(pending, source)
		}
		if len(pending) == 0 {
			break
		}
		values, roundProblems := s.systemPlugins.ResolvePlaceholderValues(ctx, pending)
		problems = append(problems, roundProblems...)
		if len(values) == 0 {
			continue
		}
		library = injectPluginValues(library, values)
	}
	return library, problems
}

// injectPluginValues 把插件返回值写回库副本；值只在本次解析内生效。
func injectPluginValues(library types.PlaceholderLibrary, values []types.SystemPluginPlaceholderValue) types.PlaceholderLibrary {
	bySource := map[string]types.SystemPluginPlaceholderValue{}
	for _, value := range values {
		bySource[pluginSourceKey(value.PluginID, value.InterfaceID)] = value
	}
	for index := range library.Placeholders {
		source := library.Placeholders[index].Source
		if source == nil || source.Kind != types.PlaceholderSourceSystemPlugin {
			continue
		}
		if value, ok := bySource[pluginSourceKey(source.PluginID, source.InterfaceID)]; ok {
			library.Placeholders[index].Name = value.Name
			library.Placeholders[index].Value = value.Value
		}
	}
	return library
}

func pluginSourceKey(pluginID string, interfaceID string) string {
	return strings.TrimSpace(pluginID) + "\x00" + strings.TrimSpace(interfaceID)
}
