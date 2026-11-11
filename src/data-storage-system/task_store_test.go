package datastorage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

// 任务只有角色与工作区两个作用域：各自落在自己的实体目录里，全局没有任务。
func TestTaskTreeScopesLandInOwnerDirectories(t *testing.T) {
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
		{types.TaskScopeRole, "developer", filepath.Join(system.paths.root, "roles", "developer", "task-tree.json")},
		{types.TaskScopeWorkspace, "frontend", filepath.Join(system.paths.root, "workspaces", "frontend", "task-tree.json")},
	}
	for _, tc := range cases {
		tree := types.TaskTree{Tasks: []types.TaskNode{{ID: "t1", Name: "第一层", Content: "复制我"}}}
		if _, err := system.SaveTaskTree(ctx, tc.scope, tc.owner, tree); err != nil {
			t.Fatalf("SaveTaskTree(scope=%s) error = %v", tc.scope, err)
		}
		assertFile(t, tc.want)
	}
}

// 全局作用域与不存在的归属都必须如实报错，不凭空生成实体目录。
func TestTaskTreeRejectsGlobalScopeAndMissingOwner(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()

	if _, err := system.LoadTaskTree(ctx, "global", ""); err == nil {
		t.Fatal("LoadTaskTree(global) = nil error, want failure")
	} else if !strings.Contains(err.Error(), "unsupported task scope") {
		t.Fatalf("global scope error = %v", err)
	}

	tree := types.TaskTree{Tasks: []types.TaskNode{{ID: "t1", Name: "第一层"}}}
	if _, err := system.SaveTaskTree(ctx, types.TaskScopeRole, "ghost", tree); err == nil {
		t.Fatal("SaveTaskTree(missing owner) = nil error, want failure")
	} else if !strings.Contains(err.Error(), "task scope owner does not exist") {
		t.Fatalf("missing owner error = %v", err)
	}
}

// 读、写、删闭环：树结构与顺序原样往返，空集合落盘后仍是空切片。
func TestTaskTreeReadWriteDeleteCycle(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	if err := system.SaveRole(ctx, types.Role{ID: "developer", Name: "Developer"}); err != nil {
		t.Fatalf("SaveRole() error = %v", err)
	}

	empty, err := system.LoadTaskTree(ctx, types.TaskScopeRole, "developer")
	if err != nil {
		t.Fatalf("LoadTaskTree() error = %v", err)
	}
	if empty.Tasks == nil || len(empty.Tasks) != 0 {
		t.Fatalf("empty tree = %#v, want empty slice", empty.Tasks)
	}

	tree := types.TaskTree{Tasks: []types.TaskNode{
		{ID: "t1", Name: "梳理需求", Description: "先看原型", Content: "复制正文一", CopyHighlight: true, Children: []types.TaskNode{
			{ID: "t1-1", Name: "读代码", Content: "复制正文二"},
		}},
		{ID: "t2", Name: "写实现"},
	}}
	saved, err := system.SaveTaskTree(ctx, types.TaskScopeRole, "developer", tree)
	if err != nil {
		t.Fatalf("SaveTaskTree() error = %v", err)
	}
	if saved.Tasks[0].CreatedAt == "" || saved.Tasks[0].UpdatedAt == "" {
		t.Fatalf("saved timestamps = %#v, want both filled", saved.Tasks[0])
	}
	if len(saved.Tasks[0].Children) != 1 || saved.Tasks[0].Children[0].Children == nil {
		t.Fatalf("saved children = %#v, want nested child with empty slice", saved.Tasks[0].Children)
	}

	loaded, err := system.LoadTaskTree(ctx, types.TaskScopeRole, "developer")
	if err != nil {
		t.Fatalf("LoadTaskTree() error = %v", err)
	}
	if len(loaded.Tasks) != 2 || loaded.Tasks[0].ID != "t1" || loaded.Tasks[1].ID != "t2" {
		t.Fatalf("loaded tree order = %#v", loaded.Tasks)
	}
	if loaded.Tasks[0].Children[0].Content != "复制正文二" {
		t.Fatalf("loaded child content = %q", loaded.Tasks[0].Children[0].Content)
	}
	// 卡片展示标记原样往返；没写这个字段的节点读出来就是关闭。
	if !loaded.Tasks[0].CopyHighlight {
		t.Fatal("loaded copy highlight = false, want true")
	}
	if loaded.Tasks[1].CopyHighlight {
		t.Fatal("loaded copy highlight = true, want false for a node without the field")
	}

	afterDelete, err := system.DeleteTaskNode(ctx, types.TaskScopeRole, "developer", "t1")
	if err != nil {
		t.Fatalf("DeleteTaskNode() error = %v", err)
	}
	if len(afterDelete.Tasks) != 1 || afterDelete.Tasks[0].ID != "t2" {
		t.Fatalf("tree after delete = %#v", afterDelete.Tasks)
	}

	if _, err := system.DeleteTaskNode(ctx, types.TaskScopeRole, "developer", "t1"); err == nil {
		t.Fatal("DeleteTaskNode(missing) = nil error, want failure")
	} else if !strings.Contains(err.Error(), "task does not exist") {
		t.Fatalf("missing task error = %v", err)
	}
}

