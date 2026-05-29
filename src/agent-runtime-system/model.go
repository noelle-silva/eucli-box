package agentruntime

import (
	"context"
	"fmt"

	"eucli-box/pkg/types"
)

func (s *system) callModel(ctx context.Context, roleContext types.RoleContext) (types.ModelResponse, error) {
	request := types.ModelRequest{Coordinate: roleContext.ModelConfig.Coordinate, Temperature: roleContext.ModelConfig.Temperature, Messages: modelMessages(roleContext), Tools: roleContext.Tools}
	response, err := s.providers.Complete(ctx, request)
	if err != nil {
		return types.ModelResponse{}, runtimeProviderFailed("failed to complete model request", err)
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
