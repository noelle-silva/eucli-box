package datastorage

import (
	"path/filepath"
	"strings"

	"eucli-box/pkg/datapaths"
	"eucli-box/pkg/types"
)

type paths struct {
	root           string
	toolBodiesRoot string
}

func newPaths(root string, toolBodiesRoot string) (paths, error) {
	if strings.TrimSpace(root) == "" {
		return paths{}, storageInvalid("root directory is required", nil)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return paths{}, storageInvalid("failed to resolve root directory", err)
	}
	toolRoot := strings.TrimSpace(toolBodiesRoot)
	if toolRoot == "" {
		toolRoot = datapaths.ToolBodiesDir(abs)
	} else {
		toolRoot, err = filepath.Abs(toolRoot)
		if err != nil {
			return paths{}, storageInvalid("failed to resolve tool bodies root", err)
		}
	}
	return paths{root: filepath.Clean(abs), toolBodiesRoot: filepath.Clean(toolRoot)}, nil
}

func (p paths) baseDirs() []string {
	return []string{p.root, p.sessionsRoot(), p.sessionRolesRoot(), p.sessionGroupsRoot(), p.sessionWorkspacesRoot(), p.rolesRoot(), p.groupsRoot(), p.workspacesRoot(), p.providersRoot(), p.toolProgramsRoot(), p.toolDataRoot(), p.stickersRoot(), p.recycleRoot(), p.requestRecordsRoot(), p.metaRoot()}
}

func (p paths) sessionsRoot() string     { return datapaths.SessionsDir(p.root) }
func (p paths) rolesRoot() string        { return filepath.Join(p.root, datapaths.RelRolesDir) }
func (p paths) groupsRoot() string       { return filepath.Join(p.root, datapaths.RelGroupsDir) }
func (p paths) workspacesRoot() string   { return filepath.Join(p.root, datapaths.RelWorkspacesDir) }
func (p paths) providersRoot() string    { return filepath.Join(p.root, datapaths.RelProvidersDir) }
func (p paths) toolProgramsRoot() string { return p.toolBodiesRoot }

// managedToolPrograms 表示工具程序由外部程序根目录托管（阶段四受托运行模式）。
func (p paths) managedToolPrograms() bool {
	return p.toolBodiesRoot != datapaths.ToolBodiesDir(p.root)
}

// toolProgramRoot 是单个工具的受管理程序根目录。
func (p paths) toolProgramRoot(toolID string) (string, error) {
	return p.safeJoin(p.toolBodiesRoot, toolID)
}
func (p paths) toolDataRoot() string { return datapaths.ToolDataDir(p.root) }
func (p paths) stickersRoot() string { return filepath.Join(p.root, datapaths.RelStickersDir) }
func (p paths) recycleRoot() string  { return filepath.Join(p.root, datapaths.RelRecycleDir) }
func (p paths) metaRoot() string     { return datapaths.MetaDir(p.root) }

func (p paths) requestRecordsRoot() string {
	return datapaths.RequestRecordsDir(p.root)
}

func (p paths) sessionFavoritesFile() string {
	return filepath.Join(p.sessionsRoot(), "favorites.json")
}

func (p paths) sessionDesksFile() string {
	return filepath.Join(p.sessionsRoot(), "desks.json")
}

func (p paths) stickerNamingConfigFile() string {
	return datapaths.StickerNamingFile(p.root)
}

func (p paths) installSourceFile(kind string) (string, error) {
	path := datapaths.InstallSourceFile(p.root, kind)
	if path == "" {
		return "", storageInvalid("unsupported install source kind "+kind, nil)
	}
	return path, nil
}

func (p paths) mermaidFixConfigFile() string { return datapaths.MermaidFixFile(p.root) }

func (p paths) chatTitleNamingConfigFile() string {
	return datapaths.ChatTitleNamingFile(p.root)
}

func (p paths) contextCompressionConfigFile() string {
	return datapaths.ContextCompressionFile(p.root)
}

func (p paths) modelRequestConfigFile() string {
	return datapaths.ModelRequestFile(p.root)
}

func (p paths) conversationImageConfigFile() string {
	return datapaths.ConversationImageFile(p.root)
}

func (p paths) toolWorkDirectoryConfigFile() string {
	return datapaths.ToolWorkDirectoryFile(p.root)
}

func (p paths) modelGroupsFile() string {
	return datapaths.ModelGroupsFile(p.root)
}

func (p paths) hookPromptLibraryFile() string {
	return datapaths.HookPromptsFile(p.root)
}

func (p paths) requestRecordConfigFile() string {
	return datapaths.RequestRecordFile(p.root)
}

func (p paths) roleDir(roleID string) (string, error) {
	return p.safeJoin(p.rolesRoot(), roleID)
}

