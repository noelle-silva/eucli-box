package types

import "strings"

// 标准能力标识：宿主与工具共同认知的唯一集合。
// 工具只能在清单中声明这些标准能力，宿主只承认声明过的能力。
const ToolCapabilitySessionAttachments = "session-attachments"

// 能力访问方式：声明与授权都以读、写、引用显式分离。
// 引用表示把会话中已有的附件挂入本次工具结果（不落盘、ID 不变），
// 与写入（产生一份新附件）是两种不同的动作。
const ToolCapabilityAccessWrite = "write"

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

// SessionAttachmentInfo 是会话附件的逻辑标识信息，不含任何物理路径。
type SessionAttachmentInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime,omitempty"`
}

// SessionAttachmentWriteRequest 是会话附件写入的标准操作意图：
// 工具只提交图片数据与名称，落盘与挂载全部由宿主代写。
type SessionAttachmentWriteRequest struct {
	Name    string `json:"name"`
	DataURL string `json:"dataUrl"`
}
