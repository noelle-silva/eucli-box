package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestConfigStore(t *testing.T) *configStore {
	t.Helper()
	store, err := newConfigStore(t.TempDir())
	if err != nil {
		t.Fatalf("newConfigStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv(devSourceEnvironment)
	})
	return store
}

func TestGetSettingsDevModeFlags(t *testing.T) {
	store := newTestConfigStore(t)
	t.Setenv(devSourceEnvironment, devSourceEnabled)
	settings, err := store.getSettings()
	if err != nil {
		t.Fatalf("getSettings() error = %v", err)
	}
	if settings["devBoxSourceEnabled"] != true {
		t.Fatalf("settings = %#v, want devBoxSourceEnabled=true", settings)
	}
	if settings["boxSourceKind"] != "" {
		t.Fatalf("settings boxSourceKind = %q, want empty default", settings["boxSourceKind"])
	}
}

func TestGetSettingsOfficialMode(t *testing.T) {
	store := newTestConfigStore(t)
	settings, err := store.getSettings()
	if err != nil {
		t.Fatalf("getSettings() error = %v", err)
	}
	if settings["devBoxSourceEnabled"] != false {
		t.Fatalf("settings = %#v, want devBoxSourceEnabled=false", settings)
	}
}

func TestUpdateBoxSourceKindDevMode(t *testing.T) {
	store := newTestConfigStore(t)
	t.Setenv(devSourceEnvironment, devSourceEnabled)
	got, err := store.updateSetting("boxSourceKind", "development")
	if err != nil {
		t.Fatalf("updateSetting() error = %v", err)
	}
	if got["boxSourceKind"] != "development" {
		t.Fatalf("got = %#v", got)
	}
	cfg, err := store.load()
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if cfg.BoxSourceKind != "development" {
		t.Fatalf("BoxSourceKind = %q, want development", cfg.BoxSourceKind)
	}
}

func TestUpdateBoxSourceKindRejectedInOfficialMode(t *testing.T) {
	store := newTestConfigStore(t)
	if _, err := store.updateSetting("boxSourceKind", "development"); err == nil {
		t.Fatal("updateSetting() error = nil, want forbidden")
	}
	cfg, err := store.load()
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if cfg.BoxSourceKind != "" {
		t.Fatalf("BoxSourceKind = %q, want unchanged", cfg.BoxSourceKind)
	}
}

func TestUpdateBoxSourceKindRejectsInvalidValue(t *testing.T) {
	store := newTestConfigStore(t)
	t.Setenv(devSourceEnvironment, devSourceEnabled)
	if _, err := store.updateSetting("boxSourceKind", "internal"); err == nil {
		t.Fatal("updateSetting() error = nil, want invalid kind error")
	}
	if _, err := store.updateSetting("boxSourceKind", true); err == nil {
		t.Fatal("updateSetting() error = nil, want non-string error")
	}
}

func TestBoxSourceKindEffectiveDefaults(t *testing.T) {
	store := newTestConfigStore(t)
	t.Setenv(devSourceEnvironment, devSourceEnabled)
	if got := store.boxSourceKindEffective(); got != localBoxSourceDevelopment {
		t.Fatalf("effective = %q, want development default in dev mode", got)
	}
	t.Setenv(devSourceEnvironment, "")
	if got := store.boxSourceKindEffective(); got != localBoxSourceOfficial {
		t.Fatalf("effective = %q, want official in official mode", got)
	}
	store = newTestConfigStore(t)
	t.Setenv(devSourceEnvironment, devSourceEnabled)
	if _, err := store.updateSetting("boxSourceKind", "official"); err != nil {
		t.Fatalf("updateSetting() error = %v", err)
	}
	if got := store.boxSourceKindEffective(); got != localBoxSourceOfficial {
		t.Fatalf("effective = %q, want official after switch", got)
	}
}

func TestResolveLocalBoxSourceOfficialMode(t *testing.T) {
	store := newTestConfigStore(t)
	source, devBoxRoot, err := resolveLocalBoxSource(store)
	if err != nil {
		t.Fatalf("resolveLocalBoxSource() error = %v", err)
	}
	if source != nil || devBoxRoot != "" {
		t.Fatalf("source = %#v devBoxRoot = %q, want nil/empty in official mode", source, devBoxRoot)
	}
}

func TestResolveLocalBoxSourceDevMode(t *testing.T) {
	store := newTestConfigStore(t)
	t.Setenv(devSourceEnvironment, devSourceEnabled)
	t.Setenv(devManifestEnvironment, filepath.Join("dev", "manifest.json"))
	t.Setenv(devArchiveEnvironment, filepath.Join("dev", "archive.zip"))
	t.Setenv(devBoxRootEnvironment, filepath.Join("dev", "root"))

	source, devBoxRoot, err := resolveLocalBoxSource(store)
	if err != nil {
		t.Fatalf("resolveLocalBoxSource() error = %v", err)
	}
	if _, ok := source.(*developmentArtifactSource); !ok {
		t.Fatalf("source = %T, want *developmentArtifactSource by default", source)
	}
	if devBoxRoot == "" {
		t.Fatalf("devBoxRoot = %q, want box root env", devBoxRoot)
	}

	if _, err := store.updateSetting("boxSourceKind", "official"); err != nil {
		t.Fatalf("updateSetting() error = %v", err)
	}
	source, devBoxRoot, err = resolveLocalBoxSource(store)
	if err != nil {
		t.Fatalf("resolveLocalBoxSource() error = %v", err)
	}
	if source != nil {
		t.Fatalf("source = %T, want nil (official source built by caller)", source)
	}
	if devBoxRoot == "" {
		t.Fatalf("devBoxRoot = %q after switching to official", devBoxRoot)
	}
}

func TestSetBoxSourceKindSyncFailureDoesNotPersist(t *testing.T) {
	store := newTestConfigStore(t)
	t.Setenv(devSourceEnvironment, devSourceEnabled)
	svc := &service{config: store, eb: newEBClient(store, clientRelease{})}
	_, err := svc.setBoxSourceKind(context.Background(), "development")
	if err == nil {
		t.Fatal("setBoxSourceKind() error = nil, want sync failure (business endpoint missing)")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "同步业务端") && !strings.Contains(strings.ToLower(err.Error()), "configured") {
		t.Fatalf("setBoxSourceKind() error = %v", err)
	}
	cfg, loadErr := store.load()
	if loadErr != nil {
		t.Fatalf("load() error = %v", loadErr)
	}
	if cfg.BoxSourceKind != "" {
		t.Fatalf("BoxSourceKind = %q, want unchanged after sync failure", cfg.BoxSourceKind)
	}
}

func TestSetBoxSourceKindRejections(t *testing.T) {
	store := newTestConfigStore(t)
	t.Setenv(devSourceEnvironment, devSourceEnabled)
	svc := &service{config: store, eb: newEBClient(store, clientRelease{})}
	if _, err := svc.setBoxSourceKind(context.Background(), "internal"); err == nil {
		t.Fatal("setBoxSourceKind() error = nil, want invalid kind error")
	}
	t.Setenv(devSourceEnvironment, "")
	if _, err := svc.setBoxSourceKind(context.Background(), "development"); err == nil {
		t.Fatal("setBoxSourceKind() error = nil, want forbidden in official mode")
	}
}
