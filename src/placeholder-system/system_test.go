package placeholder

import (
	"context"
	"path/filepath"
	"testing"

	"eucli-box/pkg/types"
)

func TestSystemSavesLoadsAndResolvesLibrary(t *testing.T) {
	system, err := NewSystem(Config{RootDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	saved, err := system.SavePlaceholderLibrary(context.Background(), types.PlaceholderLibrary{Placeholders: []types.PlaceholderItem{{Name: "名字", Value: "晶晶"}, {Name: "问候", Value: "你好，{{名字}}"}}})
	if err != nil {
		t.Fatalf("SavePlaceholderLibrary() error = %v", err)
	}
	if len(saved.Placeholders) != 2 {
		t.Fatalf("saved placeholders = %#v", saved.Placeholders)
	}
	loaded, err := system.LoadPlaceholderLibrary(context.Background())
	if err != nil {
		t.Fatalf("LoadPlaceholderLibrary() error = %v", err)
	}
	if len(loaded.Placeholders) != 2 {
		t.Fatalf("loaded placeholders = %#v", loaded.Placeholders)
	}
	result, err := system.ResolveText(context.Background(), "{{问候}}")
	if err != nil {
		t.Fatalf("ResolveText() error = %v", err)
	}
	if result.Text != "你好，晶晶" {
		t.Fatalf("resolved text = %q", result.Text)
	}
}

func TestSystemRejectsDuplicateNames(t *testing.T) {
	system, err := NewSystem(Config{RootDir: filepath.Clean(t.TempDir())})
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	_, err = system.SavePlaceholderLibrary(context.Background(), types.PlaceholderLibrary{Placeholders: []types.PlaceholderItem{{Name: "A", Value: "1"}, {Name: "A", Value: "2"}}})
	if err == nil {
		t.Fatalf("expected duplicate name error")
	}
}