// 存储层维护时间戳：新节点补创建时间，自身内容或子节点集合变了才刷新更新时间。
func TestNormalizeTaskTreeForSaveTimestamps(t *testing.T) {
	created := "2026-01-01T00:00:00Z"
	updated := "2026-02-02T00:00:00Z"
	previous := types.TaskTree{Tasks: []types.TaskNode{
		{ID: "a", Name: "父", Content: "父正文", CreatedAt: created, UpdatedAt: created, Children: []types.TaskNode{
			{ID: "b", Name: "子一", Content: "子正文", CreatedAt: created, UpdatedAt: created},
			{ID: "c", Name: "子二", Content: "", CreatedAt: created, UpdatedAt: created},
		}},
	}}

	sameContent := types.TaskTree{Tasks: []types.TaskNode{
		{ID: "a", Name: "父", Content: "父正文", Children: []types.TaskNode{
			{ID: "b", Name: "子一", Content: "子正文"},
			{ID: "c", Name: "子二", Content: ""},
		}},
	}}
	saved, err := normalizeTaskTreeForSave(sameContent, previous, updated)
	if err != nil {
		t.Fatalf("normalizeTaskTreeForSave() error = %v", err)
	}
	if saved.Tasks[0].UpdatedAt != created || saved.Tasks[0].Children[0].UpdatedAt != created {
		t.Fatalf("unchanged tree timestamps = %#v", saved.Tasks)
	}

	changedContent := types.TaskTree{Tasks: []types.TaskNode{
		{ID: "a", Name: "父", Content: "父正文", Children: []types.TaskNode{
			{ID: "b", Name: "子一", Content: "子正文改了"},
			{ID: "c", Name: "子二", Content: ""},
		}},
	}}
	saved, err = normalizeTaskTreeForSave(changedContent, previous, updated)
	if err != nil {
		t.Fatalf("normalizeTaskTreeForSave() error = %v", err)
	}
	if saved.Tasks[0].Children[0].CreatedAt != created || saved.Tasks[0].Children[0].UpdatedAt != updated {
		t.Fatalf("changed child timestamps = %#v, want createdAt kept and updatedAt refreshed", saved.Tasks[0].Children[0])
	}
	// 只改子节点内容，父节点自己的内容与子节点集合都没变，父节点时间不动。
	if saved.Tasks[0].UpdatedAt != created {
		t.Fatalf("parent updatedAt = %q, want %q", saved.Tasks[0].UpdatedAt, created)
	}

	// 子节点顺序变化算父节点本身变了。
	reordered := types.TaskTree{Tasks: []types.TaskNode{
		{ID: "a", Name: "父", Content: "父正文", Children: []types.TaskNode{
			{ID: "c", Name: "子二", Content: ""},
			{ID: "b", Name: "子一", Content: "子正文"},
		}},
	}}
	saved, err = normalizeTaskTreeForSave(reordered, previous, updated)
	if err != nil {
		t.Fatalf("normalizeTaskTreeForSave() error = %v", err)
	}
	if saved.Tasks[0].UpdatedAt != updated {
		t.Fatalf("reordered parent updatedAt = %q, want %q", saved.Tasks[0].UpdatedAt, updated)
	}

	// 只翻转卡片展示标记也算节点自己变了：该节点刷新时间，同层其他节点不动。
	highlighted := types.TaskTree{Tasks: []types.TaskNode{
		{ID: "a", Name: "父", Content: "父正文", CopyHighlight: true, Children: []types.TaskNode{
			{ID: "b", Name: "子一", Content: "子正文"},
			{ID: "c", Name: "子二", Content: ""},
		}},
	}}
	saved, err = normalizeTaskTreeForSave(highlighted, previous, updated)
	if err != nil {
		t.Fatalf("normalizeTaskTreeForSave() error = %v", err)
	}
	if !saved.Tasks[0].CopyHighlight {
		t.Fatal("copy highlight = false, want kept")
	}
	if saved.Tasks[0].UpdatedAt != updated || saved.Tasks[0].Children[0].UpdatedAt != created {
		t.Fatalf("copy highlight timestamps = %#v, want only the toggled node refreshed", saved.Tasks[0])
	}

	added := types.TaskTree{Tasks: []types.TaskNode{
		{ID: "d", Name: "新节点", Content: "新正文", CreatedAt: "1999-01-01T00:00:00Z", UpdatedAt: "1999-01-01T00:00:00Z"},
	}}
	saved, err = normalizeTaskTreeForSave(added, previous, updated)
	if err != nil {
		t.Fatalf("normalizeTaskTreeForSave() error = %v", err)
	}
	if saved.Tasks[0].CreatedAt != updated || saved.Tasks[0].UpdatedAt != updated {
		t.Fatalf("new node timestamps = %#v, want storage-given timestamps", saved.Tasks[0])
	}
}

