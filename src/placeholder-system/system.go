package placeholder

import (
	"context"
	"path/filepath"
	"strings"

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

type SystemPluginSystem interface {
	ResolvePlaceholderValues(ctx context.Context) ([]types.SystemPluginPlaceholderValue, []types.PlaceholderProblem)
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
	return &system{libraryFile: filepath.Join(filepath.Clean(abs), "meta", "placeholders.json"), systemPlugins: config.SystemPlugins}, nil
}

func (s *system) ResolveText(ctx context.Context, text string) (types.PlaceholderResolveResult, error) {
	library, err := s.LoadPlaceholderLibrary(ctx)
	if err != nil {
		return types.PlaceholderResolveResult{}, err
	}
	library, problems := s.withDynamicValues(ctx, library)
	result := Resolve(text, library)
	result.Problems = append(result.Problems, problems...)
	return result, nil
}

func (s *system) ResolvePromptMessages(ctx context.Context, messages []types.PromptMessage) ([]types.PromptMessage, error) {
	library, err := s.LoadPlaceholderLibrary(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]types.PromptMessage, len(messages))
	copy(out, messages)
	library, _ = s.withDynamicValues(ctx, library)
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
	library, pluginProblems := s.withDynamicValues(ctx, library)
	return append(Problems(library), pluginProblems...), nil
}

func (s *system) DependencyTree(ctx context.Context, name string) (types.PlaceholderDependencyNode, error) {
	library, err := s.LoadPlaceholderLibrary(ctx)
	if err != nil {
		return types.PlaceholderDependencyNode{}, err
	}
	return DependencyTree(name, library), nil
}

func (s *system) withDynamicValues(ctx context.Context, library types.PlaceholderLibrary) (types.PlaceholderLibrary, []types.PlaceholderProblem) {
	if s.systemPlugins == nil {
		return library, nil
	}
	values, problems := s.systemPlugins.ResolvePlaceholderValues(ctx)
	if len(values) == 0 {
		return library, problems
	}
	valueBySource := map[string]types.SystemPluginPlaceholderValue{}
	for _, value := range values {
		valueBySource[pluginSourceKey(value.PluginID, value.InterfaceID)] = value
	}
	for index := range library.Placeholders {
		source := library.Placeholders[index].Source
		if source == nil || source.Kind != types.PlaceholderSourceSystemPlugin {
			continue
		}
		if value, ok := valueBySource[pluginSourceKey(source.PluginID, source.InterfaceID)]; ok {
			library.Placeholders[index].Name = value.Name
			library.Placeholders[index].Value = value.Value
		}
	}
	return library, problems
}

func pluginSourceKey(pluginID string, interfaceID string) string {
	return strings.TrimSpace(pluginID) + "\x00" + strings.TrimSpace(interfaceID)
}
