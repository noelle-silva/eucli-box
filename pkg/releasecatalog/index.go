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

// IndexSchemaVersionStatic 是允许自带下载地址的索引格式版本。
// 版本 2 服务不提供发行标签的普通文件服务器；版本 1 保持官方仓库语义。
const IndexSchemaVersionStatic = 2

// Index 是官方仓库源码中集中维护的统一版本索引。
// 它是一次读取即可获得该仓库全部发布物、全部版本及完整资料的正式事实来源。
type Index struct {
	SchemaVersion int             `json:"schemaVersion"`
	UpdatedAt     time.Time       `json:"updatedAt"`
	DownloadBase  string          `json:"downloadBase,omitempty"`
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
	// URL 是版本 2 索引中该压缩包的完整下载地址；版本 1 必须留空。
	URL string `json:"url,omitempty"`
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
	if index.SchemaVersion != IndexSchemaVersion && index.SchemaVersion != IndexSchemaVersionStatic {
		return fmt.Errorf("统一版本索引 schemaVersion 必须为 %d 或 %d", IndexSchemaVersion, IndexSchemaVersionStatic)
	}
	if index.UpdatedAt.IsZero() {
		return fmt.Errorf("统一版本索引缺少 updatedAt")
	}
	if len(index.Artifacts) == 0 {
		return fmt.Errorf("统一版本索引不能为空")
	}
	if index.SchemaVersion == IndexSchemaVersion && strings.TrimSpace(index.DownloadBase) != "" {
		return fmt.Errorf("版本 %d 的统一版本索引不能声明下载基址", IndexSchemaVersion)
	}
	if err := validateDownloadBase(index.DownloadBase); err != nil {
		return fmt.Errorf("统一版本索引的下载基址无效：%w", err)
	}
	expectedKinds := map[string]struct{}{
		types.ReleaseArtifactKindTool:   {},
		types.ReleaseArtifactKindPlugin: {},
	}
	identities := map[string]struct{}{}
	for _, artifact := range index.Artifacts {
		artifact.Kind = strings.TrimSpace(artifact.Kind)
		artifact.ID = strings.TrimSpace(artifact.ID)
		if _, ok := expectedKinds[artifact.Kind]; !ok {
			return fmt.Errorf("统一版本索引包含未知发布物类别 %q", artifact.Kind)
		}
		if !validID(artifact.ID) {
			return fmt.Errorf("统一版本索引包含无效发布物 ID %q", artifact.ID)
		}
		key := artifact.Kind + ":" + artifact.ID
		if _, exists := identities[key]; exists {
			return fmt.Errorf("统一版本索引包含重复发布物 %s", key)
		}
		identities[key] = struct{}{}
		if err := validateIndexVersions(index, artifact); err != nil {
			return fmt.Errorf("%s 版本资料无效：%w", key, err)
		}
	}
	return nil
}

