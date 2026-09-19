package releasecatalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

//go:embed catalog.json
var source []byte

// releaseKinds 是正式发布物与官方来源共同允许的类别集合，发布物身份规则的唯一事实源。
var releaseKinds = map[string]struct{}{
	types.ReleaseArtifactKindTool:   {},
	types.ReleaseArtifactKindPlugin: {},
}

type Catalog struct {
	SchemaVersion    int                             `json:"schemaVersion"`
	Platform         string                          `json:"platform"`
	SourceRepository string                          `json:"sourceRepository"`
	Sources          []types.OfficialReleaseSource   `json:"sources"`
	Artifacts        []types.ReleaseArtifactIdentity `json:"artifacts"`
}

func Load() (Catalog, error) {
	var catalog Catalog
	decoder := json.NewDecoder(strings.NewReader(string(source)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, fmt.Errorf("读取正式发行清单失败：%w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Catalog{}, fmt.Errorf("读取正式发行清单失败：%w", err)
	}
	if err := Validate(catalog); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func Validate(catalog Catalog) error {
	if catalog.SchemaVersion != 1 {
		return fmt.Errorf("正式发行清单 schemaVersion 必须为 1")
	}
	if catalog.Platform != types.ReleasePlatformWindowsX64 {
		return fmt.Errorf("正式发行平台必须为 %s", types.ReleasePlatformWindowsX64)
	}
	if _, err := normalizeRepository(catalog.SourceRepository); err != nil {
		return fmt.Errorf("源码记录仓库无效：%w", err)
	}
	sources := make(map[string]types.OfficialReleaseSource, len(catalog.Sources))
	for _, item := range catalog.Sources {
		item.Kind = strings.TrimSpace(item.Kind)
		if _, ok := releaseKinds[item.Kind]; !ok {
			return fmt.Errorf("正式发行清单包含未知来源类别 %q", item.Kind)
		}
		if _, exists := sources[item.Kind]; exists {
			return fmt.Errorf("正式发行清单包含重复来源类别 %q", item.Kind)
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
		sources[item.Kind] = item
	}
	if len(sources) != len(releaseKinds) {
		return fmt.Errorf("正式发行清单必须完整声明 AI 工具和系统插件两类官方来源")
	}

	identities := map[string]struct{}{}
	for _, artifact := range catalog.Artifacts {
		artifact.Kind = strings.TrimSpace(artifact.Kind)
		artifact.ID = strings.TrimSpace(artifact.ID)
		if err := ValidateArtifactIdentity(artifact); err != nil {
			return fmt.Errorf("正式发行清单包含无效发布物：%w", err)
		}
		key := artifact.Kind + ":" + artifact.ID
		if _, exists := identities[key]; exists {
			return fmt.Errorf("正式发行清单包含重复发布物 %s", key)
		}
		identities[key] = struct{}{}
	}
	return nil
}

// ValidateArtifactIdentity 校验发布物身份：类别必须为 AI 工具或系统插件，ID 必须为合法单段标识。
// 一切接受发布物身份作为输入的开发工具统一复用此处，身份规则只维护一份。
func ValidateArtifactIdentity(identity types.ReleaseArtifactIdentity) error {
	if _, ok := releaseKinds[strings.TrimSpace(identity.Kind)]; !ok {
		return fmt.Errorf("发布物类别 %q 无效", identity.Kind)
	}
	if !validID(identity.ID) {
		return fmt.Errorf("发布物 ID %q 无效", identity.ID)
	}
	return nil
}

func (c Catalog) SourceFor(kind string) (types.OfficialReleaseSource, error) {
	kind = strings.TrimSpace(kind)
	for _, source := range c.Sources {
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
func (c Catalog) RecordRepository() (string, error) {
	normalized, err := normalizeRepository(c.SourceRepository)
	if err != nil {
		return "", fmt.Errorf("源码记录仓库无效：%w", err)
	}
	return normalized.repository, nil
}

func (c Catalog) ResolveTarget(target string) (types.ReleaseArtifactIdentity, error) {
	target = strings.TrimSpace(target)
	kind, id, ok := strings.Cut(target, ":")
	if !ok {
		return types.ReleaseArtifactIdentity{}, invalidTargetError()
	}
	identity := types.ReleaseArtifactIdentity{Kind: strings.TrimSpace(kind), ID: strings.TrimSpace(id)}
	if !c.Contains(identity) {
		return types.ReleaseArtifactIdentity{}, fmt.Errorf("%s 不在正式发布物白名单中", target)
	}
	return identity, nil
}

func (c Catalog) Contains(identity types.ReleaseArtifactIdentity) bool {
	for _, artifact := range c.Artifacts {
		if artifact.Kind == identity.Kind && artifact.ID == identity.ID {
			return true
		}
	}
	return false
}

func (c Catalog) SortedArtifacts() []types.ReleaseArtifactIdentity {
	artifacts := append([]types.ReleaseArtifactIdentity(nil), c.Artifacts...)
	sort.Slice(artifacts, func(i int, j int) bool {
		if artifacts[i].Kind != artifacts[j].Kind {
			return artifacts[i].Kind < artifacts[j].Kind
		}
		return artifacts[i].ID < artifacts[j].ID
	})
	return artifacts
}

func Target(identity types.ReleaseArtifactIdentity) string {
	return identity.Kind + ":" + identity.ID
}

func TagName(identity types.ReleaseArtifactIdentity, version string) (string, error) {
	if err := release.ValidateVersion(version); err != nil {
		return "", err
	}
	switch identity.Kind {
	case types.ReleaseArtifactKindTool, types.ReleaseArtifactKindPlugin:
		if !validID(identity.ID) {
			return "", fmt.Errorf("发布物 ID 无效")
		}
		return identity.ID + "/v" + version, nil
	default:
		return "", fmt.Errorf("发布物类别无效")
	}
}

func ArchiveName(identity types.ReleaseArtifactIdentity, version string) (string, error) {
	if _, err := TagName(identity, version); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s_%s_%s.zip", identity.Kind, identity.ID, version, types.ReleasePlatformWindowsX64), nil
}

type repositoryParts struct {
	repository string
	owner      string
	name       string
}

func normalizeRepository(value string) (repositoryParts, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return repositoryParts{}, err
	}
	if parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "github.com") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return repositoryParts{}, fmt.Errorf("必须是固定的 GitHub HTTPS 仓库地址")
	}
	cleanPath := strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/")
	parts := strings.Split(cleanPath, "/")
	if len(parts) != 2 || !validID(parts[0]) || !validID(parts[1]) {
		return repositoryParts{}, fmt.Errorf("必须精确指向一个 GitHub 仓库")
	}
	repository := "https://github.com/" + parts[0] + "/" + parts[1]
	return repositoryParts{repository: repository, owner: parts[0], name: parts[1]}, nil
}

func validID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || path.Base(value) != value {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("存在多余内容")
		}
		return err
	}
	return nil
}

func invalidTargetError() error {
	return fmt.Errorf("正式发布目标必须是 tool:<id> 或 plugin:<id>")
}
