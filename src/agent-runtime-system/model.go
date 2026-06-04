package agentruntime

import (
	"context"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

func (s *system) callModel(ctx context.Context, record *runRecord, roleContext types.RoleContext) (types.ModelResponse, error) {
	messages, err := s.modelMessages(ctx, roleContext)
	if err != nil {
		return types.ModelResponse{}, err
	}
	request := types.ModelRequest{Coordinate: roleContext.ModelConfig.Coordinate, Temperature: roleContext.ModelConfig.Temperature, Messages: messages, Tools: roleContext.NativeTools, Stream: record.stream}
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
	record.streamContent = ""
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
		content := event.Content
		if content == record.streamContent {
			return nil
		}
		contentDelta := streamContentDelta(record.streamContent, content)
		record.streamContent = content
		updateRunAssistantContent(record, content)
		if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
			return err
		}
		s.publish(record.runID, "model_stream_delta", types.RunStreamDelta{RunID: record.runID, RoleID: record.roleID, SessionID: record.session.ID, MessageID: record.messageParent.ID, ParentMessageID: record.messageParent.ParentMessageID, BranchID: record.messageParent.BranchID, ContentDelta: contentDelta, Content: content, CreatedAt: event.CreatedAt})
		return nil
	})
	if err != nil {
		return types.ModelResponse{}, runtimeProviderFailed("failed to stream model request", err)
	}
	return response, nil
}

func streamContentDelta(previous string, current string) string {
	if previous == "" {
		return current
	}
	if strings.HasPrefix(current, previous) {
		return strings.TrimPrefix(current, previous)
	}
	return current
}

func (s *system) modelMessages(ctx context.Context, roleContext types.RoleContext) ([]types.PromptMessage, error) {
	messages := make([]types.PromptMessage, 0, len(roleContext.Prompts)+len(roleContext.Messages))
	messages = append(messages, roleContext.Prompts...)
	for index, message := range roleContext.Messages {
		prompt, err := s.runtimeMessageToPrompt(ctx, message, index)
		if err != nil {
			return nil, err
		}
		messages = append(messages, prompt)
	}
	return messages, nil
}

func (s *system) runtimeMessageToPrompt(ctx context.Context, message types.Message, index int) (types.PromptMessage, error) {
	role := message.Type
	content, err := s.messagePromptContent(ctx, message)
	if err != nil {
		return types.PromptMessage{}, err
	}
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
	images, err := s.promptImagesForMessage(ctx, message)
	if err != nil {
		return types.PromptMessage{}, err
	}
	return types.PromptMessage{ID: message.ID, Role: role, Content: content, Parts: cloneMessageParts(message.Parts), Images: images, Order: index, CreatedAt: message.CreatedAt, UpdatedAt: message.UpdatedAt}, nil
}

func cloneMessageParts(parts []types.MessagePart) []types.MessagePart {
	if len(parts) == 0 {
		return nil
	}
	result := make([]types.MessagePart, len(parts))
	copy(result, parts)
	return result
}

func (s *system) messagePromptContent(ctx context.Context, message types.Message) (string, error) {
	content := message.Content
	if message.Type == "assistant" {
		visibleContent, err := s.tools.VisibleTextToolContent(ctx, content)
		if err != nil {
			return "", runtimeToolFailed("failed to filter visible assistant tool content", err)
		}
		content = visibleContent
	}
	blocks := []string{}
	for _, attachment := range message.Attachments {
		if attachment.Kind == "image" || strings.TrimSpace(attachment.Text) == "" {
			continue
		}
		name := strings.TrimSpace(attachment.Name)
		if name == "" {
			name = "文件"
		}
		lang := strings.TrimSpace(attachment.Lang)
		if lang == "" {
			lang = "text"
		}
		fullLen := attachment.FullLen
		if fullLen <= 0 {
			fullLen = len([]rune(attachment.Text))
		}
		sendLen := attachment.SendLen
		if sendLen <= 0 {
			sendLen = len([]rune(attachment.Text))
		}
		sendPct := attachment.SendPct
		if sendPct <= 0 {
			sendPct = 100
		}
		blocks = append(blocks, fmt.Sprintf("附件：%s（发送 %d%%：%d/%d 字符）\n```%s\n%s\n```", name, sendPct, sendLen, fullLen, lang, escapePromptFence(attachment.Text)))
	}
	if len(blocks) == 0 {
		return content, nil
	}
	extra := strings.Join(blocks, "\n\n")
	if strings.TrimSpace(content) == "" {
		return extra, nil
	}
	return strings.TrimSpace(content) + "\n\n" + extra, nil
}

func (s *system) promptImagesForMessage(ctx context.Context, message types.Message) ([]types.PromptImage, error) {
	images := []types.PromptImage{}
	for _, attachment := range message.Attachments {
		if attachment.Kind != "image" || strings.TrimSpace(attachment.Path) == "" {
			continue
		}
		dataURL, err := s.storage.LoadSessionAttachmentImage(ctx, attachment.Path)
		if err != nil {
			return nil, runtimeStorageFailed("failed to load message image attachment", err)
		}
		images = append(images, types.PromptImage{DataURL: dataURL})
	}
	return images, nil
}

func escapePromptFence(value string) string {
	return strings.ReplaceAll(value, "```", "``\u200b`")
}
