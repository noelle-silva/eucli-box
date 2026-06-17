package agentruntime

import (
	"context"
	"strings"

	"eucli-box/pkg/types"
)

type toolConfirmationRequest struct {
	confirmation types.ToolConfirmation
	done         chan error
}

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
	done := make(chan error, 1)
	request := toolConfirmationRequest{confirmation: confirmation, done: done}
	select {
	case ch <- request:
	case <-ctx.Done():
		return runtimeInvalid("submit confirmation cancelled", ctx.Err())
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return runtimeInvalid("submit confirmation cancelled", ctx.Err())
	}
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
	confirmationCh := make(chan toolConfirmationRequest, len(plans))
	cleanup := func(err error) {
		s.mu.Lock()
		record.pendingPlans = nil
		record.confirmationCh = nil
		s.mu.Unlock()
		drainToolConfirmationRequests(confirmationCh, err)
	}
	s.mu.Lock()
	record.pendingPlans = clonePendingPlans(plansByDecisionID)
	record.confirmationCh = confirmationCh
	s.mu.Unlock()
	_, err := s.updateRun(record.runID, types.RunStatusWaitingConfirmation, "waiting for tool confirmation")
	if err != nil {
		cleanup(err)
		return nil, err
	}
	s.publishAssistantMessageUpdate(record)
	for _, plan := range plans {
		s.publish(record.runID, "tool_confirmation_required", plan)
	}
	confirmedByDecisionID := make(map[string]types.ToolRunPlan, len(plans))
	for len(confirmedByDecisionID) < len(plans) {
		select {
		case request := <-confirmationCh:
			confirmation := request.confirmation
			decisionID := strings.TrimSpace(confirmation.DecisionID)
			plan, ok := plansByDecisionID[decisionID]
			if !ok {
				err := runtimeNotFound("pending confirmation was not found", nil)
				finishToolConfirmationRequest(request, err)
				cleanup(err)
				return nil, err
			}
			if _, exists := confirmedByDecisionID[decisionID]; exists {
				err := runtimeStateInvalid("tool confirmation was already submitted", nil)
				finishToolConfirmationRequest(request, err)
				cleanup(err)
				return nil, err
			}
			confirmed, err := s.tools.ApplyConfirmation(ctx, plan, confirmation)
			if err != nil {
				err := runtimeToolFailed("failed to apply tool confirmation", err)
				finishToolConfirmationRequest(request, err)
				cleanup(err)
				return nil, err
			}
			confirmedByDecisionID[decisionID] = confirmed
			if err := s.recordAppliedToolConfirmation(ctx, record, confirmed); err != nil {
				finishToolConfirmationRequest(request, err)
				cleanup(err)
				return nil, err
			}
			finishToolConfirmationRequest(request, nil)
			if confirmed.PlanStatus == types.ToolPlanStatusNeedsConfirmation {
				// The next waiting prompt is published after it is registered as pending.
			} else if confirmed.Decision.Status == types.PermissionStatusAllowed {
				s.publish(record.runID, "tool_confirmation_applied", confirmed.Decision)
			} else {
				s.publish(record.runID, "tool_confirmation_rejected", confirmed.Decision)
			}
		case <-ctx.Done():
			err := runtimeInvalid("run cancelled while waiting for confirmation", ctx.Err())
			cleanup(err)
			return nil, err
		}
	}
	cleanup(nil)
	if !confirmedPlansNeedMoreConfirmation(confirmedByDecisionID) {
		_, err = s.updateRun(record.runID, types.RunStatusRunning, "")
		if err != nil {
			return nil, err
		}
		s.publishAssistantMessageUpdate(record)
	}
	confirmed := make([]types.ToolRunPlan, 0, len(plans))
	for _, plan := range plans {
		confirmed = append(confirmed, confirmedByDecisionID[plan.Decision.ID])
	}
	return confirmed, nil
}

func confirmedPlansNeedMoreConfirmation(plans map[string]types.ToolRunPlan) bool {
	for _, plan := range plans {
		if plan.PlanStatus == types.ToolPlanStatusNeedsConfirmation {
			return true
		}
	}
	return false
}

func finishToolConfirmationRequest(request toolConfirmationRequest, err error) {
	if request.done == nil {
		return
	}
	request.done <- err
}

func drainToolConfirmationRequests(ch <-chan toolConfirmationRequest, err error) {
	for {
		select {
		case request := <-ch:
			finishToolConfirmationRequest(request, err)
		default:
			return
		}
	}
}

func (s *system) recordAppliedToolConfirmation(ctx context.Context, record *runRecord, plan types.ToolRunPlan) error {
	state := "rejected"
	if plan.PlanStatus == types.ToolPlanStatusNeedsConfirmation {
		state = "needs_confirmation"
	} else if plan.Decision.Status == types.PermissionStatusAllowed {
		state = "approved"
	}
	upsertRunToolPart(record, plan.Action, state, &plan.Decision, nil)
	if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
		return err
	}
	if err := s.saveRunSession(ctx, record, types.RunStatusWaitingConfirmation); err != nil {
		return err
	}
	s.publishAssistantMessageUpdate(record)
	return nil
}

func clonePendingPlans(plans map[string]types.ToolRunPlan) map[string]types.ToolRunPlan {
	cloned := make(map[string]types.ToolRunPlan, len(plans))
	for decisionID, plan := range plans {
		cloned[decisionID] = plan
	}
	return cloned
}
