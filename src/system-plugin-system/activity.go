package systemplugin

import (
	"context"
	"sync"
	"time"

	"eucli-box/pkg/types"
)

// pluginActivity 维护单个插件的活动计数和更新闸门；
// 覆盖 on-demand、persistent 请求和 cached-heartbeat 刷新，不以缓存值存在代表插件仍在运行。
type pluginActivity struct {
	mu             sync.Mutex
	activeRequests int
	updating       bool
	operationID    string
	changed        chan struct{}
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
func (a *pluginActivity) beginUpdate(operationID string, waitTimeout time.Duration) string {
	a.mu.Lock()
	if a.updating {
		a.mu.Unlock()
		return types.ArtifactErrorUpdateInProgress
	}
	a.updating = true
	a.operationID = operationID
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
			return types.ArtifactErrorPluginActive
		}
		select {
		case <-changed:
		case <-time.After(remaining):
			a.endUpdate()
			return types.ArtifactErrorPluginActive
		}
	}
}

// endUpdate 清除更新闸门。
func (a *pluginActivity) endUpdate() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.updating = false
	a.operationID = ""
	a.notifyChanged()
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
