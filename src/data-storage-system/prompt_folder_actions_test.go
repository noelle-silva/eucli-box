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

// 混排排序：收藏夹与条目同处一个列表，顺序元素形如 folder:<id> / prompt:<id>。
func TestReorderPromptFolderEntries(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	seedFolderActions(t, system)

	rootOrder := []string{"folder:z", "folder:a"}
	saved, err := system.ReorderPromptFolderEntries(ctx, types.PromptScopeGlobal, "", "", rootOrder)
	if err != nil {
		t.Fatalf("ReorderPromptFolderEntries() error = %v", err)
	}
	if strings.Join(saved.RootEntryOrder, ",") != strings.Join(rootOrder, ",") {
		t.Fatalf("root entry order = %v, want %v", saved.RootEntryOrder, rootOrder)
	}

	// 根层级只认直属内容：已经被收藏夹收走的提示词即使写进根顺序也会被裁掉。
	stale, err := system.ReorderPromptFolderEntries(ctx, types.PromptScopeGlobal, "", "", []string{"prompt:p1", "folder:a", "folder:z"})
	if err != nil {
		t.Fatalf("ReorderPromptFolderEntries() with collected prompt error = %v", err)
	}
	if strings.Join(stale.RootEntryOrder, ",") != "folder:a,folder:z" {
		t.Fatalf("root entry order = %v, want folder:a,folder:z", stale.RootEntryOrder)
	}

	insideOrder := []string{"prompt:p1", "folder:b"}
	saved, err = system.ReorderPromptFolderEntries(ctx, types.PromptScopeGlobal, "", "a", insideOrder)
	if err != nil {
		t.Fatalf("ReorderPromptFolderEntries() inside folder error = %v", err)
	}
	for _, folder := range saved.Folders {
		if folder.ID != "a" {
			continue
		}
		if strings.Join(folder.EntryOrder, ",") != "prompt:p1,folder:b" {
			t.Fatalf("folder a entry order = %v", folder.EntryOrder)
		}
	}

	// 夹内少一项：根层顺序跟着归属自动维护，夹内顺序必须逐项交代。
	if _, err := system.ReorderPromptFolderEntries(ctx, types.PromptScopeGlobal, "", "a", []string{"prompt:p1"}); err == nil {
		t.Fatal("ReorderPromptFolderEntries() with missing entries = nil error, want failure")
	} else if !strings.Contains(err.Error(), "must list every entry") {
		t.Fatalf("missing entries error = %v", err)
	}

	// 根级重复项与已归属条目都被归一到「根层直属集合」，不留下脏顺序。
	duplicated := []string{"folder:a", "folder:a", "prompt:p1", "prompt:p2", "folder:z"}
	deduped, err := system.ReorderPromptFolderEntries(ctx, types.PromptScopeGlobal, "", "", duplicated)
	if err != nil {
		t.Fatalf("ReorderPromptFolderEntries() with duplicate entry error = %v", err)
	}
	if strings.Join(deduped.RootEntryOrder, ",") != "folder:a,folder:z" {
		t.Fatalf("root entry order = %v, want folder:a,folder:z", deduped.RootEntryOrder)
	}

	// 非法前缀：格式不对直接拒绝，不猜测意图。
	if _, err := system.ReorderPromptFolderEntries(ctx, types.PromptScopeGlobal, "", "", []string{"x:a", "prompt:p1", "folder:z", "prompt:p2"}); err == nil {
		t.Fatal("ReorderPromptFolderEntries() with invalid prefix = nil error, want failure")
	}

	// 不存在的父级
	if _, err := system.ReorderPromptFolderEntries(ctx, types.PromptScopeGlobal, "", "ghost", []string{"folder:b"}); err == nil {
		t.Fatal("ReorderPromptFolderEntries() on missing parent = nil error, want failure")
	}
}

