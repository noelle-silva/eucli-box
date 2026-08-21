package datastorage

import (
	"context"
	"errors"
	"os"
	"testing"

	"eucli-box/pkg/installsource"
)

func TestInstallSourceRoundTrip(t *testing.T) {
	system := newTestSystem(t)
	if _, err := system.LoadInstallSource(context.Background()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadInstallSource() error = %v, want ErrNotExist", err)
	}
	if err := system.SaveInstallSource(context.Background(), installsource.KindDevelopment); err != nil {
		t.Fatalf("SaveInstallSource() error = %v", err)
	}
	loaded, err := system.LoadInstallSource(context.Background())
	if err != nil {
		t.Fatalf("LoadInstallSource() error = %v", err)
	}
	if loaded != installsource.KindDevelopment {
		t.Fatalf("LoadInstallSource() = %q, want %q", loaded, installsource.KindDevelopment)
	}
	if len(system.paths.installSourceFile()) == 0 {
		t.Fatalf("install source file path is empty")
	}
}

func TestInstallSourceRejectsInvalidSave(t *testing.T) {
	system := newTestSystem(t)
	if err := system.SaveInstallSource(context.Background(), installsource.Kind("internal")); err == nil {
		t.Fatal("SaveInstallSource() error = nil, want invalid kind error")
	}
}

func TestInstallSourceCorruptionFailsFast(t *testing.T) {
	system := newTestSystem(t)
	if err := os.WriteFile(system.paths.installSourceFile(), []byte(`{"kind":"internal"}`), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := system.LoadInstallSource(context.Background()); err == nil {
		t.Fatal("LoadInstallSource() error = nil, want corruption error")
	}
}
