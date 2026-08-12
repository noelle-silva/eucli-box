package releasepublish

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

// readIndex 从官方仓库源码读取统一版本索引；隔离验证环境使用同源模拟服务。
func (p *Publisher) readIndex(ctx context.Context, source types.OfficialReleaseSource) (releasecatalog.Index, error) {
	return releasecatalog.ReadIndex(ctx, p.client, source, p.indexBase())
}

// indexBase 返回读取索引与构造下载地址的基础地址；隔离验证环境使用同源模拟服务。
func (p *Publisher) indexBase() string {
	if strings.HasPrefix(p.apiBaseURL, "http://127.0.0.1:") {
		return strings.TrimRight(p.apiBaseURL, "/")
	}
	return ""
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

type DownloadResult struct {
	Product     types.ReleaseProductRecord
	ArchivePath string
	ReleaseURL  string
}

// DownloadPublished 从官方仓库的统一版本索引取得指定发布物的指定版本事实，
// 只下载该版本的一个目标压缩包并完成大小与 SHA-256 核对；
// 不列举 Release、不读取 Release 附属资料。
func (p *Publisher) DownloadPublished(ctx context.Context, identity types.ReleaseArtifactIdentity, version string, targetRoot string) (DownloadResult, error) {
	if !p.catalog.Contains(identity) {
		return DownloadResult{}, fmt.Errorf("发布物不在正式白名单中")
	}
	source, err := p.catalog.SourceFor(identity.Kind)
	if err != nil {
		return DownloadResult{}, err
	}
	sourceRepository, err := p.catalog.SourceFor(types.ReleaseArtifactKindBox)
	if err != nil {
		return DownloadResult{}, err
	}
	index, err := p.readIndex(ctx, source)
	if err != nil {
		return DownloadResult{}, err
	}
	record, ok := index.Version(identity, version)
	if !ok {
		return DownloadResult{}, fmt.Errorf("官方索引没有发布物 %s 的正式版本 %s", releasecatalog.Target(identity), version)
	}
	pkg, ok := record.PackageFor(types.ReleasePlatformWindowsX64)
	if !ok {
		return DownloadResult{}, fmt.Errorf("官方索引没有 %s 的 %s 平台压缩包", releasecatalog.Target(identity), types.ReleasePlatformWindowsX64)
	}
	archiveURL, err := releasecatalog.DownloadURL(p.indexBase(), source, pkg)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("官方索引压缩包地址无效：%w", err)
	}
	releaseURL, err := releaseTagURL(source, pkg.ReleaseTag)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("官方发行页地址无效：%w", err)
	}
	archivePayload, err := p.downloadAsset(ctx, archiveURL, pkg.SizeBytes)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("下载 %s 失败：%w", pkg.FileName, err)
	}
	if int64(len(archivePayload)) != pkg.SizeBytes || !strings.EqualFold(release.SHA256(archivePayload), pkg.SHA256) {
		return DownloadResult{}, fmt.Errorf("下载的 %s 与官方索引不一致", pkg.FileName)
	}
	targetRoot, err = filepath.Abs(strings.TrimSpace(targetRoot))
	if err != nil || strings.TrimSpace(targetRoot) == "" {
		return DownloadResult{}, fmt.Errorf("远端复核目录无效")
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return DownloadResult{}, err
	}
	archivePath := filepath.Join(targetRoot, pkg.FileName)
	if err := os.WriteFile(archivePath, archivePayload, 0o644); err != nil {
		return DownloadResult{}, err
	}
	product := types.ReleaseProductRecord{
		SchemaVersion:  release.ReleaseManifestSchemaVersion,
		Artifact:       identity,
		Version:        record.Version,
		Platform:       types.ReleasePlatformWindowsX64,
		OfficialSource: source.Repository,
		Compatibility:  record.Compatibility,
		Source:         types.ReleaseSourceRecord{Repository: sourceRepository.Repository, Commit: record.SourceRevision, Recorded: true},
		DataVersion:    record.DataVersion,
	}
	return DownloadResult{Product: product, ArchivePath: archivePath, ReleaseURL: releaseURL}, nil
}
