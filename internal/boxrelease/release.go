package boxrelease

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

//go:embed release.json
var source []byte

func Load() (types.EucliBoxRelease, error) {
	var info types.EucliBoxRelease
	if err := json.Unmarshal(source, &info); err != nil {
		return types.EucliBoxRelease{}, fmt.Errorf("读取 eucli-box 发布资料失败：%w", err)
	}
	if err := release.ValidateVersion(info.Version); err != nil {
		return types.EucliBoxRelease{}, fmt.Errorf("eucli-box 发布资料无效：%w", err)
	}
	return info, nil
}
