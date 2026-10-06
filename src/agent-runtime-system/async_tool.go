package agentruntime

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

type asyncContinuationKey struct {
	roleID      string
	groupID     string
	workspaceID string
	sessionID   string
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
		s.publishAsyncToolTaskUpdate(task)
		s.publishAssistantMessageDelta(record)
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
	continuation := types.RunContinuation{Stream: &record.stream, ReasoningEffort: types.TrimReasoningEffort(record.reasoningEffort)}
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

// executeAsyncToolTask 在后台只做真实工具执行，并更新运行期任务登记。
// 它绝不加载、修改或落盘会话：任务结果作为事件投递给所属运行，
// 由该运行（或新起的续跑运行）成为唯一写入者，把结果投影进会话。
func (s *system) executeAsyncToolTask(task types.AsyncToolTask) {
	task.Status = types.AsyncToolTaskStatusRunning
	task.StartedAt = time.Now().UTC()
	s.upsertRuntimeAsyncToolTask(task)
	s.publishAsyncToolTaskUpdate(task)

	result, err := s.tools.Execute(context.Background(), task.Plan)
	task.FinishedAt = time.Now().UTC()
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
	s.upsertRuntimeAsyncToolTask(task)
	s.publishAsyncToolTaskUpdate(task)
	s.notifyAsyncToolReady(task)
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

// recoverPersistedAsyncToolTasks 在启动时恢复上次进程遗留的异步工具任务。
// 这是启动维护动作，不是启动关键：任何一条数据读取失败只记录日志并跳过，
// 不阻断本体启动；相关会话读取时会如实报错。
func (s *system) recoverPersistedAsyncToolTasks(ctx context.Context) {
	roles, err := s.storage.ListRoles(ctx)
	if err != nil {
		log.Printf("agent-runtime-system: 异步任务恢复跳过角色清单：%v", err)
	}
	for _, role := range roles {
		roleID := strings.TrimSpace(role.ID)
		if roleID == "" {
			continue
		}
		sessions, err := s.storage.ListSessions(ctx, roleID)
		if err != nil {
			log.Printf("agent-runtime-system: 异步任务恢复跳过角色 %s 的会话清单：%v", roleID, err)
			continue
		}
		for _, summary := range sessions {
			if err := s.recoverPersistedAsyncToolSession(ctx, types.AsyncToolTask{RoleID: roleID, SessionID: summary.ID}); err != nil {
				log.Printf("agent-runtime-system: 异步任务恢复跳过会话 %s/%s：%v", roleID, summary.ID, err)
			}
		}
	}

	groups, err := s.storage.ListChatGroups(ctx)
	if err != nil {
		log.Printf("agent-runtime-system: 异步任务恢复跳过群组清单：%v", err)
	}
	for _, group := range groups {
		groupID := strings.TrimSpace(group.ID)
		if groupID == "" {
			continue
		}
		sessions, err := s.storage.ListGroupSessions(ctx, groupID)
		if err != nil {
			log.Printf("agent-runtime-system: 异步任务恢复跳过群组 %s 的会话清单：%v", groupID, err)
			continue
		}
		for _, summary := range sessions {
			if err := s.recoverPersistedAsyncToolSession(ctx, types.AsyncToolTask{GroupID: groupID, SessionID: summary.ID}); err != nil {
				log.Printf("agent-runtime-system: 异步任务恢复跳过群组会话 %s/%s：%v", groupID, summary.ID, err)
			}
		}
	}

	workspaces, err := s.storage.ListWorkspaces(ctx)
	if err != nil {
		log.Printf("agent-runtime-system: 异步任务恢复跳过工作区清单：%v", err)
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
				log.Printf("agent-runtime-system: 异步任务恢复跳过工作区 %s/角色 %s 的会话清单：%v", workspaceID, roleID, err)
				continue
			}
			for _, summary := range sessions {
				if err := s.recoverPersistedAsyncToolSession(ctx, types.AsyncToolTask{RoleID: roleID, WorkspaceID: workspaceID, SessionID: summary.ID}); err != nil {
					log.Printf("agent-runtime-system: 异步任务恢复跳过工作区会话 %s/%s/%s：%v", workspaceID, roleID, summary.ID, err)
				}
			}
		}
	}
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
	// 启动时不存在活动运行，落盘的认领标记都是上次进程遗留的陈旧认领，先清除以便重试。
	session = clearStaleAsyncToolClaims(session)
	ready := readyAsyncToolTasks(session.AsyncToolTasks)
	if len(ready) == 0 {
		return session, nil, !reflect.DeepEqual(before, session.AsyncToolTasks)
	}
	writes := make([]types.Message, 0, len(ready))
	for _, task := range ready {
		// 第二道防线：结果消息已按任务 ID 落盘则视为已回灌，只补完成标记，不再重复生成。
		if !sessionHasAsyncToolResult(session, task.ID) {
			session = appendMessage(session, asyncToolResultMessage(task))
			writes = append(writes, lastSessionMessage(session))
		}
		task.Status = types.AsyncToolTaskStatusCompleted
		task.CompletedAt = time.Now().UTC()
		session.AsyncToolTasks = upsertAsyncToolTask(session.AsyncToolTasks, task)
	}
	return session, writes, true
}

