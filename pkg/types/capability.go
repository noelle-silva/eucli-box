package types

import (
	"fmt"
	"strings"
)

// 标准能力标识：宿主与工具共同认知的唯一集合。
// 工具只能在清单中声明这些标准能力，宿主只承认声明过的能力。
const (
	ToolCapabilityWorkspace          = "workspace"
	ToolCapabilitySessionAttachments = "session-attachments"
	ToolCapabilitySessionState       = "session-state"
	// ToolCapabilitySessionMessage 是发送会话消息能力：工具请宿主把一个角色
	// 当成真实会话消息发送，并把角色停下后的最新一条消息交给工具。
	ToolCapabilitySessionMessage = "session-message"
	// ToolCapabilityWorkspaceIdentity 是工作区身份注入能力：声明后宿主把当前会话
	// 所在工作区标识随执行注入；不在工作区会话里就不注入。
	ToolCapabilityWorkspaceIdentity = "workspace-identity"
	// ToolCapabilityRoleDirectory 是角色名册读取能力：工具按工作区限定向宿主
	// 索取角色名、角色标识与角色描述。
	ToolCapabilityRoleDirectory = "role-directory"
	// ToolCapabilityToolConfirmation 是外部确认转呈能力：工具把「别处有件事要人点头」
	// 交给宿主摆到台面上，并等用户裁决后拿回结果。
	ToolCapabilityToolConfirmation = "tool-confirmation"
)

// 能力访问方式：声明与授权都以读、写、引用显式分离。
// 引用表示把会话中已有的附件挂入本次工具结果（不落盘、ID 不变），
// 与写入（产生一份新附件）是两种不同的动作。
const (
	ToolCapabilityAccessRead      = "read"
	ToolCapabilityAccessWrite     = "write"
	ToolCapabilityAccessReference = "reference"
)

