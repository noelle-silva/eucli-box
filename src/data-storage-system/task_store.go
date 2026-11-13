package datastorage

import (
	"context"
	"slices"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

// 任务树：角色与工作区各一份，整棵树存一个文件。
//
// 与提示词库同构的是落点与身份规则：作用域决定文件落点、归属实体必须存在、
// id 做身份、时间字段由存储维护。不同的是树本身即归属：子节点由父节点的
// Children 数组表达，没有单独的索引文件，也就不存在「本体与索引」两份事实
// 需要对账。任务不参与提示词组装，也不进入模型上下文。

// LoadTaskTree 读取某个归属的任务树；文件尚不存在时返回空树。
func (s *system) LoadTaskTree(ctx context.Context, scope string, ownerID string) (types.TaskTree, error) {
	file, err := s.taskScopeFile(scope, ownerID)
	if err != nil {
		return types.TaskTree{}, err
	}
	return readTaskTree(ctx, file)
}

// SaveTaskTree 整树保存某个归属的任务树。
// 时间字段由存储维护：新节点补创建时间，自身内容或子节点集合真的变了才刷新更新时间。
func (s *system) SaveTaskTree(ctx context.Context, scope string, ownerID string, tree types.TaskTree) (types.TaskTree, error) {
	file, err := s.taskScopeFile(scope, ownerID)
	if err != nil {
		return types.TaskTree{}, err
	}

	s.entityMu.Lock()
	defer s.entityMu.Unlock()

	previous, err := readTaskTree(ctx, file)
	if err != nil {
		return types.TaskTree{}, err
	}
	normalized, err := normalizeTaskTreeForSave(tree, previous, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return types.TaskTree{}, err
	}
	if err := writeJSON(ctx, file, normalized); err != nil {
		return types.TaskTree{}, storageWriteFailed("failed to save task tree", err)
	}
	return normalized, nil
}

// DeleteTaskNode 删除一个节点连同它的整棵子树：子节点只存在于父节点之下，
// 父节点不在了它们也就不在了。节点不存在时明确报错。
func (s *system) DeleteTaskNode(ctx context.Context, scope string, ownerID string, taskID string) (types.TaskTree, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return types.TaskTree{}, storageInvalid("task id is required", nil)
	}
	return s.mutateTaskTree(ctx, scope, ownerID, func(tree types.TaskTree) (types.TaskTree, error) {
		next, removed := removeTaskFromLayer(tree.Tasks, taskID)
		if !removed {
			return types.TaskTree{}, storageInvalid("task does not exist: "+taskID, nil)
		}
		tree.Tasks = next
		return tree, nil
	})
}

// CreateTaskNode 把提交的子树挂到某个父级末尾：parentID 为空表示根级。
// 节点 id 由调用方给出（与整树保存同一套身份规则），全树唯一性由保存校验收口。
func (s *system) CreateTaskNode(ctx context.Context, scope string, ownerID string, parentID string, node types.TaskNode) (types.TaskTree, error) {
	parentID = strings.TrimSpace(parentID)
	if strings.TrimSpace(node.ID) == "" {
		return types.TaskTree{}, storageInvalid("task id is required", nil)
	}
	return s.mutateTaskTree(ctx, scope, ownerID, func(tree types.TaskTree) (types.TaskTree, error) {
		if parentID == "" {
			tree.Tasks = append(tree.Tasks, node)
			return tree, nil
		}
		tasks, ok := appendTaskToLayer(tree.Tasks, parentID, node)
		if !ok {
			return types.TaskTree{}, storageInvalid("task parent does not exist: "+parentID, nil)
		}
		tree.Tasks = tasks
		return tree, nil
	})
}

