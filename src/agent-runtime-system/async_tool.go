package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

type asyncToolInterruptError struct {
	taskID string
}

type asyncContinuationKey struct {
	roleID      string
	groupID     string
	workspaceID string
	sessionID   string
}

func (err asyncToolInterruptError) Error() string {
	if strings.TrimSpace(err.taskID) == "" {
		return "tool confirmation interrupted by async tool result"
	}
	return "tool confirmation interrupted by async tool result: " + err.taskID
}

func asyncToolInterruption(taskID string) error {
	return asyncToolInterruptError{taskID: strings.TrimSpace(taskID)}
}

func isAsyncToolInterruption(err error) bool {
	var target asyncToolInterruptError
	return errors.As(err, &target)
}

func (s *system) acceptAsyncToolEntries(ctx context.Context, record *runRecord, entries []toolRunEntry) error {
	for index := range entries {
		if entries[index].HasResult || entries[index].Plan.PlanStatus != types.ToolPlanStatusReady || entries[index].Plan.InvocationMode != types.ToolInvocationModeAsync {
			continue
		}
		task, result := s.acceptAsyncToolTask(record, entries[index].Action, entries[index].Plan)
		entries[index].Result = result
		entries[index].HasResult = true
		upsertRunToolPart(record, entries[index].Action, "async_pending", &entries[index].Plan.Decision, &result)
		if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
			return err
		}
		if err := s.saveRunSession(ctx, record, types.RunStatusRunning); err != nil {
			return err
		}
		s.publishAsyncToolTaskUpdate(record.runID, task)
		s.publishAssistantMessageUpdate(record)
		go s.executeAsyncToolTask(task)
	}
	return nil
}

func (s *system) acceptAsyncToolTask(record *runRecord, action types.ToolAction, plan types.ToolRunPlan) (types.AsyncToolTask, types.ToolResult) {
	now := time.Now().UTC()
	task := types.AsyncToolTask{
		ID:                 utils.NewID("async-tool-task"),
		RunID:              record.runID,
		RoleID:             record.roleID,
		GroupID:            record.groupID,
		WorkspaceID:        record.workspaceID,
		SessionID:          record.session.ID,
		AssistantMessageID: record.activeAssistantID,
		TaskName:           action.ToolName,
		ToolName:           action.ToolName,
		Status:             types.AsyncToolTaskStatusPending,
		Continuation:       asyncToolContinuationFromRun(record),
		Action:             action,
		Plan:               plan,
		SubmittedAt:        now,
	}
	result := types.ToolResult{ID: newRuntimeID("tool-result"), ActionID: action.ID, ToolName: action.ToolName, Status: types.ToolStatusSuccess, Content: fmt.Sprintf("异步任务 %s 已受理，真实执行结果会在稍后回灌。", task.ID), Metadata: map[string]any{"asyncTaskId": task.ID, "asyncAccepted": true}, CreatedAt: now}
	record.session.AsyncToolTasks = upsertAsyncToolTask(record.session.AsyncToolTasks, task)
	s.mu.Lock()
	s.asyncTasks[task.ID] = task
	s.mu.Unlock()
	return task, result
}

func asyncToolContinuationFromRun(record *runRecord) types.RunContinuation {
	if record == nil {
		return types.RunContinuation{}
	}
	continuation := types.RunContinuation{Stream: record.stream, ReasoningEffort: types.TrimReasoningEffort(record.reasoningEffort)}
	if override, ok := types.NormalizeModelOverrideCoordinate(record.modelOverride); ok {
		continuation.ModelOverride = &override
	}
	selection := types.NormalizeHookPromptSelection(record.hookPromptSelection.Mode, record.hookPromptSelection.PresetID)
	if selection.Mode == types.HookPromptSelectionModeNone || selection.Mode == types.HookPromptSelectionModePreset {
		continuation.HookPromptMode = selection.Mode
		continuation.HookPromptPresetID = selection.PresetID
	}
	return continuation
}

