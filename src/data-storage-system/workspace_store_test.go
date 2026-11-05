package datastorage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func TestWorkspaceRegistrationKeepsMissingDirectoryAsFact(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	missing := filepath.Join(t.TempDir(), "moved-away")
	workspace := types.Workspace{
		ID:          "workspace-offline",
		Name:        "离线目录工作区",
		Directories: []types.WorkspaceDirectory{{Path: missing, Alias: "已移动目录"}},
	}
	if err := system.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatalf("SaveWorkspace() error = %v", err)
	}

	loaded, err := system.LoadWorkspace(ctx, workspace.ID)
	if err != nil {
		t.Fatalf("LoadWorkspace() error = %v", err)
	}
	if len(loaded.Directories) != 1 || loaded.Directories[0].Path != missing {
		t.Fatalf("loaded directories = %#v", loaded.Directories)
	}

	workspaces, err := system.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces() error = %v", err)
	}
	if len(workspaces) != 1 || workspaces[0].ID != workspace.ID {
		t.Fatalf("workspaces = %#v", workspaces)
	}
}

func TestListWorkspacesToleratesDirectoryThatDisappearsAfterSave(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	directory := t.TempDir()
	workspace := types.Workspace{
		ID:          "workspace-disappeared",
		Name:        "目录被移动",
		Directories: []types.WorkspaceDirectory{{Path: directory}},
	}
	if err := system.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatalf("SaveWorkspace() error = %v", err)
	}
	if err := os.RemoveAll(directory); err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}

	workspaces, err := system.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces() error = %v", err)
	}
	if len(workspaces) != 1 || workspaces[0].ID != workspace.ID {
		t.Fatalf("workspaces = %#v", workspaces)
	}
}

func TestSaveWorkspaceKeepsEntityOrderAndIndexConsistent(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	for _, workspace := range []types.Workspace{{ID: "workspace-a", Name: "A"}, {ID: "workspace-b", Name: "B"}} {
		if err := system.SaveWorkspace(ctx, workspace); err != nil {
			t.Fatalf("SaveWorkspace(%s) error = %v", workspace.ID, err)
		}
	}

	order, err := readOrderIDs(ctx, system.paths.workspaceOrderFile())
	if err != nil {
		t.Fatalf("readOrderIDs() error = %v", err)
	}
	assertIDs(t, order, []string{"workspace-b", "workspace-a"})
	index, err := readJSON[rootIndex[types.WorkspaceSummary]](ctx, filepath.Join(system.paths.workspacesRoot(), "index.json"))
	if err != nil {
		t.Fatalf("read index error = %v", err)
	}
	ids := make([]string, 0, len(index.Items))
	for _, item := range index.Items {
		ids = append(ids, item.ID)
	}
	assertIDs(t, ids, []string{"workspace-b", "workspace-a"})
}

func TestSaveWorkspaceFailureLeavesNoOrphanEntity(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	existing := types.Workspace{ID: "workspace-existing", Name: "Existing"}
	if err := system.SaveWorkspace(ctx, existing); err != nil {
		t.Fatalf("SaveWorkspace(existing) error = %v", err)
	}
	orderBefore, err := readOrderIDs(ctx, system.paths.workspaceOrderFile())
	if err != nil {
		t.Fatalf("readOrderIDs() error = %v", err)
	}
	indexTarget := filepath.Join(system.paths.workspacesRoot(), "index.json")
	if err := os.Remove(indexTarget); err != nil {
		t.Fatalf("Remove(index.json) error = %v", err)
	}
	if err := os.Mkdir(indexTarget, 0o755); err != nil {
		t.Fatalf("Mkdir(index.json) error = %v", err)
	}

	err = system.SaveWorkspace(ctx, types.Workspace{ID: "workspace-orphan", Name: "Orphan"})
	if err == nil {
		t.Fatalf("SaveWorkspace(orphan) expected error")
	}
	orphanTarget, pathErr := system.paths.workspaceDataFile("workspace-orphan")
	if pathErr != nil {
		t.Fatalf("workspaceDataFile() error = %v", pathErr)
	}
	if _, statErr := os.Stat(orphanTarget); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("orphan data file exists, stat error = %v", statErr)
	}
	orderAfter, err := readOrderIDs(ctx, system.paths.workspaceOrderFile())
	if err != nil {
		t.Fatalf("readOrderIDs() after failure = %v", err)
	}
	assertIDs(t, orderAfter, orderBefore)
}

func TestFileTransactionRollsBackCommittedFiles(t *testing.T) {
	root := t.TempDir()
	existingTarget := filepath.Join(root, "existing.json")
	if err := os.WriteFile(existingTarget, []byte("{\"value\":\"old\"}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	newTarget := filepath.Join(root, "new.json")
	failingTarget := filepath.Join(root, "failing.json")

	tx := newFileTransaction()
	tx.commitFile = func(temp string, target string) error {
		if target == failingTarget {
			return errors.New("commit failed")
		}
		return commitJSON(temp, target)
	}
	defer tx.Discard()
	if err := tx.StageJSON(context.Background(), newTarget, map[string]string{"value": "new"}); err != nil {
		t.Fatalf("StageJSON(new) error = %v", err)
	}
	if err := tx.StageJSON(context.Background(), existingTarget, map[string]string{"value": "new"}); err != nil {
		t.Fatalf("StageJSON(existing) error = %v", err)
	}
	if err := tx.StageJSON(context.Background(), failingTarget, map[string]string{"value": "new"}); err != nil {
		t.Fatalf("StageJSON(failing) error = %v", err)
	}
	if err := tx.Commit(); err == nil || !strings.Contains(err.Error(), "commit failed") {
		t.Fatalf("Commit() error = %v", err)
	}
	if _, err := os.Stat(newTarget); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new target still exists, stat error = %v", err)
	}
	payload, err := os.ReadFile(existingTarget)
	if err != nil {
		t.Fatalf("ReadFile(existing) error = %v", err)
	}
	if !strings.Contains(string(payload), `"old"`) {
		t.Fatalf("existing target content = %q, want old content restored", string(payload))
	}
}
