package systemplugin

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"eucli-box/internal/boxrelease"
	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

type System interface {
	Start(ctx context.Context) error
	ListPlugins(ctx context.Context) ([]types.SystemPluginSummary, error)
	LoadPlugin(ctx context.Context, pluginID string) (types.SystemPluginView, error)
	SavePluginUserConfig(ctx context.Context, pluginID string, config types.SystemPluginUserConfig) (types.SystemPluginView, error)
	ResolvePlaceholderValues(ctx context.Context) ([]types.SystemPluginPlaceholderValue, []types.PlaceholderProblem)
	AvailablePlaceholderInterfaces(ctx context.Context, library types.PlaceholderLibrary) ([]types.SystemPluginAvailablePlaceholderInterface, error)
	CreatePlaceholderFromInterface(ctx context.Context, library types.PlaceholderLibrary, pluginID string, interfaceID string) (types.PlaceholderLibrary, error)
	Shutdown(ctx context.Context) error
}

type Config struct {
	SourceDir  string
	DataDir    string
	Timeout    time.Duration
	BoxVersion string
}

type system struct {
	sourceDir  string
	dataDir    string
	timeout    time.Duration
	boxVersion string

	mu              sync.Mutex
	persistent      map[string]*persistentProcess
	cachedValues    map[string]cachedPlaceholderValues
	heartbeatCancel context.CancelFunc
	heartbeatWait   sync.WaitGroup
	failures        map[string]string
}

func NewSystem(config Config) (System, error) {
	sourceDir := strings.TrimSpace(config.SourceDir)
	if sourceDir == "" {
		sourceDir = "system-plugins"
	}
	dataDir := strings.TrimSpace(config.DataDir)
	if dataDir == "" {
		dataDir = filepath.Join("data", "system-plugins")
	}
	sourceAbs, err := filepath.Abs(sourceDir)
	if err != nil {
		return nil, pluginInvalid("failed to resolve system plugin source directory", err)
	}
	dataAbs, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, pluginInvalid("failed to resolve system plugin data directory", err)
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.Timeout < 0 {
		return nil, pluginInvalid("system plugin timeout cannot be negative", nil)
	}
	boxVersion := strings.TrimSpace(config.BoxVersion)
	if boxVersion == "" {
		info, err := boxrelease.Load()
		if err != nil {
			return nil, pluginInvalid("eucli-box 发布资料无效", err)
		}
		boxVersion = info.Version
	}
	if err := release.ValidateVersion(boxVersion); err != nil {
		return nil, pluginInvalid(fmt.Sprintf("eucli-box 版本无效：%v", err), err)
	}
	return &system{
		sourceDir:    filepath.Clean(sourceAbs),
		dataDir:      filepath.Clean(dataAbs),
		timeout:      config.Timeout,
		boxVersion:   boxVersion,
		persistent:   map[string]*persistentProcess{},
		cachedValues: map[string]cachedPlaceholderValues{},
		failures:     map[string]string{},
	}, nil
}

func (s *system) Start(ctx context.Context) error {
	records, err := s.discover(ctx)
	if err != nil {
		return err
	}
	heartbeatCtx, heartbeatCancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.heartbeatCancel = heartbeatCancel
	s.mu.Unlock()
	for _, record := range records {
		if record.status != types.SystemPluginStatusActive {
			continue
		}
		switch record.manifest.LifecycleType {
		case types.SystemPluginLifecyclePersistent:
			if _, err := s.ensurePersistentProcess(ctx, record); err != nil {
				s.setFailure(record.manifest.ID, err.Error())
			}
		case types.SystemPluginLifecycleCachedHeartbeat:
			if err := s.refreshCachedPlugin(ctx, record.manifest.ID); err != nil {
				s.setFailure(record.manifest.ID, err.Error())
			}
			s.startCachedHeartbeat(heartbeatCtx, record.manifest.ID, time.Duration(record.manifest.HeartbeatIntervalMs)*time.Millisecond)
		}
	}
	return nil
}

func (s *system) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	heartbeatCancel := s.heartbeatCancel
	s.heartbeatCancel = nil
	processes := s.persistent
	s.persistent = map[string]*persistentProcess{}
	s.mu.Unlock()
	if heartbeatCancel != nil {
		heartbeatCancel()
	}
	done := make(chan struct{})
	go func() {
		s.heartbeatWait.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
	case <-done:
	}
	for _, process := range processes {
		process.close(ctx)
	}
	return nil
}

func (s *system) setFailure(pluginID string, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	message = strings.TrimSpace(message)
	if message == "" {
		delete(s.failures, pluginID)
		return
	}
	s.failures[pluginID] = message
}

func (s *system) getFailure(pluginID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failures[pluginID]
}
