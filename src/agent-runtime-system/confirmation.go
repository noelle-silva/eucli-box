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
	_, err := s.updateRun(record.state.ID, types.RunStatusWaitingConfirmation, "waiting for tool confirmation")
	if err != nil {
		return types.ToolRunPlan{}, err
	}
	s.mu.Lock()
	record.pendingPlan = &plan
	record.confirmationCh = make(chan types.ToolConfirmation, 1)
	s.mu.Unlock()
	s.publish(record.state.ID, "tool_confirmation_required", plan)
	select {
	case confirmation := <-record.confirmationCh:
		confirmed, err := s.tools.ApplyConfirmation(ctx, plan, confirmation)
		s.mu.Lock()
		record.pendingPlan = nil
		record.confirmationCh = nil
		s.mu.Unlock()
		if err != nil {
			return types.ToolRunPlan{}, runtimeToolFailed("failed to apply tool confirmation", err)
		}
		_, err = s.updateRun(record.state.ID, types.RunStatusRunning, "")
		if err != nil {
			return types.ToolRunPlan{}, err
		}
		return confirmed, nil
	case <-ctx.Done():
		return types.ToolRunPlan{}, runtimeInvalid("run cancelled while waiting for confirmation", ctx.Err())
	}
}
