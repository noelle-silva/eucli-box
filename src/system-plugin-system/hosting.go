package systemplugin

import (
	"context"
	"time"

	"eucli-box/pkg/systemplugin"
	"eucli-box/pkg/types"
)

// ensureResidentSession 确保常驻插件有一条活着的控制通道；已有通道直接复用。
func (s *system) ensureResidentSession(ctx context.Context, record pluginRecord) (*session, error) {
	pluginID := record.manifest.ID
	s.mu.Lock()
	if existing := s.residents[pluginID]; existing != nil {
		s.mu.Unlock()
		return existing, nil
	}
	s.mu.Unlock()
	instance, err := s.startRecordSession(record)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if existing := s.residents[pluginID]; existing != nil {
		s.mu.Unlock()
		_ = instance.stop(context.Background())
		return existing, nil
	}
	s.residents[pluginID] = instance
	s.mu.Unlock()
	s.setFailure(pluginID, "")
	go s.watchResident(pluginID, instance)
	return instance, nil
}

func (s *system) startRecordSession(record pluginRecord) (*session, error) {
	return startSession(sessionConfig{
		pluginID:      record.manifest.ID,
		hostVersion:   s.boxVersion,
		executable:    record.executable,
		directory:     record.directory,
		dataDirectory: record.dataDirectory,
		userConfig:    copyMap(record.userConfig.UserConfig),
		defaultConfig: copyMap(record.defaultConfig),
		timeout:       s.timeout,
		stopTimeout:   time.Duration(record.manifest.Hosting.StopTimeoutMs) * time.Millisecond,
		capabilities:  record.wireCapabilities(),
		onEvent:       s.onEvent,
	})
}

// watchResident 监视常驻通道的寿命：宿主主动停机不重启；
// 非预期退出按清单的重启策略恢复，崩溃重启由这条监视承担，不新增看护实体。
func (s *system) watchResident(pluginID string, instance *session) {
	<-instance.done
	s.mu.Lock()
	if s.residents[pluginID] == instance {
		delete(s.residents, pluginID)
	}
	shuttingDown := s.shuttingDown
	s.mu.Unlock()
	if instance.stoppingByHost() || shuttingDown {
		return
	}
	// 保留更具体的既有失败事实（例如某次调用超时），只补位不覆盖。
	if s.getFailure(pluginID) == "" {
		s.setFailure(pluginID, "系统插件进程非预期退出："+failureText(instance.failureError()))
	}
	s.restartResident(pluginID)
}

func (s *system) restartResident(pluginID string) {
	time.Sleep(restartDelay)
	s.mu.Lock()
	shuttingDown := s.shuttingDown
	s.mu.Unlock()
	if shuttingDown {
		return
	}
	index, err := s.discover(context.Background())
	if err != nil {
		return
	}
	record, ok := index.find(pluginID)
	if !ok || !record.enabled || record.status != types.SystemPluginStatusActive {
		return
	}
	if !record.manifest.Hosting.Resident || record.manifest.Hosting.Restart != types.SystemPluginRestartOnFailure {
		return
	}
	if s.activityFor(pluginID).runningSnapshot() != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	if _, err := s.ensureResidentSession(ctx, record); err != nil {
		s.setFailure(pluginID, err.Error())
	}
}

// invokeCapability 按插件托管形态发起一次能力调用：
// 常驻插件复用通道；按需插件临时拉起、调用后限时停机。
func (s *system) invokeCapability(ctx context.Context, record pluginRecord, capability string, interfaces []string) (systemplugin.Message, error) {
	if record.manifest.Hosting.Resident {
		instance, err := s.ensureResidentSession(ctx, record)
		if err != nil {
			return systemplugin.Message{}, err
		}
		message, err := instance.invoke(ctx, capability, interfaces)
		if err != nil && fatalInvokeFailure(err) {
			instance.kill()
		}
		return message, err
	}
	instance, err := s.startRecordSession(record)
	if err != nil {
		return systemplugin.Message{}, err
	}
	defer func() { _ = instance.stop(context.Background()) }()
	return instance.invoke(ctx, capability, interfaces)
}

// notifyRecordConfig 把配置变更推送给常驻插件；插件自行决定刷新方式。
func (s *system) notifyRecordConfig(record pluginRecord) {
	s.mu.Lock()
	instance := s.residents[record.manifest.ID]
	s.mu.Unlock()
	if instance == nil {
		return
	}
	_ = instance.notifyConfig(copyMap(record.userConfig.UserConfig), copyMap(record.defaultConfig))
}

// stopResidentSession 主动停机指定常驻插件并从托管表中移除。
func (s *system) stopResidentSession(ctx context.Context, pluginID string) error {
	s.mu.Lock()
	instance := s.residents[pluginID]
	delete(s.residents, pluginID)
	s.mu.Unlock()
	if instance == nil {
		return nil
	}
	return instance.stop(ctx)
}

// stopAllResident 停机全部常驻插件；由系统关闭统一调用。
func (s *system) stopAllResident(ctx context.Context) {
	s.mu.Lock()
	instances := make([]*session, 0, len(s.residents))
	for _, instance := range s.residents {
		instances = append(instances, instance)
	}
	s.residents = map[string]*session{}
	s.mu.Unlock()
	for _, instance := range instances {
		_ = instance.stop(ctx)
	}
}

func failureText(err error) string {
	if err == nil {
		return "控制通道已断开"
	}
	return err.Error()
}
