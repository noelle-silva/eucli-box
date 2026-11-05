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