// clearStaleAsyncToolClaims 清除未完成任务的陈旧认领标记。
func clearStaleAsyncToolClaims(session types.Session) types.Session {
	for index := range session.AsyncToolTasks {
		task := &session.AsyncToolTasks[index]
		if task.CompletedAt.IsZero() && !task.InjectionClaimedAt.IsZero() {
			task.InjectionClaimedAt = time.Time{}
			task.InjectionClaimRunID = ""
		}
	}
	return session
}

// sessionHasAsyncToolResult 判断会话中是否已存在绑定该任务 ID 的结果消息。
func sessionHasAsyncToolResult(session types.Session, taskID string) bool {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false
	}
	for _, message := range session.Messages {
		if message.Type == types.MessageTypeAsyncToolResult && strings.TrimSpace(message.AsyncToolTaskID) == taskID {
			return true
		}
	}
	return false
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
		enqueueRunEvent(record, runEvent{kind: runEventAsyncReady, taskID: task.ID})
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
	if record == nil || record.inbox == nil {
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

// flushAsyncToolResults 在运行主干的安全点把就绪的异步结果回灌进会话并落盘，
// 随后开启新一轮模型回复。它只承载「数据到达」：把结果并入会话与模型上下文，
// 不参与循环推进判断。
func (s *system) flushAsyncToolResults(ctx context.Context, record *runRecord, contextSession *types.Session) (bool, error) {
	return s.flushReadyAsyncToolResults(ctx, record, contextSession, types.RunStatusRunning, false)
}

// flushAsyncToolResultsDuringConfirmation 在确认等待期间就地回灌就绪的异步结果。
// 等待状态不延迟数据落盘；同时保留当前助手消息锚点，让未决工具批继续落在同一条回复上。
func (s *system) flushAsyncToolResultsDuringConfirmation(ctx context.Context, record *runRecord, contextSession *types.Session) (bool, error) {
	return s.flushReadyAsyncToolResults(ctx, record, contextSession, types.RunStatusWaitingConfirmation, true)
}

func (s *system) flushReadyAsyncToolResults(ctx context.Context, record *runRecord, contextSession *types.Session, status types.RunStatus, preserveAssistant bool) (bool, error) {
	latest, err := s.loadTaskSession(ctx, types.AsyncToolTask{RoleID: record.roleID, GroupID: record.groupID, WorkspaceID: record.workspaceID, SessionID: record.session.ID})
	if err == nil {
		latest = s.recoverAsyncToolTasks(latest)
		record.session.AsyncToolTasks = types.MergeAsyncToolTasks(record.session.AsyncToolTasks, latest.AsyncToolTasks)
	}
	record.session.AsyncToolTasks = types.MergeAsyncToolTasks(record.session.AsyncToolTasks, s.runtimeAsyncToolTasks(types.AsyncToolTaskQuery{RoleID: record.roleID, GroupID: record.groupID, WorkspaceID: record.workspaceID, SessionID: record.session.ID}))
	ready := readyAsyncToolTasks(record.session.AsyncToolTasks)
	if len(ready) == 0 {
		return false, nil
	}
	flushed := false
	for _, task := range ready {
		// 1. 认领：以任务 ID 为键的跨运行原子条件写。
		// 只有认领成功的运行才生成并落盘结果消息；认领失败视为结果已被回灌，静默跳过、不报错。
		claimed, err := s.storage.ClaimAsyncToolResult(ctx, record.session, task.ID, record.runID)
		if err != nil {
			return flushed, runtimeStorageFailed("failed to claim async tool result", err)
		}
		if !claimed {
			continue
		}
		// 2. 生成结果消息：消息携带任务 ID，作为回灌幂等的第二道防线。
		before := snapshotAsyncFlush(record)
		message := asyncToolResultMessage(task)
		appendRunMessage(record, message)
		if !preserveAssistant {
			record.activeAssistantID = ""
		}
		if contextSession != nil {
			*contextSession = appendMessage(*contextSession, record.messageParent)
		}
		// 3./4. 落盘并标记：结果消息与完成标记在同一次原子落盘内写入。
		completed := task
		completed.Status = types.AsyncToolTaskStatusCompleted
		completed.CompletedAt = time.Now().UTC()
		record.session.AsyncToolTasks = upsertAsyncToolTask(record.session.AsyncToolTasks, completed)
		if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
			s.rollbackAsyncToolFlush(record, before, message.ID, task)
			s.releaseAsyncToolResultClaim(ctx, record, task)
			return flushed, err
		}
		if err := s.saveRunSession(ctx, record, status); err != nil {
			// 落盘失败必须回退认领，保留可重试，绝不丢结果；
			// 同时撤销运行副本里未落定的结果消息与完成标记，避免失败收口把它当成本次产出落盘。
			s.rollbackAsyncToolFlush(record, before, message.ID, task)
			s.releaseAsyncToolResultClaim(ctx, record, task)
			return flushed, err
		}
		// 落盘成功才广播并回收运行期登记。
		s.publishRunMessageUpdate(record, "run_message_update", record.messageParent)
		s.publishAsyncToolTaskUpdate(completed)
		s.reclaimAsyncToolTask(task.ID)
		flushed = true
	}
	return flushed, nil
}

