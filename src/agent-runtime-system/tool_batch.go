package agentruntime

import (
	"context"
	"sync"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) handleToolIntents(ctx context.Context, record *runRecord, intents []types.ToolIntent) ([]types.ToolResult, error) {
	entries := make([]toolRunEntry, 0, len(intents))
	for index, intent := range intents {
		action, err := s.tools.NormalizeIntent(ctx, intent)
		if err != nil {
			return nil, runtimeToolFailed("failed to normalize tool intent", err)
		}
		entries = append(entries, toolRunEntry{Index: index, Action: action})
		upsertRunToolPart(record, action, "requested", nil, nil)
	}
	if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
		return nil, err
	}

	for index := range entries {
		plan, err := s.tools.Prepare(ctx, record.roleID, entries[index].Action)
		if err != nil {
			return nil, runtimeToolFailed("failed to prepare tool run plan", err)
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

	pending := pendingConfirmationPlans(entries)
	if len(pending) > 0 {
		if err := s.saveRunSession(ctx, record, types.RunStatusWaitingConfirmation); err != nil {
			return nil, err
		}
		confirmed, err := s.waitForConfirmations(ctx, record, pending)
		if err != nil {
			for _, entry := range entries {
				if entry.Plan.PlanStatus != types.ToolPlanStatusNeedsConfirmation {
					continue
				}
				result := types.ToolResult{ID: newRuntimeID("tool-result"), ActionID: entry.Action.ID, ToolName: entry.Action.ToolName, Status: types.ToolStatusCancelled, Error: err.Error(), CreatedAt: time.Now().UTC()}
				upsertRunToolPart(record, entry.Action, "cancelled", &entry.Plan.Decision, &result)
			}
			if stateErr := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); stateErr != nil {
				return nil, stateErr
			}
			return nil, err
		}
		applyConfirmedPlans(entries, confirmed)
		for index := range entries {
			if entries[index].Plan.PlanStatus == types.ToolPlanStatusDenied {
				result := deniedToolResult(entries[index].Action, entries[index].Plan.Decision)
				entries[index].Result = result
				entries[index].HasResult = true
				upsertRunToolPart(record, entries[index].Action, "denied", &entries[index].Plan.Decision, &result)
			}
		}
		if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
			return nil, err
		}
		if err := s.saveRunSession(ctx, record, types.RunStatusRunning); err != nil {
			return nil, err
		}
		s.publishAssistantMessageUpdate(record)
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
	executed, updateErr := s.executeReadyTools(ctx, ready, func(result toolExecutionResult) error {
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

func (s *system) executeReadyTools(ctx context.Context, entries []toolRunEntry, onResult func(toolExecutionResult) error) ([]toolExecutionResult, error) {
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
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			toolCtx, cancel := context.WithTimeout(ctx, s.config.ToolTimeout)
			defer cancel()
			result, err := s.tools.Execute(toolCtx, entry.Plan)
			if err != nil {
				result = types.ToolResult{ID: newRuntimeID("tool-result"), ActionID: entry.Action.ID, ToolName: entry.Action.ToolName, Status: types.ToolStatusFailed, Error: err.Error(), CreatedAt: time.Now().UTC()}
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
		if entry.Plan.PlanStatus == types.ToolPlanStatusReady {
			ready = append(ready, entry)
		}
	}
	return ready
}

func orderedToolResults(entries []toolRunEntry, executed []toolExecutionResult) ([]types.ToolResult, error) {
	results := make([]types.ToolResult, 0, len(entries))
	var firstExecutionErr error
	for _, entry := range entries {
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
		if executedResult.Err != nil && firstExecutionErr == nil {
			firstExecutionErr = executedResult.Err
		}
	}
	if firstExecutionErr != nil {
		return results, runtimeToolFailed("failed to execute tool", firstExecutionErr)
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
