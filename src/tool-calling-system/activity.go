package toolcalling

import (
	"context"
	"sync"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

// toolActivity 维护单个工具的执行租约和更新闸门；
// 只依据真实执行计数判断活动，不依赖 UI 状态或运行日志。
// 更新期间同时承载运行任务的事实：取消句柄、任务基座、阶段、下载进度与结束信号。
type toolActivity struct {
	mu            sync.Mutex
	activeRequests int
	updating      bool
	operationID   string
	changed       chan struct{}
	cancel        context.CancelFunc
	phase         string
	progress      types.ReleaseOperationProgress
	baseVersion   string
	baseInstalled bool
	done          chan struct{}
}

func (a *toolActivity) ensureChanged() {
	if a.changed == nil {
		a.changed = make(chan struct{}, 1)
	}
}

func (a *toolActivity) notifyChanged() {
	a.ensureChanged()
	select {
	case a.changed <- struct{}{}:
	default:
	}
}

// acquire 在真实执行开始时调用；更新闸门开启时返回错误码，正常返回空字符串。
func (a *toolActivity) acquire() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.updating {
		if a.operationID != "" {
			return types.ArtifactErrorUpdateInProgress
		}
		return types.ArtifactErrorToolActive
	}
	a.activeRequests++
	a.notifyChanged()
	return ""
}

// release 在真实执行结束时调用；取消、超时和进程启动失败也必须释放。
func (a *toolActivity) release() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeRequests > 0 {
		a.activeRequests--
	}
	a.notifyChanged()
}

// beginUpdate 设置更新闸门并立即判定占用：
// 有真实执行、或已有更新闸门时不等待，直接返回占用码。
// 成功时保存任务取消句柄，并建立结束信号。
func (a *toolActivity) beginUpdate(operationID string, cancel context.CancelFunc) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.updating {
		if a.operationID != "" {
			return types.ArtifactErrorUpdateInProgress
		}
		return types.ArtifactErrorToolActive
	}
	if a.activeRequests > 0 {
		return types.ArtifactErrorToolActive
	}
	a.updating = true
	a.operationID = operationID
	a.cancel = cancel
	a.phase = types.ArtifactPhaseCandidate
	a.progress = types.ReleaseOperationProgress{}
	a.done = make(chan struct{})
	a.notifyChanged()
	return ""
}

// setRunningBase 固定任务基座事实：受理时刻的当前版本与已安装与否。
// 运行期间对外查询只使用该基座，不再回读磁盘。
func (a *toolActivity) setRunningBase(currentVersion string, installed bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.updating {
		return
	}
	a.baseVersion = currentVersion
	a.baseInstalled = installed
}

// endUpdate 清除更新闸门与运行事实；任务结束信号由 finishUpdate 发出。
func (a *toolActivity) endUpdate() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.updating = false
	a.operationID = ""
	a.cancel = nil
	a.phase = ""
	a.progress = types.ReleaseOperationProgress{}
	a.baseVersion = ""
	a.baseInstalled = false
	a.notifyChanged()
}

// finishUpdate 发出任务结束信号，表示本轮操作的全部收尾已经完成；幂等。
func (a *toolActivity) finishUpdate() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.done != nil {
		close(a.done)
		a.done = nil
	}
}

// updatePhase 记录当前任务阶段；只在更新期间有效。
func (a *toolActivity) updatePhase(phase string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.updating {
		return
	}
	a.phase = phase
}

// updateProgress 记录最近一次下载进度；只在更新期间有效。
func (a *toolActivity) updateProgress(progress types.ReleaseOperationProgress) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.updating {
		return
	}
	a.progress = progress
}

// runningSnapshot 返回当前更新任务的事实快照；没有进行中的任务时返回 nil。
func (a *toolActivity) runningSnapshot() *release.RunningOperationSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.updating {
		return nil
	}
	return &release.RunningOperationSnapshot{
		OperationID:    a.operationID,
		Phase:          a.phase,
		Progress:       a.progress,
		CurrentVersion: a.baseVersion,
		Installed:      a.baseInstalled,
	}
}

// cancelUpdate 取走当前任务的取消句柄；没有进行中的任务时返回 false。
// 调用方负责在取走前判断阶段是否允许取消。
func (a *toolActivity) cancelUpdate() (context.CancelFunc, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.updating || a.cancel == nil {
		return nil, false
	}
	return a.cancel, true
}

// waitDone 等待当前任务结束；没有进行中的任务时立即返回。
func (a *toolActivity) waitDone(ctx context.Context) error {
	a.mu.Lock()
	done := a.done
	a.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

// state 返回当前活动事实，供客户端展示阻止原因。
func (a *toolActivity) state() types.ArtifactActivityState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return types.ArtifactActivityState{
		Active:         a.activeRequests > 0,
		ActiveRequests: a.activeRequests,
		Updating:       a.updating,
	}
}

// waitForIdle 等待活动清零；ctx 取消时返回错误。
func (a *toolActivity) waitForIdle(ctx context.Context) error {
	for {
		a.mu.Lock()
		running := a.activeRequests
		changed := a.changed
		a.mu.Unlock()
		if running == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

// activityFor 返回工具对应的租约，不存在时创建。
func (s *system) activityFor(toolID string) *toolActivity {
	s.mu.Lock()
	defer s.mu.Unlock()
	activity := s.activities[toolID]
	if activity == nil {
		activity = &toolActivity{}
		s.activities[toolID] = activity
	}
	return activity
}
