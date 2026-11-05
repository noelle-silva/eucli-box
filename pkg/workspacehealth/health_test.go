package workspacehealth

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"eucli-box/pkg/types"
)

func TestProbeSeparatesAvailabilityFromRegistration(t *testing.T) {
	root := t.TempDir()
	available := filepath.Join(root, "available")
	if err := os.MkdirAll(available, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	filePath := filepath.Join(root, "file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	missing := filepath.Join(root, "moved-away")

	health := Probe(context.Background(), []types.WorkspaceDirectory{
		{Path: available},
		{Path: missing},
		{Path: filePath},
	})
	if health.Status != types.WorkspaceHealthDegraded {
		t.Fatalf("health status = %q, want %q", health.Status, types.WorkspaceHealthDegraded)
	}
	want := []string{
		types.WorkspaceDirectoryAvailable,
		types.WorkspaceDirectoryMissing,
		types.WorkspaceDirectoryNotDirectory,
	}
	if len(health.Directories) != len(want) {
		t.Fatalf("health directories = %#v", health.Directories)
	}
	for index, status := range want {
		if health.Directories[index].Status != status {
			t.Fatalf("directory %d status = %q, want %q", index, health.Directories[index].Status, status)
		}
	}
}

func TestProbeHealthyWhenAllDirectoriesAreAvailable(t *testing.T) {
	root := t.TempDir()
	health := Probe(context.Background(), []types.WorkspaceDirectory{{Path: root}})
	if health.Status != types.WorkspaceHealthHealthy {
		t.Fatalf("health status = %q, want %q", health.Status, types.WorkspaceHealthHealthy)
	}
	if len(health.Directories) != 1 || health.Directories[0].Status != types.WorkspaceDirectoryAvailable {
		t.Fatalf("health directories = %#v", health.Directories)
	}
}
