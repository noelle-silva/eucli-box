package types

// ToolCapabilitySessionAttachments 是 session_image 声明的会话附件能力标识。
const ToolCapabilitySessionAttachments = "session-attachments"

// ToolCapabilityAccessReference 是引用访问方式：把会话中已有的附件挂入本次
// 工具结果（不落盘、ID 不变），与写入（产生一份新附件）是两种不同的动作。
const ToolCapabilityAccessReference = "reference"

// ToolWorkspaceContext 是注入给工具的工作区路径值：注册目录表与身份。
type ToolWorkspaceContext struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Directories []WorkspaceDirectory `json:"directories,omitempty"`
}

// SessionAttachmentInfo 是会话附件的逻辑标识信息，不含任何物理路径。
type SessionAttachmentInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime,omitempty"`
}

// SessionAttachmentReferenceRequest 是会话附件引用的标准操作意图：
// 工具只提交既有附件的逻辑标识，宿主把该附件挂入本次工具结果——
// 不落盘、ID 不变、路径不变，只让它在本次回复处再出现一次。
type SessionAttachmentReferenceRequest struct {
	AttachmentID string `json:"attachmentId"`
}
