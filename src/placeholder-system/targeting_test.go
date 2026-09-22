package placeholder

import (
	"context"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

type fakePluginSystem struct {
	requests   [][]types.SystemPluginPlaceholderSource
	values     map[string]string
	problems   []types.PlaceholderProblem
	staticHits int
}

func (f *fakePluginSystem) ResolvePlaceholderValues(_ context.Context, sources []types.SystemPluginPlaceholderSource) ([]types.SystemPluginPlaceholderValue, []types.PlaceholderProblem) {
	f.requests = append(f.requests, sources)
	values := []types.SystemPluginPlaceholderValue{}
	for _, source := range sources {
		if value, ok := f.values[pluginSourceKey(source.PluginID, source.InterfaceID)]; ok {
			values = append(values, types.SystemPluginPlaceholderValue{PluginID: source.PluginID, InterfaceID: source.InterfaceID, Name: source.Name, Value: value})
		}
	}
	return values, nil
}

func (f *fakePluginSystem) PlaceholderSourceProblems(_ context.Context, _ []types.SystemPluginPlaceholderSource) []types.PlaceholderProblem {
	f.staticHits++
	return f.problems
}

func pluginPlaceholder(name string, pluginID string, interfaceID string) types.PlaceholderItem {
	return types.PlaceholderItem{Name: name, Source: &types.PlaceholderSource{Kind: types.PlaceholderSourceSystemPlugin, PluginID: pluginID, InterfaceID: interfaceID}}
}

func TestResolveTextOnlyRequestsReferencedSources(t *testing.T) {
	fake := &fakePluginSystem{values: map[string]string{
		pluginSourceKey("plugin-a", "value"): "A",
		pluginSourceKey("plugin-b", "value"): "B",
	}}
	system, err := NewSystem(Config{RootDir: t.TempDir(), SystemPlugins: fake})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	if _, err := system.SavePlaceholderLibrary(context.Background(), types.PlaceholderLibrary{Placeholders: []types.PlaceholderItem{
		pluginPlaceholder("a", "plugin-a", "value"),
		pluginPlaceholder("b", "plugin-b", "value"),
		{Name: "warmup", Value: "{{a}}"},
	}}); err != nil {
		t.Fatalf("SavePlaceholderLibrary() error = %v", err)
	}
	result, err := system.ResolveText(context.Background(), "{{warmup}}")
	if err != nil {
		t.Fatalf("ResolveText() error = %v", err)
	}
	if result.Text != "A" {
		t.Fatalf("resolved text = %q", result.Text)
	}
	if len(fake.requests) != 1 {
		t.Fatalf("requests = %#v", fake.requests)
	}
	if len(fake.requests[0]) != 1 || fake.requests[0][0].PluginID != "plugin-a" || fake.requests[0][0].InterfaceID != "value" {
		t.Fatalf("requests = %#v", fake.requests)
	}
}

func TestResolveTextExpandsDynamicReferences(t *testing.T) {
	fake := &fakePluginSystem{values: map[string]string{
		pluginSourceKey("plugin-a", "value"): "{{b}}",
		pluginSourceKey("plugin-b", "value"): "B",
	}}
	system, err := NewSystem(Config{RootDir: t.TempDir(), SystemPlugins: fake})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	if _, err := system.SavePlaceholderLibrary(context.Background(), types.PlaceholderLibrary{Placeholders: []types.PlaceholderItem{
		pluginPlaceholder("a", "plugin-a", "value"),
		pluginPlaceholder("b", "plugin-b", "value"),
	}}); err != nil {
		t.Fatalf("SavePlaceholderLibrary() error = %v", err)
	}
	result, err := system.ResolveText(context.Background(), "{{a}}")
	if err != nil {
		t.Fatalf("ResolveText() error = %v", err)
	}
	if result.Text != "B" {
		t.Fatalf("resolved text = %q", result.Text)
	}
	if len(fake.requests) != 2 {
		t.Fatalf("rounds = %#v", fake.requests)
	}
	if fake.requests[0][0].PluginID != "plugin-a" || fake.requests[1][0].PluginID != "plugin-b" {
		t.Fatalf("requests = %#v", fake.requests)
	}
}

func TestProblemsUsesStaticCheckWithoutResolving(t *testing.T) {
	fake := &fakePluginSystem{problems: []types.PlaceholderProblem{{Name: "a", Type: types.PlaceholderProblemPluginDisabled}}}
	system, err := NewSystem(Config{RootDir: t.TempDir(), SystemPlugins: fake})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	if _, err := system.SavePlaceholderLibrary(context.Background(), types.PlaceholderLibrary{Placeholders: []types.PlaceholderItem{
		pluginPlaceholder("a", "plugin-a", "value"),
	}}); err != nil {
		t.Fatalf("SavePlaceholderLibrary() error = %v", err)
	}
	problems, err := system.Problems(context.Background())
	if err != nil {
		t.Fatalf("Problems() error = %v", err)
	}
	if len(fake.requests) != 0 {
		t.Fatalf("Problems() 不应触发真实调用：%#v", fake.requests)
	}
	if fake.staticHits != 1 {
		t.Fatalf("staticHits = %d", fake.staticHits)
	}
	found := false
	for _, problem := range problems {
		if problem.Name == "a" && problem.Type == types.PlaceholderProblemPluginDisabled {
			found = true
		}
	}
	if !found {
		t.Fatalf("problems = %#v", problems)
	}
}

func TestResolvePromptMessagesUsesTargetedValues(t *testing.T) {
	fake := &fakePluginSystem{values: map[string]string{pluginSourceKey("plugin-a", "value"): "A"}}
	system, err := NewSystem(Config{RootDir: t.TempDir(), SystemPlugins: fake})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	if _, err := system.SavePlaceholderLibrary(context.Background(), types.PlaceholderLibrary{Placeholders: []types.PlaceholderItem{
		pluginPlaceholder("a", "plugin-a", "value"),
		pluginPlaceholder("b", "plugin-b", "value"),
	}}); err != nil {
		t.Fatalf("SavePlaceholderLibrary() error = %v", err)
	}
	messages, err := system.ResolvePromptMessages(context.Background(), []types.PromptMessage{{Content: "你好 {{a}}"}})
	if err != nil {
		t.Fatalf("ResolvePromptMessages() error = %v", err)
	}
	if len(messages) != 1 || !strings.Contains(messages[0].Content, "A") {
		t.Fatalf("messages = %#v", messages)
	}
	if len(fake.requests) != 1 || len(fake.requests[0]) != 1 || fake.requests[0][0].PluginID != "plugin-a" {
		t.Fatalf("requests = %#v", fake.requests)
	}
}