// ToolCapability 是工具在清单中声明的单项能力。
type ToolCapability struct {
	ID          string `json:"id"`
	Access      string `json:"access"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	// GrantRequired 是宿主派生的运行时视图字段：该项能力是否需要用户授权。
	// 工具清单不得自行声明（打包与成品校验会拒绝），仅供协议消费方渲染。
	GrantRequired bool `json:"grantRequired,omitempty"`
}

// ToolCapabilityGrantRequired 判定一项标准能力是否需要用户授权：
// 注入类能力（宿主随执行主动提供的工作环境事实，如工作区路径与工作区标识）
// 不需要授权；索取类能力（工具运行中主动索取的宿主资源）由用户授权开关把关。
func ToolCapabilityGrantRequired(id string, access string) bool {
	switch strings.TrimSpace(id) {
	case ToolCapabilityWorkspace, ToolCapabilityWorkspaceIdentity:
		return false
	default:
		return true
	}
}

// DecorateToolCapabilities 生成带运行时视图的能力列表：按标准能力性质
// 派生"是否需要用户授权"，供设置界面与协议消费方渲染，工具声明原样复制。
func DecorateToolCapabilities(capabilities []ToolCapability) []ToolCapability {
	if len(capabilities) == 0 {
		return nil
	}
	out := make([]ToolCapability, len(capabilities))
	copy(out, capabilities)
	for index := range out {
		out[index].GrantRequired = ToolCapabilityGrantRequired(out[index].ID, out[index].Access)
	}
	return out
}

// HasDeclaredCapabilityGrantFlags 报告清单中是否出现工具自行声明的授权要求；
// 授权要求由宿主按标准能力性质派生，工具不得越权声明。
func HasDeclaredCapabilityGrantFlags(capabilities []ToolCapability) bool {
	for _, capability := range capabilities {
		if capability.GrantRequired {
			return true
		}
	}
	return false
}

// ToolCapabilityGrantKey 是授权项在工具用户设置中的稳定键。
func ToolCapabilityGrantKey(id string, access string) string {
	return strings.TrimSpace(id) + ":" + strings.TrimSpace(access)
}

// IsStandardToolCapability 判定 id+access 是否为宿主支持的标准能力组合。
func IsStandardToolCapability(id string, access string) bool {
	switch strings.TrimSpace(id) {
	case ToolCapabilityWorkspace:
		return strings.TrimSpace(access) == ToolCapabilityAccessRead
	case ToolCapabilitySessionAttachments:
		access = strings.TrimSpace(access)
		return access == ToolCapabilityAccessRead || access == ToolCapabilityAccessWrite || access == ToolCapabilityAccessReference
	case ToolCapabilitySessionState:
		return strings.TrimSpace(access) == ToolCapabilityAccessRead
	case ToolCapabilitySessionMessage:
		return strings.TrimSpace(access) == ToolCapabilityAccessWrite
	case ToolCapabilityWorkspaceIdentity:
		return strings.TrimSpace(access) == ToolCapabilityAccessRead
	case ToolCapabilityRoleDirectory:
		return strings.TrimSpace(access) == ToolCapabilityAccessRead
	case ToolCapabilityToolConfirmation:
		return strings.TrimSpace(access) == ToolCapabilityAccessWrite
	default:
		return false
	}
}

// ValidateToolCapabilities 校验能力声明集合：只能是标准组合，每项必须有
// 展示名，同一组合不得重复声明。宿主、打包器与成品校验共用同一判定。
func ValidateToolCapabilities(capabilities []ToolCapability) error {
	seen := map[string]struct{}{}
	for _, capability := range capabilities {
		id := strings.TrimSpace(capability.ID)
		access := strings.TrimSpace(capability.Access)
		if !IsStandardToolCapability(id, access) {
			return fmt.Errorf("tool capability is not a supported standard capability: %s:%s", id, access)
		}
		if strings.TrimSpace(capability.Name) == "" {
			return fmt.Errorf("tool capability name is required: %s:%s", id, access)
		}
		key := ToolCapabilityGrantKey(id, access)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("tool capability is declared more than once: %s", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// ToolDeclaresCapability 判定工具定义是否声明了指定能力。
func ToolDeclaresCapability(tool ToolDefinition, id string, access string) bool {
	target := ToolCapabilityGrantKey(id, access)
	for _, capability := range tool.Capabilities {
		if ToolCapabilityGrantKey(capability.ID, capability.Access) == target {
			return true
		}
	}
	return false
}

// ToolCapabilityGranted 判定工具定义上是否带有该项能力的用户授权。
func ToolCapabilityGranted(tool ToolDefinition, id string, access string) bool {
	return tool.CapabilityGrants[ToolCapabilityGrantKey(id, access)]
}

// ToolWorkspaceContext 是注入给工具的工作区路径值：注册目录表与身份。
type ToolWorkspaceContext struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Directories []WorkspaceDirectory `json:"directories,omitempty"`
}

// ToolPathBaseDirectory 是工具解析相对路径（含命令行工作目录）的基准：
// 工作区上下文存在且注册了目录时，以首个目录为基准；否则退回宿主工作目录。
// 宿主围栏以同一基准判断路径是否出圈，两端尺子保持一致。
func ToolPathBaseDirectory(input ToolExecutionInput) string {
	if input.Workspace != nil {
		for _, directory := range input.Workspace.Directories {
			if base := strings.TrimSpace(directory.Path); base != "" {
				return base
			}
		}
	}
	return strings.TrimSpace(input.HostWorkingDirectory)
}

// 会话动态状态（活值）的标准键；含义由宿主声明。
const (
	SessionStateKeyCurrentTime          = "current-time"
	SessionStateKeySessionTitle         = "session-title"
	SessionStateKeySessionMessageCount  = "session-message-count"
	SessionStateKeySessionTokenEstimate = "session-token-estimate"
	SessionStateKeyRoleName             = "role-name"
	SessionStateKeyGroupName            = "group-name"
	SessionStateKeyWorkspaceName        = "workspace-name"
)

// SessionStateValue 是宿主声明的一项活值：键、含义与当前快照。
type SessionStateValue struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Value       any    `json:"value,omitempty"`
}

// SessionStateRequest 是活值读取请求；键留空表示读取宿主声明的全部活值。
type SessionStateRequest struct {
	Key string `json:"key,omitempty"`
}

// SessionStateResult 是活值读取响应。
type SessionStateResult struct {
	Values []SessionStateValue `json:"values"`
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

// SessionAttachmentReferenceRequest 是会话附件引用的标准操作意图：
// 工具只提交既有附件的逻辑标识，宿主把该附件挂入本次工具结果——
// 不落盘、ID 不变、路径不变，只让它在本次回复处再出现一次。
type SessionAttachmentReferenceRequest struct {
	AttachmentID string `json:"attachmentId"`
}

// ToolMessageRequest 是发送会话消息的意图：工作区标识与会话标识都可留空，
// 会话标识留空即“新建会话”，因此新建与指定会话共用同一份结构。
type ToolMessageRequest struct {
	RoleID      string `json:"roleId"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	SessionID   string `json:"sessionId,omitempty"`
	Message     string `json:"message,omitempty"`
	// ResumeRunID 非空表示「不再发新消息，只接着等这条运行」：
	// 转呈确认之后用它续等，避免重复发消息。
	ResumeRunID string `json:"resumeRunId,omitempty"`
}

