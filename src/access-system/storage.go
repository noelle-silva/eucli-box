package accesssystem

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/types"
)

const (
	persistentPortsFileName = "persistent-ports.json"
	persistentKeysFileName  = "persistent-keys.json"
	migrationCompletedFlag  = "migration-completed.flag"
)

func accessRoot(dataDir string) string {
	return filepath.Join(filepath.Clean(dataDir), "access")
}

func persistentPortsPath(dataDir string) string {
	return filepath.Join(accessRoot(dataDir), persistentPortsFileName)
}

func persistentKeysPath(dataDir string) string {
	return filepath.Join(accessRoot(dataDir), persistentKeysFileName)
}

func migrationCompletedPath(dataDir string) string {
	return filepath.Join(accessRoot(dataDir), migrationCompletedFlag)
}

// readPersistentPorts 读取全部长期端口记录。文件不存在时返回空容器，不视为错误。
func readPersistentPorts(dataDir string) (types.PersistentPortsConfig, error) {
	var config types.PersistentPortsConfig
	payload, err := os.ReadFile(persistentPortsPath(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return types.PersistentPortsConfig{Ports: []types.PersistentPort{}}, nil
	}
	if err != nil {
		return config, fmt.Errorf("读取长期端口记录失败：%w", err)
	}
	if err := json.Unmarshal(payload, &config); err != nil {
		return config, fmt.Errorf("长期端口记录格式无效：%w", err)
	}
	if err := validatePersistentPortsConfig(config); err != nil {
		return config, err
	}
	return config, nil
}

// writePersistentPorts 先写临时文件再原子替换保存长期端口记录，失败时不覆盖原文件。
func writePersistentPorts(dataDir string, config types.PersistentPortsConfig) error {
	if err := validatePersistentPortsConfig(config); err != nil {
		return err
	}
	return atomicWriteJSON(persistentPortsPath(dataDir), config)
}

// readPersistentKeys 读取全部长期 Key 记录。文件不存在时返回空容器，不视为错误。
func readPersistentKeys(dataDir string) (types.PersistentKeysConfig, error) {
	var config types.PersistentKeysConfig
	payload, err := os.ReadFile(persistentKeysPath(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return types.PersistentKeysConfig{Keys: []types.PersistentKey{}}, nil
	}
	if err != nil {
		return config, fmt.Errorf("读取长期 Key 记录失败：%w", err)
	}
	if err := json.Unmarshal(payload, &config); err != nil {
		return config, fmt.Errorf("长期 Key 记录格式无效：%w", err)
	}
	if err := validatePersistentKeysConfig(config); err != nil {
		return config, err
	}
	return config, nil
}

// writePersistentKeys 先写临时文件再原子替换保存长期 Key 记录，失败时不覆盖原文件。
func writePersistentKeys(dataDir string, config types.PersistentKeysConfig) error {
	if err := validatePersistentKeysConfig(config); err != nil {
		return err
	}
	return atomicWriteJSON(persistentKeysPath(dataDir), config)
}

func atomicWriteJSON(target string, value any) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("建立访问设置目录失败：%w", err)
	}
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("生成访问设置资料失败：%w", err)
	}
	payload = append(payload, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(target), ".access-*.tmp")
	if err != nil {
		return fmt.Errorf("建立访问设置临时文件失败：%w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("写入访问设置临时文件失败：%w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("刷新访问设置临时文件失败：%w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭访问设置临时文件失败：%w", err)
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return fmt.Errorf("原子替换访问设置文件失败：%w", err)
	}
	return nil
}

func validatePersistentPortsConfig(config types.PersistentPortsConfig) error {
	if config.Ports == nil {
		return fmt.Errorf("长期端口记录不能为空列表")
	}
	seen := map[string]struct{}{}
	ports := map[int]struct{}{}
	for _, port := range config.Ports {
		if err := validatePersistentPort(port); err != nil {
			return err
		}
		if _, exists := seen[port.ID]; exists {
			return fmt.Errorf("长期端口记录存在重复 ID：%s", port.ID)
		}
		seen[port.ID] = struct{}{}
		if port.DesiredState == types.PersistentPortDesiredEnabled {
			if _, exists := ports[port.Port]; exists {
				return fmt.Errorf("长期端口记录存在重复端口：%d", port.Port)
			}
			ports[port.Port] = struct{}{}
		}
	}
	return nil
}

func validatePersistentPort(port types.PersistentPort) error {
	if strings.TrimSpace(port.ID) == "" {
		return fmt.Errorf("长期端口记录缺少 ID")
	}
	if strings.TrimSpace(port.Name) == "" {
		return fmt.Errorf("长期端口记录缺少名称")
	}
	if port.Port < 1 || port.Port > 65535 {
		return fmt.Errorf("长期端口号无效：%d", port.Port)
	}
	if port.DesiredState != types.PersistentPortDesiredEnabled && port.DesiredState != types.PersistentPortDesiredDisabled {
		return fmt.Errorf("长期端口期望状态无效：%s", port.DesiredState)
	}
	switch port.ActualState {
	case types.PersistentPortActualRunning, types.PersistentPortActualStopped, types.PersistentPortActualFailed:
	default:
		return fmt.Errorf("长期端口实际状态无效：%s", port.ActualState)
	}
	if strings.TrimSpace(port.CreatedAt) == "" {
		return fmt.Errorf("长期端口记录缺少创建时间")
	}
	return nil
}

func validatePersistentKeysConfig(config types.PersistentKeysConfig) error {
	if config.Keys == nil {
		return fmt.Errorf("长期 Key 记录不能为空列表")
	}
	seen := map[string]struct{}{}
	for _, key := range config.Keys {
		if err := validatePersistentKey(key); err != nil {
			return err
		}
		if _, exists := seen[key.ID]; exists {
			return fmt.Errorf("长期 Key 记录存在重复 ID：%s", key.ID)
		}
		seen[key.ID] = struct{}{}
	}
	return nil
}

func validatePersistentKey(key types.PersistentKey) error {
	if strings.TrimSpace(key.ID) == "" {
		return fmt.Errorf("长期 Key 记录缺少 ID")
	}
	if strings.TrimSpace(key.Name) == "" {
		return fmt.Errorf("长期 Key 记录缺少名称")
	}
	if strings.TrimSpace(key.EncryptedKey) == "" {
		return fmt.Errorf("长期 Key 记录缺少加密内容")
	}
	if strings.TrimSpace(key.CreatedAt) == "" {
		return fmt.Errorf("长期 Key 记录缺少创建时间")
	}
	return nil
}

// isMigrationCompleted 判断旧固定设置转换是否已经完成。
func isMigrationCompleted(dataDir string) bool {
	info, err := os.Stat(migrationCompletedPath(dataDir))
	return err == nil && !info.IsDir()
}

// markMigrationCompleted 写入转换完成标记文件。
func markMigrationCompleted(dataDir string) error {
	target := migrationCompletedPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("建立访问设置目录失败：%w", err)
	}
	if err := os.WriteFile(target, []byte("completed\n"), 0o644); err != nil {
		return fmt.Errorf("写入转换完成标记失败：%w", err)
	}
	return nil
}