// MoveTaskNode 把节点移动到另一个父级末尾：toParentID 为空表示根级。
// 节点的父级由树唯一决定，因此来源无需提交；移动到自己或自己的后代会被拒绝。
func (s *system) MoveTaskNode(ctx context.Context, scope string, ownerID string, taskID string, toParentID string) (types.TaskTree, error) {
	taskID = strings.TrimSpace(taskID)
	toParentID = strings.TrimSpace(toParentID)
	if taskID == "" {
		return types.TaskTree{}, storageInvalid("task id is required", nil)
	}
	return s.mutateTaskTree(ctx, scope, ownerID, func(tree types.TaskTree) (types.TaskTree, error) {
		node, found := findTaskNode(tree.Tasks, taskID)
		if !found {
			return types.TaskTree{}, storageInvalid("task does not exist: "+taskID, nil)
		}
		if toParentID != "" {
			if _, found := findTaskNode(tree.Tasks, toParentID); !found {
				return types.TaskTree{}, storageInvalid("task parent does not exist: "+toParentID, nil)
			}
			if taskSubtreeHasID(node, toParentID) {
				return types.TaskTree{}, storageInvalid("task cannot be moved into its own subtree: "+toParentID, nil)
			}
		}

		currentParentID, found := taskParentID(tree.Tasks, taskID)
		if !found {
			return types.TaskTree{}, storageInvalid("task does not exist: "+taskID, nil)
		}
		if currentParentID == toParentID {
			return types.TaskTree{}, storageInvalid("task already lives here: "+taskID, nil)
		}

		detached, removed := removeTaskFromLayer(tree.Tasks, taskID)
		if !removed {
			return types.TaskTree{}, storageInvalid("task does not exist: "+taskID, nil)
		}
		tree.Tasks = detached
		if toParentID == "" {
			tree.Tasks = append(tree.Tasks, node)
			return tree, nil
		}
		tasks, ok := appendTaskToLayer(tree.Tasks, toParentID, node)
		if !ok {
			return types.TaskTree{}, storageInvalid("task parent does not exist: "+toParentID, nil)
		}
		tree.Tasks = tasks
		return tree, nil
	})
}

// ReorderTaskChildren 重排某一层的子节点顺序：parentID 为空表示根级。
// 提交的清单必须是本层全部子节点的一次完整排列，少一个、多一个、重复都拒绝。
func (s *system) ReorderTaskChildren(ctx context.Context, scope string, ownerID string, parentID string, taskIDs []string) (types.TaskTree, error) {
	parentID = strings.TrimSpace(parentID)
	order := make([]string, 0, len(taskIDs))
	for _, item := range taskIDs {
		order = append(order, strings.TrimSpace(item))
	}
	return s.mutateTaskTree(ctx, scope, ownerID, func(tree types.TaskTree) (types.TaskTree, error) {
		if parentID == "" {
			children, err := orderTaskNodes(tree.Tasks, order, parentID)
			if err != nil {
				return types.TaskTree{}, err
			}
			tree.Tasks = children
			return tree, nil
		}
		parent, found := findTaskNode(tree.Tasks, parentID)
		if !found {
			return types.TaskTree{}, storageInvalid("task does not exist: "+parentID, nil)
		}
		children, err := orderTaskNodes(parent.Children, order, parentID)
		if err != nil {
			return types.TaskTree{}, err
		}
		parent.Children = children
		tasks, ok := replaceTaskNode(tree.Tasks, parent)
		if !ok {
			return types.TaskTree{}, storageInvalid("task does not exist: "+parentID, nil)
		}
		tree.Tasks = tasks
		return tree, nil
	})
}

// mutateTaskTree 是任务定点动作的统一骨架：
// 同一把实体锁内读出当前树，交给动作变换，再走整树校验后落盘。
func (s *system) mutateTaskTree(ctx context.Context, scope string, ownerID string, mutate func(types.TaskTree) (types.TaskTree, error)) (types.TaskTree, error) {
	file, err := s.taskScopeFile(scope, ownerID)
	if err != nil {
		return types.TaskTree{}, err
	}

	s.entityMu.Lock()
	defer s.entityMu.Unlock()

	previous, err := readTaskTree(ctx, file)
	if err != nil {
		return types.TaskTree{}, err
	}
	// 定点动作在传入的树上原地改（切片共享底层数组），对照用的前一版必须是独立副本，
	// 否则「这次到底改了什么」会被动作自己的改写抹平，时间戳判断随之失真。
	snapshot := cloneTaskTree(previous)
	next, err := mutate(previous)
	if err != nil {
		return types.TaskTree{}, err
	}
	normalized, err := normalizeTaskTreeForSave(next, snapshot, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return types.TaskTree{}, err
	}
	if err := writeJSON(ctx, file, normalized); err != nil {
		return types.TaskTree{}, storageWriteFailed("failed to save task tree", err)
	}
	return normalized, nil
}

