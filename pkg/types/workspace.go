package types

import "time"

type WorkspaceDirectory struct {
	Path        string `json:"path"`
	Alias       string `json:"alias"`
	Description string `json:"description,omitempty"`
}

type Workspace struct {
	ID                string               `json:"id"`
	Name              string               `json:"name"`
	Directories       []WorkspaceDirectory `json:"directories"`
	Prompt            string               `json:"prompt,omitempty"`
	RegisteredRoleIDs []string             `json:"registeredRoleIds"`
	CreatedAt         time.Time            `json:"createdAt"`
	UpdatedAt         time.Time            `json:"updatedAt"`
}

type WorkspaceSummary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// 工作区身份图标的载体类型：图标只有图像文件与内联 SVG 源码两种载体。
const (
	WorkspaceIconKindImage = "image"
	WorkspaceIconKindSVG   = "svg"
)

// WorkspaceIcon 是工作区身份图标的存储事实。
// 图像图标只记录相对工作区数据目录的路径，文件本身落在工作区图标目录内；
// SVG 图标把源码内联保存，不额外落文件。
type WorkspaceIcon struct {
	Kind string `json:"kind"`
	Path string `json:"path,omitempty"`
	SVG  string `json:"svg,omitempty"`
}

// WorkspaceIconContent 是图标在接口上的可传输形态：图像以 data URL 表达，SVG 以源码表达。
// 提交与回执共用同一形状，两种载体各自只填自己那份字段。
type WorkspaceIconContent struct {
	Kind    string `json:"kind"`
	SVG     string `json:"svg,omitempty"`
	DataURL string `json:"dataUrl,omitempty"`
}

// 工作区目录的运行时健康状态。健康是探测结果，不是登记事实：
// 它不写入 data.json，只随读取结果返回给界面。
const (
	WorkspaceHealthHealthy  = "healthy"
	WorkspaceHealthDegraded = "degraded"

	WorkspaceDirectoryAvailable    = "available"
	WorkspaceDirectoryMissing      = "missing"
	WorkspaceDirectoryNotDirectory = "not_directory"
	WorkspaceDirectoryUnavailable  = "unavailable"
)

type WorkspaceDirectoryHealth struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type WorkspaceHealth struct {
	Status      string                     `json:"status"`
	Directories []WorkspaceDirectoryHealth `json:"directories"`
}