// id 是身份：空 id 与全树重复 id 都拒绝，重复检查跨层级生效。
func TestTaskTreeRejectsInvalidIDs(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	if err := system.SaveRole(ctx, types.Role{ID: "developer", Name: "Developer"}); err != nil {
		t.Fatalf("SaveRole() error = %v", err)
	}

	if _, err := system.SaveTaskTree(ctx, types.TaskScopeRole, "developer", types.TaskTree{Tasks: []types.TaskNode{{ID: " "}}}); err == nil {
		t.Fatal("SaveTaskTree(empty id) = nil error, want failure")
	} else if !strings.Contains(err.Error(), "task id is required") {
		t.Fatalf("empty id error = %v", err)
	}

	duplicated := types.TaskTree{Tasks: []types.TaskNode{
		{ID: "a", Children: []types.TaskNode{{ID: "a"}}},
	}}
	if _, err := system.SaveTaskTree(ctx, types.TaskScopeRole, "developer", duplicated); err == nil {
		t.Fatal("SaveTaskTree(duplicate id) = nil error, want failure")
	} else if !strings.Contains(err.Error(), "duplicate task id") {
		t.Fatalf("duplicate id error = %v", err)
	}
}

// 新建把子树挂到父级末尾；父级不存在或 id 重复都在写盘前失败。
func TestCreateTaskNodeAppendsToLayer(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	if err := system.SaveRole(ctx, types.Role{ID: "developer", Name: "Developer"}); err != nil {
		t.Fatalf("SaveRole() error = %v", err)
	}
	if _, err := system.SaveTaskTree(ctx, types.TaskScopeRole, "developer", types.TaskTree{Tasks: []types.TaskNode{
		{ID: "a", Name: "父"},
	}}); err != nil {
		t.Fatalf("SaveTaskTree() error = %v", err)
	}

	added, err := system.CreateTaskNode(ctx, types.TaskScopeRole, "developer", "a", types.TaskNode{ID: "a-1", Name: "子"})
	if err != nil {
		t.Fatalf("CreateTaskNode(parent) error = %v", err)
	}
	if len(added.Tasks[0].Children) != 1 || added.Tasks[0].Children[0].ID != "a-1" {
		t.Fatalf("children after create = %#v", added.Tasks[0].Children)
	}

	rooted, err := system.CreateTaskNode(ctx, types.TaskScopeRole, "developer", "", types.TaskNode{ID: "b", Name: "根级"})
	if err != nil {
		t.Fatalf("CreateTaskNode(root) error = %v", err)
	}
	if len(rooted.Tasks) != 2 || rooted.Tasks[1].ID != "b" {
		t.Fatalf("root layer after create = %#v", rooted.Tasks)
	}

	if _, err := system.CreateTaskNode(ctx, types.TaskScopeRole, "developer", "ghost", types.TaskNode{ID: "c"}); err == nil {
		t.Fatal("CreateTaskNode(missing parent) = nil error, want failure")
	} else if !strings.Contains(err.Error(), "task parent does not exist") {
		t.Fatalf("missing parent error = %v", err)
	}

	if _, err := system.CreateTaskNode(ctx, types.TaskScopeRole, "developer", "", types.TaskNode{ID: "a"}); err == nil {
		t.Fatal("CreateTaskNode(duplicate id) = nil error, want failure")
	} else if !strings.Contains(err.Error(), "duplicate task id") {
		t.Fatalf("duplicate id error = %v", err)
	}
}

