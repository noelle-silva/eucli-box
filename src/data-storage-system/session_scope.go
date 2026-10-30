package datastorage

import (
	"path/filepath"
	"strings"

	"eucli-box/pkg/types"
)

type sessionScopeKind string

const (
	sessionScopeRole      sessionScopeKind = "role"
	sessionScopeGroup     sessionScopeKind = "group"
	sessionScopeWorkspace sessionScopeKind = "workspace"
)

type sessionScope struct {
	Kind        sessionScopeKind
	ID          string
	WorkspaceID string
	RoleID      string
}

func roleSessionScope(roleID string) sessionScope {
	return sessionScope{Kind: sessionScopeRole, ID: strings.TrimSpace(roleID)}
}

func groupSessionScope(groupID string) sessionScope {
	return sessionScope{Kind: sessionScopeGroup, ID: strings.TrimSpace(groupID)}
}

func workspaceSessionScope(workspaceID string, roleID string) sessionScope {
	return sessionScope{Kind: sessionScopeWorkspace, WorkspaceID: strings.TrimSpace(workspaceID), RoleID: strings.TrimSpace(roleID)}
}

func sessionScopeFromSession(session types.Session) (sessionScope, error) {
	groupID := strings.TrimSpace(session.GroupID)
	workspaceID := strings.TrimSpace(session.WorkspaceID)
	roleID := strings.TrimSpace(session.RoleID)
	if groupID != "" {
		if roleID != "" {
			return sessionScope{}, storageInvalid("session cannot belong to both role and group", nil)
		}
		if workspaceID != "" {
			return sessionScope{}, storageInvalid("session cannot belong to both group and workspace", nil)
		}
		return cleanSessionScope(groupSessionScope(groupID))
	}
	if workspaceID != "" {
		if roleID == "" {
			return sessionScope{}, storageInvalid("workspace session roleId is required", nil)
		}
		return cleanSessionScope(workspaceSessionScope(workspaceID, roleID))
	}
	return cleanSessionScope(roleSessionScope(roleID))
}

func cleanSessionScope(scope sessionScope) (sessionScope, error) {
	scope.ID = strings.TrimSpace(scope.ID)
	scope.WorkspaceID = strings.TrimSpace(scope.WorkspaceID)
	scope.RoleID = strings.TrimSpace(scope.RoleID)
	if scope.Kind != sessionScopeRole && scope.Kind != sessionScopeGroup && scope.Kind != sessionScopeWorkspace {
		return sessionScope{}, storageInvalid("session scope is invalid", nil)
	}
	if scope.Kind == sessionScopeWorkspace {
		if _, err := cleanID(scope.WorkspaceID); err != nil {
			return sessionScope{}, err
		}
		if _, err := cleanID(scope.RoleID); err != nil {
			return sessionScope{}, err
		}
		return scope, nil
	}
	if _, err := cleanID(scope.ID); err != nil {
		return sessionScope{}, err
	}
	return scope, nil
}

// key 是 scope 在锁表与索引指纹中的唯一标识。
// 它只由清洗后的 scope 事实构成，不含路径，便于跨平台稳定比较。
func (scope sessionScope) key() string {
	switch scope.Kind {
	case sessionScopeGroup:
		return "group:" + scope.ID
	case sessionScopeWorkspace:
		return "workspace:" + scope.WorkspaceID + "/" + scope.RoleID
	default:
		return "role:" + scope.ID
	}
}

// fingerprint 是写进会话索引条目的来源指纹：
// 索引条目必须能自证属于哪个 scope，错放的索引会在读取时被判为无效并重建。
func (scope sessionScope) fingerprint() string {
	return scope.key()
}

func (s *system) sessionDataFile(scope sessionScope, sessionID string) (string, error) {
	scope, err := cleanSessionScope(scope)
	if err != nil {
		return "", err
	}
	if scope.Kind == sessionScopeGroup {
		return s.paths.groupSessionDataFile(scope.ID, sessionID)
	}
	if scope.Kind == sessionScopeWorkspace {
		return s.paths.workspaceSessionDataFile(scope.WorkspaceID, scope.RoleID, sessionID)
	}
	return s.paths.sessionDataFile(scope.ID, sessionID)
}

func (s *system) sessionDir(scope sessionScope, sessionID string) (string, error) {
	scope, err := cleanSessionScope(scope)
	if err != nil {
		return "", err
	}
	if scope.Kind == sessionScopeGroup {
		return s.paths.groupSessionDir(scope.ID, sessionID)
	}
	if scope.Kind == sessionScopeWorkspace {
		return s.paths.workspaceSessionDir(scope.WorkspaceID, scope.RoleID, sessionID)
	}
	return s.paths.sessionDir(scope.ID, sessionID)
}

func (s *system) sessionAttachmentsDir(scope sessionScope, sessionID string) (string, error) {
	scope, err := cleanSessionScope(scope)
	if err != nil {
		return "", err
	}
	if scope.Kind == sessionScopeGroup {
		return s.paths.groupSessionAttachmentsDir(scope.ID, sessionID)
	}
	if scope.Kind == sessionScopeWorkspace {
		return s.paths.workspaceSessionAttachmentsDir(scope.WorkspaceID, scope.RoleID, sessionID)
	}
	return s.paths.sessionAttachmentsDir(scope.ID, sessionID)
}

func (s *system) sessionScopeDir(scope sessionScope) (string, error) {
	scope, err := cleanSessionScope(scope)
	if err != nil {
		return "", err
	}
	if scope.Kind == sessionScopeGroup {
		return s.paths.sessionGroupDir(scope.ID)
	}
	if scope.Kind == sessionScopeWorkspace {
		return s.paths.workspaceRoleSessionsDir(scope.WorkspaceID, scope.RoleID)
	}
	return s.paths.sessionRoleDir(scope.ID)
}

func (s *system) sessionIndexFile(scope sessionScope) (string, error) {
	dir, err := s.sessionScopeDir(scope)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "index.json"), nil
}

func (s *system) workspaceDir(workspaceID string) (string, error) {
	return s.paths.safeJoin(s.paths.sessionWorkspacesRoot(), workspaceID)
}
