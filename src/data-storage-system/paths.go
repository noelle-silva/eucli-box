package datastorage

import (
	"path/filepath"
	"strings"
)

type paths struct {
	root string
}

func newPaths(root string) (paths, error) {
	if strings.TrimSpace(root) == "" {
		return paths{}, storageInvalid("root directory is required", nil)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return paths{}, storageInvalid("failed to resolve root directory", err)
	}
	return paths{root: filepath.Clean(abs)}, nil
}

func (p paths) baseDirs() []string {
	return []string{p.root, p.sessionsRoot(), p.rolesRoot(), p.providersRoot(), p.toolsRoot(), p.stickersRoot(), p.recycleRoot(), p.metaRoot()}
}

func (p paths) sessionsRoot() string  { return filepath.Join(p.root, "sessions") }
func (p paths) rolesRoot() string     { return filepath.Join(p.root, "roles") }
func (p paths) providersRoot() string { return filepath.Join(p.root, "providers") }
func (p paths) toolsRoot() string     { return filepath.Join(p.root, "tools") }
func (p paths) stickersRoot() string  { return filepath.Join(p.root, "stickers") }
func (p paths) recycleRoot() string   { return filepath.Join(p.root, "recycle") }
func (p paths) metaRoot() string      { return filepath.Join(p.root, "meta") }

func (p paths) metaVersionFile() string { return filepath.Join(p.metaRoot(), "version.json") }

func (p paths) stickerNamingConfigFile() string { return filepath.Join(p.metaRoot(), "sticker-naming.json") }

func (p paths) mermaidFixConfigFile() string { return filepath.Join(p.metaRoot(), "mermaid-fix.json") }

func (p paths) chatTitleNamingConfigFile() string { return filepath.Join(p.metaRoot(), "chat-title-naming.json") }

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

func (p paths) sessionRoleDir(roleID string) (string, error) {
	return p.safeJoin(p.sessionsRoot(), roleID)
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

func (p paths) toolDir(toolID string) (string, error) {
	return p.safeJoin(p.toolsRoot(), toolID)
}

func (p paths) toolDataFile(toolID string) (string, error) {
	dir, err := p.toolDir(toolID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data.json"), nil
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
