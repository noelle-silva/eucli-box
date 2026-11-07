package datastorage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

// 提示词仓库三作用域的落点：全局进 .meta，角色与工作区各进自己的实体目录。
func TestPromptLibraryScopesLandInOwnerDirectories(t *testing.T) {
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
		{types.PromptScopeGlobal, "", filepath.Join(system.paths.root, ".meta", "prompt-library.json")},
		{types.PromptScopeRole, "developer", filepath.Join(system.paths.root, "roles", "developer", "prompt-library.json")},
		{types.PromptScopeWorkspace, "frontend", filepath.Join(system.paths.root, "workspaces", "frontend", "prompt-library.json")},
	}
	for _, tc := range cases {
		library := types.PromptLibrary{Prompts: []types.Prompt{{ID: "p1", Name: "行文风格", Content: "多用短句"}}}
		if _, err := system.SavePromptLibrary(ctx, tc.scope, tc.owner, library); err != nil {
			t.Fatalf("SavePromptLibrary(scope=%s) error = %v", tc.scope, err)
		}
		assertFile(t, tc.want)
	}
}

// 提示词库是独立文件：身份的 data.json 里不应出现提示词正文。
func TestPromptLibraryDoesNotLeakIntoRoleDocument(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	if err := system.SaveRole(ctx, types.Role{ID: "developer", Name: "Developer"}); err != nil {
		t.Fatalf("SaveRole() error = %v", err)
	}
	library := types.PromptLibrary{Prompts: []types.Prompt{{ID: "p1", Name: "行文风格", Content: "多用短句"}}}
	if _, err := system.SavePromptLibrary(ctx, types.PromptScopeRole, "developer", library); err != nil {
		t.Fatalf("SavePromptLibrary() error = %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(system.paths.root, "roles", "developer", "data.json"))
	if err != nil {
		t.Fatalf("ReadFile(role data.json) error = %v", err)
	}
	if strings.Contains(string(payload), "多用短句") {
		t.Fatalf("role data.json contains prompt content: %s", payload)
	}
}

// 读、写、删闭环，删除不存在的条目如实报错。
func TestPromptLibraryReadWriteDeleteCycle(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()

	empty, err := system.LoadPromptLibrary(ctx, types.PromptScopeGlobal, "")
	if err != nil {
		t.Fatalf("LoadPromptLibrary() error = %v", err)
	}
	if empty.Prompts == nil || len(empty.Prompts) != 0 {
		t.Fatalf("empty library = %#v, want empty slice", empty.Prompts)
	}

	library := types.PromptLibrary{Prompts: []types.Prompt{
		{ID: "p1", Name: "行文风格", Content: "多用短句"},
		{ID: "p2", Name: "审阅口径", Content: "只报事实"},
	}}
	saved, err := system.SavePromptLibrary(ctx, types.PromptScopeGlobal, "", library)
	if err != nil {
		t.Fatalf("SavePromptLibrary() error = %v", err)
	}
	if len(saved.Prompts) != 2 || saved.Prompts[0].CreatedAt == "" || saved.Prompts[0].UpdatedAt == "" {
		t.Fatalf("saved library = %#v", saved)
	}

	loaded, err := system.LoadPromptLibrary(ctx, types.PromptScopeGlobal, "")
	if err != nil {
		t.Fatalf("LoadPromptLibrary() error = %v", err)
	}
	if len(loaded.Prompts) != 2 || loaded.Prompts[1].Name != "审阅口径" {
		t.Fatalf("loaded library = %#v", loaded)
	}

	afterDelete, err := system.DeletePrompt(ctx, types.PromptScopeGlobal, "", "p1")
	if err != nil {
		t.Fatalf("DeletePrompt() error = %v", err)
	}
	if len(afterDelete.Prompts) != 1 || afterDelete.Prompts[0].ID != "p2" {
		t.Fatalf("library after delete = %#v", afterDelete)
	}

	reloaded, err := system.LoadPromptLibrary(ctx, types.PromptScopeGlobal, "")
	if err != nil {
		t.Fatalf("LoadPromptLibrary() error = %v", err)
	}
	if len(reloaded.Prompts) != 1 || reloaded.Prompts[0].ID != "p2" {
		t.Fatalf("reloaded library = %#v", reloaded)
	}

	if _, err := system.DeletePrompt(ctx, types.PromptScopeGlobal, "", "p1"); err == nil {
		t.Fatal("DeletePrompt() on missing prompt = nil error, want failure")
	} else if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("DeletePrompt() error = %v, want missing prompt failure", err)
	}
}

// 必填与重复 id 一律快速失败，不静默丢弃条目。
func TestPromptLibraryRejectsInvalidEntries(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	cases := []struct {
		name    string
		library types.PromptLibrary
		want    string
	}{
		{"missing id", types.PromptLibrary{Prompts: []types.Prompt{{Name: "名称", Content: "内容"}}}, "prompt id is required"},
		{"missing name", types.PromptLibrary{Prompts: []types.Prompt{{ID: "p1", Content: "内容"}}}, "prompt name is required"},
		{"missing content", types.PromptLibrary{Prompts: []types.Prompt{{ID: "p1", Name: "名称", Content: "  "}}}, "prompt content is required"},
		{"duplicate id", types.PromptLibrary{Prompts: []types.Prompt{
			{ID: "p1", Name: "名称", Content: "内容"},
			{ID: "p1", Name: "另一个", Content: "内容"},
		}}, "duplicate prompt id"},
	}
	for _, tc := range cases {
		if _, err := system.SavePromptLibrary(ctx, types.PromptScopeGlobal, "", tc.library); err == nil {
			t.Fatalf("%s: SavePromptLibrary() = nil error, want failure", tc.name)
		} else if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: SavePromptLibrary() error = %v, want %q", tc.name, err, tc.want)
		}
	}
	if dataFileExists(filepath.Join(system.paths.root, ".meta", "prompt-library.json")) {
		t.Fatal("rejected library was written to disk")
	}
}

