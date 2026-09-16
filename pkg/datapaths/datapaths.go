// Package datapaths 是业务端数据目录布局的单一事实源。
// 数据区的一切目录层级、相对位置与关键文件在此集中声明和计算，
// 任何系统都通过本包获取路径，不自行拼接数据区路径。
package datapaths

import "path/filepath"

// 数据区相对路径常量（一律以 / 分隔，基准为数据根目录）。
// 目录与文件的事实命名只在出现于本列表。
const (
	RelMetaDir                = ".meta"
	RelAccessDir              = ".meta/access"
	RelVersionFile            = ".meta/version.json"
	RelBoxKeyFile             = ".meta/box.key"
	RelPortFile               = ".meta/port.json"
	RelPlaceholdersFile       = ".meta/placeholders.json"
	RelInstallSourceFile      = ".meta/install-source.json"
	RelStickerNamingFile      = ".meta/sticker-naming.json"
	RelMermaidFixFile         = ".meta/mermaid-fix.json"
	RelChatTitleNamingFile    = ".meta/chat-title-naming.json"
	RelContextCompressionFile = ".meta/context-compression.json"
	RelModelRequestFile       = ".meta/model-request.json"
	RelModelGroupsFile        = ".meta/model-groups.json"
	RelHookPromptsFile        = ".meta/hook-prompts.json"
	RelSessionsDir            = "sessions"
	RelSessionRolesDir        = "sessions/roles"
	RelSessionGroupsDir       = "sessions/groups"
	RelSessionWorkspacesDir   = "sessions/workspaces"
	RelRolesDir               = "roles"
	RelGroupsDir              = "groups"
	RelWorkspacesDir          = "workspaces"
	RelProvidersDir           = "providers"
	RelStickersDir            = "stickers"
	RelRecycleDir             = "recycle"
	RelToolDataDir            = "tool-data"
	RelToolBodiesDir          = "tool-bodies"
	RelSystemPluginsDataDir   = "system-plugins-data"
)

// Join 以数据根目录为基准合并相对路径。
func Join(root string, rel string) string {
	return filepath.Join(root, filepath.FromSlash(rel))
}

// MetaDir 返回数据区系统管理目录（hidden、置顶）。
func MetaDir(root string) string {
	return Join(root, RelMetaDir)
}

// AccessDir 返回访问控制数据目录。
func AccessDir(root string) string {
	return Join(root, RelAccessDir)
}

// ToolDataDir 返回工具数据目录。
func ToolDataDir(root string) string {
	return Join(root, RelToolDataDir)
}

// ToolBodiesDir 返回工具程序内置落点（非托管形态下位于数据区内）。
func ToolBodiesDir(root string) string {
	return Join(root, RelToolBodiesDir)
}

// SystemPluginsDataDir 返回系统插件数据目录。
func SystemPluginsDataDir(root string) string {
	return Join(root, RelSystemPluginsDataDir)
}

// SessionsDir 返回会话存储根目录。
func SessionsDir(root string) string {
	return Join(root, RelSessionsDir)
}

// VersionFile 返回数据版本事实文件。
func VersionFile(root string) string {
	return Join(root, RelVersionFile)
}

// BoxKeyFile 返回访问钥匙文件。
func BoxKeyFile(root string) string {
	return Join(root, RelBoxKeyFile)
}

// PortFile 返回网关端口配置文件。
func PortFile(root string) string {
	return Join(root, RelPortFile)
}

// PlaceholdersFile 返回占位符库文件。
func PlaceholdersFile(root string) string {
	return Join(root, RelPlaceholdersFile)
}

// InstallSourceFile 返回安装来源状态文件。
func InstallSourceFile(root string) string {
	return Join(root, RelInstallSourceFile)
}

// StickerNamingFile 返回贴纸命名配置文件。
func StickerNamingFile(root string) string {
	return Join(root, RelStickerNamingFile)
}

// MermaidFixFile 返回 Mermaid 修复配置文件。
func MermaidFixFile(root string) string {
	return Join(root, RelMermaidFixFile)
}

// ChatTitleNamingFile 返回聊天标题命名配置文件。
func ChatTitleNamingFile(root string) string {
	return Join(root, RelChatTitleNamingFile)
}

// ContextCompressionFile 返回上下文压缩配置文件。
func ContextCompressionFile(root string) string {
	return Join(root, RelContextCompressionFile)
}

// ModelRequestFile 返回模型请求配置文件。
func ModelRequestFile(root string) string {
	return Join(root, RelModelRequestFile)
}

// ModelGroupsFile 返回模型组配置文件。
func ModelGroupsFile(root string) string {
	return Join(root, RelModelGroupsFile)
}

// HookPromptsFile 返回 Hook 提示词库文件。
func HookPromptsFile(root string) string {
	return Join(root, RelHookPromptsFile)
}

// SessionRolesDir 返回角色会话存储目录。
func SessionRolesDir(root string) string {
	return Join(root, RelSessionRolesDir)
}

// SessionGroupsDir 返回群组会话存储目录。
func SessionGroupsDir(root string) string {
	return Join(root, RelSessionGroupsDir)
}

// SessionWorkspacesDir 返回工作区会话存储目录。
func SessionWorkspacesDir(root string) string {
	return Join(root, RelSessionWorkspacesDir)
}
