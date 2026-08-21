package datastorage

import (
	"context"
	"errors"

	"eucli-box/pkg/installsource"
)

// LoadInstallSource 读取安装来源配置；文件不存在时返回 storageReadFailed（含 os.ErrNotExist），
// 由装配层决定初始默认值。内容损坏（未知来源值）时快速失败，不静默归一。
func (s *system) LoadInstallSource(ctx context.Context) (installsource.Kind, error) {
	config, err := readJSON[installSourceRecord](ctx, s.paths.installSourceFile())
	if err != nil {
		return "", err
	}
	kind, err := installsource.ParseKind(string(config.Kind))
	if err != nil {
		return "", storageReadFailed("安装来源配置损坏", err)
	}
	return kind, nil
}

// SaveInstallSource 持久化安装来源配置；只接受合法来源值。
func (s *system) SaveInstallSource(ctx context.Context, kind installsource.Kind) error {
	if !kind.Valid() {
		return storageWriteFailed("保存不支持的安装来源", errors.New(string(kind)))
	}
	return writeJSON(ctx, s.paths.installSourceFile(), installSourceRecord{Kind: kind})
}

type installSourceRecord struct {
	Kind installsource.Kind `json:"kind"`
}