// 归属不存在、作用域未知、归属 id 越界三类调用错误都在落点解析处被拒绝。
func TestPromptLibraryRejectsUnknownScopeAndOwner(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	library := types.PromptLibrary{Prompts: []types.Prompt{{ID: "p1", Name: "名称", Content: "内容"}}}

	if _, err := system.SavePromptLibrary(ctx, types.PromptScopeRole, "missing-role", library); err == nil {
		t.Fatal("SavePromptLibrary() on missing role = nil error, want failure")
	}
	if _, err := system.SavePromptLibrary(ctx, types.PromptScopeWorkspace, "missing-workspace", library); err == nil {
		t.Fatal("SavePromptLibrary() on missing workspace = nil error, want failure")
	}
	if _, err := system.SavePromptLibrary(ctx, "planet", "", library); err == nil {
		t.Fatal("SavePromptLibrary() with unknown scope = nil error, want failure")
	} else if !strings.Contains(err.Error(), "unsupported prompt library scope") {
		t.Fatalf("unknown scope error = %v", err)
	}
	if _, err := system.SavePromptLibrary(ctx, types.PromptScopeRole, "../escape", library); err == nil {
		t.Fatal("SavePromptLibrary() with escaping owner id = nil error, want failure")
	}
}

// 存储层维护时间戳：新条目补创建时间，内容未变不动更新时间，内容变了才刷新。
func TestNormalizePromptLibraryForSaveTimestamps(t *testing.T) {
	created := "2026-01-01T00:00:00Z"
	updated := "2026-02-02T00:00:00Z"
	previous := types.PromptLibrary{Prompts: []types.Prompt{
		{ID: "p1", Name: "行文风格", Content: "多用短句", CreatedAt: created, UpdatedAt: created},
	}}

	sameContent := types.PromptLibrary{Prompts: []types.Prompt{
		{ID: "p1", Name: "行文风格", Content: "多用短句"},
	}}
	saved, err := normalizePromptLibraryForSave(sameContent, previous, updated)
	if err != nil {
		t.Fatalf("normalizePromptLibraryForSave() error = %v", err)
	}
	if saved.Prompts[0].CreatedAt != created || saved.Prompts[0].UpdatedAt != created {
		t.Fatalf("unchanged prompt timestamps = %#v, want %s/%s", saved.Prompts[0], created, created)
	}

	changedContent := types.PromptLibrary{Prompts: []types.Prompt{
		{ID: "p1", Name: "行文风格", Content: "多用长句"},
	}}
	saved, err = normalizePromptLibraryForSave(changedContent, previous, updated)
	if err != nil {
		t.Fatalf("normalizePromptLibraryForSave() error = %v", err)
	}
	if saved.Prompts[0].CreatedAt != created || saved.Prompts[0].UpdatedAt != updated {
		t.Fatalf("changed prompt timestamps = %#v, want %s/%s", saved.Prompts[0], created, updated)
	}

	added := types.PromptLibrary{Prompts: []types.Prompt{
		{ID: "p2", Name: "审阅口径", Content: "只报事实"},
	}}
	saved, err = normalizePromptLibraryForSave(added, previous, updated)
	if err != nil {
		t.Fatalf("normalizePromptLibraryForSave() error = %v", err)
	}
	if saved.Prompts[0].CreatedAt != updated || saved.Prompts[0].UpdatedAt != updated {
		t.Fatalf("new prompt timestamps = %#v, want %s/%s", saved.Prompts[0], updated, updated)
	}

	// 调用方带来的时间不采信，一律由存储给出。
	forged := types.PromptLibrary{Prompts: []types.Prompt{
		{ID: "p3", Name: "伪造时间", Content: "内容", CreatedAt: "1999-01-01T00:00:00Z", UpdatedAt: "1999-01-01T00:00:00Z"},
	}}
	saved, err = normalizePromptLibraryForSave(forged, types.PromptLibrary{}, updated)
	if err != nil {
		t.Fatalf("normalizePromptLibraryForSave() error = %v", err)
	}
	if saved.Prompts[0].CreatedAt != updated {
		t.Fatalf("forged timestamp kept: %#v", saved.Prompts[0])
	}
}
