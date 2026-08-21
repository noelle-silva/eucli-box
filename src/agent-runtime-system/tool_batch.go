package agentruntime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) handleToolIntents(ctx context.Context, record *runRecord, intents []types.ToolIntent) ([]types.ToolResult, error) {
	entries := make([]toolRunEntry, 0, len(intents))
	for index, intent := range intents {
		action, err := s.tools.NormalizeIntent(ctx, intent)
		if err != nil {
			if isToolContextCancelled(err) {
				return nil, err
			}
			action = fallbackToolAction(intent)
			result := failedToolResult(action, "failed to normalize tool intent: "+err.Error())
			entries = append(entries, toolRunEntry{Index: index, Action: action, Result: result, HasResult: true})
			upsertRunToolPart(record, action, "error", nil, &result)
			continue
		}
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
		plan, err := s.tools.Prepare(ctx, record.roleID, record.workspaceID, entries[index].Action)
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
		s.publish(record.runID, "tool_requested", plan)
		s.applyPreparedToolPlan(record, &entries[index])
	}
	if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
		return nil, err
	}
	if err := s.saveRunSession(ctx, record, types.RunStatusRunning); err != nil {
		return nil, err
	}
	s.publishAssistantMessageUpdate(record)

	if err := s.resolvePendingToolConfirmations(ctx, record, entries); err != nil {
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
	s.publishAssistantMessageUpdate(record)
	executed, updateErr := s.executeReadyTools(ctx, record, ready, func(result toolExecutionResult) error {
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
	if updateErr != nil {
		return nil, updateErr
	}

	return orderedToolResults(entries, executed)
}

func (s *system) resolvePendingToolConfirmations(ctx context.Context, record *runRecord, entries []toolRunEntry) error {
	for {
		pending := pendingConfirmationPlans(entries)
		if len(pending) == 0 {
			return nil
		}
		pendingIDs := pendingDecisionIDs(pending)
		if err := s.saveRunSession(ctx, record, types.RunStatusWaitingConfirmation); err != nil {
			return err
		}
		confirmed, err := s.waitForConfirmations(ctx, record, pending)
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
		if isAsyncToolInterruption(err) {
			metadata["cancelReason"] = "system_event"
		}
		result := types.ToolResult{ID: newRuntimeID("tool-result"), ActionID: entry.Action.ID, ToolName: entry.Action.ToolName, Status: types.ToolStatusCancelled, Metadata: metadata, Error: err.Error(), CreatedAt: time.Now().UTC()}
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

func (s *system) executeReadyTools(ctx context.Context, record *runRecord, entries []toolRunEntry, onResult func(toolExecutionResult) error) ([]toolExecutionResult, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	limit := s.config.MaxParallelTools
	if limit > len(entries) {
		limit = len(entries)
	}
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
				s.publishToolOutputUpdate(record, entry, update)
			})
			if cancellationErr, ok := toolExecutionCancelled(ctx, nil, err); ok {
				resultCh <- toolExecutionResult{Entry: entry, Err: cancellationErr}
				return
			}
			if err != nil {
				result = failedToolResult(entry.Action, err.Error())
			}
			resultCh <- toolExecutionResult{Entry: entry, Result: result, Err: err}
		}(index, entry)
	}
	go func() {
		wg.Wait()
		close(resultCh)
	}()
	results := make([]toolExecutionResult, 0, len(entries))
	var firstUpdateErr error
	for result := range resultCh {
		results = append(results, result)
		if isToolContextCancelled(result.Err) {
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
	return results, firstUpdateErr
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
		ids[plan.Action.ID] = plan.Decision.ID
	}
	return ids
}

func toolConfirmationProgressed(previous map[string]string, confirmed []types.ToolRunPlan) bool {
	if len(previous) != len(confirmed) {
		return false
	}
	for _, plan := range confirmed {
		previousID, ok := previous[plan.Action.ID]
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
		confirmed[plan.Action.ID] = plan
	}
	for index := range entries {
		if plan, ok := confirmed[entries[index].Action.ID]; ok {
			entries[index].Plan = plan
		}
	}
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

func toolResultPartState(result toolExecutionResult) string {
	if result.Err != nil {
		return "error"
	}
	switch result.Result.Status {
	case types.ToolStatusSuccess:
		return "completed"
	case types.ToolStatusDenied:
		return "denied"
	case types.ToolStatusCancelled:
		return "cancelled"
	default:
		return "error"
	}
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
	source := strings.TrimSpace(intent.Source)
	if source == "" {
		source = types.ToolCallSourceNative
	}
	return types.ToolAction{ID: actionID, ToolName: toolName, Arguments: arguments, Source: source, Raw: intent.Raw, CreatedAt: time.Now().UTC()}
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
