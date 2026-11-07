package agentruntime

import (
	"strings"

	"eucli-box/pkg/types"
)

// 结论交接台账：用户点完之后，等它的转呈方要能把「同意还是拒绝」取走。
// 台账在决策落定那一刻注销，所以结论必须在注销之前登记，且只交给真正等它的那一次转呈。

// confirmationOutcomeLimit 是落定结论的保留上限：只为「刚点完、工具还没取走」
// 的那一小段时间服务，取走即删；超出上限按登记顺序淘汰最旧的。
const confirmationOutcomeLimit = 256

// recordConfirmationOutcome 在决策落定、注销台账之前登记结论：
// 顺序保证「台面上已经没有这个决策」时，结论必然已经可被取走。
// 编号与裁决都取自「被受理的那一次提交」：围栏确认应用后会重新裁决并换号，
// 用返回计划里的新号登记，会让等旧号的人永远等不到结论。
func (s *system) recordConfirmationOutcome(decisionID string, approved bool) {
	decisionID = strings.TrimSpace(decisionID)
	if decisionID == "" {
		return
	}
	status := types.ToolConfirmationStatusRejected
	if approved {
		status = types.ToolConfirmationStatusApproved
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.confirmationOutcomes == nil {
		s.confirmationOutcomes = map[string]types.ToolConfirmationOutcome{}
	}
	if _, exists := s.confirmationOutcomes[decisionID]; !exists && len(s.confirmationOutcomes) >= confirmationOutcomeLimit {
		s.evictOldestConfirmationOutcomeLocked()
	}
	s.confirmationOutcomes[decisionID] = types.ToolConfirmationOutcome{Status: status, DecisionID: decisionID}
}

// recordConfirmationFailure 登记「裁决被受理、但应用失败」的如实原因：
// 与「失效」分开，让转呈方能原样报出真实问题。
func (s *system) recordConfirmationFailure(decisionID string, reason string) {
	decisionID = strings.TrimSpace(decisionID)
	if decisionID == "" {
		return
	}
	if strings.TrimSpace(reason) == "" {
		reason = "应用裁决时出错"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.confirmationOutcomes == nil {
		s.confirmationOutcomes = map[string]types.ToolConfirmationOutcome{}
	}
	s.confirmationOutcomes[decisionID] = types.ToolConfirmationOutcome{Status: types.ToolConfirmationStatusFailed, DecisionID: decisionID, Reason: reason}
}

// takeConfirmationOutcome 取走一条结论：结论只交给真正等它的那一次转呈。
func (s *system) takeConfirmationOutcome(decisionID string) (types.ToolConfirmationOutcome, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	outcome, ok := s.confirmationOutcomes[decisionID]
	if ok {
		delete(s.confirmationOutcomes, decisionID)
	}
	return outcome, ok
}

// evictOldestConfirmationOutcomeLocked 按登记顺序淘汰最旧的一条，调用方须持有 s.mu。
// 这里只做内存卫生，淘汰的结论只可能是长期没人来取的那条。
func (s *system) evictOldestConfirmationOutcomeLocked() {
	for decisionID := range s.confirmationOutcomes {
		delete(s.confirmationOutcomes, decisionID)
		return
	}
}
