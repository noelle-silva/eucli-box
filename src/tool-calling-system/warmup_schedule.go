package toolcalling

import (
	"context"
	"log"
	"time"

	"eucli-box/pkg/types"
)

// StartWarmup 启动工具预热调度：立即预热一轮，之后按配置间隔续期；
// 调度随宿主进程存活，由 Shutdown 或宿主上下文结束停止。重复启动是错误。
func (s *system) StartWarmup(ctx context.Context) error {
	s.warmupMu.Lock()
	defer s.warmupMu.Unlock()
	if s.warmupDone != nil {
		return toolInvalid("warmup scheduler is already running", nil)
	}
	warmupCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.warmupCancel = cancel
	s.warmupDone = done
	go func() {
		defer close(done)
		s.runWarmupLoop(warmupCtx)
	}()
	return nil
}

// Shutdown 停止预热调度并等待在途的一轮收尾；未启动时无动作。
func (s *system) Shutdown(ctx context.Context) error {
	s.warmupMu.Lock()
	cancel := s.warmupCancel
	done := s.warmupDone
	s.warmupCancel = nil
	s.warmupDone = nil
	s.warmupMu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return toolExecutionInvalid("warmup scheduler did not stop in time", ctx.Err())
	}
}

func (s *system) runWarmupLoop(ctx context.Context) {
	s.warmupRound(ctx)
	ticker := time.NewTicker(s.config.ToolWarmupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.warmupRound(ctx)
		}
	}
}

// warmupRound 对当前全部“可用且声明开启预热”的工具各执行一次预热。
// 单个工具的失败只记录，不中断整轮，也不影响用户。
func (s *system) warmupRound(ctx context.Context) {
	summaries, err := s.ListTools(ctx)
	if err != nil {
		log.Printf("[tool-warmup] 工具列表读取失败：%v", err)
		return
	}
	for _, summary := range summaries {
		if ctx.Err() != nil {
			return
		}
		if summary.Status == types.ToolAvailabilityUnavailable {
			continue
		}
		tool, err := s.LoadTool(ctx, summary.ID)
		if err != nil || !toolWarmupEnabled(tool) {
			continue
		}
		duration, warmupErr := s.warmupDefinedTool(ctx, tool)
		if warmupErr != nil {
			log.Printf("[tool-warmup] %s 预热失败（耗时 %s）：%v", tool.ID, duration.Round(time.Millisecond), warmupErr)
			continue
		}
		log.Printf("[tool-warmup] %s 预热完成，耗时 %s", tool.ID, duration.Round(time.Millisecond))
	}
}
