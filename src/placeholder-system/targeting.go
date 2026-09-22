package placeholder

import (
	"strings"

	"eucli-box/pkg/types"
)

// maxDynamicValueRounds 限定按引用扩展的取值轮数：
// 第一轮覆盖静态引用，后续轮覆盖插件返回值中新引入的引用；上限保证不会往复。
const maxDynamicValueRounds = 4

// referencedPluginSources 返回从种子名字出发、沿占位符值递归可达的插件来源。
// 没被引用的插件与接口不会出现在结果里；重名条目无法解析，不产生来源。
func referencedPluginSources(library types.PlaceholderLibrary, seedNames []string) []types.SystemPluginPlaceholderSource {
	byName := map[string]types.PlaceholderItem{}
	counts := map[string]int{}
	for _, item := range library.Placeholders {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		item.Name = name
		counts[name]++
		if _, exists := byName[name]; !exists {
			byName[name] = item
		}
	}
	visited := map[string]struct{}{}
	seenSource := map[string]struct{}{}
	sources := []types.SystemPluginPlaceholderSource{}
	var walk func(name string)
	walk = func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, done := visited[name]; done {
			return
		}
		visited[name] = struct{}{}
		item, ok := byName[name]
		if !ok || counts[name] != 1 {
			return
		}
		if source := item.Source; source != nil && source.Kind == types.PlaceholderSourceSystemPlugin {
			pluginID := strings.TrimSpace(source.PluginID)
			interfaceID := strings.TrimSpace(source.InterfaceID)
			if pluginID != "" && interfaceID != "" {
				key := pluginSourceKey(pluginID, interfaceID)
				if _, exists := seenSource[key]; !exists {
					seenSource[key] = struct{}{}
					sources = append(sources, types.SystemPluginPlaceholderSource{PluginID: pluginID, InterfaceID: interfaceID, Name: name})
				}
			}
		}
		for _, child := range NamesInText(item.Value) {
			walk(child)
		}
	}
	for _, name := range seedNames {
		walk(name)
	}
	return sources
}

// pluginSourcesInLibrary 返回库中全部插件来源，用于不携带文本的静态核对。
func pluginSourcesInLibrary(library types.PlaceholderLibrary) []types.SystemPluginPlaceholderSource {
	seen := map[string]struct{}{}
	sources := []types.SystemPluginPlaceholderSource{}
	for _, item := range library.Placeholders {
		source := item.Source
		if source == nil || source.Kind != types.PlaceholderSourceSystemPlugin {
			continue
		}
		pluginID := strings.TrimSpace(source.PluginID)
		interfaceID := strings.TrimSpace(source.InterfaceID)
		if pluginID == "" || interfaceID == "" {
			continue
		}
		key := pluginSourceKey(pluginID, interfaceID)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		sources = append(sources, types.SystemPluginPlaceholderSource{PluginID: pluginID, InterfaceID: interfaceID, Name: strings.TrimSpace(item.Name)})
	}
	return sources
}
