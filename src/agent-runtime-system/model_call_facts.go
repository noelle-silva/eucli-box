package agentruntime

import (
	"strings"

	"eucli-box/pkg/types"
)

// modelCallFacts 是一次模型调用需要落到助手消息上的用量真值。
// 空值表示上游没给这一项，落账时保持消息原值不动，绝不推算。
type modelCallFacts struct {
	cacheRate     *float64
	contextTokens *int
}

// applyRunModelFacts 把一个模型响应的全部事实落到本轮助手消息：
// 调用耗时与阶段跨度、本次缓存率、本次输入上下文规模，一次调用只结算一次。
func applyRunModelFacts(record *runRecord, response types.ModelResponse) {
	settleRunModelFacts(record, modelCallFacts{cacheRate: response.CacheRate, contextTokens: response.ContextTokens})
}

// applyRunModelTiming 只在助手消息诞生点承接耗时事实：
// 此刻消息刚刚出现，还没有可承接的用量真值，只落时间。
func applyRunModelTiming(record *runRecord) {
	settleRunModelFacts(record, modelCallFacts{})
}

// settleRunModelFacts 是模型调用事实的唯一落账点：先定位本轮消息，
// 再逐项写入上游给出的真值；一项都没写进去时不动消息与时间戳。
func settleRunModelFacts(record *runRecord, facts modelCallFacts) {
	if record == nil {
		return
	}
	message := runFactsTargetMessage(record)
	if message == nil {
		return
	}
	applied := applyModelDuration(message, record.modelTiming)
	if facts.cacheRate != nil {
		rate := *facts.cacheRate
		message.CacheRate = &rate
		applied = true
	}
	if facts.contextTokens != nil {
		tokens := *facts.contextTokens
		message.ContextTokens = &tokens
		applied = true
	}
	if !applied {
		return
	}
	advanceRunFactsMessage(record, message)
}

// runFactsTargetMessage 返回本轮事实的落点消息：优先当前活跃助手消息，
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