// 定点动作在副本上改：前一版对照必须保持原样，
// 否则新节点会被当成「早就存在」，父节点也看不到子节点集合的变化。
func TestCreateTaskNodeKeepsPreviousSnapshotIntact(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	if err := system.SaveRole(ctx, types.Role{ID: "developer", Name: "Developer"}); err != nil {
		t.Fatalf("SaveRole() error = %v", err)
	}
	file := filepath.Join(system.paths.root, "roles", "developer", "task-tree.json")
	fixture := types.TaskTree{Tasks: []types.TaskNode{{
		ID: "a", Name: "父", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z",
	}}}
	if err := writeJSON(ctx, file, fixture); err != nil {
		t.Fatalf("writeJSON(fixture) error = %v", err)
	}

	added, err := system.CreateTaskNode(ctx, types.TaskScopeRole, "developer", "a", types.TaskNode{ID: "a-1", Name: "子"})
	if err != nil {
		t.Fatalf("CreateTaskNode() error = %v", err)
	}
	child := added.Tasks[0].Children[0]
	if child.CreatedAt == "" || child.UpdatedAt == "" {
		t.Fatalf("new node timestamps = %#v, want storage-given timestamps", child)
	}
	if added.Tasks[0].CreatedAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("parent createdAt = %q, want kept", added.Tasks[0].CreatedAt)
	}
	if added.Tasks[0].UpdatedAt == "2026-01-01T00:00:00Z" {
		t.Fatalf("parent updatedAt = %q, want refreshed by the new child", added.Tasks[0].UpdatedAt)
	}
}

