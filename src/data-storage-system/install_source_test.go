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
	config := installsource.Config{
		Source:  "甲",
		Shelves: []installsource.Shelf{{Name: "甲", Path: "D:/shelf-a"}, {Name: "乙", Path: "D:/shelf-b"}},
	}
	if err := system.SaveInstallSource(context.Background(), config); err != nil {
		t.Fatalf("SaveInstallSource() error = %v", err)
	}
	loaded, err := system.LoadInstallSource(context.Background())
	if err != nil {
		t.Fatalf("LoadInstallSource() error = %v", err)
	}
	if loaded.Source != "甲" || len(loaded.Shelves) != 2 || loaded.Shelves[1].Name != "乙" || loaded.Shelves[1].Path != "D:/shelf-b" {
		t.Fatalf("LoadInstallSource() = %#v", loaded)
	}
	if len(system.paths.installSourceFile()) == 0 {
		t.Fatalf("install source file path is empty")
	}
}

func TestInstallSourceCorruptionFailsFast(t *testing.T) {
	system := newTestSystem(t)
	if err := os.WriteFile(system.paths.installSourceFile(), []byte(`not-json`), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := system.LoadInstallSource(context.Background()); err == nil {
		t.Fatal("LoadInstallSource() error = nil, want corruption error")
	}
}