// taskScopeFile 解析任务树文件并确认归属实体存在：
// 读写一个不存在的角色或工作区的任务，属于调用方给错了归属，
// 如实报错，不在数据区凭空生成实体目录。
func (s *system) taskScopeFile(scope string, ownerID string) (string, error) {
	file, ownerDir, err := s.paths.taskTreeFile(scope, ownerID)
	if err != nil {
		return "", err
	}
	if !directoryExists(ownerDir) {
		return "", storageInvalid("task scope owner does not exist for scope "+scope, nil)
	}
	return file, nil
}

// readTaskTree 读取树文件并校验其内容。
// 文件不存在视为空树；文件在手但内容不合规时如实报错，不静默清洗。
func readTaskTree(ctx context.Context, target string) (types.TaskTree, error) {
	if !dataFileExists(target) {
		return emptyTaskTree(), nil
	}
	tree, err := readJSON[types.TaskTree](ctx, target)
	if err != nil {
		return types.TaskTree{}, storageReadFailed("failed to read task tree", err)
	}
	if err := validateStoredTaskTree(tree); err != nil {
		return types.TaskTree{}, err
	}
	return normalizeStoredTaskTree(tree)
}

func emptyTaskTree() types.TaskTree {
	return types.TaskTree{Tasks: []types.TaskNode{}}
}

// normalizeStoredTaskTree 把落盘形态归一到当前结构：
// 空集合归位成空切片（避免 JSON 里出现 null），缺失的状态补成「未开始」，
// 认不出的状态如实报错，不静默改写成别的进度。
func normalizeStoredTaskTree(tree types.TaskTree) (types.TaskTree, error) {
	if tree.Tasks == nil {
		tree.Tasks = []types.TaskNode{}
	}
	for index := range tree.Tasks {
		node, err := normalizeStoredTaskNode(tree.Tasks[index])
		if err != nil {
			return types.TaskTree{}, err
		}
		tree.Tasks[index] = node
	}
	return tree, nil
}

func normalizeStoredTaskNode(node types.TaskNode) (types.TaskNode, error) {
	status, err := normalizeTaskStatus(node.Status)
	if err != nil {
		return types.TaskNode{}, err
	}
	node.Status = status
	if node.Children == nil {
		node.Children = []types.TaskNode{}
	}
	for index := range node.Children {
		child, err := normalizeStoredTaskNode(node.Children[index])
		if err != nil {
			return types.TaskNode{}, err
		}
		node.Children[index] = child
	}
	return node, nil
}

// normalizeTaskStatus 归一任务状态：缺省（旧数据里没有这个字段）即「未开始」，
// 其余必须是四态之一，认不出来的一律报错。
func normalizeTaskStatus(value string) (string, error) {
	status := strings.TrimSpace(value)
	if status == "" {
		return types.TaskStatusTodo, nil
	}
	switch status {
	case types.TaskStatusTodo, types.TaskStatusDoing, types.TaskStatusDone, types.TaskStatusArchived:
		return status, nil
	}
	return "", storageInvalid("unsupported task status: "+value, nil)
}