// 删节点即删整棵子树；移动不能进自己的后代，也不能原地移动。
func TestTaskNodeDeleteSubtreeAndMoveRules(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	if err := system.SaveRole(ctx, types.Role{ID: "developer", Name: "Developer"}); err != nil {
		t.Fatalf("SaveRole() error = %v", err)
	}
	if _, err := system.SaveTaskTree(ctx, types.TaskScopeRole, "developer", types.TaskTree{Tasks: []types.TaskNode{
		{ID: "a", Children: []types.TaskNode{
			{ID: "b", Children: []types.TaskNode{{ID: "c"}}},
		}},
		{ID: "d"},
	}}); err != nil {
		t.Fatalf("SaveTaskTree() error = %v", err)
	}

	afterDelete, err := system.DeleteTaskNode(ctx, types.TaskScopeRole, "developer", "b")
	if err != nil {
		t.Fatalf("DeleteTaskNode() error = %v", err)
	}
	if len(afterDelete.Tasks[0].Children) != 0 {
		t.Fatalf("children after subtree delete = %#v, want empty", afterDelete.Tasks[0].Children)
	}

	moved, err := system.MoveTaskNode(ctx, types.TaskScopeRole, "developer", "d", "a")
	if err != nil {
		t.Fatalf("MoveTaskNode() error = %v", err)
	}
	if len(moved.Tasks) != 1 || moved.Tasks[0].ID != "a" {
		t.Fatalf("root after move = %#v", moved.Tasks)
	}
	if len(moved.Tasks[0].Children) != 1 || moved.Tasks[0].Children[0].ID != "d" {
		t.Fatalf("children after move = %#v", moved.Tasks[0].Children)
	}

	if _, err := system.MoveTaskNode(ctx, types.TaskScopeRole, "developer", "a", "d"); err == nil {
		t.Fatal("MoveTaskNode(into own subtree) = nil error, want failure")
	} else if !strings.Contains(err.Error(), "cannot be moved into its own subtree") {
		t.Fatalf("own subtree error = %v", err)
	}

	if _, err := system.MoveTaskNode(ctx, types.TaskScopeRole, "developer", "d", "a"); err == nil {
		t.Fatal("MoveTaskNode(same parent) = nil error, want failure")
	} else if !strings.Contains(err.Error(), "already lives here") {
		t.Fatalf("same parent error = %v", err)
	}

	if _, err := system.MoveTaskNode(ctx, types.TaskScopeRole, "developer", "ghost", ""); err == nil {
		t.Fatal("MoveTaskNode(missing task) = nil error, want failure")
	}
}

// 重排必须提交本层全部子节点的一次完整排列；parentId 为空表示根级。
func TestReorderTaskChildrenRequiresCompleteLayer(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	if err := system.SaveRole(ctx, types.Role{ID: "developer", Name: "Developer"}); err != nil {
		t.Fatalf("SaveRole() error = %v", err)
	}
	if _, err := system.SaveTaskTree(ctx, types.TaskScopeRole, "developer", types.TaskTree{Tasks: []types.TaskNode{
		{ID: "a", Children: []types.TaskNode{{ID: "c"}, {ID: "d"}}},
		{ID: "b"},
	}}); err != nil {
		t.Fatalf("SaveTaskTree() error = %v", err)
	}

	reordered, err := system.ReorderTaskChildren(ctx, types.TaskScopeRole, "developer", "", []string{"b", "a"})
	if err != nil {
		t.Fatalf("ReorderTaskChildren(root) error = %v", err)
	}
	if reordered.Tasks[0].ID != "b" || reordered.Tasks[1].ID != "a" {
		t.Fatalf("root order = %#v", reordered.Tasks)
	}

	inner, err := system.ReorderTaskChildren(ctx, types.TaskScopeRole, "developer", "a", []string{"d", "c"})
	if err != nil {
		t.Fatalf("ReorderTaskChildren(inner) error = %v", err)
	}
	if inner.Tasks[1].Children[0].ID != "d" {
		t.Fatalf("inner order = %#v", inner.Tasks[1].Children)
	}

	cases := []struct {
		name string
		ids  []string
		want string
	}{
		{"少一个", []string{"b"}, "must list every child exactly once"},
		{"重复", []string{"b", "b"}, "duplicate id"},
		{"层外节点", []string{"b", "c"}, "outside the layer"},
		{"空 id", []string{"b", ""}, "empty id"},
	}
	for _, tc := range cases {
		if _, err := system.ReorderTaskChildren(ctx, types.TaskScopeRole, "developer", "", tc.ids); err == nil {
			t.Fatalf("ReorderTaskChildren(%s) = nil error, want failure", tc.name)
		} else if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("ReorderTaskChildren(%s) error = %v", tc.name, err)
		}
	}

	if _, err := system.ReorderTaskChildren(ctx, types.TaskScopeRole, "developer", "ghost", []string{"x"}); err == nil {
		t.Fatal("ReorderTaskChildren(missing parent) = nil error, want failure")
	}
}
