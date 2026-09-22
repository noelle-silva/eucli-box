package systemplugin

import (
	"context"

	"eucli-box/pkg/types"
)

// DisablePlugin 把插件开关置为停用：先落盘挡住新调用，等手头调用结束，
// 再停掉常驻通道。插件本体、配置与数据全部保留。
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
	if err := s.stopResidentSession(stableCtx, id); err != nil {
		return types.SystemPluginView{}, pluginExecutionFailed("system plugin process did not stop after disabling", err)
	}
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
