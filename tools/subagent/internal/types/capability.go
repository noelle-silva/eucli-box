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

// 发送结果状态，按时间顺序分两种时相：
//   - accepted：消息已经落进目标会话并起跑，会话身份此刻就已确定；
//   - replied：角色已经停下并给出答复；
//   - failed：会话已经建立，但这次运行没能跑完，原因在 Error；
//   - confirmation_required：角色卡在一件需要用户点头的事上。
const (
	ToolMessageStatusAccepted             = "accepted"
	ToolMessageStatusReplied              = "replied"
	ToolMessageStatusFailed               = "failed"
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

// ToolMessageResult 是宿主交回的发送结果：状态决定其余字段的含义。
// 除「消息还没送进会话」之外，会话身份一律非空，任何结局都能凭它回去料理会话。
type ToolMessageResult struct {
	Status       string                `json:"status"`
	RunID        string                `json:"runId,omitempty"`
	SessionID    string                `json:"sessionId"`
	MessageID    string                `json:"messageId,omitempty"`
	RoleID       string                `json:"roleId"`
	Message      string                `json:"message,omitempty"`
	Error        string                `json:"error,omitempty"`
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
