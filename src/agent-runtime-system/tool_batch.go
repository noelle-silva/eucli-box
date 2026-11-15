package agentruntime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) handleToolIntents(ctx context.Context, record *runRecord, contextSession *types.Session, intents []types.ToolIntent) ([]types.ToolResult, error) {
	entries := make([]toolRunEntry, 0, len(intents))
	for index, intent := range intents {
		if message := strings.TrimSpace(intent.ArgumentError); message != "" {
			entries = append(entries, failedIntentEntry(record, index, intent, message))
			continue
		}
		action, err := s.tools.NormalizeIntent(ctx, intent)
		if err != nil {
			if isToolContextCancelled(err) {
				return nil, err
			}
			entries = append(entries, failedIntentEntry(record, index, intent, "failed to normalize tool intent: "+err.Error()))
			continue
		}
		action.CallRef = newRuntimeID("tool-call")
		entries = append(entries, toolRunEntry{Index: index, Action: action})
		upsertRunToolPart(record, action, "requested", nil, nil)
	}
	if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
		return nil, err
	}

	for index := range entries {
		if entries[index].HasResult {
			continue
		}
		scope := types.ToolRunScope{RunID: record.runID, RoleID: record.roleID, GroupID: record.groupID, WorkspaceID: record.workspaceID, SessionID: record.session.ID}
		plan, err := s.tools.Prepare(ctx, scope, entries[index].Action)
		if err != nil {
			if isToolContextCancelled(err) {
				return nil, err
			}
			result := failedToolResult(entries[index].Action, "failed to prepare tool run plan: "+err.Error())
			entries[index].Result = result
			entries[index].HasResult = true
			upsertRunToolPart(record, entries[index].Action, "error", nil, &result)
			continue
		}
		entries[index].Plan = plan
		// 待确认决策在对外可见前先登记进台账：工具部件一旦落盘/发布，客户端即可提交并被路由。
		s.registerToolConfirmation(record, plan)
		s.publish(eventSourceFromRecord(record), "tool_requested", plan)
		s.applyPreparedToolPlan(record, &entries[index])
	}
	if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
		return nil, err
	}
	if err := s.saveRunSession(ctx, record, types.RunStatusRunning); err != nil {
		return nil, err
	}
	s.publishAssistantMessageDelta(record)

	if err := s.resolvePendingToolConfirmations(ctx, record, contextSession, entries); err != nil {
		return nil, err
	}

	if err := s.acceptAsyncToolEntries(ctx, record, entries); err != nil {
		return nil, err
	}

	ready := readyToolEntries(entries)
	for _, entry := range ready {
		upsertRunToolPart(record, entry.Action, "running", &entry.Plan.Decision, nil)
	}
	if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
		return nil, err
	}
	if err := s.saveRunSession(ctx, record, types.RunStatusRunning); err != nil {
		return nil, err
	}
	s.publishAssistantMessageDelta(record)
	executed, updateErr := s.executeReadyTools(ctx, record, ready, func(result toolExecutionResult) error {
		record.sessionMu.Lock()
		defer record.sessionMu.Unlock()
		upsertRunToolPart(record, result.Entry.Action, toolResultPartState(result), &result.Entry.Plan.Decision, &result.Result)
		if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
			return err
		}
		if err := s.saveRunSession(ctx, record, types.RunStatusRunning); err != nil {
			return err
		}
		s.publishAssistantMessageUpdate(record)
		return nil
	})
	// 取消时已经有确定结论的工具，结果就是它被中断前留下的产出事实：
	// 先把它们写进部件，别让整轮收口用一个空结果盖掉已经跑出来的内容。
	s.settleCancelledToolResults(record, executed.Cancelled)
	if updateErr != nil {
		return nil, updateErr
	}

	return orderedToolResults(entries, executed.Executed)
}

// settleCancelledToolResults 把取消瞬间已经落定的工具结果写进部件。
// 这里不落盘：整轮的取消收口会统一保存会话，这里只负责让部件先拿到事实。
func (s *system) settleCancelledToolResults(record *runRecord, settled []toolExecutionResult) {
	if len(settled) == 0 {
		return
	}
	record.sessionMu.Lock()
	defer record.sessionMu.Unlock()
	for _, result := range settled {
		upsertRunToolPart(record, result.Entry.Action, toolResultPartState(result), &result.Entry.Plan.Decision, &result.Result)
	}
	_ = s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID)
}

