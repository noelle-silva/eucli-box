package types

// ToolCapabilitySessionMessage 是发送会话消息的能力标识。
const ToolCapabilitySessionMessage = "session-message"

// ToolCapabilityRoleDirectory 是角色名册读取的能力标识。
const ToolCapabilityRoleDirectory = "role-directory"

// ToolCapabilityToolConfirmation 是外部确认转呈的能力标识。
const ToolCapabilityToolConfirmation = "tool-confirmation"

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
	Message     string `json:"message,omitempty"`
	// ResumeRunID 非空表示不再发新消息，只接着等这条运行。
	ResumeRunID string `json:"resumeRunId,omitempty"`
}

// 发送结果状态：角色已回答，或角色那边卡在一件需要用户点头的事上。
const (
	ToolMessageStatusReplied              = "replied"
	ToolMessageStatusConfirmationRequired = "confirmation_required"
)

// ToolConfirmationFact 是「角色那边正在等确认」的完整事实。
type ToolConfirmationFact struct {
	DecisionID string         `json:"decisionId"`
	ToolName   string         `json:"toolName"`
	Reason     string         `json:"reason,omitempty"`
	Arguments  map[string]any `json:"arguments,omitempty"`
	SessionID  string         `json:"sessionId"`
	RoleID     string         `json:"roleId"`
}

// ToolMessageResult 是角色停下后交还给工具的结果：状态决定其余字段的含义。
type ToolMessageResult struct {
	Status       string                `json:"status"`
	RunID        string                `json:"runId,omitempty"`
	SessionID    string                `json:"sessionId"`
	MessageID    string                `json:"messageId,omitempty"`
	RoleID       string                `json:"roleId"`
	Message      string                `json:"message,omitempty"`
	Confirmation *ToolConfirmationFact `json:"confirmation,omitempty"`
}

// 外部确认的落定结论。
const (
	ToolConfirmationStatusApproved = "approved"
	ToolConfirmationStatusRejected = "rejected"
	ToolConfirmationStatusExpired  = "expired"
	ToolConfirmationStatusFailed   = "failed"
)

// ToolConfirmationOutcome 是宿主交回的外部确认结论。
type ToolConfirmationOutcome struct {
	Status     string `json:"status"`
	DecisionID string `json:"decisionId"`
	Reason     string `json:"reason,omitempty"`
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
