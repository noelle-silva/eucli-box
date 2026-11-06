package types

// ToolCapabilitySessionMessage 是发送会话消息的能力标识。
const ToolCapabilitySessionMessage = "session-message"

// ToolCapabilityRoleDirectory 是角色名册读取的能力标识。
const ToolCapabilityRoleDirectory = "role-directory"

// ToolCapabilityAccessWrite 是写入访问方式：把一条消息真正发进会话。
const ToolCapabilityAccessWrite = "write"

// ToolCapabilityAccessRead 是读取访问方式：只取回事实，不改动任何东西。
const ToolCapabilityAccessRead = "read"

// ToolMessageRequest 是发送会话消息的意图：工作区标识与会话标识都可留空，
// 会话标识留空即新建会话。
type ToolMessageRequest struct {
	RoleID      string `json:"roleId"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	SessionID   string `json:"sessionId,omitempty"`
	Message     string `json:"message"`
}

// ToolMessageResult 是角色停下后交还给工具的最新一条消息。
type ToolMessageResult struct {
	SessionID string `json:"sessionId"`
	MessageID string `json:"messageId"`
	RoleID    string `json:"roleId"`
	Message   string `json:"message"`
}

// ToolRoleInfo 是角色名册里的一项：名、标识与角色自己的描述。
type ToolRoleInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ToolRoleDirectoryRequest 是角色名册读取请求：工作区标识留空即索取全局名册。
type ToolRoleDirectoryRequest struct {
	WorkspaceID string `json:"workspaceId,omitempty"`
}

// ToolRoleDirectoryResult 是角色名册响应：带回索取范围与角色清单。
type ToolRoleDirectoryResult struct {
	WorkspaceID string         `json:"workspaceId,omitempty"`
	Roles       []ToolRoleInfo `json:"roles"`
}

// ToolWorkspaceContext 是注入给工具的工作区路径值：注册目录表与身份。
type ToolWorkspaceContext struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Directories []WorkspaceDirectory `json:"directories,omitempty"`
}