func (s *system) resolvePendingToolConfirmations(ctx context.Context, record *runRecord, contextSession *types.Session, entries []toolRunEntry) error {
	for {
		pending := pendingConfirmationPlans(entries)
		if len(pending) == 0 {
			return nil
		}
		pendingIDs := pendingDecisionIDs(pending)
		if err := s.saveRunSession(ctx, record, types.RunStatusWaitingConfirmation); err != nil {
			return err
		}
		confirmed, err := s.waitForConfirmations(ctx, record, contextSession, pending)
		if err != nil {
			s.markPendingToolsCancelled(record, entries, err)
			if stateErr := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); stateErr != nil {
				return stateErr
			}
			return err
		}
		applyConfirmedPlans(entries, confirmed)
		if !toolConfirmationProgressed(pendingIDs, confirmed) {
			return runtimeStateInvalid("tool confirmation did not progress", nil)
		}
		s.applyDeniedToolPlans(record, entries)
		if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
			return err
		}
		if err := s.saveRunSession(ctx, record, types.RunStatusRunning); err != nil {
			return err
		}
		s.publishAssistantMessageUpdate(record)
	}
}

func (s *system) markPendingToolsCancelled(record *runRecord, entries []toolRunEntry, err error) {
	for _, entry := range entries {
		if entry.Plan.PlanStatus != types.ToolPlanStatusNeedsConfirmation {
			continue
		}
		metadata := map[string]any{"cancelReason": "user_interrupted"}
		result := types.ToolResult{ID: newRuntimeID("tool-result"), ActionID: entry.Action.ID, ToolName: entry.Action.ToolName, Status: types.ToolStatusCancelled, Content: types.AppendToolExecutionMarker("", types.ToolStatusCancelled, types.ToolExecutionNoticeCancelled), Metadata: metadata, Error: err.Error(), CreatedAt: time.Now().UTC()}
		upsertRunToolPart(record, entry.Action, "cancelled", &entry.Plan.Decision, &result)
	}
}

func (s *system) applyDeniedToolPlans(record *runRecord, entries []toolRunEntry) {
	for index := range entries {
		if entries[index].Plan.PlanStatus != types.ToolPlanStatusDenied || entries[index].HasResult {
			continue
		}
		result := deniedToolResult(entries[index].Action, entries[index].Plan.Decision)
		entries[index].Result = result
		entries[index].HasResult = true
		upsertRunToolPart(record, entries[index].Action, "denied", &entries[index].Plan.Decision, &result)
	}
}

func (s *system) applyPreparedToolPlan(record *runRecord, entry *toolRunEntry) {
	switch entry.Plan.PlanStatus {
	case types.ToolPlanStatusDenied:
		result := deniedToolResult(entry.Action, entry.Plan.Decision)
		entry.Result = result
		entry.HasResult = true
		upsertRunToolPart(record, entry.Action, "denied", &entry.Plan.Decision, &result)
	case types.ToolPlanStatusNeedsConfirmation:
		upsertRunToolPart(record, entry.Action, "needs_confirmation", &entry.Plan.Decision, nil)
	}
}

