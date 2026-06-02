package agentruntime

import (
	"context"
	"fmt"

	"eucli-box/pkg/types"
)

func (s *system) callModel(ctx context.Context, record *runRecord, roleContext types.RoleContext) (types.ModelResponse, error) {
	request := types.ModelRequest{Coordinate: roleContext.ModelConfig.Coordinate, Temperature: roleContext.ModelConfig.Temperature, Messages: modelMessages(roleContext), Tools: roleContext.Tools, Stream: record.stream}
	if record.stream {
		return s.callModelStream(ctx, record, request)
	}
	response, err := s.providers.Complete(ctx, request)
	if err != nil {
		return types.ModelResponse{}, runtimeProviderFailed("failed to complete model request", err)
	}
	return response, nil
}

func (s *system) callModelStream(ctx context.Context, record *runRecord, request types.ModelRequest) (types.ModelResponse, error) {
	ensureRunAssistantMessage(record)
	if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
		return types.ModelResponse{}, err
	}
	if err := s.saveSession(ctx, record.session, types.RunStatusRunning); err != nil {
		return types.ModelResponse{}, err
	}
	response, err := s.providers.CompleteStream(ctx, request, func(event types.ModelStreamEvent) error {
		if event.Type != types.ModelStreamEventContentDelta {
			return nil
		}
		updateRunAssistantContent(record, event.Content)
		if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
			return err
		}
		s.publish(record.runID, "model_stream_delta", types.RunStreamDelta{RunID: record.runID, RoleID: record.roleID, SessionID: record.session.ID, MessageID: record.messageParent.ID, ParentMessageID: record.messageParent.ParentMessageID, BranchID: record.messageParent.BranchID, ContentDelta: event.ContentDelta, Content: event.Content, CreatedAt: event.CreatedAt})
		return nil
	})
	if err != nil {
		return types.ModelResponse{}, runtimeProviderFailed("failed to stream model request", err)
	}
	return response, nil
}

func modelMessages(roleContext types.RoleContext) []types.PromptMessage {
	messages := make([]types.PromptMessage, 0, len(roleContext.Prompts)+len(roleContext.Messages))
	messages = append(messages, roleContext.Prompts...)
	for index, message := range roleContext.Messages {
		messages = append(messages, runtimeMessageToPrompt(message, index))
	}
	return messages
}

func runtimeMessageToPrompt(message types.Message, index int) types.PromptMessage {
	role := message.Type
	content := message.Content
	switch message.Type {
	case "user", "assistant":
	case "tool":
		role = "user"
		content = fmt.Sprintf("Tool %s returned: %s", message.ToolName, message.Content)
	case "failure":
		role = "user"
		content = "Runtime failure: " + message.Reason
	default:
		role = "user"
	}
	return types.PromptMessage{ID: message.ID, Role: role, Content: content, Order: index}
}
