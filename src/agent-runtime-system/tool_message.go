package agentruntime

import (
	"context"
	"strings"

	"eucli-box/pkg/types"
)

// DeliverToolMessage 是「发送会话消息」能力的实现：
// 它把请求交给日常发送消息的同一入口起跑，等这条运行自己停下，
// 再把角色最新一条消息原样交给工具。等待期间不设超时，
// 故障失联由工具控制的看门狗判定。
func (s *system) DeliverToolMessage(ctx context.Context, request types.ToolMessageRequest) (types.ToolMessageResult, error) {
	roleID := strings.TrimSpace(request.RoleID)
	workspaceID := strings.TrimSpace(request.WorkspaceID)
	message := strings.TrimSpace(request.Message)
	if roleID == "" {
		return types.ToolMessageResult{}, runtimeInvalid("role id is required", nil)
	}
	if message == "" {
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

	state, err := s.StartRun(ctx, types.RunRequest{
		RoleID:      roleID,
		WorkspaceID: workspaceID,
		SessionID:   strings.TrimSpace(request.SessionID),
		Message:     message,
	})
	if err != nil {
		return types.ToolMessageResult{}, err
	}
	terminal, stopped, err := s.waitToolMessageRun(ctx, state, events)
	if err != nil {
		return types.ToolMessageResult{}, err
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
			return types.ToolMessageResult{SessionID: session.ID, MessageID: item.ID, RoleID: terminal.RoleID, Message: item.Content}, nil
		}
	}
	return types.ToolMessageResult{}, runtimeStateInvalid("会话中没有找到本次运行的最后一条消息", nil)
}

// waitToolMessageRun 等待目标运行停下：调用方上下文取消即按「停止」处理，
// 走与客户端停止按钮完全相同的入口，然后继续等这次停止自己的收尾。
func (s *system) waitToolMessageRun(ctx context.Context, state types.RunState, events <-chan types.RunEvent) (types.RunState, bool, error) {
	terminal := state
	stopped := false
	stopChannel := ctx.Done()
	for isActiveRunStatus(terminal.Status) {
		select {
		case event, ok := <-events:
			if !ok {
				return terminal, stopped, runtimeStateInvalid("会话消息等待被中断", nil)
			}
			if event.RunID != terminal.ID {
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
	return terminal, stopped, nil
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