func (s *system) executeReadyTools(ctx context.Context, record *runRecord, entries []toolRunEntry, onResult func(toolExecutionResult) error) (toolExecutionBatch, error) {
	if len(entries) == 0 {
		return toolExecutionBatch{}, nil
	}
	limit := s.config.MaxParallelTools
	if limit > len(entries) {
		limit = len(entries)
	}
	// 工具执行协程只读这一份不可变身份快照，不触碰运行状态；
	// 实时输出的发布因此不会与运行主干并发读写同一条运行。
	outputContext := toolOutputContextFromRun(record)
	semaphore := make(chan struct{}, limit)
	resultCh := make(chan toolExecutionResult, len(entries))
	var wg sync.WaitGroup
	for index, entry := range entries {
		wg.Add(1)
		go func(resultIndex int, entry toolRunEntry) {
			defer wg.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				resultCh <- toolExecutionResult{Entry: entry, Err: context.Canceled}
				return
			}
			if cancellationErr, ok := toolExecutionCancelled(ctx, nil, nil); ok {
				resultCh <- toolExecutionResult{Entry: entry, Err: cancellationErr}
				return
			}
			result, err := s.tools.ExecuteWithOutputUpdate(ctx, entry.Plan, func(update types.ToolOutputUpdate) {
				s.publishToolOutputUpdate(outputContext, entry, update)
			})
			if cancellationErr, ok := toolExecutionCancelled(ctx, nil, err); ok {
				// 取消不改写已经拿到的结论：执行边界翻译出了什么状态就带什么状态回去，
				// 要不要落定由批次层按「是否已有确定结论」决定。
				resultCh <- toolExecutionResult{Entry: entry, Result: result, Err: cancellationErr}
				return
			}
			if err != nil && strings.TrimSpace(string(result.Status)) == "" {
				// 只有执行边界没给出任何终局时才兜底造失败结果：
				// 已经带状态的结果携带的是工具自己的事实，不能被这条兜底覆盖。
				result = failedToolResult(entry.Action, err.Error())
			}
			resultCh <- toolExecutionResult{Entry: entry, Result: result, Err: err}
		}(index, entry)
	}
	go func() {
		wg.Wait()
		close(resultCh)
	}()
	batch := toolExecutionBatch{Executed: make([]toolExecutionResult, 0, len(entries))}
	var firstUpdateErr error
	for result := range resultCh {
		batch.Executed = append(batch.Executed, result)
		if isToolContextCancelled(result.Err) {
			if hasSettledToolResult(result.Result) {
				batch.Cancelled = append(batch.Cancelled, result)
			}
			if firstUpdateErr == nil {
				firstUpdateErr = result.Err
			}
			continue
		}
		if onResult == nil {
			continue
		}
		if err := onResult(result); err != nil && firstUpdateErr == nil {
			firstUpdateErr = err
		}
	}
	return batch, firstUpdateErr
}

// hasSettledToolResult 判定取消瞬间这次调用是否已经带有值得保留的结论。
// 成功与被取消都是工具自己的终局事实，必须原样保留；
// 失败只是取消过程的副作用（工具观察到父进程被终止后自行报错），
// 由整轮收口按取消落定，不能记成一次工具失败。
func hasSettledToolResult(result types.ToolResult) bool {
	switch result.Status {
	case types.ToolStatusSuccess, types.ToolStatusCancelled:
		return true
	default:
		return false
	}
}

func pendingConfirmationPlans(entries []toolRunEntry) []types.ToolRunPlan {
	plans := []types.ToolRunPlan{}
	for _, entry := range entries {
		if entry.Plan.PlanStatus == types.ToolPlanStatusNeedsConfirmation {
			plans = append(plans, entry.Plan)
		}
	}
	return plans
}

func pendingDecisionIDs(plans []types.ToolRunPlan) map[string]string {
	ids := make(map[string]string, len(plans))
	for _, plan := range plans {
		ids[toolCallRef(plan.Action)] = plan.Decision.ID
	}
	return ids
}

func toolConfirmationProgressed(previous map[string]string, confirmed []types.ToolRunPlan) bool {
	if len(previous) != len(confirmed) {
		return false
	}
	for _, plan := range confirmed {
		previousID, ok := previous[toolCallRef(plan.Action)]
		if !ok {
			return false
		}
		if plan.PlanStatus == types.ToolPlanStatusNeedsConfirmation && plan.Decision.ID == previousID {
			return false
		}
	}
	return true
}

func applyConfirmedPlans(entries []toolRunEntry, plans []types.ToolRunPlan) {
	confirmed := map[string]types.ToolRunPlan{}
	for _, plan := range plans {
		confirmed[toolCallRef(plan.Action)] = plan
	}
	for index := range entries {
		if plan, ok := confirmed[toolCallRef(entries[index].Action)]; ok {
			entries[index].Plan = plan
		}
	}
}

// toolCallRef 返回一次工具调用的内部唯一标识：优先用运行期分配的 CallRef，
// 缺失时回落到模型给的调用编号。它只用于运行期定位部件，不用于与模型通信。
func toolCallRef(action types.ToolAction) string {
	if ref := strings.TrimSpace(action.CallRef); ref != "" {
		return ref
	}
	return strings.TrimSpace(action.ID)
}

func readyToolEntries(entries []toolRunEntry) []toolRunEntry {
	ready := []toolRunEntry{}
	for _, entry := range entries {
		if entry.HasResult {
			continue
		}
		if entry.Plan.PlanStatus == types.ToolPlanStatusReady {
			ready = append(ready, entry)
		}
	}
	return ready
}

