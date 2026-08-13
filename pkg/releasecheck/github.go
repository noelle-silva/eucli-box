package releasecheck

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

// ReleaseCandidate 是已经由官方统一版本索引确认过的可下载事实；
// 不携带任何来自 Release 附属清单的信息。
type ReleaseCandidate struct {
	Artifact         types.ReleaseArtifactIdentity
	Version          string
	PublishedAt      time.Time
	SourceRevision   string
	SourceRepository string
	DataVersion      string
	Compatibility    *types.EucliBoxCompatibility
	ReleaseNotes     string
	OfficialSource   string
	ReleaseURL       string
	ArchiveURL       string
	SizeBytes        int64
	SHA256           string
}

// CandidateReader 是业务端系统读取官方候选的只读接口；工具和插件安装共用同一份。
type CandidateReader interface {
	LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*ReleaseCandidate, error)
}

// PackageSource 把已核对候选转换成可下载事实；不执行网络请求。
func (c ReleaseCandidate) PackageSource() (release.ArtifactPackageSource, error) {
	source := release.ArtifactPackageSource{
		Artifact: c.Artifact,
		Product: types.ReleaseProductRecord{
			SchemaVersion:  release.ReleaseManifestSchemaVersion,
			Artifact:       c.Artifact,
			Version:        c.Version,
			Platform:       types.ReleasePlatformWindowsX64,
			OfficialSource: c.OfficialSource,
			Compatibility:  c.Compatibility,
			Source:         types.ReleaseSourceRecord{Repository: c.SourceRepository, Commit: c.SourceRevision, Recorded: true},
			DataVersion:    c.DataVersion,
		},
		ArchiveURL: c.ArchiveURL,
		SizeBytes:  c.SizeBytes,
		SHA256:     c.SHA256,
	}
	if err := release.ValidateArtifactPackageSource(source); err != nil {
		return release.ArtifactPackageSource{}, err
	}
	return source, nil
}

func (c *Checker) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*ReleaseCandidate, error) {
	if !c.catalog.Contains(identity) {
		return nil, fmt.Errorf("发布物不在正式白名单中")
	}
	source, err := c.catalog.SourceFor(identity.Kind)
	if err != nil {
		return nil, err
	}
	sourceRepository, err := c.catalog.SourceFor(types.ReleaseArtifactKindBox)
	if err != nil {
		return nil, err
	}
	index, err := c.readIndex(ctx, source)
	if err != nil {
		return nil, err
	}
	version, ok := index.LatestVersion(identity)
	if !ok {
		return nil, fmt.Errorf("%s 官方索引没有该发布物的正式版本", identity.ID)
	}
	pkg, ok := version.PackageFor(types.ReleasePlatformWindowsX64)
	if !ok {
		return nil, fmt.Errorf("%s 官方索引没有 %s 平台压缩包", identity.ID, types.ReleasePlatformWindowsX64)
	}
	archiveURL, err := releasecatalog.DownloadURL(c.downloadBase, source, pkg)
	if err != nil {
		return nil, fmt.Errorf("%s 官方索引压缩包地址无效：%w", identity.ID, err)
	}
	releaseURL, err := releaseTagURL(source, pkg.ReleaseTag)
	if err != nil {
		return nil, fmt.Errorf("%s 官方发行页地址无效：%w", identity.ID, err)
	}
	return &ReleaseCandidate{
		Artifact:         identity,
		Version:          version.Version,
		PublishedAt:      version.PublishedAt,
		SourceRevision:   version.SourceRevision,
		SourceRepository: sourceRepository.Repository,
		DataVersion:      version.DataVersion,
		Compatibility:    version.Compatibility,
		ReleaseNotes:     strings.TrimSpace(version.ReleaseNotes),
		OfficialSource:   source.Repository,
		ReleaseURL:       releaseURL,
		ArchiveURL:       archiveURL,
		SizeBytes:        pkg.SizeBytes,
		SHA256:           pkg.SHA256,
	}, nil
}

func (c *Checker) readIndex(ctx context.Context, source types.OfficialReleaseSource) (releasecatalog.Index, error) {
	return releasecatalog.ReadIndex(ctx, c.client, source, c.indexBaseURL)
}

func releaseTagURL(source types.OfficialReleaseSource, tag string) (string, error) {
	if strings.TrimSpace(tag) == "" {
		return "", fmt.Errorf("发行标签不能为空")
	}
	parsed, err := url.Parse(strings.TrimSuffix(source.Repository, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("官方来源地址无效")
	}
	return fmt.Sprintf("%s/releases/tag/%s", strings.TrimSuffix(source.Repository, "/"), url.PathEscape(tag)), nil
}
