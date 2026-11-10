package datastorage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
		Source: "甲",
		Sources: []installsource.Source{
			{Name: "甲", Kind: installsource.SourceKindShelf, Path: "D:/shelf-a"},
			{Name: "乙", Kind: installsource.SourceKindShelf, Path: "D:/shelf-b"},
		},
	}
	if err := store.SaveInstallSource(context.Background(), config); err != nil {
		t.Fatalf("SaveInstallSource() error = %v", err)
	}
	loaded, err := store.LoadInstallSource(context.Background())
	if err != nil {
		t.Fatalf("LoadInstallSource() error = %v", err)
	}
	if loaded.Source != "甲" || len(loaded.Sources) != 2 || loaded.Sources[1].Name != "乙" || loaded.Sources[1].Path != "D:/shelf-b" {
		t.Fatalf("LoadInstallSource() = %#v", loaded)
	}
	if _, err := system.InstallSourceStore("盒"); err == nil {
		t.Fatal("InstallSourceStore(unknown kind) error = nil")
	}
}

// TestInstallSourceReadsLegacyShelves 验证旧版货架配置读入后自动升格为本机来源。
func TestInstallSourceReadsLegacyShelves(t *testing.T) {
	system := newTestSystem(t)
	path, err := system.paths.installSourceFile(types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("installSourceFile(tool) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	legacy := []byte(`{"source":"甲","shelves":[{"name":"甲","path":"D:/shelf-a"}]}`)
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	store, err := system.InstallSourceStore(types.ReleaseArtifactKindTool)
	if err != nil {
		t.Fatalf("InstallSourceStore() error = %v", err)
	}
	loaded, err := store.LoadInstallSource(context.Background())
	if err != nil {
		t.Fatalf("LoadInstallSource() error = %v", err)
	}
	if loaded.Source != "甲" || len(loaded.Sources) != 1 {
		t.Fatalf("legacy config = %#v", loaded)
	}
	if loaded.Sources[0].Kind != installsource.SourceKindShelf || loaded.Sources[0].Path != "D:/shelf-a" {
		t.Fatalf("legacy source = %#v", loaded.Sources[0])
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
	toolConfig := installsource.Config{Source: "工具架", Sources: []installsource.Source{{Name: "工具架", Kind: installsource.SourceKindShelf, Path: "D:/tools"}}}
	if err := toolStore.SaveInstallSource(context.Background(), toolConfig); err != nil {
		t.Fatalf("tool SaveInstallSource() error = %v", err)
	}
	if err := pluginStore.SaveInstallSource(context.Background(), installsource.DefaultConfig()); err != nil {
		t.Fatalf("plugin SaveInstallSource() error = %v", err)
	}
	loaded, err := toolStore.LoadInstallSource(context.Background())
	if err != nil || loaded.Source != "工具架" || len(loaded.Sources) != 1 {
		t.Fatalf("tool LoadInstallSource() = %#v err=%v", loaded, err)
	}
	loaded, err = pluginStore.LoadInstallSource(context.Background())
	if err != nil || loaded.Source != installsource.OfficialSource || len(loaded.Sources) != 0 {
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