func (s *system) executeAsyncToolTask(task types.AsyncToolTask) {
	task.Status = types.AsyncToolTaskStatusRunning
	task.StartedAt = time.Now().UTC()
	if err := s.saveAsyncToolTaskWithRetry(context.Background(), task); err != nil {
		task.Error = "异步任务状态落盘失败: " + err.Error()
		s.upsertRuntimeAsyncToolTask(task)
	}
	s.publishAsyncToolTaskUpdate(task.RunID, task)

	result, err := s.tools.Execute(context.Background(), task.Plan)
	finished := time.Now().UTC()
	task.FinishedAt = finished
	if err != nil {
		result = failedToolResult(task.Action, err.Error())
	}
	if result.Status == types.ToolStatusSuccess {
		task.Status = types.AsyncToolTaskStatusSucceeded
	} else {
		task.Status = types.AsyncToolTaskStatusFailed
		task.Error = strings.TrimSpace(result.Error)
		if task.Error == "" {
			task.Error = strings.TrimSpace(result.Content)
		}
	}
	task.Result = &result
	if err := s.saveAsyncToolTaskWithRetry(context.Background(), task); err != nil {
		task.Error = strings.TrimSpace(task.Error)
		if task.Error != "" {
			task.Error += "\n"
		}
		task.Error += "异步任务结果落盘失败: " + err.Error()
		s.upsertRuntimeAsyncToolTask(task)
	}
	s.publishAsyncToolTaskUpdate(task.RunID, task)
	s.notifyAsyncToolReady(task)
}

func (s *system) saveAsyncToolTaskWithRetry(ctx context.Context, task types.AsyncToolTask) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := s.saveAsyncToolTask(ctx, task); err != nil {
			lastErr = err
			if attempt < 2 {
				time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
			}
			continue
		}
		return nil
	}
	return lastErr
}

func (s *system) saveAsyncToolTask(ctx context.Context, task types.AsyncToolTask) error {
	s.upsertRuntimeAsyncToolTask(task)
	session, err := s.loadTaskSession(ctx, task)
	if err != nil {
		return runtimeStorageFailed("failed to load async tool task session", err)
	}
	session.AsyncToolTasks = upsertAsyncToolTask(session.AsyncToolTasks, task)
	status := types.RunStatus(session.Status)
	if strings.TrimSpace(string(status)) == "" {
		status = types.RunStatusRunning
	}
	if err := s.storage.SaveSessionMessages(ctx, types.SessionMessageSave{Session: session, Status: status}); err != nil {
		return runtimeStorageFailed("failed to save async tool task", err)
	}
	return nil
}

func (s *system) upsertRuntimeAsyncToolTask(task types.AsyncToolTask) {
	s.mu.Lock()
	s.asyncTasks[task.ID] = task
	s.mu.Unlock()
}

func (s *system) loadTaskSession(ctx context.Context, task types.AsyncToolTask) (types.Session, error) {
	if strings.TrimSpace(task.GroupID) != "" {
		return s.storage.LoadGroupSession(ctx, task.GroupID, task.SessionID)
	}
	if strings.TrimSpace(task.WorkspaceID) != "" {
		return s.storage.LoadWorkspaceSession(ctx, task.WorkspaceID, task.RoleID, task.SessionID)
	}
	return s.storage.LoadSession(ctx, task.RoleID, task.SessionID)
}