func validateIndexVersions(index Index, artifact IndexArtifact) error {
	if len(artifact.Versions) == 0 {
		return fmt.Errorf("必须包含至少一条正式版本记录")
	}
	versions := map[string]struct{}{}
	for _, version := range artifact.Versions {
		version.Version = strings.TrimSpace(version.Version)
		version.SourceRevision = strings.TrimSpace(version.SourceRevision)
		version.DataVersion = strings.TrimSpace(version.DataVersion)
		if err := release.ValidateFormalVersion(version.Version); err != nil {
			return fmt.Errorf("版本 %q 必须是三段正式版本：%w", version.Version, err)
		}
		if _, exists := versions[version.Version]; exists {
			return fmt.Errorf("同一发布物不能包含重复版本 %s", version.Version)
		}
		versions[version.Version] = struct{}{}
		if version.PublishedAt.IsZero() {
			return fmt.Errorf("版本 %s 缺少 publishedAt", version.Version)
		}
		if index.SchemaVersion == IndexSchemaVersion {
			if err := validateSourceRevision(version.SourceRevision); err != nil {
				return fmt.Errorf("版本 %s 的源码记录无效：%w", version.Version, err)
			}
		} else if version.SourceRevision != "" {
			if err := validateSourceRevision(version.SourceRevision); err != nil {
				return fmt.Errorf("版本 %s 的源码记录无效：%w", version.Version, err)
			}
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
			if err := validateIndexPackage(index, pkg, version.Version, expectedTag, expectedFile); err != nil {
				return err
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

// validateIndexPackage 按索引格式版本校验单个平台压缩包记录。
// 版本 1 服务代码托管仓库：标签、文件名、下载地址三段事实必须与发布身份一致。
// 版本 2 服务普通文件服务器：允许省略标签与源码提交，但必须能给出下载地址。
func validateIndexPackage(index Index, pkg IndexPackage, versionValue string, expectedTag string, expectedFile string) error {
	if index.SchemaVersion == IndexSchemaVersion {
		if strings.TrimSpace(pkg.URL) != "" {
			return fmt.Errorf("版本 %s 的平台记录不能带下载地址（版本 %d 索引）", versionValue, IndexSchemaVersion)
		}
		if pkg.ReleaseTag != expectedTag {
			return fmt.Errorf("版本 %s 的 Release 标签必须为 %s", versionValue, expectedTag)
		}
	} else {
		if pkg.ReleaseTag != "" && pkg.ReleaseTag != expectedTag {
			return fmt.Errorf("版本 %s 的 Release 标签必须为 %s", versionValue, expectedTag)
		}
		if strings.TrimSpace(pkg.URL) != "" {
			if err := ValidateSourceAddress(pkg.URL); err != nil {
				return fmt.Errorf("版本 %s 的下载地址无效：%w", versionValue, err)
			}
		} else if strings.TrimSpace(index.DownloadBase) == "" {
			return fmt.Errorf("版本 %s 缺少下载地址与下载基址", versionValue)
		}
	}
	if pkg.FileName != expectedFile {
		return fmt.Errorf("版本 %s 的压缩包文件名必须为 %s", versionValue, expectedFile)
	}
	return nil
}

// NewStaticIndexURL 校验并返回静态清单地址：固定 HTTPS、有主机、不带账号信息。
func NewStaticIndexURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if err := ValidateSourceAddress(value); err != nil {
		return "", err
	}
	return value, nil
}

// validateDownloadBase 校验下载基址：空值合法，非空必须是固定 HTTPS 地址。
func validateDownloadBase(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return ValidateSourceAddress(value)
}

// ValidateSourceAddress 校验网络地址：固定 HTTPS；隔离验证允许回环主机的 HTTP。
func ValidateSourceAddress(value string) error {
	return types.ValidateReleaseSourceAddress(value)
}

func validateIndexVersionFacts(kind string, version IndexVersion) error {
	switch kind {
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

// IndexRawURL 构造代码托管仓库统一版本索引的原始文件地址。
// 读取索引不使用仓库接口，也不调用发行列表接口。
func IndexRawURL(base string, repository Repository, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("索引引用不能为空")
	}
	if strings.ContainsAny(ref, " \t\r\n\\") || ref == "." || ref == ".." {
		return "", fmt.Errorf("索引引用无效")
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = "https://raw.githubusercontent.com"
	}
	return fmt.Sprintf("%s/%s/%s/%s/%s", base, url.PathEscape(repository.Owner), url.PathEscape(repository.Name), url.PathEscape(ref), IndexPath), nil
}

// ReadIndex 从代码托管仓库读取一份统一版本索引并完成完整校验。
func ReadIndex(ctx context.Context, client HTTPDoer, repository Repository, ref string, base string) (Index, error) {
	endpoint, err := IndexRawURL(base, repository, ref)
	if err != nil {
		return Index{}, err
	}
	return ReadIndexURL(ctx, client, endpoint, repository.Address)
}

// ReadIndexURL 从固定 HTTPS 地址读取一份统一版本索引并完成完整校验。
func ReadIndexURL(ctx context.Context, client HTTPDoer, endpoint string, displaySource string) (Index, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		return Index{}, fmt.Errorf("读取统一版本索引需要网络请求能力")
	}
	endpoint = strings.TrimSpace(endpoint)
	if err := ValidateSourceAddress(endpoint); err != nil {
		return Index{}, fmt.Errorf("统一版本索引地址无效：%w", err)
	}
	display := strings.TrimSpace(displaySource)
	if display == "" {
		display = endpoint
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Index{}, err
	}
	request.Header.Set("Accept", "application/vnd.github.raw+json")
	request.Header.Set("User-Agent", "eucli-box-release-check/1.0")
	response, err := client.Do(request)
	if err != nil {
		return Index{}, fmt.Errorf("读取 %s 统一版本索引失败：%w", display, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return Index{}, fmt.Errorf("读取 %s 统一版本索引失败：%w", display, err)
	}
	if response.StatusCode != http.StatusOK {
		return Index{}, fmt.Errorf("读取 %s 统一版本索引失败：远端返回 %d", display, response.StatusCode)
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

// DownloadURL 解析压缩包下载地址：索引自带地址优先，其次索引下载基址，
// 最后按代码托管仓库的发行路径拼装。base 非空时覆盖仓库地址。
func DownloadURL(base string, repository string, downloadBase string, pkg IndexPackage) (string, error) {
	if strings.TrimSpace(pkg.URL) != "" {
		return strings.TrimSpace(pkg.URL), nil
	}
	fileName := strings.TrimSpace(pkg.FileName)
	if fileName == "" {
		return "", fmt.Errorf("压缩包文件名无效")
	}
	if strings.TrimSpace(downloadBase) != "" {
		return strings.TrimRight(strings.TrimSpace(downloadBase), "/") + "/" + url.PathEscape(fileName), nil
	}
	tag := strings.TrimSpace(pkg.ReleaseTag)
	if tag == "" {
		return "", fmt.Errorf("压缩包缺少下载地址与发行标签")
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = strings.TrimSuffix(strings.TrimSpace(repository), "/")
	}
	if base == "" {
		return "", fmt.Errorf("压缩包缺少下载基址")
	}
	return fmt.Sprintf("%s/releases/download/%s/%s", base, url.PathEscape(tag), url.PathEscape(fileName)), nil
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
