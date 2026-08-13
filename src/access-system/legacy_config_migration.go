package accesssystem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

const (
	legacyConvertedKeyName  = "从旧配置转换"
	legacyFixedPort         = 8765
	legacyBoxKeyPath        = "meta/box.key"
)

// migrateLegacyAccessConfig 把旧的固定端口和固定 Key 一次性转为第一条长期端口和长期 Key。
// 转换完成后删除旧保存方式并写入完成标记；验证失败时保留旧文件并返回错误。
// 已经在完成标记后启动或不存在旧文件时直接返回，不做任何改动。
func migrateLegacyAccessConfig(ctx context.Context, dataDir string, keys *PersistentKeyManager, ports *PersistentPortManager) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if keys == nil || ports == nil {
		return fmt.Errorf("访问设置管理器未初始化")
	}
	if isMigrationCompleted(dataDir) {
		return nil
	}
	fixedKey, err := readLegacyFixedKey(dataDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	fixedKey = strings.TrimSpace(fixedKey)
	if fixedKey == "" {
		return nil
	}

	existingKeys, err := keys.List(ctx)
	if err != nil {
		return fmt.Errorf("读取长期 Key 记录失败：%w", err)
	}
	existingPorts, err := ports.List(ctx)
	if err != nil {
		return fmt.Errorf("读取长期端口记录失败：%w", err)
	}
	keyAlreadyConverted := containsConvertedKey(existingKeys)
	portAlreadyConverted := containsConvertedPort(existingPorts)

	convertedKey := keyAlreadyConverted
	if !keyAlreadyConverted {
		created, err := keys.SaveKey(ctx, legacyConvertedKeyName, fixedKey, true, nil)
		if err != nil {
			return fmt.Errorf("转换旧固定 Key 失败：%w", err)
		}
		convertedKey = created.ID != ""
	}
	port := types.PersistentPort{
		ID:           utils.NewID("port"),
		Name:         legacyConvertedKeyName,
		Port:         legacyFixedPort,
		DesiredState: types.PersistentPortDesiredDisabled,
		ActualState:  types.PersistentPortActualStopped,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	if convertedKey {
		port.DesiredState = types.PersistentPortDesiredEnabled
	}
	if !portAlreadyConverted {
		if err := ports.SavePort(ctx, port); err != nil {
			return fmt.Errorf("转换旧固定端口失败：%w", err)
		}
	}

	// 验证转换结果：新记录可以读取，DPAPI 可以还原原始固定 Key。
	keysAfter, err := keys.List(ctx)
	if err != nil {
		return fmt.Errorf("验证长期 Key 转换结果失败：%w", err)
	}
	if !containsConvertedKey(keysAfter) {
		return fmt.Errorf("验证长期 Key 转换结果失败：转换记录不存在")
	}
	if !keyAlreadyConverted {
		convertedID := convertedKeyID(keysAfter)
		plain, revealErr := keys.Reveal(ctx, convertedID)
		if revealErr != nil {
			return fmt.Errorf("验证长期 Key 解密失败：%w", revealErr)
		}
		if plain != fixedKey {
			return fmt.Errorf("验证长期 Key 解密失败：无法还原原始固定 Key")
		}
	}
	portsAfter, err := ports.List(ctx)
	if err != nil {
		return fmt.Errorf("验证长期端口转换结果失败：%w", err)
	}
	if !containsConvertedPort(portsAfter) {
		return fmt.Errorf("验证长期端口转换结果失败：转换记录不存在")
	}

	// 全部验证成功后才删除旧保存方式并写入完成标记。
	if err := os.Remove(filepath.Join(dataDir, legacyBoxKeyPath)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("删除旧固定 Key 文件失败：%w", err)
	}
	if err := markMigrationCompleted(dataDir); err != nil {
		return err
	}
	return nil
}

// readLegacyFixedKey 读取旧固定 Key 文件；文件不存在时返回 os.ErrNotExist。
func readLegacyFixedKey(dataDir string) (string, error) {
	payload, err := os.ReadFile(filepath.Join(filepath.Clean(dataDir), legacyBoxKeyPath))
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func containsConvertedKey(views []types.PersistentKeyView) bool {
	for _, view := range views {
		if view.Name == legacyConvertedKeyName {
			return true
		}
	}
	return false
}

func containsConvertedPort(ports []types.PersistentPort) bool {
	for _, port := range ports {
		if port.Name == legacyConvertedKeyName {
			return true
		}
	}
	return false
}

func convertedKeyID(views []types.PersistentKeyView) string {
	for _, view := range views {
		if view.Name == legacyConvertedKeyName {
			return view.ID
		}
	}
	return ""
}