func orderedToolResults(entries []toolRunEntry, executed []toolExecutionResult) ([]types.ToolResult, error) {
	results := make([]types.ToolResult, 0, len(entries))
	for _, entry := range entries {
		if entry.HasResult {
			results = append(results, entry.Result)
			continue
		}
		if entry.Plan.PlanStatus == types.ToolPlanStatusDenied {
			if !entry.HasResult {
				return nil, runtimeStateInvalid("denied tool result was not produced", nil)
			}
			results = append(results, entry.Result)
			continue
		}
		executedResult, ok := toolExecutionResultByIndex(executed, entry.Index)
		if !ok {
			return nil, runtimeStateInvalid("tool execution result was not produced", nil)
		}
		results = append(results, executedResult.Result)
	}
	return results, nil
}

func toolExecutionResultByIndex(results []toolExecutionResult, index int) (toolExecutionResult, bool) {
	for _, result := range results {
		if result.Entry.Index == index {
			return result, true
		}
	}
	return toolExecutionResult{}, false
}

// toolResultPartState 由结果自身给出的终局决定部件状态。
// 错误只用来解释「没有终局」的情况：工具被取消时可能已经成功跑完，
// 那时结果状态是 success，部件就必须留在已完成，不能被取消标成失败。
func toolResultPartState(result toolExecutionResult) string {
	switch result.Result.Status {
	case types.ToolStatusSuccess:
		return "completed"
	case types.ToolStatusDenied:
		return "denied"
	case types.ToolStatusCancelled:
		return "cancelled"
	case types.ToolStatusFailed:
		return "error"
	}
	return "error"
}

func deniedToolResult(action types.ToolAction, decision types.PermissionDecision) types.ToolResult {
	return types.ToolResult{ID: newRuntimeID("tool-result"), ActionID: action.ID, ToolName: action.ToolName, Status: types.ToolStatusDenied, Error: decision.Reason, CreatedAt: time.Now().UTC()}
}

func failedToolResult(action types.ToolAction, message string) types.ToolResult {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "tool execution failed"
	}
	return types.ToolResult{ID: newRuntimeID("tool-result"), ActionID: action.ID, ToolName: action.ToolName, Status: types.ToolStatusFailed, Content: message, Error: message, CreatedAt: time.Now().UTC()}
}

// failedIntentEntry 把一次无法执行的工具意图落定为该次调用的失败结果：
// 走与正常工具结果同一条回喂通道，不升级为整轮失败，让模型看到并自行纠正。
func failedIntentEntry(record *runRecord, index int, intent types.ToolIntent, message string) toolRunEntry {
	action := fallbackToolAction(intent)
	action.CallRef = newRuntimeID("tool-call")
	result := failedToolResult(action, message)
	upsertRunToolPart(record, action, "error", nil, &result)
	return toolRunEntry{Index: index, Action: action, Result: result, HasResult: true}
}

func fallbackToolAction(intent types.ToolIntent) types.ToolAction {
	actionID := strings.TrimSpace(intent.ID)
	if actionID == "" {
		actionID = newRuntimeID("tool-action")
	}
	toolName := strings.TrimSpace(intent.ToolName)
	if toolName == "" {
		toolName = "unknown_tool"
	}
	arguments := map[string]any{}
	for key, value := range intent.Arguments {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		arguments[trimmed] = value
	}
	return types.ToolAction{ID: actionID, ToolName: toolName, Arguments: arguments, Raw: intent.Raw, CreatedAt: time.Now().UTC()}
}

func isToolContextCancelled(err error) bool {
	return errors.Is(err, context.Canceled)
}

func toolExecutionCancelled(parent context.Context, tool context.Context, err error) (error, bool) {
	if isToolContextCancelled(err) {
		return err, true
	}
	if contextIsCancelled(parent) || contextIsCancelled(tool) {
		return context.Canceled, true
	}
	return nil, false
}

func contextIsCancelled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	return errors.Is(ctx.Err(), context.Canceled)
}

type toolRunEntry struct {
	Index     int
	Action    types.ToolAction
	Plan      types.ToolRunPlan
	Result    types.ToolResult
	HasResult bool
}

type toolExecutionResult struct {
	Entry  toolRunEntry
	Result types.ToolResult
	Err    error
}

// toolExecutionBatch 是一批工具执行的收集事实。
// Executed 保留全部执行结果（含被取消的），供按调用顺序取回；
// Cancelled 单独挑出「取消瞬间已经有确定结论」的那些——它们带着工具被中断前的产出，
// 必须写进部件，不能留给整轮收口用一个空结果覆盖。
type toolExecutionBatch struct {
	Executed  []toolExecutionResult
	Cancelled []toolExecutionResult
}
