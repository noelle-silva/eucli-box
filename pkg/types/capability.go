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
)

// 能力访问方式：声明与授权都以读、写显式分离。
const (
	ToolCapabilityAccessRead  = "read"
	ToolCapabilityAccessWrite = "write"
)

// ToolCapability 是工具在清单中声明的单项能力。
type ToolCapability struct {
	ID          string `json:"id"`
	Access      string `json:"access"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
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
		return access == ToolCapabilityAccessRead || access == ToolCapabilityAccessWrite
	case ToolCapabilitySessionState:
		return strings.TrimSpace(access) == ToolCapabilityAccessRead
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
