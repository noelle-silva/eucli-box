package types

// ToolCapabilitySessionMessage 是发送会话消息的能力标识：本工具唯一的动作来源。
const ToolCapabilitySessionMessage = "session-message"

// ToolCapabilityAccessWrite 是写入访问方式：把一条消息真正发进会话。
const ToolCapabilityAccessWrite = "write"

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

// ToolWorkspaceContext 是注入给工具的工作区路径值：注册目录表与身份。
type ToolWorkspaceContext struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Directories []WorkspaceDirectory `json:"directories,omitempty"`
}
