package agentruntime

import (
	"testing"
	"time"

	"eucli-box/pkg/types"
)

// 首字只认首个真实增量：后续思考或正文增量不推后首个时刻。
func TestModelCallTimingFirstTokenUsesEarliestIncrement(t *testing.T) {
	start := time.Date(2026, 6, 4, 18, 0, 0, 0, time.UTC)
	timing := modelCallTiming{}
	timing.begin(start)
	timing.markReasoning(start.Add(700 * time.Millisecond))
	timing.markContent(start.Add(1500 * time.Millisecond))
	timing.markReasoning(start.Add(1800 * time.Millisecond))

	if got := timing.firstTokenMs(); got != 700 {
		t.Fatalf("firstTokenMs = %d, want 700", got)
	}
}

// 本轮调用尚未结束时，消息诞生点也能承接已经发生的首字；总耗时保持无值。
func TestApplyModelDurationKeepsFirstTokenBeforeCallFinishes(t *testing.T) {
	start := time.Date(2026, 6, 4, 18, 0, 0, 0, time.UTC)
	timing := modelCallTiming{}
	timing.begin(start)
	timing.markReasoning(start.Add(250 * time.Millisecond))

	message := types.Message{}
	if !applyModelDuration(&message, timing) {
		t.Fatal("applyModelDuration() = false, want true for first token")
	}
	if message.FirstTokenMs != 250 {
		t.Fatalf("FirstTokenMs = %d, want 250", message.FirstTokenMs)
	}
	if message.ModelDurationMs != 0 {
		t.Fatalf("ModelDurationMs = %d, want 0 before finish", message.ModelDurationMs)
	}
}