// 收藏与取消收藏：加入追加到末尾，重复加入与移出不存在的条目明确报错，本体不动。
func TestAddAndRemovePromptInFolder(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	seedFolderActions(t, system)

	saved, err := system.AddPromptToFolder(ctx, types.PromptScopeGlobal, "", "a", "p2")
	if err != nil {
		t.Fatalf("AddPromptToFolder() error = %v", err)
	}
	for _, folder := range saved.Folders {
		if folder.ID != "a" {
			continue
		}
		if strings.Join(folder.PromptIDs, ",") != "p1,p2" {
			t.Fatalf("folder a prompt ids = %v, want p1,p2", folder.PromptIDs)
		}
		if strings.Join(folder.EntryOrder, ",") != "folder:b,prompt:p1,prompt:p2" {
			t.Fatalf("folder a entry order = %v", folder.EntryOrder)
		}
	}

	if _, err := system.AddPromptToFolder(ctx, types.PromptScopeGlobal, "", "a", "p2"); err == nil {
		t.Fatal("AddPromptToFolder() duplicate = nil error, want failure")
	} else if !strings.Contains(err.Error(), "already contains") {
		t.Fatalf("duplicate error = %v", err)
	}

	saved, err = system.RemovePromptFromFolder(ctx, types.PromptScopeGlobal, "", "a", "p2")
	if err != nil {
		t.Fatalf("RemovePromptFromFolder() error = %v", err)
	}
	for _, folder := range saved.Folders {
		if folder.ID != "a" {
			continue
		}
		if strings.Join(folder.PromptIDs, ",") != "p1" {
			t.Fatalf("folder a prompt ids after remove = %v", folder.PromptIDs)
		}
		if strings.Join(folder.EntryOrder, ",") != "folder:b,prompt:p1" {
			t.Fatalf("folder a entry order after remove = %v", folder.EntryOrder)
		}
	}

	if _, err := system.RemovePromptFromFolder(ctx, types.PromptScopeGlobal, "", "a", "p2"); err == nil {
		t.Fatal("RemovePromptFromFolder() missing membership = nil error, want failure")
	}

	library, err := system.LoadPromptLibrary(ctx, types.PromptScopeGlobal, "")
	if err != nil {
		t.Fatalf("LoadPromptLibrary() error = %v", err)
	}
	if len(library.Prompts) != 2 {
		t.Fatalf("prompt library = %d, want 2", len(library.Prompts))
	}
}

// 收藏夹移动：只换挂载位置，内容与顺序保持；自移、移进后代、移到原地都拒绝。
func TestMovePromptFolder(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	seedFolderActions(t, system)

	saved, err := system.MovePromptFolder(ctx, types.PromptScopeGlobal, "", "b", "a", "")
	if err != nil {
		t.Fatalf("MovePromptFolder() error = %v", err)
	}
	if strings.Join(saved.RootFolderIDs, ",") != "a,z,b" {
		t.Fatalf("root folder ids = %v, want a,z,b", saved.RootFolderIDs)
	}
	for _, folder := range saved.Folders {
		if folder.ID == "a" && len(folder.FolderIDs) != 0 {
			t.Fatalf("folder a children = %v, want empty", folder.FolderIDs)
		}
		if folder.ID == "b" && strings.Join(folder.FolderIDs, ",") != "c" {
			t.Fatalf("folder b children = %v, want c", folder.FolderIDs)
		}
	}
	if len(saved.RootEntryOrder) == 0 || saved.RootEntryOrder[len(saved.RootEntryOrder)-1] != "folder:b" {
		t.Fatalf("root entry order = %v, want folder:b at the end", saved.RootEntryOrder)
	}

	if _, err := system.MovePromptFolder(ctx, types.PromptScopeGlobal, "", "b", "a", "b"); err == nil {
		t.Fatal("MovePromptFolder() into itself = nil error, want failure")
	}
	if _, err := system.MovePromptFolder(ctx, types.PromptScopeGlobal, "", "b", "a", "c"); err == nil {
		t.Fatal("MovePromptFolder() into descendant = nil error, want failure")
	} else if !strings.Contains(err.Error(), "descendant") {
		t.Fatalf("descendant error = %v", err)
	}
	if _, err := system.MovePromptFolder(ctx, types.PromptScopeGlobal, "", "a", "", ""); err == nil {
		t.Fatal("MovePromptFolder() to the same parent = nil error, want failure")
	}
	if _, err := system.MovePromptFolder(ctx, types.PromptScopeGlobal, "", "b", "a", "ghost"); err == nil {
		t.Fatal("MovePromptFolder() into missing parent = nil error, want failure")
	}
}

