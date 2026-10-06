package datastorage

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

// ClaimAsyncToolResult 原子认领一条异步任务的回灌权：
// 以任务 ID 为键，仅当落盘会话中该任务既未认领也未完成时认领成功；
// 否则返回 (false, nil)，表示该结果已被其它运行认领或回灌。
// 认领判据始终是落盘会话中的权威标记，与各运行的内存副本无关。
func (s *system) ClaimAsyncToolResult(ctx context.Context, session types.Session, taskID string, runID string) (bool, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false, storageInvalid("async tool task id is required", nil)
	}
	cleaned, err := cleanSessionScopeFromSession(session)
	if err != nil {
		return false, err
	}
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	loaded, err := s.loadSession(ctx, cleaned, session.ID)
	if err != nil {
		return false, err
	}
	index := storedAsyncToolTaskIndex(loaded.AsyncToolTasks, taskID)
	if index < 0 {
		return false, nil
	}
	task := loaded.AsyncToolTasks[index]
	if !task.CompletedAt.IsZero() || !task.InjectionClaimedAt.IsZero() {
		return false, nil
	}
	now := time.Now().UTC()
	task.InjectionClaimedAt = now
	task.InjectionClaimRunID = strings.TrimSpace(runID)
	loaded.AsyncToolTasks[index] = task
	loaded.UpdatedAt = now
	if _, err := s.writeSessionData(ctx, loaded, now); err != nil {
		return false, err
	}
	if err := s.rebuildSessionIndexesForScope(ctx, cleaned); err != nil {
		return false, err
	}
	return true, nil
}

// ReleaseAsyncToolResultClaim 回退一次回灌认领：
// 仅当落盘会话中该任务由 runID 认领且尚未完成时清除认领标记，保留可重试，绝不丢结果。
func (s *system) ReleaseAsyncToolResultClaim(ctx context.Context, session types.Session, taskID string, runID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return storageInvalid("async tool task id is required", nil)
	}
	cleaned, err := cleanSessionScopeFromSession(session)
	if err != nil {
		return err
	}
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	loaded, err := s.loadSession(ctx, cleaned, session.ID)
	if err != nil {
		return err
	}
	index := storedAsyncToolTaskIndex(loaded.AsyncToolTasks, taskID)
	if index < 0 {
		return nil
	}
	task := loaded.AsyncToolTasks[index]
	if !task.CompletedAt.IsZero() || task.InjectionClaimedAt.IsZero() {
		return nil
	}
	if owner := strings.TrimSpace(task.InjectionClaimRunID); owner != "" && owner != strings.TrimSpace(runID) {
		return nil
	}
	now := time.Now().UTC()
	task.InjectionClaimedAt = time.Time{}
	task.InjectionClaimRunID = ""
	loaded.AsyncToolTasks[index] = task
	loaded.UpdatedAt = now
	if _, err := s.writeSessionData(ctx, loaded, now); err != nil {
		return err
	}
	return s.rebuildSessionIndexesForScope(ctx, cleaned)
}

func cleanSessionScopeFromSession(session types.Session) (sessionScope, error) {
	scope, err := sessionScopeFromSession(session)
	if err != nil {
		return sessionScope{}, err
	}
	return cleanSessionScope(scope)
}

func storedAsyncToolTaskIndex(tasks []types.AsyncToolTask, taskID string) int {
	for index := range tasks {
		if strings.TrimSpace(tasks[index].ID) == taskID {
			return index
		}
	}
	return -1
}
