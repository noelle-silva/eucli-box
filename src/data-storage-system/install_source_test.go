package datastorage

import (
	"context"
	"errors"
	"os"
	"testing"

	"eucli-box/pkg/installsource"
	"eucli-box/pkg/types"
)

func TestInstallSourceRoundTrip(t *testing.T) {
	system := newTestSystem(t)
	store, err := system.InstallSourceStore(types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("InstallSourceStore() error = %v", err)
	}
	if _, err := store.LoadInstallSource(context.Background()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadInstallSource() error = %v, want ErrNotExist", err)
	}
	config := installsource.Config{
		Source:  "甲",
		Shelves: []installsource.Shelf{{Name: "甲", Path: "D:/shelf-a"}, {Name: "乙", Path: "D:/shelf-b"}},
	}
	if err := store.SaveInstallSource(context.Background(), config); err != nil {
		t.Fatalf("SaveInstallSource() error = %v", err)
	}
	loaded, err := store.LoadInstallSource(context.Background())
	if err != nil {
		t.Fatalf("LoadInstallSource() error = %v", err)
	}
	if loaded.Source != "甲" || len(loaded.Shelves) != 2 || loaded.Shelves[1].Name != "乙" || loaded.Shelves[1].Path != "D:/shelf-b" {
		t.Fatalf("LoadInstallSource() = %#v", loaded)
	}
	if _, err := system.InstallSourceStore("盒"); err == nil {
		t.Fatal("InstallSourceStore(unknown kind) error = nil")
	}
}

// TestInstallSourceCategoriesUseSeparateFiles 验证两类来源配置各存各的文件：
// 一类损坏不影响另一类读取。
func TestInstallSourceCategoriesUseSeparateFiles(t *testing.T) {
	system := newTestSystem(t)
	toolStore, err := system.InstallSourceStore(types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("InstallSourceStore(tool) error = %v", err)
	}
	pluginStore, err := system.InstallSourceStore(types.ReleaseArtifactKindPlugin)
	if err != nil {
		t.Fatalf("InstallSourceStore(plugin) error = %v", err)
	}
	toolConfig := installsource.Config{Source: "工具架", Shelves: []installsource.Shelf{{Name: "工具架", Path: "D:/tools"}}}
	if err := toolStore.SaveInstallSource(context.Background(), toolConfig); err != nil {
		t.Fatalf("tool SaveInstallSource() error = %v", err)
	}
	if err := pluginStore.SaveInstallSource(context.Background(), installsource.DefaultConfig()); err != nil {
		t.Fatalf("plugin SaveInstallSource() error = %v", err)
	}
	loaded, err := toolStore.LoadInstallSource(context.Background())
	if err != nil || loaded.Source != "工具架" || len(loaded.Shelves) != 1 {
		t.Fatalf("tool LoadInstallSource() = %#v err=%v", loaded, err)
	}
	loaded, err = pluginStore.LoadInstallSource(context.Background())
	if err != nil || loaded.Source != installsource.OfficialSource || len(loaded.Shelves) != 0 {
		t.Fatalf("plugin LoadInstallSource() = %#v err=%v", loaded, err)
	}

	toolPath, err := system.paths.installSourceFile(types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("installSourceFile(tool) error = %v", err)
	}
	if err := os.WriteFile(toolPath, []byte(`not-json`), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := toolStore.LoadInstallSource(context.Background()); err == nil {
		t.Fatal("corrupted tool LoadInstallSource() error = nil")
	}
	loaded, err = pluginStore.LoadInstallSource(context.Background())
	if err != nil || loaded.Source != installsource.OfficialSource {
		t.Fatalf("plugin unaffected LoadInstallSource() = %#v err=%v", loaded, err)
	}
}

func TestInstallSourceCorruptionFailsFast(t *testing.T) {
	system := newTestSystem(t)
	path, err := system.paths.installSourceFile(types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("installSourceFile(tool) error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`not-json`), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	store, err := system.InstallSourceStore(types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("InstallSourceStore(tool) error = %v", err)
	}
	if _, err := store.LoadInstallSource(context.Background()); err == nil {
		t.Fatal("LoadInstallSource() error = nil, want corruption error")
	}
}
