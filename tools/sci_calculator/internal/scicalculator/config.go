package scicalculator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	PythonEnv               string       `json:"pythonEnv"`
	BundledPythonExecutable string       `json:"bundledPythonExecutable"`
	DefaultPythonExecutable string       `json:"defaultPythonExecutable"`
	Limits                  LimitsConfig `json:"limits"`
}

type LimitsConfig struct {
	MaxOutputChars     int `json:"maxOutputChars"`
	MaxExpressionChars int `json:"maxExpressionChars"`
}

func loadConfig(toolDir string) (Config, error) {
	if strings.TrimSpace(toolDir) == "" {
		return Config{}, fmt.Errorf("tool directory is required")
	}
	payload, err := os.ReadFile(filepath.Join(toolDir, "config.json"))
	if err != nil {
		return Config{}, fmt.Errorf("read config.json: %w", err)
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode config.json: %w", err)
	}
	if err := validateConfig(config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func validateConfig(config Config) error {
	if strings.TrimSpace(config.DefaultPythonExecutable) == "" {
		return fmt.Errorf("defaultPythonExecutable is required")
	}
	if config.Limits.MaxOutputChars <= 0 {
		return fmt.Errorf("limits.maxOutputChars must be greater than zero")
	}
	if config.Limits.MaxExpressionChars <= 0 {
		return fmt.Errorf("limits.maxExpressionChars must be greater than zero")
	}
	return nil
}
