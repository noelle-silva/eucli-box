package types

// 任务的作用域：任务只挂在角色与工作区名下，各自一份。
// 与提示词库同构，作用域只决定文件落点，不代表跨作用域的继承、覆盖或优先级；
// 任务不参与提示词组装，也不进入模型上下文，只用于存储与记录。
const (
	TaskScopeRole      = "role"
	TaskScopeWorkspace = "workspace"
)

// 任务的四个状态：未开始、进行中、已完成、归档。
// 状态只是给人看的进度标记，不参与任何上下文组装；
// 归档只影响卡片墙是否展示该任务，数据本身完整保留。
const (
	TaskStatusTodo     = "todo"
	TaskStatusDoing    = "doing"
	TaskStatusDone     = "done"
	TaskStatusArchived = "archived"
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
// Content 是可复制文本，唯一去向是剪贴板；
// CopyHighlight 是卡片展示标记：开启后该任务的复制键用主题色标出，
// 它不改变复制内容与复制行为，也不参与上下文组装；
// Status 是进度四态（todo / doing / done / archived），缺省即未开始，
// 归档只影响卡片墙是否展示，数据不删。
// 子节点集合与顺序都只认 Children 数组，时间字段由存储维护。
type TaskNode struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Description   string     `json:"description,omitempty"`
	Content       string     `json:"content"`
	CopyHighlight bool       `json:"copyHighlight,omitempty"`
	Status        string     `json:"status,omitempty"`
	Children      []TaskNode `json:"children"`
	CreatedAt     string     `json:"createdAt,omitempty"`
	UpdatedAt     string     `json:"updatedAt,omitempty"`
}
