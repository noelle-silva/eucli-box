package datastorage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

// 收藏夹与提示词本体同目录同作用域，三处各一份文件。
func TestPromptFoldersScopesLandInOwnerDirectories(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	if err := system.SaveRole(ctx, types.Role{ID: "developer", Name: "Developer"}); err != nil {
		t.Fatalf("SaveRole() error = %v", err)
	}
	if err := system.SaveWorkspace(ctx, types.Workspace{ID: "frontend", Name: "Frontend", RegisteredRoleIDs: []string{}}); err != nil {
		t.Fatalf("SaveWorkspace() error = %v", err)
	}

	cases := []struct {
		scope string
		owner string
		want  string
	}{
		{types.PromptScopeGlobal, "", filepath.Join(system.paths.root, ".meta", "prompt-folders.json")},
		{types.PromptScopeRole, "developer", filepath.Join(system.paths.root, "roles", "developer", "prompt-folders.json")},
		{types.PromptScopeWorkspace, "frontend", filepath.Join(system.paths.root, "workspaces", "frontend", "prompt-folders.json")},
	}
	for _, tc := range cases {
		if _, err := system.SavePromptFolders(ctx, tc.scope, tc.owner, emptyPromptFolderLibrary()); err != nil {
			t.Fatalf("SavePromptFolders(scope=%s) error = %v", tc.scope, err)
		}
		assertFile(t, tc.want)
	}
}

// 嵌套与顺序都要原样往返：根级顺序、子夹顺序、条目顺序都由数组顺序表达。
func TestPromptFoldersNestingAndOrderRoundTrip(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	if _, err := system.SavePromptLibrary(ctx, types.PromptScopeGlobal, "", types.PromptLibrary{Prompts: []types.Prompt{
		{ID: "p1", Name: "一", Content: "内容一"},
		{ID: "p2", Name: "二", Content: "内容二"},
		{ID: "p3", Name: "三", Content: "内容三"},
	}}); err != nil {
		t.Fatalf("SavePromptLibrary() error = %v", err)
	}

	library := types.PromptFolderLibrary{
		RootFolderIDs: []string{"work", "life"},
		Folders: []types.PromptFolder{
			{ID: "work", Name: "工作", FolderIDs: []string{"work-backend"}, PromptIDs: []string{"p2", "p1"}},
			{ID: "work-backend", Name: "后端", FolderIDs: []string{}, PromptIDs: []string{"p3"}},
			{ID: "life", Name: "生活", FolderIDs: []string{}, PromptIDs: []string{"p1", "p3"}},
		},
	}
	saved, err := system.SavePromptFolders(ctx, types.PromptScopeGlobal, "", library)
	if err != nil {
		t.Fatalf("SavePromptFolders() error = %v", err)
	}
	if saved.Folders[0].CreatedAt == "" {
		t.Fatalf("saved folder has no createdAt: %#v", saved.Folders[0])
	}

	loaded, err := system.LoadPromptFolders(ctx, types.PromptScopeGlobal, "")
	if err != nil {
		t.Fatalf("LoadPromptFolders() error = %v", err)
	}
	if strings.Join(loaded.RootFolderIDs, ",") != "work,life" {
		t.Fatalf("root order = %v", loaded.RootFolderIDs)
	}
	if strings.Join(loaded.Folders[0].FolderIDs, ",") != "work-backend" {
		t.Fatalf("child order = %v", loaded.Folders[0].FolderIDs)
	}
	if strings.Join(loaded.Folders[0].PromptIDs, ",") != "p2,p1" {
		t.Fatalf("prompt order = %v", loaded.Folders[0].PromptIDs)
	}
	if strings.Join(loaded.Folders[2].PromptIDs, ",") != "p1,p3" {
		t.Fatalf("second root prompt order = %v", loaded.Folders[2].PromptIDs)
	}
}

