package systemplugin

import (
	"context"
	"time"

	"eucli-box/pkg/types"
)

type cachedPlaceholderValues struct {
	values    []types.SystemPluginPlaceholderValue
	updatedAt time.Time
}

func (s *system) startCachedHeartbeat(ctx context.Context, pluginID string, interval time.Duration) {
	s.heartbeatWait.Add(1)
	go func() {
		defer s.heartbeatWait.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.refreshCachedPlugin(ctx, pluginID); err != nil {
					s.setFailure(pluginID, err.Error())
				}
			}
		}
	}()
}

func (s *system) refreshCachedPlugin(ctx context.Context, pluginID string) error {
	records, err := s.discover(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.manifest.ID != pluginID {
			continue
		}
		if record.manifest.LifecycleType != types.SystemPluginLifecycleCachedHeartbeat {
			return pluginInvalid("system plugin is not cached-heartbeat", nil)
		}
		if record.executable == "" {
			return pluginExecutionFailed(nonEmpty(record.statusMessage, "system plugin executable is unavailable"), nil)
		}
		values, err := s.resolveRecord(ctx, record)
		if err != nil {
			return err
		}
		s.setCachedValues(record.manifest.ID, values)
		s.setFailure(record.manifest.ID, "")
		return nil
	}
	return pluginInvalid("system plugin was not found", nil)
}

func (s *system) setCachedValues(pluginID string, values []types.SystemPluginPlaceholderValue) {
	copyValues := append([]types.SystemPluginPlaceholderValue(nil), values...)
	s.mu.Lock()
	s.cachedValues[pluginID] = cachedPlaceholderValues{values: copyValues, updatedAt: time.Now().UTC()}
	s.mu.Unlock()
}

func (s *system) clearCachedValues(pluginID string) {
	s.mu.Lock()
	delete(s.cachedValues, pluginID)
	s.mu.Unlock()
}

func (s *system) cachedValuesForRecord(record pluginRecord) ([]types.SystemPluginPlaceholderValue, bool) {
	s.mu.Lock()
	entry, ok := s.cachedValues[record.manifest.ID]
	s.mu.Unlock()
	if !ok || len(entry.values) == 0 || entry.updatedAt.IsZero() {
		return nil, false
	}
	values := make([]types.SystemPluginPlaceholderValue, 0, len(entry.values))
	for _, value := range entry.values {
		values = append(values, types.SystemPluginPlaceholderValue{PluginID: value.PluginID, InterfaceID: value.InterfaceID, Name: value.Name, Value: value.Value})
	}
	return values, true
}
