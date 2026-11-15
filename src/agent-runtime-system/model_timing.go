package agentruntime

import (
	"time"
)

// modelCallTiming 记录一轮模型调用的墙钟耗时与各输出阶段的时间跨度。
// 它是 AI 回复耗时事实的唯一采集点：总耗时归属消息，思考与书写跨度归属对应部件。
type modelCallTiming struct {
	startedAt           time.Time
	finishedAt          time.Time
	firstTokenAt        time.Time
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
	t.markToken(at)
	if t.reasoningStartedAt.IsZero() {
		t.reasoningStartedAt = at
	}
	t.reasoningFinishedAt = at
}

// markContent 以真实推进的正文增量为界：首个增量定起点，每个增量推进终点。
func (t *modelCallTiming) markContent(at time.Time) {
	t.markToken(at)
	if t.contentStartedAt.IsZero() {
		t.contentStartedAt = at
	}
	t.contentFinishedAt = at
}

// markToken 记录首个真实增量到达的时刻，是「首字」的唯一事实源：
// 思考与正文共用这一落点，谁先到，首字就是谁。重复或缺失的增量不覆盖首个时刻。
func (t *modelCallTiming) markToken(at time.Time) {
	if t.firstTokenAt.IsZero() {
		t.firstTokenAt = at
	}
}

func (t modelCallTiming) totalMs() int64 {
	return spanMilliseconds(t.startedAt, t.finishedAt)
}

// firstTokenMs 返回从调用开始到首个真实增量到达的等待时长；没有增量时为零。
func (t modelCallTiming) firstTokenMs() int64 {
	return spanMilliseconds(t.startedAt, t.firstTokenAt)
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