// 提示词移动到：先摘原夹索引、再追加目标夹；原地与重复目标拒绝，根级两个方向都可用。
func TestMovePromptToFolder(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	seedFolderActions(t, system)

	saved, err := system.MovePromptToFolder(ctx, types.PromptScopeGlobal, "", "p1", "a", "z")
	if err != nil {
		t.Fatalf("MovePromptToFolder() error = %v", err)
	}
	for _, folder := range saved.Folders {
		if folder.ID == "a" && len(folder.PromptIDs) != 0 {
			t.Fatalf("folder a prompt ids = %v, want empty", folder.PromptIDs)
		}
		if folder.ID == "z" && strings.Join(folder.PromptIDs, ",") != "p1" {
			t.Fatalf("folder z prompt ids = %v, want p1", folder.PromptIDs)
		}
	}

	// 从根级加入某个夹：只追加索引
	saved, err = system.MovePromptToFolder(ctx, types.PromptScopeGlobal, "", "p2", "", "a")
	if err != nil {
		t.Fatalf("MovePromptToFolder() from root error = %v", err)
	}
	for _, folder := range saved.Folders {
		if folder.ID == "a" && strings.Join(folder.PromptIDs, ",") != "p2" {
			t.Fatalf("folder a prompt ids = %v, want p2", folder.PromptIDs)
		}
	}

	// 移出到根级：只摘索引
	saved, err = system.MovePromptToFolder(ctx, types.PromptScopeGlobal, "", "p2", "a", "")
	if err != nil {
		t.Fatalf("MovePromptToFolder() to root error = %v", err)
	}
	for _, folder := range saved.Folders {
		if folder.ID == "a" && len(folder.PromptIDs) != 0 {
			t.Fatalf("folder a prompt ids after remove = %v, want empty", folder.PromptIDs)
		}
	}

	if _, err := system.MovePromptToFolder(ctx, types.PromptScopeGlobal, "", "p1", "z", "z"); err == nil {
		t.Fatal("MovePromptToFolder() to the same folder = nil error, want failure")
	}
	if _, err := system.MovePromptToFolder(ctx, types.PromptScopeGlobal, "", "p1", "a", "z"); err == nil {
		t.Fatal("MovePromptToFolder() into a folder that already has it = nil error, want failure")
	} else if !strings.Contains(err.Error(), "already contains prompt") {
		t.Fatalf("target error = %v", err)
	}

	if _, err := system.MovePromptToFolder(ctx, types.PromptScopeGlobal, "", "p2", "c", "a"); err == nil {
		t.Fatal("MovePromptToFolder() from a folder without the prompt = nil error, want failure")
	} else if !strings.Contains(err.Error(), "does not contain prompt") {
		t.Fatalf("source error = %v", err)
	}
}