// 结构问题一律快速失败：多父挂载、孤儿、缺子夹、自引用、同夹重复索引、悬空索引。
func TestPromptFoldersRejectBrokenStructures(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	if _, err := system.SavePromptLibrary(ctx, types.PromptScopeGlobal, "", types.PromptLibrary{Prompts: []types.Prompt{
		{ID: "p1", Name: "一", Content: "内容一"},
	}}); err != nil {
		t.Fatalf("SavePromptLibrary() error = %v", err)
	}

	cases := []struct {
		name    string
		library types.PromptFolderLibrary
		want    string
	}{
		{
			"missing child",
			types.PromptFolderLibrary{
				RootFolderIDs: []string{"a"},
				Folders: []types.PromptFolder{
					{ID: "a", Name: "甲", FolderIDs: []string{"ghost"}, PromptIDs: []string{}},
				},
			},
			"references missing folder",
		},
		{
			"self reference",
			types.PromptFolderLibrary{
				RootFolderIDs: []string{"a"},
				Folders: []types.PromptFolder{
					{ID: "a", Name: "甲", FolderIDs: []string{"a"}, PromptIDs: []string{}},
				},
			},
			"cannot contain itself",
		},
		{
			"duplicate prompt index",
			types.PromptFolderLibrary{
				RootFolderIDs: []string{"a"},
				Folders: []types.PromptFolder{
					{ID: "a", Name: "甲", FolderIDs: []string{}, PromptIDs: []string{"p1", "p1"}},
				},
			},
			"indexes prompt more than once",
		},
		{
			"dangling prompt index",
			types.PromptFolderLibrary{
				RootFolderIDs: []string{"a"},
				Folders: []types.PromptFolder{
					{ID: "a", Name: "甲", FolderIDs: []string{}, PromptIDs: []string{"ghost-prompt"}},
				},
			},
			"indexes missing prompt",
		},
		{
			"missing name",
			types.PromptFolderLibrary{
				RootFolderIDs: []string{"a"},
				Folders: []types.PromptFolder{
					{ID: "a", Name: " ", FolderIDs: []string{}, PromptIDs: []string{}},
				},
			},
			"prompt folder name is required",
		},
	}
	for _, tc := range cases {
		if _, err := system.SavePromptFolders(ctx, types.PromptScopeGlobal, "", tc.library); err == nil {
			t.Fatalf("%s: SavePromptFolders() = nil error, want failure", tc.name)
		} else if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: error = %v, want %q", tc.name, err, tc.want)
		}
	}
	if dataFileExists(filepath.Join(system.paths.root, ".meta", "prompt-folders.json")) {
		t.Fatal("rejected folder library was written to disk")
	}
}

// 删除本体时，所有收藏夹里指向它的索引同时摘掉，其余条目顺序保持不变。
func TestDeletePromptDetachesFolderIndexes(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	if _, err := system.SavePromptLibrary(ctx, types.PromptScopeGlobal, "", types.PromptLibrary{Prompts: []types.Prompt{
		{ID: "p1", Name: "一", Content: "内容一"},
		{ID: "p2", Name: "二", Content: "内容二"},
		{ID: "p3", Name: "三", Content: "内容三"},
	}}); err != nil {
		t.Fatalf("SavePromptLibrary() error = %v", err)
	}
	if _, err := system.SavePromptFolders(ctx, types.PromptScopeGlobal, "", types.PromptFolderLibrary{
		RootFolderIDs: []string{"a", "b"},
		Folders: []types.PromptFolder{
			{ID: "a", Name: "甲", FolderIDs: []string{}, PromptIDs: []string{"p1", "p2", "p3"}},
			{ID: "b", Name: "乙", FolderIDs: []string{}, PromptIDs: []string{"p2"}},
		},
	}); err != nil {
		t.Fatalf("SavePromptFolders() error = %v", err)
	}

	if _, err := system.DeletePrompt(ctx, types.PromptScopeGlobal, "", "p2"); err != nil {
		t.Fatalf("DeletePrompt() error = %v", err)
	}

	folders, err := system.LoadPromptFolders(ctx, types.PromptScopeGlobal, "")
	if err != nil {
		t.Fatalf("LoadPromptFolders() error = %v", err)
	}
	if strings.Join(folders.Folders[0].PromptIDs, ",") != "p1,p3" {
		t.Fatalf("folder a prompt ids = %v, want p1,p3", folders.Folders[0].PromptIDs)
	}
	if len(folders.Folders[1].PromptIDs) != 0 {
		t.Fatalf("folder b prompt ids = %v, want empty", folders.Folders[1].PromptIDs)
	}

	library, err := system.LoadPromptLibrary(ctx, types.PromptScopeGlobal, "")
	if err != nil {
		t.Fatalf("LoadPromptLibrary() error = %v", err)
	}
	if len(library.Prompts) != 2 {
		t.Fatalf("prompt library size = %d, want 2", len(library.Prompts))
	}
}

