package agentruntime

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

// toolConfirmationPollInterval 是转呈确认的观察节奏：只查两张内存表，
// 不设业务超时——等的是人，不是机器。
const toolConfirmationPollInterval = 100 * time.Millisecond

// RelayToolConfirmation 是「工具把别处待确认的事摆到台面上」的唯一入口：
// 把该决策投影到发起这次工具调用的运行上，等用户裁决后把结论交回工具。
func (s *system) RelayToolConfirmation(ctx context.Context, plan types.ToolRunPlan, fact types.ToolConfirmationFact) (types.ToolConfirmationOutcome, error) {
	runID := strings.TrimSpace(plan.Scope.RunID)
	decisionID := strings.TrimSpace(fact.DecisionID)
	if runID == "" {
		return types.ToolConfirmationOutcome{}, runtimeInvalid("tool confirmation requires the calling run", nil)
	}
	if decisionID == "" {
		return types.ToolConfirmationOutcome{}, runtimeInvalid("tool confirmation requires a decision id", nil)
	}
	// 已经落定的决策不必再摆：把结论直接交回。
	if outcome, ok := s.takeConfirmationOutcome(decisionID); ok {
		return outcome, nil
	}
	if !s.confirmationDecisionPending(decisionID) {
		return types.ToolConfirmationOutcome{Status: types.ToolConfirmationStatusExpired, DecisionID: decisionID}, nil
	}
	if err := s.projectToolConfirmation(ctx, runID, plan.Action, fact); err != nil {
		return types.ToolConfirmationOutcome{}, err
	}
	outcome, err := s.awaitToolConfirmation(ctx, decisionID)
	s.clearToolConfirmationProjection(ctx, runID, plan.Action)
	if err != nil {
		return types.ToolConfirmationOutcome{}, err
	}
	return outcome, nil
}

// projectToolConfirmation 把外部待确认摆到调用方那次工具调用的位置上：
// 部件状态改成待确认、决策指向别处的编号、展示内容换成被拦下的那件事。
func (s *system) projectToolConfirmation(ctx context.Context, runID string, action types.ToolAction, fact types.ToolConfirmationFact) error {
	record := s.runRecordByID(runID)
	if record == nil {
		return runtimeNotFound("calling run was not found", nil)
	}
	decision := types.PermissionDecision{
		ID:       strings.TrimSpace(fact.DecisionID),
		ToolName: strings.TrimSpace(fact.ToolName),
		Status:   types.PermissionStatusNeedsConfirmation,
		Reason:   toolConfirmationReason(fact),
	}
	projected := action
	projected.ToolName = toolConfirmationLabel(action.ToolName, fact.ToolName)
	projected.Arguments = fact.Arguments
	record.sessionMu.Lock()
	upsertRunToolPart(record, projected, "needs_confirmation", &decision, nil)
	s.publishAssistantMessageUpdate(record)
	err := s.saveRunSession(ctx, record, record.state.Status)
	record.sessionMu.Unlock()
	return err
}

// clearToolConfirmationProjection 在裁决落定后把部件从待确认状态收回，
// 让调用方继续显示为运行中，直到它自己交出结果。
func (s *system) clearToolConfirmationProjection(ctx context.Context, runID string, action types.ToolAction) {
	record := s.runRecordByID(runID)
	if record == nil {
		return
	}
	record.sessionMu.Lock()
	upsertRunToolPart(record, action, "running", nil, nil)
	s.publishAssistantMessageUpdate(record)
	_ = s.saveRunSession(ctx, record, record.state.Status)
	record.sessionMu.Unlock()
}

// awaitToolConfirmation 等用户裁决：结论一到就取走；决策从台面上消失则视为失效。
func (s *system) awaitToolConfirmation(ctx context.Context, decisionID string) (types.ToolConfirmationOutcome, error) {
	ticker := time.NewTicker(toolConfirmationPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return types.ToolConfirmationOutcome{}, runtimeInvalid("tool confirmation relay cancelled", ctx.Err())
		case <-ticker.C:
			if outcome, ok := s.takeConfirmationOutcome(decisionID); ok {
				return outcome, nil
			}
			if !s.confirmationDecisionPending(decisionID) {
				return types.ToolConfirmationOutcome{Status: types.ToolConfirmationStatusExpired, DecisionID: decisionID}, nil
			}
		}
	}
}

// confirmationDecisionPending 判定别处的决策是否仍在台面上；调用方须持有 s.mu。
func (s *system) confirmationDecisionPending(decisionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.confirmationRunLocked(decisionID) != nil
}

// runRecordByID 按运行标识取回运行记录。
func (s *system) runRecordByID(runID string) *runRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[strings.TrimSpace(runID)]
}

// toolConfirmationLabel 给待确认部件一个看得懂的名字：调用方工具与真正被拦下的工具。
func toolConfirmationLabel(callerTool string, blockedTool string) string {
	caller := strings.TrimSpace(callerTool)
	blocked := strings.TrimSpace(blockedTool)
	switch {
	case caller == "":
		return blocked
	case blocked == "" || blocked == caller:
		return caller
	default:
		return caller + " › " + blocked
	}
}

// toolConfirmationReason 把被拦下的事写成一句话：卡片正文要能读懂。
func toolConfirmationReason(fact types.ToolConfirmationFact) string {
	reason := strings.TrimSpace(fact.Reason)
	toolName := strings.TrimSpace(fact.ToolName)
	if toolName == "" {
		return reason
	}
	if reason == "" {
		return toolName + " 需要确认后才能继续"
	}
	return toolName + "：" + reason
}
