package systemplugin

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	SourceDir string
	DataDir   string
	Timeout   time.Duration
}

type system struct {
	sourceDir string
	dataDir   string
	timeout   time.Duration

	mu         sync.Mutex
	persistent map[string]*persistentProcess
	failures   map[string]string
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
	return &system{sourceDir: filepath.Clean(sourceAbs), dataDir: filepath.Clean(dataAbs), timeout: config.Timeout, persistent: map[string]*persistentProcess{}, failures: map[string]string{}}, nil
}

func (s *system) Start(ctx context.Context) error {
	records, err := s.discover(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.manifest.LifecycleType != types.SystemPluginLifecyclePersistent || record.status != types.SystemPluginStatusActive {
			continue
		}
		if _, err := s.ensurePersistentProcess(ctx, record); err != nil {
			s.setFailure(record.manifest.ID, err.Error())
		}
	}
	return nil
}

func (s *system) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	processes := s.persistent
	s.persistent = map[string]*persistentProcess{}
	s.mu.Unlock()
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
