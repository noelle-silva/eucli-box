package datastorage

import (
	"context"

	"eucli-box/pkg/installsource"
)

// InstallSourceStore 按发布物类别返回安装来源配置存储；两个类别各自独立文件，互不牵连。
// 结构校验由安装来源状态本体负责（NewState 快速失败），本层只负责原样读写。
func (s *system) InstallSourceStore(kind string) (installsource.Store, error) {
	path, err := s.paths.installSourceFile(kind)
	if err != nil {
		return nil, err
	}
	return installSourceStore{path: path}, nil
}

// installSourceStore 是绑定单份文件的来源配置读写适配器。
type installSourceStore struct {
	path string
}

// LoadInstallSource 读取该类别来源配置；文件不存在时返回含 os.ErrNotExist 的读取错误。
func (st installSourceStore) LoadInstallSource(ctx context.Context) (installsource.Config, error) {
	config, err := readJSON[installsource.Config](ctx, st.path)
	if err != nil {
		return installsource.Config{}, err
	}
	return config, nil
}

// SaveInstallSource 持久化该类别来源配置。
func (st installSourceStore) SaveInstallSource(ctx context.Context, config installsource.Config) error {
	return writeJSON(ctx, st.path, config)
}