// ToolMessageStatus 是发送会话消息的结果状态：
// 角色已经答复，或者角色那边卡在一件需要用户点头的事上。
const (
	ToolMessageStatusReplied              = "replied"
	ToolMessageStatusConfirmationRequired = "confirmation_required"
)

// ToolConfirmationFact 是「角色那边正在等确认」这件事的完整事实：
// 哪个决策、哪个工具、被拦下的原因与入参，以及在哪个会话里等。
type ToolConfirmationFact struct {
	DecisionID string         `json:"decisionId"`
	ToolName   string         `json:"toolName"`
	Reason     string         `json:"reason,omitempty"`
	Arguments  map[string]any `json:"arguments,omitempty"`
	SessionID  string         `json:"sessionId"`
	RoleID     string         `json:"roleId"`
}

// ToolMessageResult 是发送会话消息的响应：状态决定其余字段的含义。
//   - replied：带回角色停下后最新的一条消息；
//   - confirmation_required：带回待确认的决策事实，消息字段留空。
type ToolMessageResult struct {
	Status       string                   `json:"status"`
	RunID        string                   `json:"runId,omitempty"`
	SessionID    string                   `json:"sessionId"`
	MessageID    string                   `json:"messageId,omitempty"`
	RoleID       string                   `json:"roleId"`
	Message      string                   `json:"message,omitempty"`
	Confirmation *ToolConfirmationFact `json:"confirmation,omitempty"`
}

// ToolConfirmationOutcome 是外部确认的落定结论：宿主把用户裁决原样交回工具。
const (
	ToolConfirmationStatusApproved = "approved"
	ToolConfirmationStatusRejected = "rejected"
	// ToolConfirmationStatusExpired 表示该决策在台面上已不存在（已失效或已收尾）。
	ToolConfirmationStatusExpired = "expired"
	// ToolConfirmationStatusFailed 表示裁决已被受理，但应用裁决这一步出错：
	// 原因如实带回，不与「失效」混为一谈。
	ToolConfirmationStatusFailed = "failed"
)

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

// ToolRoleDirectoryRequest 是角色名册读取请求：工作区标识留空即索取全局名册，
// 带上则只索取注册在该工作区里的角色。
type ToolRoleDirectoryRequest struct {
	WorkspaceID string `json:"workspaceId,omitempty"`
}

// ToolRoleDirectoryResult 是角色名册响应：带回本次索取的范围与角色清单。
type ToolRoleDirectoryResult struct {
	WorkspaceID string         `json:"workspaceId,omitempty"`
	Roles       []ToolRoleInfo `json:"roles"`
}
