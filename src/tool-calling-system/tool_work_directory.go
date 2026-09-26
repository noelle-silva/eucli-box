package toolcalling

import (
	"context"
	"errors"
	"os"
	"strings"

	"eucli-box/pkg/types"
)

// LoadToolWorkDirectoryConfig 读取工具默认工作目录配置：恒有值，空值回默认。
func (s *system) LoadToolWorkDirectoryConfig(ctx context.Context) (types.ToolWorkDirectoryConfig, error) {
	config, err := s.storage.LoadToolWorkDirectoryConfig(ctx)
	if err != nil {
		return types.ToolWorkDirectoryConfig{}, toolStorageFailed("failed to load tool work directory config", err)
	}
	config.Directory = types.NormalizeToolWorkDirectory(config.Directory)
	return config, nil
}

// SaveToolWorkDirectoryConfig 保存工具默认工作目录配置：目录必须真实可创建，
// 校验失败即拒绝保存，避免把不可用的位置写进配置。
func (s *system) SaveToolWorkDirectoryConfig(ctx context.Context, config types.ToolWorkDirectoryConfig) (types.ToolWorkDirectoryConfig, error) {
	config.Directory = types.NormalizeToolWorkDirectory(config.Directory)
	if err := ensureToolWorkDirectory(config.Directory); err != nil {
		return types.ToolWorkDirectoryConfig{}, toolInvalid("tool work directory cannot be created: "+config.Directory, err)
	}
	saved, err := s.storage.SaveToolWorkDirectoryConfig(ctx, config)
	if err != nil {
		return types.ToolWorkDirectoryConfig{}, toolStorageFailed("failed to save tool work directory config", err)
	}
	return saved, nil
}

// toolWorkDirectory 解析正式执行与预热共用的工作目录并保证其存在；
// 目录不可用时快速失败，绝不回退到 eucli-box 部署目录。
func (s *system) toolWorkDirectory(ctx context.Context) (string, error) {
	config, err := s.LoadToolWorkDirectoryConfig(ctx)
	if err != nil {
		return "", err
	}
	if err := ensureToolWorkDirectory(config.Directory); err != nil {
		return "", toolExecutionInvalid("tool work directory is not available: "+config.Directory, err)
	}
	return config.Directory, nil
}

func ensureToolWorkDirectory(directory string) error {
	if strings.TrimSpace(directory) == "" {
		return errors.New("tool work directory is empty")
	}
	return os.MkdirAll(directory, 0o755)
}
