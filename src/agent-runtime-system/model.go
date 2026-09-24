package agentruntime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) callModel(ctx context.Context, record *runRecord, roleContext types.RoleContext) (types.ModelResponse, error) {
	messages, err := s.modelMessages(ctx, record, roleContext)
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
	record.modelTiming.begin(nowUTC())
	if record.stream {
		response, err := s.callModelStream(ctx, record, request)
		if err != nil {
			return types.ModelResponse{}, err
		}
		record.modelTiming.finish(nowUTC())
		return response, nil
	}
	response, err := s.providers.Complete(ctx, request)
	if err != nil {
		return types.ModelResponse{}, runtimeProviderFailed("failed to complete model request", err)
	}
	record.modelTiming.finish(nowUTC())
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
			record.modelTiming.markContent(time.Now().UTC())
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
			s.publish(record.runID, "model_stream_delta", types.RunStreamDelta{RunID: record.runID, RoleID: record.roleID, GroupID: record.groupID, WorkspaceID: record.workspaceID, SessionID: record.session.ID, MessageID: record.messageParent.ID, ParentMessageID: record.messageParent.ParentMessageID, BranchID: record.messageParent.BranchID, ContentDelta: contentDelta, Content: content, CreatedAt: event.CreatedAt})
			s.publishAssistantMessageUpdate(record)
			return nil
		case types.ModelStreamEventReasoningDelta:
			reasoning := event.Reasoning
			if reasoning == record.streamReasoning && event.ReasoningSignature == record.streamReasoningSignature && event.ReasoningData == record.streamReasoningData {
				return nil
			}
			record.modelTiming.markReasoning(time.Now().UTC())
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

func (s *system) modelMessages(ctx context.Context, record *runRecord, roleContext types.RoleContext) ([]types.PromptMessage, error) {
	messages := make([]types.PromptMessage, 0, len(roleContext.Prompts)+len(roleContext.Messages))
	messages = append(messages, roleContext.Prompts...)
	latestUserIndex := -1
	for index, message := range roleContext.Messages {
		prompt, err := s.runtimeMessageToPrompt(ctx, message, index)
		if err != nil {
			return nil, err
		}
		if prompt.Role == "user" {
			latestUserIndex = len(messages)
		}
		messages = append(messages, prompt)
	}
	messages, err := s.applyHookPromptPreset(ctx, record, roleContext, messages, latestUserIndex)
	if err != nil {
		return nil, err
	}
	return s.resolvePromptPlaceholders(ctx, messages)
}

func (s *system) resolvePromptPlaceholders(ctx context.Context, messages []types.PromptMessage) ([]types.PromptMessage, error) {
	out, err := s.placeholders.ResolvePromptMessages(ctx, messages)
	if err != nil {
		return nil, runtimeStorageFailed("failed to resolve placeholders", err)
	}
	return out, nil
}

func (s *system) runtimeMessageToPrompt(ctx context.Context, message types.Message, index int) (types.PromptMessage, error) {
	role := message.Type
	content := message.Content
	switch message.Type {
	case "user", "assistant":
	case types.MessageTypeSystemControl:
		role = "system"
		if isCompressionSummaryMessage(message) {
			content = compressionSummaryPromptContent(message)
		}
	case "tool":
		role = "user"
		content = toolResultPromptContent(message, content)
	case types.MessageTypeAsyncToolResult:
		role = "user"
		content = toolResultPromptContent(message, content)
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

func toolResultPromptContent(message types.Message, content string) string {
	return fmt.Sprintf("Tool %s returned: %s", message.ToolName, content)
}

func cloneMessageParts(parts []types.MessagePart) []types.MessagePart {
	if len(parts) == 0 {
		return nil
	}
	result := make([]types.MessagePart, len(parts))
	copy(result, parts)
	return result
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

