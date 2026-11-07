package agentruntime

import (
	"context"
	"strings"

	"eucli-box/pkg/types"
)

// toolConfirmationRequiredEventType 是运行抛出待确认时的事件名：
// 发布方与消费方共用这一个常量，避免两处字面量漂移。
const toolConfirmationRequiredEventType = "tool_confirmation_required"

// DeliverToolMessage 是「发送会话消息」能力的实现：
// 它把请求交给日常发送消息的同一入口起跑，等这条运行自己停下，
// 再把角色最新一条消息原样交给工具。等待期间不设超时，
// 故障失联由工具控制的看门狗判定。
// 带上续等标识时不再发新消息，只接着等那条运行——转呈确认之后用它续等。
func (s *system) DeliverToolMessage(ctx context.Context, request types.ToolMessageRequest) (types.ToolMessageResult, error) {
	roleID := strings.TrimSpace(request.RoleID)
	workspaceID := strings.TrimSpace(request.WorkspaceID)
	message := strings.TrimSpace(request.Message)
	resumeRunID := strings.TrimSpace(request.ResumeRunID)
	if roleID == "" {
		return types.ToolMessageResult{}, runtimeInvalid("role id is required", nil)
	}
	if message == "" && resumeRunID == "" {
		return types.ToolMessageResult{}, runtimeInvalid("message is required", nil)
	}
	// 订阅先于起跑：运行可能在起跑函数返回前就已落定终态，
	// 先订阅才不会漏掉那一条终态事件。
	subscriptionCtx, stopSubscription := context.WithCancel(context.Background())
	defer stopSubscription()
	events, unsubscribe, err := s.Subscribe(subscriptionCtx)
	if err != nil {
		return types.ToolMessageResult{}, err
	}
	defer unsubscribe()

	state, err := s.toolMessageRunToWatch(ctx, request, roleID, workspaceID, message, resumeRunID)
	if err != nil {
		return types.ToolMessageResult{}, err
	}
	terminal, stopped, confirmation, err := s.waitToolMessageRun(ctx, state, events)
	if err != nil {
		return types.ToolMessageResult{}, err
	}
	if confirmation != nil {
		return types.ToolMessageResult{
			Status:       types.ToolMessageStatusConfirmationRequired,
			RunID:        terminal.ID,
			SessionID:    terminal.SessionID,
			RoleID:       roleID,
			Confirmation: confirmation,
		}, nil
	}
	if stopped || terminal.Status == types.RunStatusCancelled {
		return types.ToolMessageResult{}, runtimeStateInvalid("会话消息已取消", nil)
	}
	if terminal.Status != types.RunStatusCompleted {
		return types.ToolMessageResult{}, runtimeStateInvalid(toolMessageFailureReason(terminal), nil)
	}
	messageID := s.runFinalMessageID(terminal.ID)
	if messageID == "" {
		return types.ToolMessageResult{}, runtimeStateInvalid("运行没有产出可回传的消息", nil)
	}
	session, err := s.loadToolMessageSession(ctx, workspaceID, roleID, terminal.SessionID)
	if err != nil {
		return types.ToolMessageResult{}, err
	}
	for _, item := range session.Messages {
		if item.ID == messageID {
			return types.ToolMessageResult{
				Status:    types.ToolMessageStatusReplied,
				RunID:     terminal.ID,
				SessionID: session.ID,
				MessageID: item.ID,
				RoleID:    terminal.RoleID,
				Message:   item.Content,
			}, nil
		}
	}
	return types.ToolMessageResult{}, runtimeStateInvalid("会话中没有找到本次运行的最后一条消息", nil)
}

// toolMessageRunToWatch 决定这次要观察哪条运行：新请求就照日常入口起跑，
// 续等请求则只认既有运行，不发新消息、也不另起会话。
func (s *system) toolMessageRunToWatch(ctx context.Context, request types.ToolMessageRequest, roleID string, workspaceID string, message string, resumeRunID string) (types.RunState, error) {
	if resumeRunID == "" {
		return s.StartRun(ctx, types.RunRequest{
			RoleID:      roleID,
			WorkspaceID: workspaceID,
			SessionID:   strings.TrimSpace(request.SessionID),
			Message:     message,
		})
	}
	state, ok := s.getRunState(resumeRunID)
	if !ok {
		return types.RunState{}, runtimeNotFound("run to resume was not found", nil)
	}
	if !strings.EqualFold(strings.TrimSpace(state.RoleID), roleID) {
		return types.RunState{}, runtimeInvalid("run to resume belongs to another role", nil)
	}
	return state, nil
}

