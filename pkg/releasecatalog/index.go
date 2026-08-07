package releasecatalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

// IndexPath 是官方仓库源码中统一版本索引的固定路径。
const IndexPath = "release-catalog/index.json"

// IndexSchemaVersion 是统一版本索引的资料格式版本。
const IndexSchemaVersion = 1

// Index 是官方仓库源码中集中维护的统一版本索引。
// 它是一次读取即可获得该仓库全部发布物、全部版本及完整资料的正式事实来源。
type Index struct {
	SchemaVersion int             `json:"schemaVersion"`
	UpdatedAt     time.Time       `json:"updatedAt"`
	Artifacts     []IndexArtifact `json:"artifacts"`
}

// IndexArtifact 是索引中一个独立发布物及其全部正式版本记录。
type IndexArtifact struct {
	Kind     string         `json:"kind"`
	ID       string         `json:"id"`
	Versions []IndexVersion `json:"versions"`
}

// IndexVersion 是索引中一个正式版本记录。
type IndexVersion struct {
	Version        string                  `json:"version"`
	PublishedAt    time.Time               `json:"publishedAt"`
	SourceRevision string                  `json:"sourceRevision"`
	DataVersion    string                  `json:"dataVersion,omitempty"`
	Compatibility  *types.EucliBoxCompatibility `json:"compatibility,omitempty"`
	ReleaseNotes   string                  `json:"releaseNotes,omitempty"`
	Packages       []IndexPackage          `json:"packages"`
}

// IndexPackage 是索引中一个平台压缩包记录。
type IndexPackage struct {
	Platform   string `json:"platform"`
	ReleaseTag string `json:"releaseTag"`
	FileName   string `json:"fileName"`
	SizeBytes  int64  `json:"sizeBytes"`
	SHA256     string `json:"sha256"`
}

// HTTPDoer 是索引读取所需的 HTTP 请求能力。
type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

// DecodeIndex 严格解析并校验一份统一版本索引。
func DecodeIndex(payload []byte) (Index, error) {
	var index Index
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&index); err != nil {
		return Index{}, fmt.Errorf("统一版本索引无效：%w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Index{}, fmt.Errorf("统一版本索引无效：%w", err)
	}
	if err := ValidateIndex(index); err != nil {
		return Index{}, err
	}
	return index, nil
}

// ValidateIndex 校验统一版本索引的完整正式结构。
func ValidateIndex(index Index) error {
	if index.SchemaVersion != IndexSchemaVersion {
		return fmt.Errorf("统一版本索引 schemaVersion 必须为 %d", IndexSchemaVersion)
	}
	if index.UpdatedAt.IsZero() {
		return fmt.Errorf("统一版本索引缺少 updatedAt")
	}
	if len(index.Artifacts) == 0 {
		return fmt.Errorf("统一版本索引不能为空")
	}
	expectedKinds := map[string]struct{}{
		types.ReleaseArtifactKindBox:    {},
		types.ReleaseArtifactKindTool:   {},
		types.ReleaseArtifactKindPlugin: {},
	}
	identities := map[string]struct{}{}
	boxCount := 0
	for _, artifact := range index.Artifacts {
		artifact.Kind = strings.TrimSpace(artifact.Kind)
		artifact.ID = strings.TrimSpace(artifact.ID)
		if _, ok := expectedKinds[artifact.Kind]; !ok {
			return fmt.Errorf("统一版本索引包含未知发布物类别 %q", artifact.Kind)
		}
		if !validID(artifact.ID) {
			return fmt.Errorf("统一版本索引包含无效发布物 ID %q", artifact.ID)
		}
		if artifact.Kind == types.ReleaseArtifactKindBox {
			boxCount++
			if artifact.ID != types.ReleaseArtifactKindBox {
				return fmt.Errorf("业务端发布物 ID 必须为 %s", types.ReleaseArtifactKindBox)
			}
		}
		key := artifact.Kind + ":" + artifact.ID
		if _, exists := identities[key]; exists {
			return fmt.Errorf("统一版本索引包含重复发布物 %s", key)
		}
		identities[key] = struct{}{}
		if err := validateIndexVersions(artifact); err != nil {
			return fmt.Errorf("%s 版本资料无效：%w", key, err)
		}
	}
	if boxCount > 1 {
		return fmt.Errorf("统一版本索引只能包含一个业务端发布物")
	}
	return nil
}

