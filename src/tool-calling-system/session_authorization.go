package toolcalling

import (
	"context"
	"strings"

	"eucli-box/pkg/types"
)

// loadSessionForScope 是会话读取的唯一入口：按执行身份定位会话档案。
// 能力服务与放行判断共用同一实现，保证二者对会话的理解一致。
func loadSessionForScope(ctx context.Context, storage StorageSystem, scope types.ToolRunScope) (types.Session, error) {
	sessionID := strings.TrimSpace(scope.SessionID)
	if sessionID == "" {
		return types.Session{}, toolInvalid("tool run scope is missing session id", nil)
	}
	if groupID := strings.TrimSpace(scope.GroupID); groupID != "" {
		return storage.LoadGroupSession(ctx, groupID, sessionID)
	}
	if workspaceID := strings.TrimSpace(scope.WorkspaceID); workspaceID != "" {
		return storage.LoadWorkspaceSession(ctx, workspaceID, strings.TrimSpace(scope.RoleID), sessionID)
	}
	return storage.LoadSession(ctx, strings.TrimSpace(scope.RoleID), sessionID)
}

// sessionToolAuthorized 判断工具是否已在本会话被放行。
// 读取失败一律视为未放行：放行是加速通道，不能因读不到会话而误放。
func (s *system) sessionToolAuthorized(ctx context.Context, scope types.ToolRunScope, toolID string) bool {
	session, err := loadSessionForScope(ctx, s.storage, scope)
	if err != nil {
		return false
	}
	return types.SessionToolAuthorized(session.Metadata, toolID)
}

// resolveSessionAuthorization 把命中放行的“需询问”提升为“直接允许”。
// 只提升询问，不改变硬性拒绝：未获角色许可的工具不会被会话放行复活。
func resolveSessionAuthorization(decision types.PermissionDecision, authorized bool) types.PermissionDecision {
	if authorized && decision.Status == types.PermissionStatusNeedsConfirmation {
		decision.Status = types.PermissionStatusAllowed
		decision.Reason = "tool authorized for this session"
	}
	return decision
}

// rememberToolAuthorization 把“本会话内始终同意”落进会话放行清单。
// 会话身份缺失时无事可记；写入失败向上抛，不静默吞掉用户的放行意图。
func (s *system) rememberToolAuthorization(ctx context.Context, scope types.ToolRunScope, toolID string) error {
	if strings.TrimSpace(toolID) == "" || strings.TrimSpace(scope.SessionID) == "" {
		return nil
	}
	var err error
	switch {
	case strings.TrimSpace(scope.GroupID) != "":
		_, err = s.storage.UpdateGroupSessionToolAuthorization(ctx, strings.TrimSpace(scope.GroupID), strings.TrimSpace(scope.SessionID), toolID, true)
	case strings.TrimSpace(scope.WorkspaceID) != "":
		_, err = s.storage.UpdateWorkspaceSessionToolAuthorization(ctx, strings.TrimSpace(scope.WorkspaceID), strings.TrimSpace(scope.RoleID), strings.TrimSpace(scope.SessionID), toolID, true)
	default:
		_, err = s.storage.UpdateSessionToolAuthorization(ctx, strings.TrimSpace(scope.RoleID), strings.TrimSpace(scope.SessionID), toolID, true)
	}
	if err != nil {
		return toolStorageFailed("failed to remember session tool authorization", err)
	}
	return nil
}
