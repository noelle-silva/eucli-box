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

type Catalog struct {
	SchemaVersion int                             `json:"schemaVersion"`
	Platform      string                          `json:"platform"`
	Sources       []types.OfficialReleaseSource   `json:"sources"`
	Artifacts     []types.ReleaseArtifactIdentity `json:"artifacts"`
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
	expectedKinds := map[string]struct{}{
		types.ReleaseArtifactKindBox:    {},
		types.ReleaseArtifactKindTool:   {},
		types.ReleaseArtifactKindPlugin: {},
	}
	sources := make(map[string]types.OfficialReleaseSource, len(catalog.Sources))
	for _, item := range catalog.Sources {
		item.Kind = strings.TrimSpace(item.Kind)
		if _, ok := expectedKinds[item.Kind]; !ok {
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
	if len(sources) != len(expectedKinds) {
		return fmt.Errorf("正式发行清单必须完整声明业务端、AI 工具和系统插件三个官方来源")
	}

	identities := map[string]struct{}{}
	boxCount := 0
	for _, artifact := range catalog.Artifacts {
		artifact.Kind = strings.TrimSpace(artifact.Kind)
		artifact.ID = strings.TrimSpace(artifact.ID)
		if _, ok := expectedKinds[artifact.Kind]; !ok {
			return fmt.Errorf("正式发行清单包含未知发布物类别 %q", artifact.Kind)
		}
		if !validID(artifact.ID) {
			return fmt.Errorf("正式发行清单包含无效发布物 ID %q", artifact.ID)
		}
		if artifact.Kind == types.ReleaseArtifactKindBox {
			boxCount++
			if artifact.ID != types.ReleaseArtifactKindBox {
				return fmt.Errorf("业务端发布物 ID 必须为 %s", types.ReleaseArtifactKindBox)
			}
		}
		key := artifact.Kind + ":" + artifact.ID
		if _, exists := identities[key]; exists {
			return fmt.Errorf("正式发行清单包含重复发布物 %s", key)
		}
		identities[key] = struct{}{}
	}
	if boxCount != 1 {
		return fmt.Errorf("正式发行清单必须且只能包含一个业务端发布物")
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

func (c Catalog) ResolveTarget(target string) (types.ReleaseArtifactIdentity, error) {
	target = strings.TrimSpace(target)
	identity := types.ReleaseArtifactIdentity{}
	switch target {
	case types.ReleaseArtifactKindBox:
		identity = types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: types.ReleaseArtifactKindBox}
	default:
		kind, id, ok := strings.Cut(target, ":")
		if !ok {
			return types.ReleaseArtifactIdentity{}, invalidTargetError()
		}
		identity = types.ReleaseArtifactIdentity{Kind: strings.TrimSpace(kind), ID: strings.TrimSpace(id)}
	}
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
	if identity.Kind == types.ReleaseArtifactKindBox {
		return types.ReleaseArtifactKindBox
	}
	return identity.Kind + ":" + identity.ID
}

func TagName(identity types.ReleaseArtifactIdentity, version string) (string, error) {
	if err := release.ValidateVersion(version); err != nil {
		return "", err
	}
	switch identity.Kind {
	case types.ReleaseArtifactKindBox:
		if identity.ID != types.ReleaseArtifactKindBox {
			return "", fmt.Errorf("业务端发布物身份无效")
		}
		return "v" + version, nil
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
	name := identity.Kind
	if identity.Kind != types.ReleaseArtifactKindBox {
		name += "-" + identity.ID
	}
	return fmt.Sprintf("%s_%s_%s.zip", name, version, types.ReleasePlatformWindowsX64), nil
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
	return fmt.Errorf("正式发布目标必须是 eucli-box、tool:<id> 或 plugin:<id>")
}
