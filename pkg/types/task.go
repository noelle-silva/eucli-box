package types

// 任务的作用域：任务只挂在角色与工作区名下，各自一份。
// 与提示词库同构，作用域只决定文件落点，不代表跨作用域的继承、覆盖或优先级；
// 任务不参与提示词组装，也不进入模型上下文，只用于存储与记录。
const (
	TaskScopeRole      = "role"
	TaskScopeWorkspace = "workspace"
)

// TaskTree 是一个作用域下的任务树。
// 树只有一份真相：子节点由父节点的 Children 数组表达，
// 不另存 parentId，也不另存顺序索引，因此不存在悬空引用与顺序冲突。
type TaskTree struct {
	Tasks []TaskNode `json:"tasks"`
}

// TaskNode 是任务树上的一个节点：自己带内容，也可以是下一层的容器。
// ID 是身份，全树唯一；Name 允许为空（展示名由界面兜底）；
// Description 与 Name 一样是给人看的标注，不参与任何上下文组装；
// Content 是可复制文本，唯一去向是剪贴板。
// 子节点集合与顺序都只认 Children 数组，时间字段由存储维护。
type TaskNode struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Content     string     `json:"content"`
	Children    []TaskNode `json:"children"`
	CreatedAt   string     `json:"createdAt,omitempty"`
	UpdatedAt   string     `json:"updatedAt,omitempty"`
}
