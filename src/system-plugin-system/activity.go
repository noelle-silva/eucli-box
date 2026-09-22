package systemplugin

import (
	"context"
	"sync"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

// pluginActivity 维护单个插件的活动计数和更新闸门；
// 覆盖按需与常驻插件的真实能力调用，不以进程存在代表插件仍在工作。
// 更新期间同时承载运行任务的事实：取消句柄、任务基座、阶段、下载进度与结束信号。
type pluginActivity struct {
	mu             sync.Mutex
	lifecycleMu    sync.Mutex
	activeRequests int
	updating       bool
	operationID    string
	changed        chan struct{}
	cancel         context.CancelFunc
	phase          string
	progress       types.ReleaseOperationProgress
	baseVersion    string
	baseInstalled  bool
	done           chan struct{}
}

// tryBeginLifecycle 尝试开始一次插件启停动作；同一插件同时只允许一个启停动作。
func (a *pluginActivity) tryBeginLifecycle() bool {
	return a.lifecycleMu.TryLock()
}

func (a *pluginActivity) endLifecycle() {
	a.lifecycleMu.Unlock()
}

func (a *pluginActivity) ensureChanged() {
	if a.changed == nil {
		a.changed = make(chan struct{}, 1)
	}
}

func (a *pluginActivity) notifyChanged() {
	a.ensureChanged()
	select {
	case a.changed <- struct{}{}:
	default:
	}
}

// acquire 在真实请求或刷新开始时调用；更新闸门开启时返回错误码，正常返回空字符串。
func (a *pluginActivity) acquire() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.updating {
		if a.operationID != "" {
			return types.ArtifactErrorUpdateInProgress
		}
		return types.ArtifactErrorPluginActive
	}
	a.activeRequests++
	a.notifyChanged()
	return ""
}

// release 在真实请求或刷新结束时调用；取消、超时和进程启动失败也必须释放。
func (a *pluginActivity) release() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeRequests > 0 {
		a.activeRequests--
	}
	a.notifyChanged()
}

// beginUpdate 设置更新闸门并等待已开始的活动全部结束；
// 超过等待时间时清除闸门并返回 PLUGIN_ACTIVE。
// 成功时保存任务取消句柄，并建立结束信号。
func (a *pluginActivity) beginUpdate(operationID string, cancel context.CancelFunc, waitTimeout time.Duration) string {
	a.mu.Lock()
	if a.updating {
		a.mu.Unlock()
		return types.ArtifactErrorUpdateInProgress
	}
	a.updating = true
	a.operationID = operationID
	a.cancel = cancel
	a.phase = types.ArtifactPhaseCandidate
	a.progress = types.ReleaseOperationProgress{}
	a.done = make(chan struct{})
	changed := a.changed
	a.mu.Unlock()

	deadline := time.Now().Add(waitTimeout)
	for {
		a.mu.Lock()
		running := a.activeRequests
		a.mu.Unlock()
		if running == 0 {
			return ""
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			a.endUpdate()
			a.finishUpdate()
			return types.ArtifactErrorPluginActive
		}
		select {
		case <-changed:
		case <-time.After(remaining):
			a.endUpdate()
			a.finishUpdate()
			return types.ArtifactErrorPluginActive
		}
	}
}

// setRunningBase 固定任务基座事实：受理时刻的当前版本与已安装与否。
// 运行期间对外查询只使用该基座，不再回读磁盘。
func (a *pluginActivity) setRunningBase(currentVersion string, installed bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.updating {
		return
	}
	a.baseVersion = currentVersion
	a.baseInstalled = installed
}

// endUpdate 清除更新闸门与运行事实；任务结束信号由 finishUpdate 发出。
func (a *pluginActivity) endUpdate() {
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

// finishUpdate 发出任务结束信号，表示本轮操作的全部收尾（含生命周期恢复）已经完成；幂等。
func (a *pluginActivity) finishUpdate() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.done != nil {
		close(a.done)
		a.done = nil
	}
}

// updatePhase 记录当前任务阶段；只在更新期间有效。
func (a *pluginActivity) updatePhase(phase string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.updating {
		return
	}
	a.phase = phase
}

// updateProgress 记录最近一次下载进度；只在更新期间有效。
func (a *pluginActivity) updateProgress(progress types.ReleaseOperationProgress) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.updating {
		return
	}
	a.progress = progress
}

// runningSnapshot 返回当前更新任务的事实快照；没有进行中的任务时返回 nil。
func (a *pluginActivity) runningSnapshot() *release.RunningOperationSnapshot {
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
func (a *pluginActivity) cancelUpdate() (context.CancelFunc, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.updating || a.cancel == nil {
		return nil, false
	}
	return a.cancel, true
}

// waitDone 等待当前任务结束；没有进行中的任务时立即返回。
func (a *pluginActivity) waitDone(ctx context.Context) error {
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

// state 返回当前活动事实。
func (a *pluginActivity) state() types.ArtifactActivityState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return types.ArtifactActivityState{
		Active:         a.activeRequests > 0,
		ActiveRequests: a.activeRequests,
		Updating:       a.updating,
	}
}

// waitForIdle 等待活动清零；ctx 取消时返回错误。
func (a *pluginActivity) waitForIdle(ctx context.Context) error {
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

// activityFor 返回插件对应的租约，不存在时创建。
func (s *system) activityFor(pluginID string) *pluginActivity {
	s.mu.Lock()
	defer s.mu.Unlock()
	activity := s.activities[pluginID]
	if activity == nil {
		activity = &pluginActivity{}
		s.activities[pluginID] = activity
	}
	return activity
}