// waitToolMessageRun 等待目标运行停下：调用方上下文取消即按「停止」处理，
// 走与客户端停止按钮完全相同的入口，然后继续等这次停止自己的收尾。
// 途中若目标运行抛出待确认事件，就把该决策事实交出去，由上层转成结果返回。
func (s *system) waitToolMessageRun(ctx context.Context, state types.RunState, events <-chan types.RunEvent) (types.RunState, bool, *types.ToolConfirmationFact, error) {
	terminal := state
	stopped := false
	stopChannel := ctx.Done()
	for isActiveRunStatus(terminal.Status) {
		select {
		case event, ok := <-events:
			if !ok {
				return terminal, stopped, nil, runtimeStateInvalid("会话消息等待被中断", nil)
			}
			if event.RunID != terminal.ID {
				continue
			}
			if event.Type == toolConfirmationRequiredEventType {
				if plan, ok := event.Payload.(types.ToolRunPlan); ok {
					if confirmation, ok := toolMessageConfirmationFromPlan(plan, terminal.SessionID, terminal.RoleID); ok {
						return terminal, stopped, confirmation, nil
					}
				}
				continue
			}
			next, ok := event.Payload.(types.RunState)
			if !ok || isActiveRunStatus(next.Status) {
				continue
			}
			terminal = next
		case <-stopChannel:
			stopChannel = nil
			stopped = true
			_ = s.CancelRun(context.Background(), terminal.ID)
		}
	}
	return terminal, stopped, nil, nil
}

// toolMessageConfirmationFromPlan 把目标运行的待确认计划压成对外事实：
// 只带出决策编号、工具名、原因与入参，以及等确认的会话身份。
func toolMessageConfirmationFromPlan(plan types.ToolRunPlan, sessionID string, roleID string) (*types.ToolConfirmationFact, bool) {
	decisionID := strings.TrimSpace(plan.Decision.ID)
	if decisionID == "" {
		return nil, false
	}
	return &types.ToolConfirmationFact{
		DecisionID: decisionID,
		ToolName:   strings.TrimSpace(plan.Tool.Name),
		Reason:     strings.TrimSpace(plan.Decision.Reason),
		Arguments:  plan.Action.Arguments,
		SessionID:  strings.TrimSpace(sessionID),
		RoleID:     strings.TrimSpace(roleID),
	}, true
}

// runFinalMessageID 读取运行内存记录里最后一条消息的标识：
// 运行状态快照不逐次同步该字段，终态事件到达时以运行记录为准。
func (s *system) runFinalMessageID(runID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.runs[runID]
	if !ok {
		return ""
	}
	return record.lastMessageID
}

// loadToolMessageSession 按本次运行的会话身份读回会话：工作区会话带角色维度。
func (s *system) loadToolMessageSession(ctx context.Context, workspaceID string, roleID string, sessionID string) (types.Session, error) {
	if sessionID == "" {
		return types.Session{}, runtimeStateInvalid("运行没有会话身份", nil)
	}
	if workspaceID != "" {
		session, err := s.storage.LoadWorkspaceSession(ctx, workspaceID, roleID, sessionID)
		if err != nil {
			return types.Session{}, runtimeStorageFailed("failed to load workspace session", err)
		}
		return session, nil
	}
	session, err := s.storage.LoadSession(ctx, roleID, sessionID)
	if err != nil {
		return types.Session{}, runtimeStorageFailed("failed to load session", err)
	}
	return session, nil
}

// toolMessageFailureReason 把运行失败的原因原样取出：优先最具体的一条错误消息，
// 没有错误载荷时才回落到运行原因。
func toolMessageFailureReason(state types.RunState) string {
	if message := mostSpecificErrorMessage(state.Error); message != "" {
		return message
	}
	if reason := strings.TrimSpace(state.Reason); reason != "" {
		return reason
	}
	return "会话消息运行失败"
}