func (s *system) recoverPersistedAsyncToolTasks(ctx context.Context) error {
	roles, err := s.storage.ListRoles(ctx)
	if err != nil {
		return runtimeStorageFailed("failed to list roles for async tool recovery", err)
	}
	for _, role := range roles {
		roleID := strings.TrimSpace(role.ID)
		if roleID == "" {
			continue
		}
		sessions, err := s.storage.ListSessions(ctx, roleID)
		if err != nil {
			return runtimeStorageFailed("failed to list role sessions for async tool recovery", err)
		}
		for _, summary := range sessions {
			if err := s.recoverPersistedAsyncToolSession(ctx, types.AsyncToolTask{RoleID: roleID, SessionID: summary.ID}); err != nil {
				return err
			}
		}
	}

	groups, err := s.storage.ListChatGroups(ctx)
	if err != nil {
		return runtimeStorageFailed("failed to list groups for async tool recovery", err)
	}
	for _, group := range groups {
		groupID := strings.TrimSpace(group.ID)
		if groupID == "" {
			continue
		}
		sessions, err := s.storage.ListGroupSessions(ctx, groupID)
		if err != nil {
			return runtimeStorageFailed("failed to list group sessions for async tool recovery", err)
		}
		for _, summary := range sessions {
			if err := s.recoverPersistedAsyncToolSession(ctx, types.AsyncToolTask{GroupID: groupID, SessionID: summary.ID}); err != nil {
				return err
			}
		}
	}

	workspaces, err := s.storage.ListWorkspaces(ctx)
	if err != nil {
		return runtimeStorageFailed("failed to list workspaces for async tool recovery", err)
	}
	for _, workspace := range workspaces {
		workspaceID := strings.TrimSpace(workspace.ID)
		if workspaceID == "" {
			continue
		}
		for _, role := range roles {
			roleID := strings.TrimSpace(role.ID)
			if roleID == "" {
				continue
			}
			sessions, err := s.storage.ListWorkspaceSessions(ctx, workspaceID, roleID)
			if err != nil {
				return runtimeStorageFailed("failed to list workspace sessions for async tool recovery", err)
			}
			for _, summary := range sessions {
				if err := s.recoverPersistedAsyncToolSession(ctx, types.AsyncToolTask{RoleID: roleID, WorkspaceID: workspaceID, SessionID: summary.ID}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *system) recoverPersistedAsyncToolSession(ctx context.Context, locator types.AsyncToolTask) error {
	if strings.TrimSpace(locator.SessionID) == "" {
		return nil
	}
	session, err := s.loadTaskSession(ctx, locator)
	if err != nil {
		return runtimeStorageFailed("failed to load async tool recovery session", err)
	}
	recovered, writes, changed := s.recoverSessionAsyncToolTasks(session)
	if !changed {
		return nil
	}
	status := types.RunStatus(recovered.Status)
	if strings.TrimSpace(string(status)) == "" {
		status = types.RunStatusRunning
	}
	if err := s.storage.SaveSessionMessages(ctx, types.SessionMessageSave{Session: recovered, Writes: messageWrites(writes), Status: status}); err != nil {
		return runtimeStorageFailed("failed to save recovered async tool session", err)
	}
	return nil
}

func (s *system) recoverSessionAsyncToolTasks(session types.Session) (types.Session, []types.Message, bool) {
	before := append([]types.AsyncToolTask(nil), session.AsyncToolTasks...)
	session = s.recoverAsyncToolTasks(session)
	ready := readyAsyncToolTasks(session.AsyncToolTasks)
	if len(ready) == 0 {
		return session, nil, !reflect.DeepEqual(before, session.AsyncToolTasks)
	}
	messages := asyncToolResultMessages(ready)
	writes := make([]types.Message, 0, len(messages))
	for _, message := range messages {
		session = appendMessage(session, message)
		writes = append(writes, lastSessionMessage(session))
	}
	for _, task := range ready {
		task.Status = types.AsyncToolTaskStatusCompleted
		task.CompletedAt = time.Now().UTC()
		session.AsyncToolTasks = upsertAsyncToolTask(session.AsyncToolTasks, task)
	}
	return session, writes, true
}

func messageWrites(messages []types.Message) []types.SessionMessageWrite {
	writes := make([]types.SessionMessageWrite, 0, len(messages))
	for _, message := range messages {
		writes = append(writes, types.SessionMessageWrite{Message: message})
	}
	return writes
}

func (s *system) notifyAsyncToolReady(task types.AsyncToolTask) {
	s.mu.Lock()
	hasActiveRun := false
	for _, record := range s.runs {
		if !asyncToolTaskMatchesRun(record, task) || !isActiveRunStatus(record.state.Status) {
			continue
		}
		hasActiveRun = true
		select {
		case record.asyncToolCh <- task.ID:
		default:
		}
	}
	startContinuation := false
	key := asyncContinuationKeyFromTask(task)
	if !hasActiveRun && key.sessionID != "" {
		if _, exists := s.asyncContinuations[key]; !exists {
			s.asyncContinuations[key] = struct{}{}
			startContinuation = true
		}
	}
	s.mu.Unlock()
	if startContinuation {
		go s.continueAfterAsyncToolReady(context.Background(), task, key)
	}
}

func asyncToolTaskMatchesRun(record *runRecord, task types.AsyncToolTask) bool {
	if record == nil || record.asyncToolCh == nil {
		return false
	}
	return strings.TrimSpace(record.roleID) == strings.TrimSpace(task.RoleID) &&
		strings.TrimSpace(record.groupID) == strings.TrimSpace(task.GroupID) &&
		strings.TrimSpace(record.workspaceID) == strings.TrimSpace(task.WorkspaceID) &&
		strings.TrimSpace(record.session.ID) == strings.TrimSpace(task.SessionID)
}

func asyncContinuationKeyFromTask(task types.AsyncToolTask) asyncContinuationKey {
	return asyncContinuationKey{roleID: strings.TrimSpace(task.RoleID), groupID: strings.TrimSpace(task.GroupID), workspaceID: strings.TrimSpace(task.WorkspaceID), sessionID: strings.TrimSpace(task.SessionID)}
}

func (s *system) continueAfterAsyncToolReady(ctx context.Context, task types.AsyncToolTask, key asyncContinuationKey) {
	defer s.releaseAsyncContinuation(key)
	session, err := s.loadTaskSession(ctx, task)
	if err != nil {
		return
	}
	request := runRequestFromAsyncToolContinuation(task, key, lastSessionMessage(session).ID)
	if strings.TrimSpace(request.ContextMessageID) == "" {
		return
	}
	if s.hasActiveAsyncContinuationTarget(key) {
		return
	}
	_, _ = s.StartRun(ctx, request)
}

func runRequestFromAsyncToolContinuation(task types.AsyncToolTask, key asyncContinuationKey, contextMessageID string) types.RunRequest {
	request := types.RunRequest{RoleID: key.roleID, GroupID: key.groupID, WorkspaceID: key.workspaceID, SessionID: key.sessionID, ContextMessageID: strings.TrimSpace(contextMessageID)}
	continuation := task.Continuation
	request.Stream = continuation.Stream
	request.ReasoningEffort = types.TrimReasoningEffort(continuation.ReasoningEffort)
	if continuation.ModelOverride != nil {
		override := *continuation.ModelOverride
		if normalized, ok := types.NormalizeModelOverrideCoordinate(override); ok {
			request.ModelOverride = &normalized
		}
	}
	selection := types.NormalizeHookPromptSelection(continuation.HookPromptMode, continuation.HookPromptPresetID)
	if selection.Mode == types.HookPromptSelectionModeNone || selection.Mode == types.HookPromptSelectionModePreset {
		request.HookPromptMode = selection.Mode
		request.HookPromptPresetID = selection.PresetID
	}
	return request
}

func (s *system) releaseAsyncContinuation(key asyncContinuationKey) {
	s.mu.Lock()
	delete(s.asyncContinuations, key)
	s.mu.Unlock()
}

func (s *system) hasActiveAsyncContinuationTarget(key asyncContinuationKey) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.runs {
		if record == nil || !isActiveRunStatus(record.state.Status) {
			continue
		}
		if strings.TrimSpace(record.roleID) == key.roleID && strings.TrimSpace(record.groupID) == key.groupID && strings.TrimSpace(record.workspaceID) == key.workspaceID && strings.TrimSpace(record.session.ID) == key.sessionID {
			return true
		}
	}
	return false
}

func (s *system) flushAsyncToolResults(ctx context.Context, record *runRecord, contextSession *types.Session) (bool, error) {
	latest, err := s.loadTaskSession(ctx, types.AsyncToolTask{RoleID: record.roleID, GroupID: record.groupID, WorkspaceID: record.workspaceID, SessionID: record.session.ID})
	if err == nil {
		latest = s.recoverAsyncToolTasks(latest)
		record.session.AsyncToolTasks = mergeRuntimeAsyncToolTasks(record.session.AsyncToolTasks, latest.AsyncToolTasks)
	}
	record.session.AsyncToolTasks = mergeRuntimeAsyncToolTasks(record.session.AsyncToolTasks, s.runtimeAsyncToolTasks(types.AsyncToolTaskQuery{RoleID: record.roleID, GroupID: record.groupID, WorkspaceID: record.workspaceID, SessionID: record.session.ID}))
	ready := readyAsyncToolTasks(record.session.AsyncToolTasks)
	if len(ready) == 0 {
		return false, nil
	}
	flushedMessages := make([]types.Message, 0, len(ready))
	for _, message := range asyncToolResultMessages(ready) {
		appendRunMessage(record, message)
		record.activeAssistantID = ""
		flushedMessages = append(flushedMessages, record.messageParent)
		if contextSession != nil {
			*contextSession = appendMessage(*contextSession, record.messageParent)
		}
	}
	for _, task := range ready {
		task.Status = types.AsyncToolTaskStatusCompleted
		task.CompletedAt = time.Now().UTC()
		record.session.AsyncToolTasks = upsertAsyncToolTask(record.session.AsyncToolTasks, task)
		s.mu.Lock()
		s.asyncTasks[task.ID] = task
		s.mu.Unlock()
		s.publishAsyncToolTaskUpdate(record.runID, task)
	}
	if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
		return false, err
	}
	if err := s.saveRunSession(ctx, record, types.RunStatusRunning); err != nil {
		return false, err
	}
	for _, message := range flushedMessages {
		s.publishRunMessageUpdate(record, "run_message_update", message)
	}
	return true, nil
}

func readyAsyncToolTasks(tasks []types.AsyncToolTask) []types.AsyncToolTask {
	ready := []types.AsyncToolTask{}
	for _, task := range tasks {
		if !task.CompletedAt.IsZero() {
			continue
		}
		if task.Status == types.AsyncToolTaskStatusSucceeded || task.Status == types.AsyncToolTaskStatusFailed {
			ready = append(ready, task)
		}
	}
	return ready
}

func asyncToolResultMessages(tasks []types.AsyncToolTask) []types.Message {
	messages := make([]types.Message, 0, len(tasks))
	for _, task := range tasks {
		messages = append(messages, asyncToolResultMessage(task))
	}
	return messages
}

func asyncToolResultContent(task types.AsyncToolTask) string {
	status := "失败"
	content := strings.TrimSpace(task.Error)
	if task.Result != nil && task.Result.Status == types.ToolStatusSuccess {
		status = "成功"
		content = strings.TrimSpace(task.Result.Content)
	} else if task.Result != nil && content == "" {
		content = strings.TrimSpace(task.Result.Content)
	}
	return fmt.Sprintf("以下是异步任务 %s 的执行结果\n工具：%s\n状态：%s\n结果：%s", task.ID, asyncToolResultToolName(task), status, content)
}

func asyncToolResultToolName(task types.AsyncToolTask) string {
	if toolName := strings.TrimSpace(task.ToolName); toolName != "" {
		return toolName
	}
	return strings.TrimSpace(task.Action.ToolName)
}

func upsertAsyncToolTask(tasks []types.AsyncToolTask, task types.AsyncToolTask) []types.AsyncToolTask {
	id := strings.TrimSpace(task.ID)
	if id == "" {
		return tasks
	}
	for index := range tasks {
		if strings.TrimSpace(tasks[index].ID) == id {
			tasks[index] = task
			return tasks
		}
	}
	return append(tasks, task)
}

func mergeRuntimeAsyncToolTasks(left []types.AsyncToolTask, right []types.AsyncToolTask) []types.AsyncToolTask {
	for _, task := range right {
		left = upsertAsyncToolTask(left, task)
	}
	return left
}

func (s *system) publishAsyncToolTaskUpdate(runID string, task types.AsyncToolTask) {
	s.publish(runID, "async_tool_task_update", task)
}

func (s *system) ListAsyncToolTasks(ctx context.Context, query types.AsyncToolTaskQuery) ([]types.AsyncToolTask, error) {
	if err := ctx.Err(); err != nil {
		return nil, runtimeInvalid("list async tool tasks context is cancelled", err)
	}
	tasks := s.runtimeAsyncToolTasks(query)
	if strings.TrimSpace(query.SessionID) != "" {
		if session, err := s.loadTaskSession(ctx, types.AsyncToolTask{RoleID: query.RoleID, GroupID: query.GroupID, WorkspaceID: query.WorkspaceID, SessionID: query.SessionID}); err == nil {
			session = s.recoverAsyncToolTasks(session)
			for _, task := range session.AsyncToolTasks {
				if asyncToolTaskMatches(task, query) {
					tasks = upsertAsyncToolTask(tasks, task)
				}
			}
			status := types.RunStatus(session.Status)
			if strings.TrimSpace(string(status)) == "" {
				status = types.RunStatusRunning
			}
			_ = s.storage.SaveSessionMessages(ctx, types.SessionMessageSave{Session: session, Status: status})
		}
	}
	return tasks, nil
}

func (s *system) runtimeAsyncToolTasks(query types.AsyncToolTaskQuery) []types.AsyncToolTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks := make([]types.AsyncToolTask, 0, len(s.asyncTasks))
	for _, task := range s.asyncTasks {
		if asyncToolTaskMatches(task, query) {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func asyncToolTaskMatches(task types.AsyncToolTask, query types.AsyncToolTaskQuery) bool {
	if strings.TrimSpace(query.RoleID) != "" && strings.TrimSpace(task.RoleID) != strings.TrimSpace(query.RoleID) {
		return false
	}
	if strings.TrimSpace(query.GroupID) != "" && strings.TrimSpace(task.GroupID) != strings.TrimSpace(query.GroupID) {
		return false
	}
	if strings.TrimSpace(query.WorkspaceID) != "" && strings.TrimSpace(task.WorkspaceID) != strings.TrimSpace(query.WorkspaceID) {
		return false
	}
	if strings.TrimSpace(query.SessionID) != "" && strings.TrimSpace(task.SessionID) != strings.TrimSpace(query.SessionID) {
		return false
	}
	return true
}