// asyncFlushSnapshot 记录一次回灌前运行副本的消息锚点，供回滚恢复。
type asyncFlushSnapshot struct {
	messageParent     types.Message
	lastMessageID     string
	activeAssistantID string
}

func snapshotAsyncFlush(record *runRecord) asyncFlushSnapshot {
	if record == nil {
		return asyncFlushSnapshot{}
	}
	return asyncFlushSnapshot{messageParent: record.messageParent, lastMessageID: record.lastMessageID, activeAssistantID: record.activeAssistantID}
}

// rollbackAsyncToolFlush 撤销一次未落定的回灌：移除结果消息与完成标记，恢复消息锚点，
// 让运行收口落盘的是回灌前的真实状态，而不是一次未成立的产出。
func (s *system) rollbackAsyncToolFlush(record *runRecord, snapshot asyncFlushSnapshot, messageID string, task types.AsyncToolTask) {
	if record == nil {
		return
	}
	record.session.Messages = removeRunMessageByID(record.session.Messages, messageID)
	delete(record.ownedMessageIDs, messageID)
	record.session.AsyncToolTasks = upsertAsyncToolTask(record.session.AsyncToolTasks, task)
	record.messageParent = snapshot.messageParent
	record.lastMessageID = snapshot.lastMessageID
	record.activeAssistantID = snapshot.activeAssistantID
}

func removeRunMessageByID(messages []types.Message, messageID string) []types.Message {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return messages
	}
	out := make([]types.Message, 0, len(messages))
	for _, message := range messages {
		if strings.TrimSpace(message.ID) == messageID {
			continue
		}
		out = append(out, message)
	}
	return out
}

// releaseAsyncToolResultClaim 回退一次未落定的认领，让该结果保留可重试。
func (s *system) releaseAsyncToolResultClaim(ctx context.Context, record *runRecord, task types.AsyncToolTask) {
	if record == nil {
		return
	}
	if err := s.storage.ReleaseAsyncToolResultClaim(ctx, record.session, task.ID, record.runID); err != nil {
		log.Printf("agent-runtime-system: 回退异步结果认领失败 task=%s: %v", task.ID, err)
	}
}

// reclaimAsyncToolTask 回收一条已完成回灌的异步任务运行期登记；
// 权威副本已在会话存储，内存登记删除不影响查询与恢复。
func (s *system) reclaimAsyncToolTask(taskID string) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return
	}
	s.mu.Lock()
	delete(s.asyncTasks, taskID)
	s.mu.Unlock()
}

func readyAsyncToolTasks(tasks []types.AsyncToolTask) []types.AsyncToolTask {
	ready := []types.AsyncToolTask{}
	for _, task := range tasks {
		// 已完成或已被认领的任务都不再是「就绪待回灌」。
		if !task.CompletedAt.IsZero() || !task.InjectionClaimedAt.IsZero() {
			continue
		}
		if task.Status == types.AsyncToolTaskStatusSucceeded || task.Status == types.AsyncToolTaskStatusFailed {
			ready = append(ready, task)
		}
	}
	return ready
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

func (s *system) publishAsyncToolTaskUpdate(task types.AsyncToolTask) {
	s.publish(eventSourceFromAsyncTask(task), "async_tool_task_update", task)
}

func (s *system) ListAsyncToolTasks(ctx context.Context, query types.AsyncToolTaskQuery) ([]types.AsyncToolTask, error) {
	if err := ctx.Err(); err != nil {
		return nil, runtimeInvalid("list async tool tasks context is cancelled", err)
	}
	// 读取只读快照：绝不修改会话、绝不回写状态。
	// 会话落盘的任务作为基线，运行期登记（更鲜活）覆盖其上。
	tasks := []types.AsyncToolTask{}
	if strings.TrimSpace(query.SessionID) != "" {
		if session, err := s.loadTaskSession(ctx, types.AsyncToolTask{RoleID: query.RoleID, GroupID: query.GroupID, WorkspaceID: query.WorkspaceID, SessionID: query.SessionID}); err == nil {
			for _, task := range session.AsyncToolTasks {
				if asyncToolTaskMatches(task, query) {
					tasks = upsertAsyncToolTask(tasks, task)
				}
			}
		}
	}
	tasks = types.MergeAsyncToolTasks(tasks, s.runtimeAsyncToolTasks(query))
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
