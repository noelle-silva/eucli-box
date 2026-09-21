package agentruntime

import (
	"strings"
	"time"

	"eucli-box/pkg/types"
)

// modelCallTiming 记录一轮模型调用的墙钟耗时与各输出阶段的时间跨度。
// 它是 AI 回复耗时事实的唯一采集点：总耗时归属消息，思考与书写跨度归属对应部件。
type modelCallTiming struct {
	startedAt           time.Time
	finishedAt          time.Time
	reasoningStartedAt  time.Time
	reasoningFinishedAt time.Time
	contentStartedAt    time.Time
	contentFinishedAt   time.Time
}

func (t *modelCallTiming) begin(at time.Time) {
	*t = modelCallTiming{startedAt: at}
}

func (t *modelCallTiming) finish(at time.Time) {
	t.finishedAt = at
}

// markReasoning 以真实推进的思考增量为界：首个增量定起点，每个增量推进终点。
func (t *modelCallTiming) markReasoning(at time.Time) {
	if t.reasoningStartedAt.IsZero() {
		t.reasoningStartedAt = at
	}
	t.reasoningFinishedAt = at
}

// markContent 以真实推进的正文增量为界：首个增量定起点，每个增量推进终点。
func (t *modelCallTiming) markContent(at time.Time) {
	if t.contentStartedAt.IsZero() {
		t.contentStartedAt = at
	}
	t.contentFinishedAt = at
}

func (t modelCallTiming) totalMs() int64 {
	return spanMilliseconds(t.startedAt, t.finishedAt)
}

func (t modelCallTiming) reasoningMs() int64 {
	return spanMilliseconds(t.reasoningStartedAt, t.reasoningFinishedAt)
}

func (t modelCallTiming) contentMs() int64 {
	return spanMilliseconds(t.contentStartedAt, t.contentFinishedAt)
}

// spanMilliseconds 返回两个时刻之间的毫秒跨度；任一时刻缺失或顺序异常时返回 0，
// 表示该跨度没有可依据的事实，不虚构时长。
func spanMilliseconds(start time.Time, end time.Time) int64 {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

// applyRunModelTiming 把最近一次模型调用的耗时事实落到本轮助手消息与对应部件上。
func applyRunModelTiming(record *runRecord) {
	if record == nil {
		return
	}
	timing := record.modelTiming
	totalMs := timing.totalMs()
	if totalMs <= 0 {
		return
	}
	messageID := strings.TrimSpace(record.activeAssistantID)
	if messageID == "" && record.messageParent.Type == "assistant" {
		messageID = strings.TrimSpace(record.messageParent.ID)
	}
	if messageID == "" {
		return
	}
	now := nowUTC()
	for index := range record.session.Messages {
		if record.session.Messages[index].ID != messageID {
			continue
		}
		message := &record.session.Messages[index]
		message.ModelDurationMs = totalMs
		applyMessagePartDurations(message, timing)
		message.UpdatedAt = now
		record.messageParent = *message
		record.lastMessageID = message.ID
		record.session.UpdatedAt = now
		record.session.LastActive = now
		return
	}
}

func applyMessagePartDurations(message *types.Message, timing modelCallTiming) {
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
