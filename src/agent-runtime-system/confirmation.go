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

// SubmitToolConfirmation 只按决策台账把确认意图投进目标运行的唯一队列，并等待主脑回执。
// 它不判运行是否处于等待确认状态：决策一旦登记进台账即对外可见、即可路由；
// 台账中已无该决策（已落定或运行终态清账）才明确回绝，绝不让投递方空等。
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
	// 路由与入队同受 s.mu 保护，与主脑的「消费并注销」串行化：确认不会落在无人消费的缝隙里。
	s.mu.Lock()
	target := s.confirmationRunLocked(decisionID)
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

// registerToolConfirmation 把一个待确认决策登记进运行台账。调用点必须在任何使该决策
// 对外可见的动作（发布事件、写工具部件、落盘）之前，保证「可见即可路由」。
func (s *system) registerToolConfirmation(record *runRecord, plan types.ToolRunPlan) {
	if record == nil || plan.PlanStatus != types.ToolPlanStatusNeedsConfirmation {
		return
	}
	decisionID := strings.TrimSpace(plan.Decision.ID)
	if decisionID == "" {
		return
	}
	s.mu.Lock()
	if record.confirmationLedger == nil {
		record.confirmationLedger = map[string]types.ToolRunPlan{}
	}
	record.confirmationLedger[decisionID] = plan
	s.mu.Unlock()
}

// deregisterToolConfirmation 在决策被应用/拒绝后把它从台账注销。
func (s *system) deregisterToolConfirmation(record *runRecord, decisionID string) {
	decisionID = strings.TrimSpace(decisionID)
	if record == nil || decisionID == "" {
		return
	}
	s.mu.Lock()
	delete(record.confirmationLedger, decisionID)
	s.mu.Unlock()
}

// discardToolConfirmations 在运行放弃本次派生状态（如重解析起点重试）时清空台账。
func (s *system) discardToolConfirmations(record *runRecord) {
	if record == nil {
		return
	}
	s.mu.Lock()
	record.confirmationLedger = nil
	s.mu.Unlock()
}

// toolConfirmationPending 按台账判定该决策是否仍待确认，是主脑受理确认意图的唯一判据。
func (s *system) toolConfirmationPending(record *runRecord, decisionID string) bool {
	decisionID = strings.TrimSpace(decisionID)
	if record == nil || decisionID == "" {
		return false
	}
	s.mu.Lock()
	_, ok := record.confirmationLedger[decisionID]
	s.mu.Unlock()
	return ok
}

// confirmationRunLocked 按决策台账定位持有该决策的运行；调用方须持有 s.mu。
func (s *system) confirmationRunLocked(decisionID string) *runRecord {
	for _, record := range s.runs {
		if _, ok := record.confirmationLedger[decisionID]; ok {
			return record
		}
	}
	return nil
}

// clearConfirmationsLocked 在运行终态统一清账：注销台账中全部决策，并回绝队列里尚未落定的
// 提交，让投递方拿到明确错误而非空等。调用方须持有 s.mu，以保证与提交侧「按台账路由并入队」
// 的串行化，杜绝「已清账却仍被路由进队、进而无人应答」的缝隙。
func (s *system) clearConfirmationsLocked(record *runRecord) {
	if record == nil {
		return
	}
	record.confirmationLedger = nil
	if record.inbox == nil {
		return
	}
	for {
		select {
		case event := <-record.inbox:
			if event.kind == runEventToolConfirmation && event.confirmation != nil {
				finishToolConfirmationRequest(*event.confirmation, runtimeStateInvalid("run is no longer waiting for confirmation", nil))
			}
		default:
			return
		}
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

// waitForConfirmations 是主脑消费确认意图的安全点：它在安全点读取运行队列里的确认意图，
// 以「台账中仍存在该决策」为唯一受理判据（而非运行是否正在等待），落定后注销该决策。
// 台账已在决策对外可见前登记，因此更早提交的确认也会被受理。
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
		if err != nil {
			// 收口时把队列里尚未消费的确认意图一并回绝，避免投递方空等；台账随运行终态统一清账。
			s.drainRunInbox(record)
		}
	}
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
				// 受理判据是台账中仍存在该决策：已落定/已清账的决策在此被明确回绝。
				if !s.toolConfirmationPending(record, decisionID) {
					err := runtimeNotFound("pending confirmation was not found", nil)
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
				// 决策已落定：先从台账注销，再由 recordAppliedToolConfirmation 登记可能产生的下一层待确认决策。
				s.deregisterToolConfirmation(record, decisionID)
				confirmedByDecisionID[decisionID] = confirmed
				if err := s.recordAppliedToolConfirmation(ctx, record, confirmed); err != nil {
					finishToolConfirmationRequest(*request, err)
					cleanup(err)
					return nil, err
				}
				finishToolConfirmationRequest(*request, nil)
				if confirmed.PlanStatus == types.ToolPlanStatusNeedsConfirmation {
					// 下一层等待提示在登记进台账后才发布。
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
	// 若本次确认产生了下一层待确认决策，先登记进台账，再让它对外可见。
	s.registerToolConfirmation(record, plan)
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