func validateIndexVersions(artifact IndexArtifact) error {
	if len(artifact.Versions) == 0 {
		return fmt.Errorf("必须包含至少一条正式版本记录")
	}
	versions := map[string]struct{}{}
	for _, version := range artifact.Versions {
		version.Version = strings.TrimSpace(version.Version)
		version.SourceRevision = strings.TrimSpace(version.SourceRevision)
		version.DataVersion = strings.TrimSpace(version.DataVersion)
		if err := release.ValidateVersion(version.Version); err != nil {
			return fmt.Errorf("版本 %q 无效：%w", version.Version, err)
		}
		if _, exists := versions[version.Version]; exists {
			return fmt.Errorf("同一发布物不能包含重复版本 %s", version.Version)
		}
		versions[version.Version] = struct{}{}
		if version.PublishedAt.IsZero() {
			return fmt.Errorf("版本 %s 缺少 publishedAt", version.Version)
		}
		if err := validateSourceRevision(version.SourceRevision); err != nil {
			return fmt.Errorf("版本 %s 的源码记录无效：%w", version.Version, err)
		}
		identity := types.ReleaseArtifactIdentity{Kind: artifact.Kind, ID: artifact.ID}
		expectedTag, err := TagName(identity, version.Version)
		if err != nil {
			return fmt.Errorf("版本 %s 标签无效：%w", version.Version, err)
		}
		expectedFile, err := ArchiveName(identity, version.Version)
		if err != nil {
			return fmt.Errorf("版本 %s 文件名无效：%w", version.Version, err)
		}
		if err := validateIndexVersionFacts(artifact.Kind, version); err != nil {
			return fmt.Errorf("版本 %s 事实无效：%w", version.Version, err)
		}
		if len(version.Packages) == 0 {
			return fmt.Errorf("版本 %s 缺少压缩包资料", version.Version)
		}
		platforms := map[string]struct{}{}
		for _, pkg := range version.Packages {
			pkg.Platform = strings.TrimSpace(pkg.Platform)
			pkg.ReleaseTag = strings.TrimSpace(pkg.ReleaseTag)
			pkg.FileName = strings.TrimSpace(pkg.FileName)
			if pkg.Platform != types.ReleasePlatformWindowsX64 {
				return fmt.Errorf("版本 %s 的平台必须为 %s", version.Version, types.ReleasePlatformWindowsX64)
			}
			if _, exists := platforms[pkg.Platform]; exists {
				return fmt.Errorf("版本 %s 包含重复平台 %s", version.Version, pkg.Platform)
			}
			platforms[pkg.Platform] = struct{}{}
			if pkg.ReleaseTag != expectedTag {
				return fmt.Errorf("版本 %s 的 Release 标签必须为 %s", version.Version, expectedTag)
			}
			if pkg.FileName != expectedFile {
				return fmt.Errorf("版本 %s 的压缩包文件名必须为 %s", version.Version, expectedFile)
			}
			if pkg.SizeBytes <= 0 {
				return fmt.Errorf("版本 %s 的压缩包大小无效", version.Version)
			}
			if len(pkg.SHA256) != sha256.Size*2 {
				return fmt.Errorf("版本 %s 的压缩包缺少有效 SHA-256", version.Version)
			}
			if _, err := hex.DecodeString(pkg.SHA256); err != nil {
				return fmt.Errorf("版本 %s 的压缩包 SHA-256 无效", version.Version)
			}
		}
	}
	return nil
}

func validateIndexVersionFacts(kind string, version IndexVersion) error {
	switch kind {
	case types.ReleaseArtifactKindBox:
		if version.Compatibility != nil {
			return fmt.Errorf("业务端发行不能声明对自身的适用范围")
		}
		if err := release.ValidateVersion(version.DataVersion); err != nil {
			return fmt.Errorf("业务端目标数据版本无效：%w", err)
		}
	case types.ReleaseArtifactKindTool, types.ReleaseArtifactKindPlugin:
		if version.Compatibility == nil {
			return fmt.Errorf("工具和插件必须声明业务端适用范围")
		}
		if err := release.ValidateEucliBoxCompatibility(*version.Compatibility); err != nil {
			return fmt.Errorf("业务端适用范围无效：%w", err)
		}
		if strings.TrimSpace(version.DataVersion) != "" {
			return fmt.Errorf("工具和插件不能声明业务端目标数据版本")
		}
	default:
		return fmt.Errorf("发布物类别无效")
	}
	return nil
}

func validateSourceRevision(value string) error {
	if len(value) != 40 {
		return fmt.Errorf("必须使用完整 Git 提交编号")
	}
	for _, char := range value {
		if char >= '0' && char <= '9' || char >= 'a' && char <= 'f' {
			continue
		}
		return fmt.Errorf("提交编号无效")
	}
	return nil
}

