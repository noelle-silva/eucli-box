package types

// 标准能力标识：宿主与工具共同认知的唯一集合。
// 工具只能在清单中声明这些标准能力，宿主只承认声明过的能力。
const (
	ToolCapabilityWorkspace          = "workspace"
	ToolCapabilitySessionAttachments = "session-attachments"
	ToolCapabilitySessionState       = "session-state"
)

// 能力访问方式：声明与授权都以读、写、引用显式分离。
// 引用表示把会话中已有的附件挂入本次工具结果（不落盘、ID 不变），
// 与写入（产生一份新附件）是两种不同的动作。
const (
	ToolCapabilityAccessRead      = "read"
	ToolCapabilityAccessWrite     = "write"
	ToolCapabilityAccessReference = "reference"
)

// ToolWorkspaceContext 是注入给工具的工作区路径值：注册目录表与身份。
type ToolWorkspaceContext struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Directories []WorkspaceDirectory `json:"directories,omitempty"`
}

// SessionAttachmentsReadRequest 是会话附件读取请求：列出清单或读取单张图片。
const (
	SessionAttachmentOperationList = "list"
	SessionAttachmentOperationRead = "read"
)

type SessionAttachmentsReadRequest struct {
	Operation    string `json:"operation"`
	AttachmentID string `json:"attachmentId,omitempty"`
}

// SessionAttachmentInfo 是会话附件的逻辑标识信息，不含任何物理路径。
type SessionAttachmentInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime,omitempty"`
}

// SessionAttachmentsListResult 是会话图片附件清单响应。
type SessionAttachmentsListResult struct {
	Attachments []SessionAttachmentInfo `json:"attachments"`
}

// SessionAttachmentData 是单张会话图片附件的数据响应。
type SessionAttachmentData struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Mime    string `json:"mime,omitempty"`
	DataURL string `json:"dataUrl"`
}

// SessionAttachmentWriteRequest 是会话附件写入的标准操作意图：
// 工具只提交图片数据与名称，落盘与挂载全部由宿主代写。
type SessionAttachmentWriteRequest struct {
	Name    string `json:"name"`
	DataURL string `json:"dataUrl"`
}
