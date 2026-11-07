package datastorage

import (
	"context"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

// 准备一个三层收藏夹与两条提示词：a → b → c，a 索引 p1，b 索引 p2。
func seedFolderActions(t *testing.T, system *system) {
	t.Helper()
	ctx := context.Background()
	if _, err := system.SavePromptLibrary(ctx, types.PromptScopeGlobal, "", types.PromptLibrary{Prompts: []types.Prompt{
		{ID: "p1", Name: "一", Content: "内容一"},
		{ID: "p2", Name: "二", Content: "内容二"},
	}}); err != nil {
		t.Fatalf("SavePromptLibrary() error = %v", err)
	}
	if _, err := system.SavePromptFolders(ctx, types.PromptScopeGlobal, "", types.PromptFolderLibrary{
		RootFolderIDs: []string{"a", "z"},
		Folders: []types.PromptFolder{
			{ID: "a", Name: "甲", FolderIDs: []string{"b"}, PromptIDs: []string{"p1"}},
			{ID: "b", Name: "乙", FolderIDs: []string{"c"}, PromptIDs: []string{"p2"}},
			{ID: "c", Name: "丙", FolderIDs: []string{}, PromptIDs: []string{}},
			{ID: "z", Name: "边", FolderIDs: []string{}, PromptIDs: []string{}},
		},
	}); err != nil {
		t.Fatalf("SavePromptFolders() error = %v", err)
	}
}

// 删除中间收藏夹：子夹按原顺序上移到原位置，被删夹的书签消失，本体不受影响。
func TestDeletePromptFolderMovesChildrenUp(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	seedFolderActions(t, system)

	saved, err := system.DeletePromptFolder(ctx, types.PromptScopeGlobal, "", "b")
	if err != nil {
		t.Fatalf("DeletePromptFolder() error = %v", err)
	}
	if len(saved.Folders) != 3 {
		t.Fatalf("folders = %d, want 3", len(saved.Folders))
	}
	if strings.Join(saved.Folders[0].FolderIDs, ",") != "c" {
		t.Fatalf("parent folder ids = %v, want c", saved.Folders[0].FolderIDs)
	}
	if strings.Join(saved.Folders[0].PromptIDs, ",") != "p1" {
		t.Fatalf("parent prompt ids = %v, want p1", saved.Folders[0].PromptIDs)
	}
	if strings.Join(saved.RootFolderIDs, ",") != "a,z" {
		t.Fatalf("root order = %v, want a,z", saved.RootFolderIDs)
	}
	library, err := system.LoadPromptLibrary(ctx, types.PromptScopeGlobal, "")
	if err != nil {
		t.Fatalf("LoadPromptLibrary() error = %v", err)
	}
	if len(library.Prompts) != 2 {
		t.Fatalf("prompt library = %d prompts, want 2", len(library.Prompts))
	}
}

func TestDeletePromptFolderMissing(t *testing.T) {
	system := newTestSystem(t)
	seedFolderActions(t, system)
	if _, err := system.DeletePromptFolder(context.Background(), types.PromptScopeGlobal, "", "ghost"); err == nil {
		t.Fatal("DeletePromptFolder() on missing folder = nil error, want failure")
	} else if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error = %v", err)
	}
}

// 重命名只换名字，结构与顺序一律不动。
func TestRenamePromptFolder(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	seedFolderActions(t, system)

	saved, err := system.RenamePromptFolder(ctx, types.PromptScopeGlobal, "", "b", "  后端  ")
	if err != nil {
		t.Fatalf("RenamePromptFolder() error = %v", err)
	}
	if saved.Folders[1].Name != "后端" {
		t.Fatalf("renamed folder name = %q, want 后端", saved.Folders[1].Name)
	}
	if strings.Join(saved.Folders[1].FolderIDs, ",") != "c" || strings.Join(saved.Folders[1].PromptIDs, ",") != "p2" {
		t.Fatalf("rename touched structure: %#v", saved.Folders[1])
	}
	if _, err := system.RenamePromptFolder(ctx, types.PromptScopeGlobal, "", "b", "   "); err == nil {
		t.Fatal("RenamePromptFolder() with blank name = nil error, want failure")
	}
	if _, err := system.RenamePromptFolder(ctx, types.PromptScopeGlobal, "", "ghost", "名"); err == nil {
		t.Fatal("RenamePromptFolder() on missing folder = nil error, want failure")
	}
}

// 兄弟排序只换位置：提交清单必须与现有兄弟完全一致。
func TestReorderPromptFolders(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	seedFolderActions(t, system)

	saved, err := system.ReorderPromptFolders(ctx, types.PromptScopeGlobal, "", "", []string{"z", "a"})
	if err != nil {
		t.Fatalf("ReorderPromptFolders() error = %v", err)
	}
	if strings.Join(saved.RootFolderIDs, ",") != "z,a" {
		t.Fatalf("root order = %v, want z,a", saved.RootFolderIDs)
	}

	if _, err := system.ReorderPromptFolders(ctx, types.PromptScopeGlobal, "", "", []string{"z"}); err == nil {
		t.Fatal("ReorderPromptFolders() with missing sibling = nil error, want failure")
	} else if !strings.Contains(err.Error(), "exactly the current root folders") {
		t.Fatalf("error = %v", err)
	}

	if _, err := system.ReorderPromptFolders(ctx, types.PromptScopeGlobal, "", "a", []string{"b"}); err != nil {
		t.Fatalf("ReorderPromptFolders() on single child error = %v", err)
	}
	if _, err := system.ReorderPromptFolders(ctx, types.PromptScopeGlobal, "", "ghost", []string{}); err == nil {
		t.Fatal("ReorderPromptFolders() on missing parent = nil error, want failure")
	}
}

// 夹内条目排序只换位置：提交清单必须与现有索引完全一致。
func TestReorderFolderPrompts(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	seedFolderActions(t, system)
	if _, err := system.SavePromptFolders(ctx, types.PromptScopeGlobal, "", types.PromptFolderLibrary{
		RootFolderIDs: []string{"a", "z"},
		Folders: []types.PromptFolder{
			{ID: "a", Name: "甲", FolderIDs: []string{"b"}, PromptIDs: []string{"p1", "p2"}},
			{ID: "b", Name: "乙", FolderIDs: []string{"c"}, PromptIDs: []string{}},
			{ID: "c", Name: "丙", FolderIDs: []string{}, PromptIDs: []string{}},
			{ID: "z", Name: "边", FolderIDs: []string{}, PromptIDs: []string{}},
		},
	}); err != nil {
		t.Fatalf("SavePromptFolders() error = %v", err)
	}

	saved, err := system.ReorderFolderPrompts(ctx, types.PromptScopeGlobal, "", "a", []string{"p2", "p1"})
	if err != nil {
		t.Fatalf("ReorderFolderPrompts() error = %v", err)
	}
	if strings.Join(saved.Folders[0].PromptIDs, ",") != "p2,p1" {
		t.Fatalf("prompt order = %v, want p2,p1", saved.Folders[0].PromptIDs)
	}

	if _, err := system.ReorderFolderPrompts(ctx, types.PromptScopeGlobal, "", "a", []string{"p2"}); err == nil {
		t.Fatal("ReorderFolderPrompts() dropping an index = nil error, want failure")
	} else if !strings.Contains(err.Error(), "exactly the current prompts") {
		t.Fatalf("error = %v", err)
	}
	if _, err := system.ReorderFolderPrompts(ctx, types.PromptScopeGlobal, "", "ghost", []string{}); err == nil {
		t.Fatal("ReorderFolderPrompts() on missing folder = nil error, want failure")
	}
}