// validateStoredTaskTree 判定磁盘上的树是否合规：id 必须存在且全树唯一。
// 树结构本身不允许自引用或成环，因此这里不需要额外的引用校验。
func validateStoredTaskTree(tree types.TaskTree) error {
	seen := make(map[string]struct{}, countTaskNodes(tree.Tasks))
	var walk func(nodes []types.TaskNode) error
	walk = func(nodes []types.TaskNode) error {
		for _, node := range nodes {
			if strings.TrimSpace(node.ID) == "" {
				return storageInvalid("stored task tree contains a node without id", nil)
			}
			if _, ok := seen[node.ID]; ok {
				return storageInvalid("stored task tree contains duplicate id: "+node.ID, nil)
			}
			seen[node.ID] = struct{}{}
			if err := walk(node.Children); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(tree.Tasks)
}

// normalizeTaskTreeForSave 校验并规范化待保存的树。
// 时间字段一律由存储给出，调用方携带的时间不采信。
// 对照用的前一版也走同一套归一：旧数据里缺失的字段（例如状态）在两边都补齐之后
// 再比较，否则「空」与补出来的默认值会被误判成一次真实修改。
func normalizeTaskTreeForSave(tree types.TaskTree, previous types.TaskTree, now string) (types.TaskTree, error) {
	normalizedPrevious, err := normalizeStoredTaskTree(previous)
	if err != nil {
		return types.TaskTree{}, err
	}
	previousByID := make(map[string]types.TaskNode, countTaskNodes(normalizedPrevious.Tasks))
	indexTaskNodes(normalizedPrevious.Tasks, previousByID)

	seen := make(map[string]struct{}, countTaskNodes(tree.Tasks))
	tasks := make([]types.TaskNode, 0, len(tree.Tasks))
	for _, node := range tree.Tasks {
		normalized, err := normalizeTaskNodeForSave(node, previousByID, seen, now)
		if err != nil {
			return types.TaskTree{}, err
		}
		tasks = append(tasks, normalized)
	}
	return types.TaskTree{Tasks: tasks}, nil
}

func normalizeTaskNodeForSave(node types.TaskNode, previousByID map[string]types.TaskNode, seen map[string]struct{}, now string) (types.TaskNode, error) {
	node.ID = strings.TrimSpace(node.ID)
	node.Name = strings.TrimSpace(node.Name)
	node.Description = strings.TrimSpace(node.Description)
	if node.ID == "" {
		return types.TaskNode{}, storageInvalid("task id is required", nil)
	}
	if _, ok := seen[node.ID]; ok {
		return types.TaskNode{}, storageInvalid("duplicate task id: "+node.ID, nil)
	}
	seen[node.ID] = struct{}{}

	status, err := normalizeTaskStatus(node.Status)
	if err != nil {
		return types.TaskNode{}, err
	}
	node.Status = status

	children := make([]types.TaskNode, 0, len(node.Children))
	for _, child := range node.Children {
		normalized, err := normalizeTaskNodeForSave(child, previousByID, seen, now)
		if err != nil {
			return types.TaskNode{}, err
		}
		children = append(children, normalized)
	}
	node.Children = children

	existing, exists := previousByID[node.ID]
	switch {
	case !exists:
		node.CreatedAt = now
		node.UpdatedAt = now
	case taskNodeChanged(existing, node):
		node.CreatedAt = existing.CreatedAt
		node.UpdatedAt = now
	default:
		node.CreatedAt = existing.CreatedAt
		node.UpdatedAt = existing.UpdatedAt
	}
	return node, nil
}

// taskNodeChanged 判定节点自身是否真的变化：名字、备注、正文、卡片展示标记、进度状态，
// 以及子节点集合或顺序的变化都算这个节点变了。
func taskNodeChanged(previous types.TaskNode, current types.TaskNode) bool {
	return previous.Name != current.Name ||
		previous.Description != current.Description ||
		previous.Content != current.Content ||
		previous.CopyHighlight != current.CopyHighlight ||
		previous.Status != current.Status ||
		!slices.Equal(taskChildIDs(previous.Children), taskChildIDs(current.Children))
}

// removeTaskFromLayer 从一层里摘掉目标节点（若它在本层），否则递归到子层。
// 摘掉节点即摘掉它的整棵子树，调用方无需再逐层清理。
func removeTaskFromLayer(nodes []types.TaskNode, taskID string) ([]types.TaskNode, bool) {
	for index := range nodes {
		if nodes[index].ID == taskID {
			return slices.Concat(nodes[:index], nodes[index+1:]), true
		}
		next, removed := removeTaskFromLayer(nodes[index].Children, taskID)
		if removed {
			nodes[index].Children = next
			return nodes, true
		}
	}
	return nodes, false
}

// appendTaskToLayer 把节点挂到目标父级末尾；父级不存在时 ok 为 false。
func appendTaskToLayer(nodes []types.TaskNode, parentID string, node types.TaskNode) ([]types.TaskNode, bool) {
	for index := range nodes {
		if nodes[index].ID == parentID {
			nodes[index].Children = append(nodes[index].Children, node)
			return nodes, true
		}
		next, ok := appendTaskToLayer(nodes[index].Children, parentID, node)
		if ok {
			nodes[index].Children = next
			return nodes, true
		}
	}
	return nodes, false
}

// replaceTaskNode 用同 id 的新节点替换树里的旧节点；找不到时 ok 为 false。
func replaceTaskNode(nodes []types.TaskNode, node types.TaskNode) ([]types.TaskNode, bool) {
	for index := range nodes {
		if nodes[index].ID == node.ID {
			nodes[index] = node
			return nodes, true
		}
		next, ok := replaceTaskNode(nodes[index].Children, node)
		if ok {
			nodes[index].Children = next
			return nodes, true
		}
	}
	return nodes, false
}

// findTaskNode 按 id 在整棵树里找节点。
func findTaskNode(nodes []types.TaskNode, taskID string) (types.TaskNode, bool) {
	for _, node := range nodes {
		if node.ID == taskID {
			return node, true
		}
		if found, ok := findTaskNode(node.Children, taskID); ok {
			return found, true
		}
	}
	return types.TaskNode{}, false
}

// taskParentID 返回节点的父级 id：空串表示根级；节点不存在时 ok 为 false。
func taskParentID(nodes []types.TaskNode, taskID string) (string, bool) {
	return taskParentInLayer(nodes, "", taskID)
}

// taskParentInLayer 在一层里找目标节点：本层命中则父级就是这层的持有者。
func taskParentInLayer(nodes []types.TaskNode, parentID string, taskID string) (string, bool) {
	for _, node := range nodes {
		if node.ID == taskID {
			return parentID, true
		}
		if found, ok := taskParentInLayer(node.Children, node.ID, taskID); ok {
			return found, true
		}
	}
	return "", false
}

// taskSubtreeHasID 判定 id 是否落在该节点自己的子树里（含自身）。
func taskSubtreeHasID(node types.TaskNode, taskID string) bool {
	if node.ID == taskID {
		return true
	}
	for _, child := range node.Children {
		if taskSubtreeHasID(child, taskID) {
			return true
		}
	}
	return false
}

// orderTaskNodes 按提交的清单重排一层：清单必须覆盖本层每个子节点恰好一次。
func orderTaskNodes(nodes []types.TaskNode, taskIDs []string, parentID string) ([]types.TaskNode, error) {
	label := "root"
	if parentID != "" {
		label = parentID
	}
	if len(taskIDs) != len(nodes) {
		return nil, storageInvalid("task order for "+label+" must list every child exactly once", nil)
	}
	byID := make(map[string]types.TaskNode, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	ordered := make([]types.TaskNode, 0, len(nodes))
	seen := make(map[string]struct{}, len(taskIDs))
	for _, id := range taskIDs {
		if id == "" {
			return nil, storageInvalid("task order contains an empty id", nil)
		}
		if _, ok := seen[id]; ok {
			return nil, storageInvalid("task order contains duplicate id: "+id, nil)
		}
		node, ok := byID[id]
		if !ok {
			return nil, storageInvalid("task order references a node outside the layer: "+id, nil)
		}
		seen[id] = struct{}{}
		ordered = append(ordered, node)
	}
	return ordered, nil
}

// cloneTaskTree 深拷贝一棵树：动作在副本上原地改，对照用的前一版保持原样。
func cloneTaskTree(tree types.TaskTree) types.TaskTree {
	return types.TaskTree{Tasks: cloneTaskNodes(tree.Tasks)}
}

func cloneTaskNodes(nodes []types.TaskNode) []types.TaskNode {
	cloned := make([]types.TaskNode, 0, len(nodes))
	for _, node := range nodes {
		node.Children = cloneTaskNodes(node.Children)
		cloned = append(cloned, node)
	}
	return cloned
}

// taskChildIDs 取一层子节点的 id 序列。
func taskChildIDs(nodes []types.TaskNode) []string {
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	return ids
}

// countTaskNodes 统计一棵子树的节点数量。
func countTaskNodes(nodes []types.TaskNode) int {
	count := len(nodes)
	for _, node := range nodes {
		count += countTaskNodes(node.Children)
	}
	return count
}

// indexTaskNodes 把子树里的节点按 id 摊平进索引。
func indexTaskNodes(nodes []types.TaskNode, byID map[string]types.TaskNode) {
	for _, node := range nodes {
		byID[node.ID] = node
		indexTaskNodes(node.Children, byID)
	}
}
