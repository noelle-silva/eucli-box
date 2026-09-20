package releasecatalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

//go:embed sources.json
var sourcesPayload []byte

// Sources 是官方发布来源的固定事实：源码记录仓库与两类发布物的固定官方仓库。
// 只描述"去哪里读取官方发行"，不包含任何发布物名册。
type Sources struct {
	SchemaVersion    int                           `json:"schemaVersion"`
	Platform         string                        `json:"platform"`
	SourceRepository string                        `json:"sourceRepository"`
	Sources          []types.OfficialReleaseSource `json:"sources"`
}

// LoadSources 读取并校验官方发布来源配置。
func LoadSources() (Sources, error) {
	var sources Sources
	decoder := json.NewDecoder(strings.NewReader(string(sourcesPayload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&sources); err != nil {
		return Sources{}, fmt.Errorf("读取官方来源配置失败：%w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Sources{}, fmt.Errorf("读取官方来源配置失败：%w", err)
	}
	if err := ValidateSources(sources); err != nil {
		return Sources{}, err
	}
	return sources, nil
}

// ValidateSources 校验官方来源配置的完整结构：平台、源码记录仓库与两类固定官方来源。
func ValidateSources(sources Sources) error {
	if sources.SchemaVersion != 1 {
		return fmt.Errorf("官方来源配置 schemaVersion 必须为 1")
	}
	if sources.Platform != types.ReleasePlatformWindowsX64 {
		return fmt.Errorf("官方来源平台必须为 %s", types.ReleasePlatformWindowsX64)
	}
	if _, err := normalizeRepository(sources.SourceRepository); err != nil {
		return fmt.Errorf("源码记录仓库无效：%w", err)
	}
	seen := make(map[string]struct{}, len(sources.Sources))
	for _, item := range sources.Sources {
		item.Kind = strings.TrimSpace(item.Kind)
		if _, ok := releaseKinds[item.Kind]; !ok {
			return fmt.Errorf("官方来源配置包含未知来源类别 %q", item.Kind)
		}
		if _, exists := seen[item.Kind]; exists {
			return fmt.Errorf("官方来源配置包含重复来源类别 %q", item.Kind)
		}
		normalized, err := normalizeRepository(item.Repository)
		if err != nil {
			return fmt.Errorf("%s 官方来源无效：%w", item.Kind, err)
		}
		item.Repository = normalized.repository
		item.Owner = normalized.owner
		item.Name = normalized.name
		item.Ref = strings.TrimSpace(item.Ref)
		if item.Ref == "" || strings.ContainsAny(item.Ref, " \t\r\n\\") || item.Ref == "." || item.Ref == ".." {
			return fmt.Errorf("%s 官方来源必须指定固定索引引用", item.Kind)
		}
		seen[item.Kind] = struct{}{}
	}
	if len(seen) != len(releaseKinds) {
		return fmt.Errorf("官方来源配置必须完整声明 AI 工具和系统插件两类官方来源")
	}
	return nil
}

// SourceFor 返回指定类别的固定官方来源。
func (s Sources) SourceFor(kind string) (types.OfficialReleaseSource, error) {
	kind = strings.TrimSpace(kind)
	for _, source := range s.Sources {
		if source.Kind != kind {
			continue
		}
		normalized, err := normalizeRepository(source.Repository)
		if err != nil {
			return types.OfficialReleaseSource{}, err
		}
		source.Repository = normalized.repository
		source.Owner = normalized.owner
		source.Name = normalized.name
		source.Ref = strings.TrimSpace(source.Ref)
		return source, nil
	}
	return types.OfficialReleaseSource{}, fmt.Errorf("发布物类别 %q 没有固定官方来源", kind)
}

// RecordRepository 返回工具与插件成品源码记录使用的固定仓库地址。
func (s Sources) RecordRepository() (string, error) {
	normalized, err := normalizeRepository(s.SourceRepository)
	if err != nil {
		return "", fmt.Errorf("源码记录仓库无效：%w", err)
	}
	return normalized.repository, nil
}
