package releasecheck

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

const (
	defaultIndexBase = "https://raw.githubusercontent.com"
	defaultTimeout   = 15 * time.Second
)

type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

type Config struct {
	Client       HTTPDoer
	IndexBase    string
	DownloadBase string
	Timeout      time.Duration
}

// Checker 按需读取官方统一版本索引；自身不保存任何检查结果。
type Checker struct {
	catalog      releasecatalog.Catalog
	client       HTTPDoer
	indexBaseURL string
	downloadBase string
}

func New(config Config) (*Checker, error) {
	catalog, err := releasecatalog.Load()
	if err != nil {
		return nil, err
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: defaultTimeout}
	}
	config.IndexBase = strings.TrimRight(strings.TrimSpace(config.IndexBase), "/")
	if config.IndexBase == "" {
		config.IndexBase = defaultIndexBase
	}
	config.DownloadBase = strings.TrimRight(strings.TrimSpace(config.DownloadBase), "/")
	if config.Timeout == 0 {
		config.Timeout = defaultTimeout
	}
	if config.Timeout < 0 {
		return nil, fmt.Errorf("发行来源读取超时不能为负数")
	}
	return &Checker{
		catalog:      catalog,
		client:       config.Client,
		indexBaseURL: config.IndexBase,
		downloadBase: config.DownloadBase,
	}, nil
}

// CandidateReader 是业务端系统读取单个发布物最新候选的只读接口；安装与更新系统共用。
type CandidateReader interface {
	LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*ReleaseCandidate, error)
}

// CandidateRecord 是某个发布物的一次候选读取结果：
// Candidate 非空表示读取成功；FailureReason 非空表示该发布物在来源中不可用及原因。
type CandidateRecord struct {
	Artifact      types.ReleaseArtifactIdentity
	Candidate     *ReleaseCandidate
	FailureReason string
}

// ListCandidates 按分类读取该分类正式白名单内全部发布物的候选事实。
// 每个分类只读取一次对应官方仓库的统一版本索引；
// 不读取其他分类仓库，不列举 Release，不读取 Release 附属资料。
// 读取成功的发布物给出完整候选；索引中缺该发布物、缺平台压缩包或地址无效的发布物按失败记录。
func (c *Checker) ListCandidates(ctx context.Context, kind string) ([]CandidateRecord, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	kind = strings.TrimSpace(kind)
	whitelist := make([]types.ReleaseArtifactIdentity, 0, len(c.catalog.Artifacts))
	for _, artifact := range c.catalog.Artifacts {
		if artifact.Kind == kind {
			whitelist = append(whitelist, artifact)
		}
	}
	if len(whitelist) == 0 {
		return []CandidateRecord{}, nil
	}
	source, err := c.catalog.SourceFor(kind)
	if err != nil {
		return nil, err
	}
	sourceRepository, err := c.catalog.RecordRepository()
	if err != nil {
		return nil, err
	}
	index, err := c.readIndex(ctx, source)
	if err != nil {
		return nil, err
	}
	records := make([]CandidateRecord, 0, len(whitelist))
	for _, identity := range whitelist {
		record := CandidateRecord{Artifact: identity}
		version, ok := index.LatestVersion(identity)
		if !ok {
			record.FailureReason = fmt.Sprintf("%s 官方索引没有该发布物的正式版本", identity.ID)
			records = append(records, record)
			continue
		}
		pkg, ok := version.PackageFor(types.ReleasePlatformWindowsX64)
		if !ok {
			record.FailureReason = fmt.Sprintf("%s 官方索引没有 %s 平台压缩包", identity.ID, types.ReleasePlatformWindowsX64)
			records = append(records, record)
			continue
		}
		candidate, buildErr := c.buildCandidate(identity, source, sourceRepository, version, pkg)
		if buildErr != nil {
			record.FailureReason = buildErr.Error()
			records = append(records, record)
			continue
		}
		record.Candidate = candidate
		records = append(records, record)
	}
	return records, nil
}

// LatestCandidate 读取单个发布物的最新候选，供安装与更新系统使用。
func (c *Checker) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*ReleaseCandidate, error) {
	if !c.catalog.Contains(identity) {
		return nil, fmt.Errorf("发布物不在正式白名单中")
	}
	source, err := c.catalog.SourceFor(identity.Kind)
	if err != nil {
		return nil, err
	}
	sourceRepository, err := c.catalog.RecordRepository()
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
	candidate, err := c.buildCandidate(identity, source, sourceRepository, version, pkg)
	if err != nil {
		return nil, err
	}
	return candidate, nil
}

func (c *Checker) buildCandidate(identity types.ReleaseArtifactIdentity, source types.OfficialReleaseSource, sourceRepository string, version releasecatalog.IndexVersion, pkg releasecatalog.IndexPackage) (*ReleaseCandidate, error) {
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
		SourceRepository: sourceRepository,
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
