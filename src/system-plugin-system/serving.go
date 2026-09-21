package systemplugin

import (
	"eucli-box/pkg/types"
)

// acquireServing 是所有插件能力调用的统一服役闸门：
// 任何真实调用（现在的占位符解析、缓存心跳刷新，未来的新能力）都必须先通过它。
// disabled 为真表示插件被用户停用；blocked 非空表示被拒绝的其他原因（程序不可用、更新中）。
// 通过后调用方必须配对调用 releaseServing。
func (s *system) acquireServing(record pluginRecord) (disabled bool, blocked string) {
	if !record.enabled {
		return true, ""
	}
	if record.status != types.SystemPluginStatusActive {
		return false, nonEmpty(record.statusMessage, "system plugin is unavailable")
	}
	return false, s.activityFor(record.manifest.ID).acquire()
}

func (s *system) releaseServing(pluginID string) {
	s.activityFor(pluginID).release()
}
