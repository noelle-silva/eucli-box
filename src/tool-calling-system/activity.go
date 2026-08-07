package toolcalling

import (
	"context"
	"sync"
	"time"

	"eucli-box/pkg/types"
)

const defaultUpdateWaitTimeout = 30 * time.Second

// toolActivity 维护单个工具的执行租约和更新闸门；
// 只依据真实执行计数判断活动，不依赖 UI 状态或运行日志。
type toolActivity struct {
	mu            sync.Mutex
	activeRequests int
	updating      bool
	operationID   string
	changed       chan struct{}
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

// beginUpdate 设置更新闸门并等待已开始的执行全部结束；
// 超过等待时间时清除闸门并返回 TOOL_ACTIVE。
func (a *toolActivity) beginUpdate(operationID string, waitTimeout time.Duration) string {
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
			return types.ArtifactErrorToolActive
		}
		select {
		case <-changed:
		case <-time.After(remaining):
			a.endUpdate()
			return types.ArtifactErrorToolActive
		}
	}
}

// endUpdate 清除更新闸门。
func (a *toolActivity) endUpdate() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.updating = false
	a.operationID = ""
	a.notifyChanged()
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
