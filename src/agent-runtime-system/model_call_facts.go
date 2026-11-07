package agentruntime

import (
	"strings"

	"eucli-box/pkg/types"
)

// modelCallFacts 是一次模型调用需要落到消息上的用量真值。
// 空值表示上游没给这一项，落账时保持消息原值不动，绝不推算。
type modelCallFacts struct {
	cacheRate          *float64
	contextTokens      *int
	contextTotalTokens *int
	requestModel       *types.RequestModel
}

// applyRunModelFacts 把一个模型响应的全部事实落到本轮消息上：
// 调用耗时与阶段跨度、本次缓存率、本次输入上下文规模、回复完成后的上下文规模。
func applyRunModelFacts(record *runRecord, response types.ModelResponse) {
	settleRunModelFacts(record, modelCallFacts{
		cacheRate:          response.CacheRate,
		contextTokens:      response.ContextTokens,
		contextTotalTokens: response.ContextTotalTokens,
		requestModel:       response.RequestModel,
	})
}

// applyRunModelTiming 只在助手消息诞生点承接耗时事实：
// 此刻消息刚刚出现，还没有可承接的用量真值，只落时间。
func applyRunModelTiming(record *runRecord) {
	settleRunModelFacts(record, modelCallFacts{})
}

// settleRunModelFacts 是模型调用事实的唯一落账点：
// 输入侧上下文归属本轮起点消息，回复后总量、缓存率与耗时归属本轮助手消息。
func settleRunModelFacts(record *runRecord, facts modelCallFacts) {
	if record == nil {
		return
	}
	inputApplied := settleInputMessageContext(record, facts.contextTokens)
	message := runFactsTargetMessage(record)
	applied := false
	if message != nil {
		applied = applyModelDuration(message, record.modelTiming)
		if facts.cacheRate != nil {
			rate := *facts.cacheRate
			message.CacheRate = &rate
			applied = true
		}
		if facts.contextTotalTokens != nil {
			tokens := *facts.contextTotalTokens
			message.ContextTokens = &tokens
			applied = true
		}
		if facts.requestModel != nil {
			requestModel := *facts.requestModel
			message.RequestModel = &requestModel
			applied = true
		}
	}
	switch {
	case applied:
		advanceRunFactsMessage(record, message)
	case inputApplied:
		advanceRunSessionTimestamps(record)
	}
}

// settleInputMessageContext 把「发出去的这一刻模型读到的上下文」记在本轮起点消息上。
// 只在该消息还没有这份事实时写入：它是这条消息发出那一刻的记录，
// 同一条消息后续引发的工具回合不再改写它。
func settleInputMessageContext(record *runRecord, tokens *int) bool {
	if tokens == nil {
		return false
	}
	message := runFactsInputMessage(record)
	if message == nil || message.ContextTokens != nil {
		return false
	}
	value := *tokens
	message.ContextTokens = &value
	message.UpdatedAt = nowUTC()
	return true
}

// runFactsInputMessage 返回本轮请求的起点消息：用户消息的上下文规模记在它身上。
func runFactsInputMessage(record *runRecord) *types.Message {
	messageID := strings.TrimSpace(record.inputMessageID)
	if messageID == "" {
		return nil
	}
	index := sessionMessageIndex(record.session.Messages, messageID)
	if index < 0 {
		return nil
	}
	return &record.session.Messages[index]
}

// runFactsTargetMessage 返回用量事实的落点消息：优先当前活跃助手消息，
// 没有活跃标识时退回本轮父消息（须本身是助手消息）。
func runFactsTargetMessage(record *runRecord) *types.Message {
	messageID := strings.TrimSpace(record.activeAssistantID)
	if messageID == "" && record.messageParent.Type == "assistant" {
		messageID = strings.TrimSpace(record.messageParent.ID)
	}
	if messageID == "" {
		return nil
	}
	index := sessionMessageIndex(record.session.Messages, messageID)
	if index < 0 {
		return nil
	}
	return &record.session.Messages[index]
}

// applyModelDuration 写入调用总耗时与各输出阶段跨度；没有时间事实时对应项保持原值。
func applyModelDuration(message *types.Message, timing modelCallTiming) bool {
	totalMs := timing.totalMs()
	if totalMs <= 0 {
		return false
	}
	message.ModelDurationMs = totalMs
	applyModelPartDurations(message, timing)
	return true
}

func applyModelPartDurations(message *types.Message, timing modelCallTiming) {
	reasoningMs := timing.reasoningMs()
	contentMs := timing.contentMs()
	for index := range message.Parts {
		part := &message.Parts[index]
		switch part.Type {
		case "reasoning":
			if reasoningMs > 0 {
				part.DurationMs = reasoningMs
			}
		case "text":
			if contentMs > 0 {
				part.DurationMs = contentMs
			}
		}
	}
}

// advanceRunFactsMessage 在事实写入后推进消息与记录指针，让后续保存与推送拿到这份新事实。
func advanceRunFactsMessage(record *runRecord, message *types.Message) {
	now := nowUTC()
	message.UpdatedAt = now
	record.messageParent = *message
	record.lastMessageID = message.ID
	record.session.UpdatedAt = now
	record.session.LastActive = now
}

// advanceRunSessionTimestamps 推进会话级时间戳，让这次事实写入被当成一次真实变更。
func advanceRunSessionTimestamps(record *runRecord) {
	now := nowUTC()
	record.session.UpdatedAt = now
	record.session.LastActive = now
}
