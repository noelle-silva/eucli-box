package types

import "strings"

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