// 收藏夹时间戳由存储维护：新建补时间，结构未变不动更新时间，变了才刷新。
func TestNormalizePromptFolderLibraryTimestamps(t *testing.T) {
	created := "2026-01-01T00:00:00Z"
	updated := "2026-02-02T00:00:00Z"
	prompts := types.PromptLibrary{Prompts: []types.Prompt{{ID: "p1", Name: "一", Content: "内容一"}}}
	previous := types.PromptFolderLibrary{
		RootFolderIDs: []string{"a"},
		Folders: []types.PromptFolder{
			{ID: "a", Name: "甲", FolderIDs: []string{}, PromptIDs: []string{"p1"}, CreatedAt: created, UpdatedAt: created},
		},
	}

	same := types.PromptFolderLibrary{
		RootFolderIDs: []string{"a"},
		Folders: []types.PromptFolder{
			{ID: "a", Name: "甲", FolderIDs: []string{}, PromptIDs: []string{"p1"}},
		},
	}
	saved, err := normalizePromptFolderLibraryForSave(same, previous, prompts, updated)
	if err != nil {
		t.Fatalf("normalizePromptFolderLibraryForSave() error = %v", err)
	}
	if saved.Folders[0].CreatedAt != created || saved.Folders[0].UpdatedAt != created {
		t.Fatalf("unchanged folder timestamps = %#v", saved.Folders[0])
	}

	renamed := types.PromptFolderLibrary{
		RootFolderIDs: []string{"a"},
		Folders: []types.PromptFolder{
			{ID: "a", Name: "甲改名", FolderIDs: []string{}, PromptIDs: []string{"p1"}},
		},
	}
	saved, err = normalizePromptFolderLibraryForSave(renamed, previous, prompts, updated)
	if err != nil {
		t.Fatalf("normalizePromptFolderLibraryForSave() error = %v", err)
	}
	if saved.Folders[0].CreatedAt != created || saved.Folders[0].UpdatedAt != updated {
		t.Fatalf("changed folder timestamps = %#v", saved.Folders[0])
	}

	fresh := types.PromptFolderLibrary{
		RootFolderIDs: []string{"b"},
		Folders: []types.PromptFolder{
			{ID: "b", Name: "新建", FolderIDs: []string{}, PromptIDs: []string{}},
		},
	}
	saved, err = normalizePromptFolderLibraryForSave(fresh, previous, prompts, updated)
	if err != nil {
		t.Fatalf("normalizePromptFolderLibraryForSave() error = %v", err)
	}
	if saved.Folders[0].CreatedAt != updated || saved.Folders[0].UpdatedAt != updated {
		t.Fatalf("new folder timestamps = %#v", saved.Folders[0])
	}
}

// 空库读取给出空集合而不是 null，且空收藏夹相邻也能保存。
func TestPromptFoldersEmptyLibrary(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	empty, err := system.LoadPromptFolders(ctx, types.PromptScopeGlobal, "")
	if err != nil {
		t.Fatalf("LoadPromptFolders() error = %v", err)
	}
	if empty.RootFolderIDs == nil || empty.Folders == nil {
		t.Fatalf("empty folder library = %#v, want empty slices", empty)
	}
	if _, err := system.SavePromptFolders(ctx, types.PromptScopeGlobal, "", types.PromptFolderLibrary{
		RootFolderIDs: []string{"a", "b"},
		Folders: []types.PromptFolder{
			{ID: "a", Name: "甲", FolderIDs: []string{}, PromptIDs: []string{}},
			{ID: "b", Name: "乙", FolderIDs: []string{}, PromptIDs: []string{}},
		},
	}); err != nil {
		t.Fatalf("SavePromptFolders() error = %v", err)
	}
	loaded, err := system.LoadPromptFolders(ctx, types.PromptScopeGlobal, "")
	if err != nil {
		t.Fatalf("LoadPromptFolders() error = %v", err)
	}
	if strings.Join(loaded.RootFolderIDs, ",") != "a,b" {
		t.Fatalf("root order = %v", loaded.RootFolderIDs)
	}
	if loaded.Folders[0].PromptIDs == nil {
		t.Fatal("empty prompt index should round-trip as empty slice")
	}
}

