package systemplugin

import (
	"context"

	"eucli-box/pkg/types"
)

// DisablePlugin 把插件开关置为停用：先落盘挡住新调用，等手头调用结束，
// 再停止全部生命周期并清掉缓存值。插件本体、配置与数据全部保留。
func (s *system) DisablePlugin(ctx context.Context, pluginID string) (types.SystemPluginView, error) {
	record, err := s.findRecord(ctx, pluginID)
	if err != nil {
		return types.SystemPluginView{}, err
	}
	if record.manifest.ID == "" {
		return types.SystemPluginView{}, pluginInvalid("system plugin identity is unavailable", nil)
	}
	id := record.manifest.ID
	activity := s.activityFor(id)
	if activity.runningSnapshot() != nil {
		return types.SystemPluginView{}, pluginInvalid("system plugin is updating", nil)
	}
	if !activity.tryBeginLifecycle() {
		return types.SystemPluginView{}, pluginInvalid("system plugin is executing another lifecycle action", nil)
	}
	defer activity.endLifecycle()
	if err := s.savePluginState(ctx, id, pluginState{Enabled: false}); err != nil {
		return types.SystemPluginView{}, err
	}
	// 新调用已被停用状态挡住；等待已经开始的调用结束，超时则强制收尾，保证停用闭环。
	stableCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.updateWaitTimeout)
	defer cancel()
	_ = activity.waitForIdle(stableCtx)
	if err := s.stopDisabledLifecycle(ctx, stableCtx, id); err != nil {
		return types.SystemPluginView{}, err
	}
	s.clearCachedValues(id)
	return s.LoadPlugin(ctx, id)
}

// EnablePlugin 把插件开关置为启用并恢复生命周期；
// 恢复失败只记录故障，启用意图保留——开关与程序可用性互不混淆。
func (s *system) EnablePlugin(ctx context.Context, pluginID string) (types.SystemPluginView, error) {
	record, err := s.findRecord(ctx, pluginID)
	if err != nil {
		return types.SystemPluginView{}, err
	}
	if record.manifest.ID == "" {
		return types.SystemPluginView{}, pluginInvalid("system plugin identity is unavailable", nil)
	}
	id := record.manifest.ID
	activity := s.activityFor(id)
	if activity.runningSnapshot() != nil {
		return types.SystemPluginView{}, pluginInvalid("system plugin is updating", nil)
	}
	if !activity.tryBeginLifecycle() {
		return types.SystemPluginView{}, pluginInvalid("system plugin is executing another lifecycle action", nil)
	}
	defer activity.endLifecycle()
	if err := s.savePluginState(ctx, id, pluginState{Enabled: true}); err != nil {
		return types.SystemPluginView{}, err
	}
	s.restorePluginLifecycle(ctx, id)
	return s.LoadPlugin(ctx, id)
}

// stopDisabledLifecycle 停掉停用插件的全部生命周期：
// 缓存心跳停止刷新；长驻进程先优雅退出，超时后强制终止并等待真实退出。
func (s *system) stopDisabledLifecycle(requestCtx context.Context, stableCtx context.Context, pluginID string) error {
	_ = s.stopCachedHeartbeat(stableCtx, pluginID)
	s.mu.Lock()
	process := s.persistent[pluginID]
	delete(s.persistent, pluginID)
	s.mu.Unlock()
	if process == nil {
		return nil
	}
	if err := process.stopGracefully(stableCtx); err == nil {
		return nil
	}
	forceCtx, forceCancel := context.WithTimeout(context.WithoutCancel(requestCtx), s.updateWaitTimeout)
	defer forceCancel()
	if err := process.forceStopAndWait(forceCtx); err != nil {
		return pluginExecutionFailed("system plugin process did not stop after disabling", err)
	}
	return nil
}
