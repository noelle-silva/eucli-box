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

// SubmitToolConfirmation 只把确认决定投进目标运行的唯一队列，并等待主脑回执。
// 它不修改待确认表、不直接落定状态：确认由该运行的主脑在安全点消费并收口。
func (s *system) SubmitToolConfirmation(ctx context.Context, confirmation types.ToolConfirmation) error {
	if err := ctx.Err(); err != nil {
		return runtimeInvalid("submit confirmation cancelled", err)
	}
	decisionID := strings.TrimSpace(confirmation.DecisionID)
	if decisionID == "" {
		return runtimeInvalid("confirmation decision id is required", nil)
	}
	done := make(chan error, 1)
	request := &toolConfirmationRequest{confirmation: confirmation, done: done}
	// 待确认表既是路由索引也是主脑的落定对象：投递与主脑的收口同受 s.mu 保护，
	// 使「入队」与「收口清表」串行化，确认不会落在无人消费的缝隙里。
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
	if !enqueueRunEvent(target, runEvent{kind: runEventToolConfirmation, confirmation: request}) {
		s.mu.Unlock()
		return runtimeStateInvalid("run confirmation queue is full", nil)
	}
	s.mu.Unlock()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return runtimeInvalid("submit confirmation cancelled", ctx.Err())
	}
}

func (s *system) waitForConfirmation(ctx context.Context, record *runRecord, contextSession *types.Session, plan types.ToolRunPlan) (types.ToolRunPlan, error) {
	confirmed, err := s.waitForConfirmations(ctx, record, contextSession, []types.ToolRunPlan{plan})
	if err != nil {
		return types.ToolRunPlan{}, err
	}
	if len(confirmed) != 1 {
		return types.ToolRunPlan{}, runtimeStateInvalid("tool confirmation count mismatch", nil)
	}
	return confirmed[0], nil
}

func (s *system) waitForConfirmations(ctx context.Context, record *runRecord, contextSession *types.Session, plans []types.ToolRunPlan) ([]types.ToolRunPlan, error) {
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
	cleanup := func(err error) {
		s.mu.Lock()
		record.pendingPlans = nil
		s.mu.Unlock()
		if err != nil {
			// 收口时把队列里尚未消费的确认意图一并回绝，避免投递方空等。
			s.drainRunInbox(record)
		}
	}
	s.mu.Lock()
	record.pendingPlans = clonePendingPlans(plansByDecisionID)
	s.mu.Unlock()
	_, err := s.updateRun(record.runID, types.RunStatusWaitingConfirmation, "waiting for tool confirmation")
	if err != nil {
		cleanup(err)
		return nil, err
	}
	s.publishAssistantMessageUpdate(record)
	for _, plan := range plans {
		s.publish(eventSourceFromRecord(record), "tool_confirmation_required", plan)
	}
	confirmedByDecisionID := make(map[string]types.ToolRunPlan, len(plans))
	for len(confirmedByDecisionID) < len(plans) {
		select {
		case event := <-record.inbox:
			switch event.kind {
			case runEventToolConfirmation:
				request := event.confirmation
				if request == nil {
					continue
				}
				confirmation := request.confirmation
				decisionID := strings.TrimSpace(confirmation.DecisionID)
				plan, ok := plansByDecisionID[decisionID]
				if !ok {
					err := runtimeNotFound("pending confirmation was not found", nil)
					finishToolConfirmationRequest(*request, err)
					cleanup(err)
					return nil, err
				}
				if _, exists := confirmedByDecisionID[decisionID]; exists {
					err := runtimeStateInvalid("tool confirmation was already submitted", nil)
					finishToolConfirmationRequest(*request, err)
					cleanup(err)
					return nil, err
				}
				confirmed, err := s.tools.ApplyConfirmation(ctx, plan, confirmation)
				if err != nil {
					err := runtimeToolFailed("failed to apply tool confirmation", err)
					finishToolConfirmationRequest(*request, err)
					cleanup(err)
					return nil, err
				}
				confirmedByDecisionID[decisionID] = confirmed
				if err := s.recordAppliedToolConfirmation(ctx, record, confirmed); err != nil {
					finishToolConfirmationRequest(*request, err)
					cleanup(err)
					return nil, err
				}
				finishToolConfirmationRequest(*request, nil)
				if confirmed.PlanStatus == types.ToolPlanStatusNeedsConfirmation {
					// The next waiting prompt is published after it is registered as pending.
				} else if confirmed.Decision.Status == types.PermissionStatusAllowed {
					s.publish(eventSourceFromRecord(record), "tool_confirmation_applied", confirmed.Decision)
				} else {
					s.publish(eventSourceFromRecord(record), "tool_confirmation_rejected", confirmed.Decision)
				}
			case runEventAsyncReady:
				// 异步结果就绪只是数据到达：就地回灌并落盘，不推进也不打断本次等待。
				if _, err := s.flushAsyncToolResultsDuringConfirmation(ctx, record, contextSession); err != nil {
					cleanup(err)
					return nil, err
				}
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