// 删除收藏夹的三种情形：父级还引用、删中间夹留下孤儿都被拒绝；
// 正确删除只摘索引关系，提示词本体一条都不少。
func TestDeletePromptFolderScenarios(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	if _, err := system.SavePromptLibrary(ctx, types.PromptScopeGlobal, "", types.PromptLibrary{Prompts: []types.Prompt{
		{ID: "p1", Name: "一", Content: "内容一"},
		{ID: "p2", Name: "二", Content: "内容二"},
	}}); err != nil {
		t.Fatalf("SavePromptLibrary() error = %v", err)
	}
	base := types.PromptFolderLibrary{
		RootFolderIDs: []string{"a", "c"},
		Folders: []types.PromptFolder{
			{ID: "a", Name: "甲", FolderIDs: []string{"b"}, PromptIDs: []string{"p1"}},
			{ID: "b", Name: "乙", FolderIDs: []string{}, PromptIDs: []string{"p2"}},
			{ID: "c", Name: "丙", FolderIDs: []string{}, PromptIDs: []string{}},
		},
	}
	if _, err := system.SavePromptFolders(ctx, types.PromptScopeGlobal, "", base); err != nil {
		t.Fatalf("SavePromptFolders() error = %v", err)
	}

	// 只从 Folders 里删掉子夹，父级的 FolderIDs 还引用它。
	stillReferenced := types.PromptFolderLibrary{
		RootFolderIDs: []string{"a", "c"},
		Folders: []types.PromptFolder{
			{ID: "a", Name: "甲", FolderIDs: []string{"b"}, PromptIDs: []string{"p1"}},
			{ID: "c", Name: "丙", FolderIDs: []string{}, PromptIDs: []string{}},
		},
	}
	if _, err := system.SavePromptFolders(ctx, types.PromptScopeGlobal, "", stillReferenced); err == nil {
		t.Fatal("deleting a referenced child folder = nil error, want failure")
	} else if !strings.Contains(err.Error(), "references missing folder") {
		t.Fatalf("referenced child error = %v", err)
	}

	// 删掉中间夹后，子夹仍可存在：收藏夹是索引，允许不出现在根级下面。
	orphanChild := types.PromptFolderLibrary{
		RootFolderIDs: []string{"c"},
		Folders: []types.PromptFolder{
			{ID: "b", Name: "乙", FolderIDs: []string{}, PromptIDs: []string{"p2"}},
			{ID: "c", Name: "丙", FolderIDs: []string{}, PromptIDs: []string{}},
		},
	}
	if _, err := system.SavePromptFolders(ctx, types.PromptScopeGlobal, "", orphanChild); err != nil {
		t.Fatalf("SavePromptFolders() with detached folder error = %v", err)
	}

	// 正确删除：父的 FolderIDs 一并摘掉子夹，子夹从库里消失。
	removed := types.PromptFolderLibrary{
		RootFolderIDs: []string{"a", "c"},
		Folders: []types.PromptFolder{
			{ID: "a", Name: "甲", FolderIDs: []string{}, PromptIDs: []string{"p1"}},
			{ID: "c", Name: "丙", FolderIDs: []string{}, PromptIDs: []string{}},
		},
	}
	saved, err := system.SavePromptFolders(ctx, types.PromptScopeGlobal, "", removed)
	if err != nil {
		t.Fatalf("SavePromptFolders() error = %v", err)
	}
	if len(saved.Folders) != 2 {
		t.Fatalf("folders after delete = %d, want 2", len(saved.Folders))
	}
	if len(saved.Folders[0].FolderIDs) != 0 {
		t.Fatalf("parent folder ids after delete = %v, want empty", saved.Folders[0].FolderIDs)
	}
	if strings.Join(saved.Folders[0].PromptIDs, ",") != "p1" {
		t.Fatalf("parent prompt ids after delete = %v, want p1", saved.Folders[0].PromptIDs)
	}

	// 收藏夹的删除不碰提示词本体：p2 只是失去书签，内容还在。
	library, err := system.LoadPromptLibrary(ctx, types.PromptScopeGlobal, "")
	if err != nil {
		t.Fatalf("LoadPromptLibrary() error = %v", err)
	}
	if len(library.Prompts) != 2 {
		t.Fatalf("prompt library after folder delete = %d prompts, want 2", len(library.Prompts))
	}
}
