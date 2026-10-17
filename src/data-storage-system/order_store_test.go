package datastorage

import (
	"context"
	"testing"

	"eucli-box/pkg/types"
)

func TestRoleOrderIsPersistedAsCollectionFact(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	for _, role := range []types.Role{{ID: "role-a", Name: "A"}, {ID: "role-b", Name: "B"}} {
		if err := system.SaveRole(ctx, role); err != nil {
			t.Fatalf("SaveRole(%s) error = %v", role.ID, err)
		}
	}

	assertRoleIDs(t, system, []string{"role-b", "role-a"})
	if err := system.ReorderRoles(ctx, []string{"role-a", "role-b"}); err != nil {
		t.Fatalf("ReorderRoles() error = %v", err)
	}
	assertRoleIDs(t, system, []string{"role-a", "role-b"})

	if err := system.SaveRole(ctx, types.Role{ID: "role-a", Name: "A2"}); err != nil {
		t.Fatalf("SaveRole(existing) error = %v", err)
	}
	assertRoleIDs(t, system, []string{"role-a", "role-b"})

	if err := system.SaveRole(ctx, types.Role{ID: "role-c", Name: "C"}); err != nil {
		t.Fatalf("SaveRole(new) error = %v", err)
	}
	assertRoleIDs(t, system, []string{"role-c", "role-a", "role-b"})

	if err := system.DeleteRole(ctx, "role-a"); err != nil {
		t.Fatalf("DeleteRole() error = %v", err)
	}
	assertRoleIDs(t, system, []string{"role-c", "role-b"})
}

func TestWorkspaceOrderIsPersistedAsCollectionFact(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	for _, workspace := range []types.Workspace{{ID: "workspace-a", Name: "A"}, {ID: "workspace-b", Name: "B"}} {
		if err := system.SaveWorkspace(ctx, workspace); err != nil {
			t.Fatalf("SaveWorkspace(%s) error = %v", workspace.ID, err)
		}
	}

	assertWorkspaceIDs(t, system, []string{"workspace-b", "workspace-a"})
	if err := system.ReorderWorkspaces(ctx, []string{"workspace-a", "workspace-b"}); err != nil {
		t.Fatalf("ReorderWorkspaces() error = %v", err)
	}
	assertWorkspaceIDs(t, system, []string{"workspace-a", "workspace-b"})

	if err := system.SaveWorkspace(ctx, types.Workspace{ID: "workspace-c", Name: "C"}); err != nil {
		t.Fatalf("SaveWorkspace(new) error = %v", err)
	}
	assertWorkspaceIDs(t, system, []string{"workspace-c", "workspace-a", "workspace-b"})

	if err := system.DeleteWorkspace(ctx, "workspace-b"); err != nil {
		t.Fatalf("DeleteWorkspace() error = %v", err)
	}
	assertWorkspaceIDs(t, system, []string{"workspace-c", "workspace-a"})
}

func TestReorderRejectsUnknownOrDuplicateIDs(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	if err := system.SaveRole(ctx, types.Role{ID: "role-a", Name: "A"}); err != nil {
		t.Fatalf("SaveRole() error = %v", err)
	}
	if err := system.ReorderRoles(ctx, []string{"role-missing"}); err == nil {
		t.Fatalf("ReorderRoles(unknown) expected error")
	}
	if err := system.ReorderRoles(ctx, []string{"role-a", "role-a"}); err == nil {
		t.Fatalf("ReorderRoles(duplicate) expected error")
	}
	if err := system.ReorderRoles(ctx, nil); err == nil {
		t.Fatalf("ReorderRoles(empty) expected error")
	}
	assertRoleIDs(t, system, []string{"role-a"})
}

func assertRoleIDs(t *testing.T, system System, expected []string) {
	t.Helper()
	roles, err := system.ListRoles(context.Background())
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}
	actual := make([]string, 0, len(roles))
	for _, role := range roles {
		actual = append(actual, role.ID)
	}
	assertIDs(t, actual, expected)
}

func assertWorkspaceIDs(t *testing.T, system System, expected []string) {
	t.Helper()
	workspaces, err := system.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaces() error = %v", err)
	}
	actual := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		actual = append(actual, workspace.ID)
	}
	assertIDs(t, actual, expected)
}

func assertIDs(t *testing.T, actual []string, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("ids = %#v, want %#v", actual, expected)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("ids = %#v, want %#v", actual, expected)
		}
	}
}
