package workspaceprompt

import (
	"strings"

	"eucli-box/pkg/types"
)

func Content(workspace types.Workspace) string {
	blocks := []string{}
	name := strings.TrimSpace(workspace.Name)
	if name != "" {
		blocks = append(blocks, "当前工作区："+name)
	}
	if len(workspace.Directories) > 0 {
		lines := []string{"工作区注册目录："}
		for _, directory := range workspace.Directories {
			alias := strings.TrimSpace(directory.Alias)
			if alias == "" {
				alias = "未命名目录"
			}
			line := "- " + alias + "：" + strings.TrimSpace(directory.Path)
			if description := strings.TrimSpace(directory.Description); description != "" {
				line += "（" + description + "）"
			}
			lines = append(lines, line)
		}
		lines = append(lines, "路径围栏说明：文件读写和命令工作目录应优先保持在上述注册目录内；如需超出范围，系统会先询问用户。")
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	if prompt := strings.TrimSpace(workspace.Prompt); prompt != "" {
		blocks = append(blocks, "工作区自定义提示词：\n"+prompt)
	}
	return strings.TrimSpace(strings.Join(blocks, "\n\n"))
}
