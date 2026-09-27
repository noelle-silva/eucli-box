package datastorage

import (
	"context"

	"eucli-box/pkg/installsource"
)

// LoadInstallSource 读取安装来源配置；文件不存在时返回 storageReadFailed（含 os.ErrNotExist）。
// 结构校验由安装来源状态本体负责（NewState 快速失败），本层只负责原样读写。
func (s *system) LoadInstallSource(ctx context.Context) (installsource.Config, error) {
	config, err := readJSON[installsource.Config](ctx, s.paths.installSourceFile())
	if err != nil {
		return installsource.Config{}, err
	}
	return config, nil
}

// SaveInstallSource 持久化安装来源配置；校验由状态本体先完成。
func (s *system) SaveInstallSource(ctx context.Context, config installsource.Config) error {
	return writeJSON(ctx, s.paths.installSourceFile(), config)
}
