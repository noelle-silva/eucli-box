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

type Config struct {
	RootDir string
}

type system struct {
	libraryFile string
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
	return &system{libraryFile: filepath.Join(filepath.Clean(abs), "meta", "placeholders.json")}, nil
}

func (s *system) ResolveText(ctx context.Context, text string) (types.PlaceholderResolveResult, error) {
	library, err := s.LoadPlaceholderLibrary(ctx)
	if err != nil {
		return types.PlaceholderResolveResult{}, err
	}
	return Resolve(text, library), nil
}

func (s *system) ResolvePromptMessages(ctx context.Context, messages []types.PromptMessage) ([]types.PromptMessage, error) {
	library, err := s.LoadPlaceholderLibrary(ctx)
	if err != nil {
		return nil, err
	}
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
	return Problems(library), nil
}

func (s *system) DependencyTree(ctx context.Context, name string) (types.PlaceholderDependencyNode, error) {
	library, err := s.LoadPlaceholderLibrary(ctx)
	if err != nil {
		return types.PlaceholderDependencyNode{}, err
	}
	return DependencyTree(name, library), nil
}