// 本体新增条目后，根级顺序要自动跟上；收进收藏夹后又要从根顺序里离场。
func TestAddPromptToFolderAfterNewPromptSyncsRootOrder(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	seedFolderActions(t, system)

	if _, err := system.SavePromptLibrary(ctx, types.PromptScopeGlobal, "", types.PromptLibrary{Prompts: []types.Prompt{
		{ID: "p1", Name: "一", Content: "内容一"},
		{ID: "p2", Name: "二", Content: "内容二"},
		{ID: "p3", Name: "三", Content: "内容三"},
	}}); err != nil {
		t.Fatalf("SavePromptLibrary() error = %v", err)
	}

	beforeAdd, err := system.LoadPromptFolders(ctx, types.PromptScopeGlobal, "")
	if err != nil {
		t.Fatalf("LoadPromptFolders() error = %v", err)
	}
	if !containsKey(beforeAdd.RootEntryOrder, "prompt:p3") {
		t.Fatalf("root entry order = %v, want prompt:p3 included before collection", beforeAdd.RootEntryOrder)
	}

	saved, err := system.AddPromptToFolder(ctx, types.PromptScopeGlobal, "", "a", "p3")
	if err != nil {
		t.Fatalf("AddPromptToFolder() after new prompt error = %v", err)
	}
	if containsKey(saved.RootEntryOrder, "prompt:p3") {
		t.Fatalf("root entry order = %v, want prompt:p3 removed after collection", saved.RootEntryOrder)
	}
}

func containsKey(keys []string, want string) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}

// 收藏夹可被多个上级收录：同一个夹同时挂在根级与别的夹下面是合法的。
func TestFolderCanBeCollectedInMultiplePlaces(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	seedFolderActions(t, system)

	saved, err := system.AddFolderToFolder(ctx, types.PromptScopeGlobal, "", "c", "z")
	if err != nil {
		t.Fatalf("AddFolderToFolder() error = %v", err)
	}
	// c 原来挂在 b 下，现在同时挂在 z 下，两处都在。
	inZ, inB := false, false
	for _, folder := range saved.Folders {
		if folder.ID == "z" && len(folder.FolderIDs) == 1 && folder.FolderIDs[0] == "c" {
			inZ = true
		}
		if folder.ID == "b" && strings.Join(folder.FolderIDs, ",") == "c" {
			inB = true
		}
	}
	if !inZ || !inB {
		t.Fatalf("multi-parent collect failed: inZ=%v inB=%v", inZ, inB)
	}

	// 再收藏到根级也应成立
	saved, err = system.AddFolderToFolder(ctx, types.PromptScopeGlobal, "", "c", "")
	if err != nil {
		t.Fatalf("AddFolderToFolder() to root error = %v", err)
	}
	foundAtRoot := false
	for _, id := range saved.RootFolderIDs {
		if id == "c" {
			foundAtRoot = true
		}
	}
	if !foundAtRoot {
		t.Fatalf("root folder ids = %v, want c included", saved.RootFolderIDs)
	}

	if _, err := system.AddFolderToFolder(ctx, types.PromptScopeGlobal, "", "c", "z"); err == nil {
		t.Fatal("AddFolderToFolder() duplicate = nil error, want failure")
	}
}

// 摘除只影响指定位置：别处的引用与夹本身都保留。
func TestRemoveFolderFromParentKeepsOtherReferences(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	seedFolderActions(t, system)

	if _, err := system.AddFolderToFolder(ctx, types.PromptScopeGlobal, "", "c", "z"); err != nil {
		t.Fatalf("AddFolderToFolder() error = %v", err)
	}
	saved, err := system.RemoveFolderFromParent(ctx, types.PromptScopeGlobal, "", "c", "z")
	if err != nil {
		t.Fatalf("RemoveFolderFromParent() error = %v", err)
	}
	for _, folder := range saved.Folders {
		if folder.ID == "z" && len(folder.FolderIDs) != 0 {
			t.Fatalf("folder z children = %v, want empty", folder.FolderIDs)
		}
		if folder.ID == "b" && strings.Join(folder.FolderIDs, ",") != "c" {
			t.Fatalf("folder b children = %v, want c kept", folder.FolderIDs)
		}
	}
	if _, err := system.RemoveFolderFromParent(ctx, types.PromptScopeGlobal, "", "c", "z"); err == nil {
		t.Fatal("RemoveFolderFromParent() twice = nil error, want failure")
	}
}
