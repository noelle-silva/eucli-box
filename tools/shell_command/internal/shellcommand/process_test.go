package shellcommand

import (
	"testing"
	"time"
)

// TestWaitPipeDrainedFinishesWhenPipesClosed 验证板块三：两条管道都到 EOF 时，排水立即结束，
// 不等满完整时长。
func TestWaitPipeDrainedFinishesWhenPipesClosed(t *testing.T) {
	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})
	go func() {
		close(stdoutDone)
		close(stderrDone)
	}()
	if !waitPipeDrained(stdoutDone, stderrDone, 5*time.Second) {
		t.Fatalf("waitPipeDrained should finish as soon as both pipes are drained")
	}
}

// TestWaitPipeDrainedGivesUpAfterTimeout 验证板块三：管道迟迟不 EOF（后代进程仍持有写端）时，
// 排水在届期后放弃等待，工具不被卡死。
func TestWaitPipeDrainedGivesUpAfterTimeout(t *testing.T) {
	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})
	go func() {
		close(stdoutDone)
	}()
	started := time.Now()
	if waitPipeDrained(stdoutDone, stderrDone, 50*time.Millisecond) {
		t.Fatalf("waitPipeDrained should give up when the pipe stays open")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("waitPipeDrained took too long: %v", elapsed)
	}
}
