package agentruntime

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

func (s *system) Subscribe(ctx context.Context) (<-chan types.RunEvent, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, runtimeInvalid("subscription context is cancelled", err)
	}
	ch := make(chan types.RunEvent, 128)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()
	unsubscribe := func() {
		s.removeSubscriber(ch)
	}
	go func() {
		<-ctx.Done()
		s.removeSubscriber(ch)
	}()
	return ch, unsubscribe, nil
}

func (s *system) publish(runID string, eventType string, payload any) {
	event := types.RunEvent{ID: utils.NewID("event"), RunID: runID, Type: eventType, Payload: payload, CreatedAt: time.Now().UTC()}
	s.mu.Lock()
	defer s.mu.Unlock()
	if record, ok := s.runs[runID]; ok && record != nil {
		event.GroupID = record.groupID
		event.WorkspaceID = record.workspaceID
	}
	for ch := range s.subscribers {
		select {
		case ch <- event:
		default:
			delete(s.subscribers, ch)
			close(ch)
		}
	}
}

func (s *system) publishAssistantMessageUpdate(record *runRecord) {
	message, ok := currentRunAssistantMessage(record)
	if !ok {
		return
	}
	s.publishRunMessageUpdate(record, "assistant_message_update", message)
}

func (s *system) publishRunMessageUpdate(record *runRecord, eventType string, message types.Message) {
	state, _ := s.getRunState(record.runID)
	now := time.Now().UTC()
	s.publish(record.runID, eventType, types.RunAssistantMessageUpdate{RunID: record.runID, RoleID: record.roleID, GroupID: record.groupID, WorkspaceID: record.workspaceID, SessionID: record.session.ID, Stream: record.stream, Status: state.Status, Reason: state.Reason, Retry: cloneRunRetryInfo(state.Retry), Error: cloneErrorPayload(state.Error), Message: cloneRunMessageSnapshot(message), CreatedAt: now})
}

func currentRunAssistantMessage(record *runRecord) (types.Message, bool) {
	messageID := strings.TrimSpace(record.activeAssistantID)
	if messageID == "" && record.messageParent.Type == "assistant" {
		messageID = strings.TrimSpace(record.messageParent.ID)
	}
	if messageID == "" {
		return types.Message{}, false
	}
	message, ok := messageByID(record.session.Messages, messageID)
	if !ok || message.Type != "assistant" {
		return types.Message{}, false
	}
	return message, true
}

func cloneRunMessageSnapshot(message types.Message) types.Message {
	if message.Control != nil {
		control := *message.Control
		message.Control = &control
	}
	message.Error = cloneErrorPayload(message.Error)
	message.Parts = cloneRunMessageParts(message.Parts)
	if len(message.Attachments) > 0 {
		message.Attachments = append([]types.MessageAttachment(nil), message.Attachments...)
	}
	message.TokenEstimate = types.EstimateMessageTokenCount(message)
	return message
}

func cloneRunMessageParts(parts []types.MessagePart) []types.MessagePart {
	if len(parts) == 0 {
		return nil
	}
	out := make([]types.MessagePart, len(parts))
	for index, part := range parts {
		out[index] = part
		out[index].Input = cloneMapAny(part.Input)
		out[index].Display = cloneMapAny(part.Display)
		if part.Decision != nil {
			decision := *part.Decision
			out[index].Decision = &decision
		}
		if part.Result != nil {
			result := *part.Result
			result.Metadata = cloneMapAny(part.Result.Metadata)
			out[index].Result = &result
		}
	}
	return out
}

func cloneMapAny(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneAny(value)
	}
	return out
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMapAny(typed)
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = cloneAny(item)
		}
		return out
	default:
		return value
	}
}
