package agentruntime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) callModel(ctx context.Context, record *runRecord, roleContext types.RoleContext) (types.ModelResponse, error) {
	messages, err := s.modelMessages(ctx, roleContext)
	if err != nil {
		return types.ModelResponse{}, err
	}
	coordinate := roleContext.ModelConfig.Coordinate
	if override, ok := types.NormalizeModelOverrideCoordinate(record.modelOverride); ok {
		coordinate = override
	}
	request := types.ModelRequest{Coordinate: coordinate, Temperature: roleContext.ModelConfig.Temperature, Messages: messages, ReasoningEffort: record.reasoningEffort, Tools: roleContext.NativeTools, Stream: record.stream}
	return s.callModelWithRetry(ctx, record, request)
}

func (s *system) callModelWithRetry(ctx context.Context, record *runRecord, request types.ModelRequest) (types.ModelResponse, error) {
	maxAttempts := modelRetryLimit(record.stream)
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return types.ModelResponse{}, err
		}
		response, err := s.callModelOnce(ctx, record, request)
		if err == nil {
			_, _ = s.setRunRetry(record.runID, nil)
			return response, nil
		}
		nextAttempt := attempt + 1
		failure := errorPayloadFromError(err, "")
		if nextAttempt > maxAttempts {
			_, _ = s.setRunRetry(record.runID, nil)
			return types.ModelResponse{}, err
		}
		decision := modelRetryDecisionForError(err, nextAttempt)
		if !decision.Retryable {
			_, _ = s.setRunRetry(record.runID, nil)
			return types.ModelResponse{}, err
		}
		retry := newRunRetryInfo(nextAttempt, maxAttempts, decision.Delay, retryMessage(nextAttempt, maxAttempts, decision.Message), failure)
		if state, setErr := s.setRunRetry(record.runID, retry); setErr == nil {
			s.publish(record.runID, "run_retrying", state)
			s.publishAssistantMessageUpdate(record)
		}
		if err := sleepModelRetry(ctx, decision.Delay); err != nil {
			_, _ = s.setRunRetry(record.runID, nil)
			return types.ModelResponse{}, err
		}
	}
}

func sleepModelRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *system) callModelOnce(ctx context.Context, record *runRecord, request types.ModelRequest) (types.ModelResponse, error) {
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
	record.streamReasoning = ""
	record.streamReasoningSignature = ""
	record.streamReasoningData = ""
	response, err := s.providers.CompleteStream(ctx, request, func(event types.ModelStreamEvent) error {
		switch event.Type {
		case types.ModelStreamEventContentDelta:
			content := event.Content
			if content == record.streamContent {
				return nil
			}
			contentDelta := streamContentDelta(record.streamContent, content)
			record.streamContent = content
			_, hadAssistant := activeRunAssistant(record)
			updateRunAssistantContent(record, content)
			if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
				return err
			}
			if !hadAssistant {
				if err := s.saveRunSession(ctx, record, types.RunStatusRunning); err != nil {
					return err
				}
			}
			s.publish(record.runID, "model_stream_delta", types.RunStreamDelta{RunID: record.runID, RoleID: record.roleID, GroupID: record.groupID, SessionID: record.session.ID, MessageID: record.messageParent.ID, ParentMessageID: record.messageParent.ParentMessageID, BranchID: record.messageParent.BranchID, ContentDelta: contentDelta, Content: content, CreatedAt: event.CreatedAt})
			s.publishAssistantMessageUpdate(record)
			return nil
		case types.ModelStreamEventReasoningDelta:
			reasoning := event.Reasoning
			if reasoning == record.streamReasoning && event.ReasoningSignature == record.streamReasoningSignature && event.ReasoningData == record.streamReasoningData {
				return nil
			}
			record.streamReasoning = reasoning
			record.streamReasoningSignature = event.ReasoningSignature
			record.streamReasoningData = event.ReasoningData
			_, hadAssistant := activeRunAssistant(record)
			updateRunAssistantReasoning(record, reasoning, event.ReasoningSource, event.ReasoningSignature, event.ReasoningData)
			if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
				return err
			}
			if !hadAssistant {
				if err := s.saveRunSession(ctx, record, types.RunStatusRunning); err != nil {
					return err
				}
			}
			s.publishAssistantMessageUpdate(record)
			return nil
		default:
			return nil
		}
	})
	if err != nil {
		return types.ModelResponse{}, runtimeProviderFailed("failed to stream model request", err)
	}
	if strings.TrimSpace(response.Content) == "" && strings.TrimSpace(record.streamContent) != "" {
		response.Content = record.streamContent
	}
	if strings.TrimSpace(response.Reasoning) == "" && strings.TrimSpace(record.streamReasoning) != "" {
		response.Reasoning = record.streamReasoning
	}
	if strings.TrimSpace(response.ReasoningSignature) == "" && strings.TrimSpace(record.streamReasoningSignature) != "" {
		response.ReasoningSignature = record.streamReasoningSignature
	}
	if strings.TrimSpace(response.ReasoningData) == "" && strings.TrimSpace(record.streamReasoningData) != "" {
		response.ReasoningData = record.streamReasoningData
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
	case types.MessageTypeSystemControl:
		role = "system"
		if isCompressionSummaryMessage(message) {
			content = compressionSummaryPromptContent(message)
		}
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