// IndexRawURL 构造固定官方仓库统一版本索引的原始文件地址。
// 读取索引不使用 GitHub REST Contents API，也不调用 Release 列表接口。
func IndexRawURL(base string, source types.OfficialReleaseSource) (string, error) {
	source.Ref = strings.TrimSpace(source.Ref)
	if source.Ref == "" {
		return "", fmt.Errorf("索引引用不能为空")
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = "https://raw.githubusercontent.com"
	}
	return fmt.Sprintf("%s/%s/%s/%s/%s", base, url.PathEscape(source.Owner), url.PathEscape(source.Name), url.PathEscape(source.Ref), IndexPath), nil
}

// ReadIndex 从固定官方仓库读取一份统一版本索引并完成完整校验。
func ReadIndex(ctx context.Context, client HTTPDoer, source types.OfficialReleaseSource, base string) (Index, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		return Index{}, fmt.Errorf("读取统一版本索引需要网络请求能力")
	}
	endpoint, err := IndexRawURL(base, source)
	if err != nil {
		return Index{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Index{}, err
	}
	request.Header.Set("Accept", "application/vnd.github.raw+json")
	request.Header.Set("User-Agent", "eucli-box-release-check/1.0")
	response, err := client.Do(request)
	if err != nil {
		return Index{}, fmt.Errorf("读取 %s 统一版本索引失败：%w", source.Repository, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return Index{}, fmt.Errorf("读取 %s 统一版本索引失败：%w", source.Repository, err)
	}
	if response.StatusCode != http.StatusOK {
		return Index{}, fmt.Errorf("读取 %s 统一版本索引失败：远端返回 %d", source.Repository, response.StatusCode)
	}
	return DecodeIndex(payload)
}

// ArtifactFor 返回索引中指定发布物的记录。
func (i Index) ArtifactFor(kind string, id string) (IndexArtifact, bool) {
	for _, artifact := range i.Artifacts {
		if artifact.Kind == kind && artifact.ID == id {
			return artifact, true
		}
	}
	return IndexArtifact{}, false
}

// LatestVersion 返回索引中该发布物版本号最高的正式版本。
func (i Index) LatestVersion(identity types.ReleaseArtifactIdentity) (IndexVersion, bool) {
	artifact, ok := i.ArtifactFor(identity.Kind, identity.ID)
	if !ok || len(artifact.Versions) == 0 {
		return IndexVersion{}, false
	}
	latest := artifact.Versions[0]
	for _, candidate := range artifact.Versions[1:] {
		order, err := release.CompareVersions(candidate.Version, latest.Version)
		if err == nil && order > 0 {
			latest = candidate
		}
	}
	return latest, true
}

// Version 返回索引中该发布物指定版本的记录。
func (i Index) Version(identity types.ReleaseArtifactIdentity, version string) (IndexVersion, bool) {
	artifact, ok := i.ArtifactFor(identity.Kind, identity.ID)
	if !ok {
		return IndexVersion{}, false
	}
	for _, candidate := range artifact.Versions {
		if candidate.Version == version {
			return candidate, true
		}
	}
	return IndexVersion{}, false
}

// PackageFor 返回版本记录中指定平台的压缩包资料。
func (v IndexVersion) PackageFor(platform string) (IndexPackage, bool) {
	for _, pkg := range v.Packages {
		if pkg.Platform == platform {
			return pkg, true
		}
	}
	return IndexPackage{}, false
}

// DownloadURL 由本地固定官方仓库、releaseTag 和 fileName 构造压缩包直接下载地址。
func DownloadURL(base string, source types.OfficialReleaseSource, pkg IndexPackage) (string, error) {
	if strings.TrimSpace(pkg.ReleaseTag) == "" || strings.TrimSpace(pkg.FileName) == "" {
		return "", fmt.Errorf("压缩包标签或文件名无效")
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = strings.TrimSuffix(source.Repository, "/")
	}
	return fmt.Sprintf("%s/releases/download/%s/%s", base, url.PathEscape(pkg.ReleaseTag), url.PathEscape(pkg.FileName)), nil
}

// SortedArtifactKeys 返回索引中全部发布物的稳定排序身份。
func (i Index) SortedArtifactKeys() []types.ReleaseArtifactIdentity {
	keys := make([]types.ReleaseArtifactIdentity, 0, len(i.Artifacts))
	for _, artifact := range i.Artifacts {
		keys = append(keys, types.ReleaseArtifactIdentity{Kind: artifact.Kind, ID: artifact.ID})
	}
	sort.Slice(keys, func(a int, b int) bool {
		if keys[a].Kind != keys[b].Kind {
			return keys[a].Kind < keys[b].Kind
		}
		return keys[a].ID < keys[b].ID
	})
	return keys
}
