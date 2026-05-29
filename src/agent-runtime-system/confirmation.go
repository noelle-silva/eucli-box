package agentruntime

import (
	"context"

	"eucli-box/pkg/types"
)

func (s *system) SubmitToolConfirmation(ctx context.Context, confirmation types.ToolConfirmation) error {
	if confirmation.DecisionID == "" {
		return runtimeInvalid("confirmation decision id is required", nil)
	}
	s.mu.Lock()
	var target *runRecord
	for _, record := range s.runs {
		if record.pendingPlan != nil && record.pendingPlan.Decision.ID == confirmation.DecisionID {
			target = record
			break
		}
	}
	if target == nil {
		s.mu.Unlock()
		return runtimeNotFound("pending confirmation was not found", nil)
	}
	state := target.state
	if state.Status != types.RunStatusWaitingConfirmation {
		s.mu.Unlock()
		return runtimeStateInvalid("run is not waiting for confirmation", nil)
	}
	ch := target.confirmationCh
	s.mu.Unlock()
	select {
	case ch <- confirmation:
		return nil
	case <-ctx.Done():
		return runtimeInvalid("submit confirmation cancelled", ctx.Err())
	}
}

func (s *system) waitForConfirmation(ctx context.Context, record *runRecord, plan types.ToolRunPlan) (types.ToolRunPlan, error) {
	_, err := s.updateRun(record.runID, types.RunStatusWaitingConfirmation, "waiting for tool confirmation")
	if err != nil {
		return types.ToolRunPlan{}, err
	}
	cleanup := func() {
		s.mu.Lock()
		record.pendingPlan = nil
		record.confirmationCh = nil
		s.mu.Unlock()
	}
	s.mu.Lock()
	record.pendingPlan = &plan
	record.confirmationCh = make(chan types.ToolConfirmation, 1)
	s.mu.Unlock()
	s.publish(record.runID, "tool_confirmation_required", plan)
	select {
	case confirmation := <-record.confirmationCh:
		confirmed, err := s.tools.ApplyConfirmation(ctx, plan, confirmation)
		cleanup()
		if err != nil {
			return types.ToolRunPlan{}, runtimeToolFailed("failed to apply tool confirmation", err)
		}
		_, err = s.updateRun(record.runID, types.RunStatusRunning, "")
		if err != nil {
			return types.ToolRunPlan{}, err
		}
		if confirmed.Decision.Status == types.PermissionStatusAllowed {
			s.publish(record.runID, "tool_confirmation_applied", confirmed.Decision)
		} else {
			s.publish(record.runID, "tool_confirmation_rejected", confirmed.Decision)
		}
		return confirmed, nil
	case <-ctx.Done():
		cleanup()
		return types.ToolRunPlan{}, runtimeInvalid("run cancelled while waiting for confirmation", ctx.Err())
	}
}