func (p paths) roleDataFile(roleID string) (string, error) {
	dir, err := p.roleDir(roleID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data.json"), nil
}

func (p paths) roleOrderFile() string {
	return filepath.Join(p.rolesRoot(), "order.json")
}

func (p paths) groupDir(groupID string) (string, error) {
	return p.safeJoin(p.groupsRoot(), groupID)
}

func (p paths) groupDataFile(groupID string) (string, error) {
	dir, err := p.groupDir(groupID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data.json"), nil
}

func (p paths) workspaceDir(workspaceID string) (string, error) {
	return p.safeJoin(p.workspacesRoot(), workspaceID)
}

func (p paths) workspaceDataFile(workspaceID string) (string, error) {
	dir, err := p.workspaceDir(workspaceID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data.json"), nil
}

func (p paths) workspaceOrderFile() string {
	return filepath.Join(p.workspacesRoot(), "order.json")
}

// workspaceIconRelativeDir 是工作区图标资源相对工作区数据目录的路径。
// 它既是图标目录的落点，也是图标存储事实里路径字段的基准。
const workspaceIconRelativeDir = "attachments/icon"

func (p paths) workspaceIconDir(workspaceID string) (string, error) {
	dir, err := p.workspaceDir(workspaceID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filepath.FromSlash(workspaceIconRelativeDir)), nil
}

// promptScopeLocation 是一个作用域的文件落点事实：
// dir 是作用域目录（全局为 .meta），ownerDir 是归属实体目录（全局为空）。
type promptScopeLocation struct {
	dir      string
	ownerDir string
}

// promptScopeLocation 解析「作用域 → 目录」。这套映射只存在这一处：
// 全局落在 .meta，角色与工作区落在各自实体目录，归属 id 经 safeJoin 校验，
// 未知作用域直接报错。
func (p paths) promptScopeLocation(scope string, ownerID string) (promptScopeLocation, error) {
	switch scope {
	case types.PromptScopeGlobal:
		return promptScopeLocation{dir: p.metaRoot()}, nil
	case types.PromptScopeRole:
		dir, err := p.roleDir(ownerID)
		if err != nil {
			return promptScopeLocation{}, err
		}
		return promptScopeLocation{dir: dir, ownerDir: dir}, nil
	case types.PromptScopeWorkspace:
		dir, err := p.workspaceDir(ownerID)
		if err != nil {
			return promptScopeLocation{}, err
		}
		return promptScopeLocation{dir: dir, ownerDir: dir}, nil
	default:
		return promptScopeLocation{}, storageInvalid("unsupported prompt scope: "+scope, nil)
	}
}

// promptScopeFile 返回作用域目录下一个具体文件的位置与归属目录。
// 提示词本体与收藏夹共用这一条落点解析，文件名由调用方给出。
func (p paths) promptScopeFile(scope string, ownerID string, fileName string) (string, string, error) {
	location, err := p.promptScopeLocation(scope, ownerID)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(location.dir, fileName), location.ownerDir, nil
}

// taskScopeDir 解析任务的作用域目录。
// 任务只有角色与工作区两个归属，目录仍由 roleDir / workspaceDir 单点解析，
// 因此任务树与提示词库落在同一个实体目录里；全局作用域在这里明确拒绝。
func (p paths) taskScopeDir(scope string, ownerID string) (string, error) {
	switch scope {
	case types.TaskScopeRole:
		return p.roleDir(ownerID)
	case types.TaskScopeWorkspace:
		return p.workspaceDir(ownerID)
	default:
		return "", storageInvalid("unsupported task scope: "+scope, nil)
	}
}

// taskTreeFile 返回任务树文件的位置与归属目录。
// 任务只有一份数据文件，文件名在这里连同目录一起确定；
// 归属目录同时作为「实体是否存在」的判据由调用方检查。
func (p paths) taskTreeFile(scope string, ownerID string) (string, string, error) {
	dir, err := p.taskScopeDir(scope, ownerID)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(dir, datapaths.RelTaskTreeFile), dir, nil
}

func (p paths) sessionRoleDir(roleID string) (string, error) {
	return p.safeJoin(p.sessionRolesRoot(), roleID)
}

func (p paths) sessionRolesRoot() string {
	return datapaths.SessionRolesDir(p.root)
}

func (p paths) sessionGroupsRoot() string {
	return datapaths.SessionGroupsDir(p.root)
}

func (p paths) sessionWorkspacesRoot() string {
	return datapaths.SessionWorkspacesDir(p.root)
}

func (p paths) sessionGroupDir(groupID string) (string, error) {
	return p.safeJoin(p.sessionGroupsRoot(), groupID)
}

func (p paths) sessionDir(roleID string, sessionID string) (string, error) {
	roleDir, err := p.sessionRoleDir(roleID)
	if err != nil {
		return "", err
	}
	return p.safeJoin(roleDir, sessionID)
}

func (p paths) sessionDataFile(roleID string, sessionID string) (string, error) {
	dir, err := p.sessionDir(roleID, sessionID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data.json"), nil
}

func (p paths) sessionAttachmentsDir(roleID string, sessionID string) (string, error) {
	dir, err := p.sessionDir(roleID, sessionID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "attachments"), nil
}

func (p paths) sessionAttachmentDir(roleID string, sessionID string, attachmentID string) (string, error) {
	dir, err := p.sessionAttachmentsDir(roleID, sessionID)
	if err != nil {
		return "", err
	}
	return p.safeJoin(dir, attachmentID)
}

func (p paths) groupSessionDir(groupID string, sessionID string) (string, error) {
	groupDir, err := p.sessionGroupDir(groupID)
	if err != nil {
		return "", err
	}
	return p.safeJoin(groupDir, sessionID)
}

func (p paths) groupSessionDataFile(groupID string, sessionID string) (string, error) {
	dir, err := p.groupSessionDir(groupID, sessionID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data.json"), nil
}

func (p paths) groupSessionAttachmentsDir(groupID string, sessionID string) (string, error) {
	dir, err := p.groupSessionDir(groupID, sessionID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "attachments"), nil
}

func (p paths) groupSessionAttachmentDir(groupID string, sessionID string, attachmentID string) (string, error) {
	dir, err := p.groupSessionAttachmentsDir(groupID, sessionID)
	if err != nil {
		return "", err
	}
	return p.safeJoin(dir, attachmentID)
}

func (p paths) workspaceRoleSessionsDir(workspaceID string, roleID string) (string, error) {
	workspaceDir, err := p.safeJoin(p.sessionWorkspacesRoot(), workspaceID)
	if err != nil {
		return "", err
	}
	return p.safeJoin(workspaceDir, roleID)
}

func (p paths) workspaceSessionDir(workspaceID string, roleID string, sessionID string) (string, error) {
	roleDir, err := p.workspaceRoleSessionsDir(workspaceID, roleID)
	if err != nil {
		return "", err
	}
	return p.safeJoin(roleDir, sessionID)
}

func (p paths) workspaceSessionDataFile(workspaceID string, roleID string, sessionID string) (string, error) {
	dir, err := p.workspaceSessionDir(workspaceID, roleID, sessionID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data.json"), nil
}

func (p paths) workspaceSessionAttachmentsDir(workspaceID string, roleID string, sessionID string) (string, error) {
	dir, err := p.workspaceSessionDir(workspaceID, roleID, sessionID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "attachments"), nil
}

func (p paths) workspaceSessionAttachmentDir(workspaceID string, roleID string, sessionID string, attachmentID string) (string, error) {
	dir, err := p.workspaceSessionAttachmentsDir(workspaceID, roleID, sessionID)
	if err != nil {
		return "", err
	}
	return p.safeJoin(dir, attachmentID)
}

func (p paths) providerDir(providerID string) (string, error) {
	return p.safeJoin(p.providersRoot(), providerID)
}

func (p paths) providerDataFile(providerID string) (string, error) {
	dir, err := p.providerDir(providerID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data.json"), nil
}

func (p paths) providerOrderFile() string {
	return filepath.Join(p.providersRoot(), "order.json")
}

func (p paths) toolBodyDir(toolID string) (string, error) {
	return p.safeJoin(p.toolProgramsRoot(), toolID)
}

func (p paths) toolOrderFile() string {
	return filepath.Join(p.toolProgramsRoot(), "order.json")
}

func (p paths) toolBodyDefinitionFile(toolID string) (string, error) {
	dir, err := p.toolBodyDir(toolID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "definition.json"), nil
}

func (p paths) toolDataDir(toolID string) (string, error) {
	return p.safeJoin(p.toolDataRoot(), toolID)
}

func (p paths) toolUserSettingsFile(toolID string) (string, error) {
	dir, err := p.toolDataDir(toolID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

func (p paths) stickerCategoryDir(categoryName string) (string, error) {
	name, err := cleanStickerCategoryName(categoryName)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(p.stickersRoot(), name)
	if !isWithin(p.stickersRoot(), joined) {
		return "", storageInvalid("path escapes stickers root", nil)
	}
	return joined, nil
}

func (p paths) stickerCategoriesOrderFile() string {
	return filepath.Join(p.stickersRoot(), "order.json")
}

func (p paths) stickerItemsOrderFile(categoryName string) (string, error) {
	dir, err := p.stickerCategoryDir(categoryName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "order.json"), nil
}

func (p paths) stickerItemDir(categoryName string, stickerID string) (string, error) {
	categoryDir, err := p.stickerCategoryDir(categoryName)
	if err != nil {
		return "", err
	}
	return p.safeJoin(categoryDir, stickerID)
}

func (p paths) stickerItemDataFile(categoryName string, stickerID string) (string, error) {
	dir, err := p.stickerItemDir(categoryName, stickerID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data.json"), nil
}

func (p paths) safeJoin(base string, id string) (string, error) {
	cleanID, err := cleanID(id)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(base, cleanID)
	if !isWithin(base, joined) {
		return "", storageInvalid("path escapes data root", nil)
	}
	return joined, nil
}

func cleanID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", storageInvalid("id is required", nil)
	}
	if id == "." || id == ".." || strings.Contains(id, "..") || strings.ContainsAny(id, `/\\`) {
		return "", storageInvalid("id contains unsafe path characters", nil)
	}
	return id, nil
}

func isWithin(base string, child string) bool {
	base = filepath.Clean(base)
	child = filepath.Clean(child)
	rel, err := filepath.Rel(base, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
