package agentruntime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

// LoadConversationImageConfig 读取会话图片机制配置。
func (s *system) LoadConversationImageConfig(ctx context.Context) (types.ConversationImageConfig, error) {
	config, err := s.storage.LoadConversationImageConfig(ctx)
	if err != nil {
		return types.ConversationImageConfig{}, runtimeStorageFailed("failed to load conversation image config", err)
	}
	return config, nil
}

// SaveConversationImageConfig 保存会话图片机制配置；张数越界即拒绝。
func (s *system) SaveConversationImageConfig(ctx context.Context, config types.ConversationImageConfig) (types.ConversationImageConfig, error) {
	if err := validateConversationImageConfig(config); err != nil {
		return types.ConversationImageConfig{}, err
	}
	saved, err := s.storage.SaveConversationImageConfig(ctx, config)
	if err != nil {
		return types.ConversationImageConfig{}, runtimeStorageFailed("failed to save conversation image config", err)
	}
	return saved, nil
}

// validateConversationImageConfig 校验图片预算张数在合法范围内。
func validateConversationImageConfig(config types.ConversationImageConfig) error {
	if config.OriginalBudgetCount < types.ConversationImageOriginalBudgetMin || config.OriginalBudgetCount > types.ConversationImageOriginalBudgetMax {
		return runtimeInvalid("original budget count is out of range", nil)
	}
	if config.HistoryBudgetCount < types.ConversationImageHistoryBudgetMin || config.HistoryBudgetCount > types.ConversationImageHistoryBudgetMax {
		return runtimeInvalid("history budget count is out of range", nil)
	}
	return nil
}

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
		rollback := captureAssistantRollback(record)
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
		// 重试前先把上一次失败尝试写进本条助手消息的正文、思考与工具片段回滚干净，
		// 让本次重试只保留自己的真实产出，不与失败残留拼接。
		if err := s.rollbackAssistantOutput(ctx, record, rollback); err != nil {
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

// assistantRollback 记录一次模型尝试开始前助手消息的状态；
// 重试时据此把失败尝试写进该消息的产出回滚到尝试前的状态。
type assistantRollback struct {
	messageID string
	message   types.Message
	existed   bool
}

func captureAssistantRollback(record *runRecord) assistantRollback {
	messageID := strings.TrimSpace(record.activeAssistantID)
	if messageID == "" {
		return assistantRollback{}
	}
	message, ok := messageByID(record.session.Messages, messageID)
	if !ok || message.Type != "assistant" {
		return assistantRollback{}
	}
	return assistantRollback{messageID: messageID, message: cloneRunMessageSnapshot(message), existed: true}
}

// rollbackAssistantOutput 把上一次失败尝试写进本条助手消息的正文、思考与工具片段
// 回滚到尝试前的状态。只作用于本条助手消息，不牵动其它消息与已落定的历史。
// 若该消息在尝试前并不存在，则清空其产出但保留消息本体，
// 使紧随其后的重试写回同一条消息，客户端无需新增删除逻辑。
func (s *system) rollbackAssistantOutput(ctx context.Context, record *runRecord, rollback assistantRollback) error {
	messageID := strings.TrimSpace(record.activeAssistantID)
	if messageID == "" {
		return nil
	}
	index := sessionMessageIndex(record.session.Messages, messageID)
	if index < 0 || record.session.Messages[index].Type != "assistant" {
		return nil
	}
	if rollback.existed && rollback.messageID == messageID {
		record.session.Messages[index] = cloneRunMessageSnapshot(rollback.message)
	} else {
		clearAssistantOutput(&record.session.Messages[index])
	}
	now := nowUTC()
	message := record.session.Messages[index]
	message.UpdatedAt = now
	message.TokenEstimate = types.EstimateMessageTokenCount(message)
	record.session.Messages[index] = message
	record.messageParent = message
	record.lastMessageID = message.ID
	record.session.UpdatedAt = now
	record.session.LastActive = now
	return s.saveRunSession(ctx, record, types.RunStatusRunning)
}

func clearAssistantOutput(message *types.Message) {
	message.Content = ""
	message.Parts = nil
	message.Attachments = nil
	message.ModelDurationMs = 0
}

func sessionMessageIndex(messages []types.Message, messageID string) int {
	messageID = strings.TrimSpace(messageID)
	for index := range messages {
		if strings.TrimSpace(messages[index].ID) == messageID {
			return index
		}
	}
	return -1
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
			s.publishAssistantMessageDelta(record)
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
			s.publishAssistantMessageDelta(record)
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
	imagePlan, err := s.buildConversationImagePlan(ctx, roleContext.Messages)
	if err != nil {
		return nil, err
	}
	messages := make([]types.PromptMessage, 0, len(roleContext.Prompts)+len(roleContext.Messages))
	messages = append(messages, roleContext.Prompts...)
	latestUserIndex := -1
	for index, message := range roleContext.Messages {
		prompt, err := s.runtimeMessageToPrompt(ctx, message, index, imagePlan)
		if err != nil {
			return nil, err
		}
		if prompt.Role == "user" {
			latestUserIndex = len(messages)
		}
		messages = append(messages, prompt)
	}
	messages, err = s.applyHookPromptPreset(ctx, record, roleContext, messages, latestUserIndex)
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

func (s *system) runtimeMessageToPrompt(ctx context.Context, message types.Message, index int, imagePlan *conversationImagePlan) (types.PromptMessage, error) {
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
	images, err := s.promptImagesForMessage(ctx, message, index, imagePlan)
	if err != nil {
		return types.PromptMessage{}, err
	}
	prompt := types.PromptMessage{ID: message.ID, Role: role, Content: content, Parts: cloneMessageParts(message.Parts), Order: index, CreatedAt: message.CreatedAt, UpdatedAt: message.UpdatedAt}
	// 工具产物图不从助手消息的普通图片通道发出：它们跟随工具结果，
	// 由协议适配层搬运（OpenAI 系补用户消息、Anthropic 系并入 tool_result）。
	if message.Type == "assistant" {
		if err := validateToolImagePairing(prompt.Parts, images); err != nil {
			return types.PromptMessage{}, err
		}
		prompt.ToolImages = images
	} else {
		prompt.Images = promptImages(images)
	}
	return prompt, nil
}

// validateToolImagePairing 校验每张工具产物图都能配到该消息上的一次工具调用；
// 配不上即快速失败，不静默丢弃。
func validateToolImagePairing(parts []types.MessagePart, images []types.PromptToolImage) error {
	if len(images) == 0 {
		return nil
	}
	callIDs := map[string]struct{}{}
	for _, part := range parts {
		if part.Type != "tool" {
			continue
		}
		if callID := strings.TrimSpace(part.CallID); callID != "" {
			callIDs[callID] = struct{}{}
		}
	}
	for _, image := range images {
		if _, ok := callIDs[image.CallID]; !ok {
			return runtimeInvalid("tool produced image cannot be paired with a tool call", nil)
		}
	}
	return nil
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

func (s *system) promptImagesForMessage(ctx context.Context, message types.Message, messageIndex int, imagePlan *conversationImagePlan) ([]types.PromptToolImage, error) {
	images := []types.PromptToolImage{}
	for attachmentIndex, attachment := range messageImageAttachments(message) {
		decision := imagePlan.decisionFor(messageIndex, attachmentIndex)
		if decision.Placeholder != "" {
			images = append(images, types.PromptToolImage{CallID: strings.TrimSpace(attachment.CallID), AttachmentID: strings.TrimSpace(attachment.ID), Placeholder: decision.Placeholder})
			continue
		}
		dataURL, err := s.loadPromptImageDataURL(ctx, attachment, decision.UsePreview)
		if err != nil {
			return nil, err
		}
		images = append(images, types.PromptToolImage{CallID: strings.TrimSpace(attachment.CallID), AttachmentID: strings.TrimSpace(attachment.ID), DataURL: dataURL})
	}
	return images, nil
}

// loadPromptImageDataURL 装载一张发往模型的图片：按决策读取小副本或原图；
// 小副本缺失时如实回退原图（小副本只是发送优化，原图永远可用）。
func (s *system) loadPromptImageDataURL(ctx context.Context, attachment types.MessageAttachment, usePreview bool) (string, error) {
	if usePreview {
		previewPath := strings.TrimSpace(attachment.PreviewPath)
		if previewPath != "" {
			dataURL, err := s.storage.LoadSessionAttachmentPreviewImage(ctx, previewPath)
			if err == nil {
				return dataURL, nil
			}
		}
	}
	dataURL, err := s.storage.LoadSessionAttachmentImage(ctx, attachment.Path)
	if err != nil {
		return "", runtimeStorageFailed("failed to load message image attachment", err)
	}
	return dataURL, nil
}

// promptImages 把工具产物图片视图降为普通提示词图片（用户消息附件路径）；
// 附件标识与占位文字随视图保留，供预算机制与协议层使用。
func promptImages(images []types.PromptToolImage) []types.PromptImage {
	if len(images) == 0 {
		return nil
	}
	out := make([]types.PromptImage, 0, len(images))
	for _, image := range images {
		out = append(out, types.PromptImage{AttachmentID: image.AttachmentID, DataURL: image.DataURL, Placeholder: image.Placeholder})
	}
	return out
}
