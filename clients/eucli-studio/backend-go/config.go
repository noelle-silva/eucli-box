package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const configFileName = "eucli-studio-client-config.json"

type clientConfig struct {
	EucliBoxURL          string           `json:"eucliBoxUrl"`
	EucliBoxKey          string           `json:"eucliBoxKey"`
	KeepBoxRunningOnExit bool             `json:"keepBoxRunningOnExit,omitempty"`
	BoxSourceKind       string           `json:"boxSourceKind,omitempty"`
	Projection           projectionConfig `json:"projection"`
}

type projectionConfig struct {
	UpdatedAt         int64             `json:"updatedAt,omitempty"`
	UI                map[string]any    `json:"ui,omitempty"`
	Settings          map[string]any    `json:"settings,omitempty"`
	RoleOrder         []string          `json:"roleOrder,omitempty"`
	GroupOrder        []string          `json:"groupOrder,omitempty"`
	RoleFolders       map[string]string `json:"roleFolders,omitempty"`
	GroupFolders      map[string]string `json:"groupFolders,omitempty"`
	ProviderFolders   map[string]string `json:"providerFolders,omitempty"`
	ModelGroupFolders map[string]string `json:"modelGroupFolders,omitempty"`
	ActiveChatByRole  map[string]string `json:"activeChatByRole,omitempty"`
	ActiveChatByGroup map[string]string `json:"activeChatByGroup,omitempty"`
}

type configStore struct {
	path string
	mu   sync.Mutex
}

func newConfigStore(dataDir string) (*configStore, error) {
	dir := strings.TrimSpace(dataDir)
	if dir == "" {
		return nil, errors.New("FW_APP_DATA_DIR is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &configStore{path: filepath.Join(dir, configFileName)}, nil
}

func (s *configStore) load() (clientConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *configStore) loadLocked() (clientConfig, error) {
	var cfg clientConfig
	payload, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return cfg, err
	}
	cfg.EucliBoxURL = normalizeBaseURL(cfg.EucliBoxURL)
	cfg.EucliBoxKey = strings.TrimSpace(cfg.EucliBoxKey)
	cfg.Projection = normalizeProjection(cfg.Projection)
	return cfg, nil
}

func (s *configStore) save(next clientConfig) (clientConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	projection := normalizeProjection(next.Projection)
	projection.UpdatedAt = nowMillis()
	cfg := clientConfig{
		EucliBoxURL:          normalizeBaseURL(next.EucliBoxURL),
		EucliBoxKey:          strings.TrimSpace(next.EucliBoxKey),
		KeepBoxRunningOnExit: next.KeepBoxRunningOnExit,
		BoxSourceKind:        strings.TrimSpace(next.BoxSourceKind),
		Projection:           projection,
	}
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return clientConfig{}, err
	}
	if err := os.WriteFile(s.path, append(payload, '\n'), 0o600); err != nil {
		return clientConfig{}, err
	}
	return cfg, nil
}

func (s *configStore) updateProjection(fn func(*projectionConfig)) (projectionConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.loadLocked()
	if err != nil {
		return projectionConfig{}, err
	}
	projection := normalizeProjection(cfg.Projection)
	fn(&projection)
	projection = normalizeProjection(projection)
	projection.UpdatedAt = nowMillis()
	cfg.Projection = projection
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return projectionConfig{}, err
	}
	if err := os.WriteFile(s.path, append(payload, '\n'), 0o600); err != nil {
		return projectionConfig{}, err
	}
	return cfg.Projection, nil
}

// getSettings 读取客户端设置；开发模式时附带安装来源与开发标记。
func (s *configStore) getSettings() (map[string]any, error) {
	cfg, err := s.load()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"keepBoxRunningOnExit": cfg.KeepBoxRunningOnExit,
		"devBoxSourceEnabled":  devBoxSourceEnabled(),
		"boxSourceKind":        cfg.BoxSourceKind,
	}, nil
}

