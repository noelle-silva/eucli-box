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
	EucliBoxURL string `json:"eucliBoxUrl"`
	EucliBoxKey string `json:"eucliBoxKey"`
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
	return cfg, nil
}

func (s *configStore) save(next clientConfig) (clientConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := clientConfig{
		EucliBoxURL: normalizeBaseURL(next.EucliBoxURL),
		EucliBoxKey: strings.TrimSpace(next.EucliBoxKey),
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
