package agentruntime

import (
	"context"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) SubmitToolConfirmation(ctx context.Context, confirmation types.ToolConfirmation) error {
	if err := ctx.Err(); err != nil {
		return runtimeInvalid("submit confirmation cancelled", err)
	}
	decisionID := strings.TrimSpace(confirmation.DecisionID)
	if decisionID == "" {
		return runtimeInvalid("confirmation decision id is required", nil)
	}
	s.mu.Lock()
	var target *runRecord
	for _, record := range s.runs {
		if _, ok := record.pendingPlans[decisionID]; ok {
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
	if ch == nil {
		s.mu.Unlock()
		return runtimeStateInvalid("run has no confirmation channel", nil)
	}
	delete(target.pendingPlans, decisionID)
	s.mu.Unlock()
	ch <- confirmation
	return nil
}

func (s *system) waitForConfirmation(ctx context.Context, record *runRecord, plan types.ToolRunPlan) (types.ToolRunPlan, error) {
	confirmed, err := s.waitForConfirmations(ctx, record, []types.ToolRunPlan{plan})
	if err != nil {
		return types.ToolRunPlan{}, err
	}
	if len(confirmed) != 1 {
		return types.ToolRunPlan{}, runtimeStateInvalid("tool confirmation count mismatch", nil)
	}
	return confirmed[0], nil
}

func (s *system) waitForConfirmations(ctx context.Context, record *runRecord, plans []types.ToolRunPlan) ([]types.ToolRunPlan, error) {
	if len(plans) == 0 {
		return nil, nil
	}
	plansByDecisionID := make(map[string]types.ToolRunPlan, len(plans))
	for _, plan := range plans {
		decisionID := strings.TrimSpace(plan.Decision.ID)
		if decisionID == "" {
			return nil, runtimeStateInvalid("tool confirmation requires decision id", nil)
		}
		if _, exists := plansByDecisionID[decisionID]; exists {
			return nil, runtimeStateInvalid("duplicate tool confirmation decision id", nil)
		}
		plansByDecisionID[decisionID] = plan
	}
	confirmationCh := make(chan types.ToolConfirmation, len(plans))
	cleanup := func() {
		s.mu.Lock()
		record.pendingPlans = nil
		record.confirmationCh = nil
		s.mu.Unlock()
	}
	s.mu.Lock()
	record.pendingPlans = clonePendingPlans(plansByDecisionID)
	record.confirmationCh = confirmationCh
	s.mu.Unlock()
	_, err := s.updateRun(record.runID, types.RunStatusWaitingConfirmation, "waiting for tool confirmation")
	if err != nil {
		cleanup()
		return nil, err
	}
	for _, plan := range plans {
		s.publish(record.runID, "tool_confirmation_required", plan)
	}
	confirmedByDecisionID := make(map[string]types.ToolRunPlan, len(plans))
	for len(confirmedByDecisionID) < len(plans) {
		select {
		case confirmation := <-confirmationCh:
			decisionID := strings.TrimSpace(confirmation.DecisionID)
			plan, ok := plansByDecisionID[decisionID]
			if !ok {
				cleanup()
				return nil, runtimeNotFound("pending confirmation was not found", nil)
			}
			if _, exists := confirmedByDecisionID[decisionID]; exists {
				cleanup()
				return nil, runtimeStateInvalid("tool confirmation was already submitted", nil)
			}
			confirmed, err := s.tools.ApplyConfirmation(ctx, plan, confirmation)
			if err != nil {
				cleanup()
				return nil, runtimeToolFailed("failed to apply tool confirmation", err)
			}
			confirmedByDecisionID[decisionID] = confirmed
			if confirmed.Decision.Status == types.PermissionStatusAllowed {
				s.publish(record.runID, "tool_confirmation_applied", confirmed.Decision)
			} else {
				s.publish(record.runID, "tool_confirmation_rejected", confirmed.Decision)
			}
		case <-ctx.Done():
			cleanup()
			return nil, runtimeInvalid("run cancelled while waiting for confirmation", ctx.Err())
		}
	}
	cleanup()
	_, err = s.updateRun(record.runID, types.RunStatusRunning, "")
	if err != nil {
		return nil, err
	}
	confirmed := make([]types.ToolRunPlan, 0, len(plans))
	for _, plan := range plans {
		confirmed = append(confirmed, confirmedByDecisionID[plan.Decision.ID])
	}
	return confirmed, nil
}

func clonePendingPlans(plans map[string]types.ToolRunPlan) map[string]types.ToolRunPlan {
	cloned := make(map[string]types.ToolRunPlan, len(plans))
	for decisionID, plan := range plans {
		cloned[decisionID] = plan
	}
	return cloned
}