// updateSetting 保存单个客户端设置字段。
func (s *configStore) updateSetting(name string, value any) (map[string]any, error) {
	switch name {
	case "keepBoxRunningOnExit":
		enabled, ok := value.(bool)
		if !ok {
			return nil, newError("BAD_REQUEST", "keepBoxRunningOnExit 必须是布尔值")
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		cfg, err := s.loadLocked()
		if err != nil {
			return nil, err
		}
		cfg.KeepBoxRunningOnExit = enabled
		if err := s.writeLocked(cfg); err != nil {
			return nil, err
		}
		return map[string]any{"keepBoxRunningOnExit": enabled}, nil
	case "boxSourceKind":
		if !devBoxSourceEnabled() {
			return nil, newError("FORBIDDEN", "正式模式不允许切换业务端安装来源")
		}
		kind, ok := value.(string)
		if !ok {
			return nil, newError("BAD_REQUEST", "boxSourceKind 必须是来源类别")
		}
		kind = strings.TrimSpace(kind)
		if kind != string(localBoxSourceOfficial) && kind != string(localBoxSourceDevelopment) {
			return nil, newError("BAD_REQUEST", "boxSourceKind 只能是 official 或 development")
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		cfg, err := s.loadLocked()
		if err != nil {
			return nil, err
		}
		cfg.BoxSourceKind = kind
		if err := s.writeLocked(cfg); err != nil {
			return nil, err
		}
		return map[string]any{"boxSourceKind": kind}, nil
	default:
		return nil, newError("BAD_REQUEST", "不支持的客户端设置："+name)
	}
}

func (s *configStore) writeLocked(cfg clientConfig) error {
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(payload, '\n'), 0o600)
}

// boxSourceKindEffective 返回业务端进程应使用的安装来源；配置为空时按开发模式默认值。
func (s *configStore) boxSourceKindEffective() localBoxSourceKind {
	cfg, err := s.load()
	if err != nil {
		return localBoxSourceOfficial
	}
	if cfg.BoxSourceKind == string(localBoxSourceDevelopment) {
		return localBoxSourceDevelopment
	}
	if cfg.BoxSourceKind == string(localBoxSourceOfficial) {
		return localBoxSourceOfficial
	}
	if devBoxSourceEnabled() {
		return localBoxSourceDevelopment
	}
	return localBoxSourceOfficial
}

func devBoxSourceEnabled() bool {
	return strings.TrimSpace(os.Getenv(devSourceEnvironment)) == devSourceEnabled
}

// clearLegacyConnection 清理客户端设置中保存的旧地址和旧 Key 副本；
// 只在客户端已经通过本机受托连接成功连接后调用。
func (s *configStore) clearLegacyConnection() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.loadLocked()
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.EucliBoxURL) == "" && strings.TrimSpace(cfg.EucliBoxKey) == "" {
		return nil
	}
	cfg.EucliBoxURL = ""
	cfg.EucliBoxKey = ""
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(payload, '\n'), 0o600)
}

func normalizeProjection(value projectionConfig) projectionConfig {
	if value.UI == nil {
		value.UI = map[string]any{}
	}
	if value.Settings == nil {
		value.Settings = map[string]any{}
	}
	if value.RoleOrder == nil {
		value.RoleOrder = []string{}
	}
	if value.GroupOrder == nil {
		value.GroupOrder = []string{}
	}
	if value.RoleFolders == nil {
		value.RoleFolders = map[string]string{}
	}
	if value.GroupFolders == nil {
		value.GroupFolders = map[string]string{}
	}
	if value.ProviderFolders == nil {
		value.ProviderFolders = map[string]string{}
	}
	if value.ModelGroupFolders == nil {
		value.ModelGroupFolders = map[string]string{}
	}
	if value.ActiveChatByRole == nil {
		value.ActiveChatByRole = map[string]string{}
	}
	if value.ActiveChatByGroup == nil {
		value.ActiveChatByGroup = map[string]string{}
	}
	return value
}

func (s *configStore) requireConfigured() (clientConfig, error) {
	cfg, err := s.load()
	if err != nil {
		return clientConfig{}, err
	}
	if strings.TrimSpace(cfg.EucliBoxURL) == "" {
		return clientConfig{}, newError("EUCLI_BOX_NOT_CONFIGURED", "eucli-box 地址未配置")
	}
	return cfg, nil
}

func normalizeBaseURL(value string) string {
	url := strings.TrimSpace(value)
	url = strings.TrimRight(url, "/")
	return url
}
