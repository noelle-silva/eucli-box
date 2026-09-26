package datastorage

import (
	"path/filepath"
	"strings"

	"eucli-box/pkg/datapaths"
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

func (p paths) stickerNamingConfigFile() string {
	return datapaths.StickerNamingFile(p.root)
}

func (p paths) installSourceFile() string { return datapaths.InstallSourceFile(p.root) }

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

func (p paths) toolBodyDir(toolID string) (string, error) {
	return p.safeJoin(p.toolProgramsRoot(), toolID)
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
