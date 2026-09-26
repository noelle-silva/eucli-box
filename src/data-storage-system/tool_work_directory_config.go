package datastorage

import (
	"context"
	"time"

	"eucli-box/pkg/types"
)

// LoadToolWorkDirectoryConfig 读取工具默认工作目录配置；文件不存在时返回默认值。
func (s *system) LoadToolWorkDirectoryConfig(ctx context.Context) (types.ToolWorkDirectoryConfig, error) {
	return loadMetaConfig(ctx, s.paths.toolWorkDirectoryConfigFile(), defaultToolWorkDirectoryConfig(), normalizeToolWorkDirectoryConfigForStorage)
}

// SaveToolWorkDirectoryConfig 保存工具默认工作目录配置；空目录归一为默认值。
func (s *system) SaveToolWorkDirectoryConfig(ctx context.Context, config types.ToolWorkDirectoryConfig) (types.ToolWorkDirectoryConfig, error) {
	config = normalizeToolWorkDirectoryConfigForStorage(config)
	config.UpdatedAt = time.Now().UTC()
	if err := writeJSON(ctx, s.paths.toolWorkDirectoryConfigFile(), config); err != nil {
		return types.ToolWorkDirectoryConfig{}, err
	}
	return config, nil
}

func defaultToolWorkDirectoryConfig() types.ToolWorkDirectoryConfig {
	return types.ToolWorkDirectoryConfig{Directory: types.DefaultToolWorkDirectory()}
}

// normalizeToolWorkDirectoryConfigForStorage 归一化配置：空值回默认目录、
// 非空值解析为绝对路径；配置恒有值，不存在"未设置"状态。
func normalizeToolWorkDirectoryConfigForStorage(config types.ToolWorkDirectoryConfig) types.ToolWorkDirectoryConfig {
	config.Directory = types.NormalizeToolWorkDirectory(config.Directory)
	if config.UpdatedAt.IsZero() {
		config.UpdatedAt = time.Now().UTC()
	}
	return config
}
