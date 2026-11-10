package releasecheck

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

// IndexSource 描述候选清单从哪里读取、压缩包如何定位、发布物源码记录指向哪里。
// 官方来源与用户注册的网络来源都由此表达，读取与下载规则只按提供方式分支。
type IndexSource struct {
	Name             string
	Kind             string
	Repository       string
	Ref              string
	IndexURL         string
	RecordRepository string
}

// validate 校验来源事实：代码托管仓库与静态清单各自只允许自己的字段。
func (s IndexSource) validate() error {
	name := strings.TrimSpace(s.Name)
	if name == "" {
		return fmt.Errorf("候选来源缺少名字")
	}
	switch strings.TrimSpace(s.Kind) {
	case types.ReleaseSourceKindGithub:
		if strings.TrimSpace(s.IndexURL) != "" {
			return fmt.Errorf("代码托管来源 %q 不能填写清单地址", name)
		}
		if _, err := releasecatalog.ParseRepository(s.Repository); err != nil {
			return fmt.Errorf("代码托管来源 %q 无效：%w", name, err)
		}
		if strings.TrimSpace(s.Ref) == "" {
			return fmt.Errorf("代码托管来源 %q 缺少固定引用", name)
		}
		return nil
	case types.ReleaseSourceKindRemote:
		if strings.TrimSpace(s.Repository) != "" || strings.TrimSpace(s.Ref) != "" {
			return fmt.Errorf("静态清单来源 %q 不能填写仓库地址或固定引用", name)
		}
		if _, err := releasecatalog.NewStaticIndexURL(s.IndexURL); err != nil {
			return fmt.Errorf("静态清单来源 %q 无效：%w", name, err)
		}
		return nil
	default:
		return fmt.Errorf("候选来源 %q 的提供方式 %q 无效", name, s.Kind)
	}
}

// reference 返回候选事实使用的来源描述。
func (s IndexSource) reference() types.ReleaseSourceRef {
	return types.ReleaseSourceRef{
		Name:       strings.TrimSpace(s.Name),
		Kind:       strings.TrimSpace(s.Kind),
		Repository: strings.TrimSpace(s.Repository),
		IndexURL:   strings.TrimSpace(s.IndexURL),
		Ref:        strings.TrimSpace(s.Ref),
	}
}

// readIndex 读取该来源的统一版本索引。
func (s IndexSource) readIndex(ctx context.Context, client HTTPDoer, indexBase string) (releasecatalog.Index, error) {
	switch strings.TrimSpace(s.Kind) {
	case types.ReleaseSourceKindGithub:
		repository, err := releasecatalog.ParseRepository(s.Repository)
		if err != nil {
			return releasecatalog.Index{}, err
		}
		return releasecatalog.ReadIndex(ctx, client, repository, s.Ref, indexBase)
	case types.ReleaseSourceKindRemote:
		return releasecatalog.ReadIndexURL(ctx, client, s.IndexURL, s.Name)
	default:
		return releasecatalog.Index{}, fmt.Errorf("候选来源 %q 的提供方式无效", s.Name)
	}
}

// downloadURL 解析该来源下某个压缩包的下载地址。
// 网络来源的压缩包必须与清单同一主机：网络门控只放行清单主机。
func (s IndexSource) downloadURL(index releasecatalog.Index, pkg releasecatalog.IndexPackage, downloadBase string) (string, error) {
	value, err := releasecatalog.DownloadURL(downloadBase, s.Repository, index.DownloadBase, pkg)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(s.Kind) != types.ReleaseSourceKindRemote {
		return value, nil
	}
	indexURL, err := url.Parse(strings.TrimSpace(s.IndexURL))
	if err != nil {
		return "", err
	}
	target, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(indexURL.Hostname(), target.Hostname()) {
		return "", fmt.Errorf("压缩包地址必须与清单地址同一主机")
	}
	return value, nil
}

// releaseURL 返回该来源下某个发行标签的发行页地址；静态清单没有发行页。
func (s IndexSource) releaseURL(pkg releasecatalog.IndexPackage) (string, error) {
	if strings.TrimSpace(s.Kind) != types.ReleaseSourceKindGithub {
		return "", nil
	}
	return releaseTagURL(s.Repository, pkg.ReleaseTag)
}

// Reader 按一份来源事实读取候选；自身不保存任何检查结果。
type Reader struct {
	source       IndexSource
	client       HTTPDoer
	indexBase    string
	downloadBase string
}

// NewSourceReader 构造只读候选读取器；同一来源的读取规则只在此维护一份。
func NewSourceReader(source IndexSource, config Config) (*Reader, error) {
	if err := source.validate(); err != nil {
		return nil, err
	}
	normalized, err := config.normalized()
	if err != nil {
		return nil, err
	}
	return &Reader{
		source:       source,
		client:       normalized.Client,
		indexBase:    normalized.IndexBase,
		downloadBase: normalized.DownloadBase,
	}, nil
}

// ListCandidates 读取该来源索引内指定分类全部发布物的候选事实。
// 每个分类只读取一次索引；缺平台压缩包或地址无效的发布物按失败记录。
func (r *Reader) ListCandidates(ctx context.Context, kind string) ([]CandidateRecord, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	kind = strings.TrimSpace(kind)
	index, err := r.source.readIndex(ctx, r.client, r.indexBase)
	if err != nil {
		return nil, err
	}
	records := make([]CandidateRecord, 0, len(index.Artifacts))
	for _, artifact := range index.Artifacts {
		if artifact.Kind != kind {
			continue
		}
		identity := types.ReleaseArtifactIdentity{Kind: artifact.Kind, ID: artifact.ID}
		record := CandidateRecord{Artifact: identity}
		version, ok := index.LatestVersion(identity)
		if !ok {
			// 索引校验保证每个发布物至少有一条版本记录；防御性跳过不产生失败条目。
			continue
		}
		pkg, ok := version.PackageFor(types.ReleasePlatformWindowsX64)
		if !ok {
			record.FailureReason = fmt.Sprintf("%s 索引没有 %s 平台压缩包", identity.ID, types.ReleasePlatformWindowsX64)
			records = append(records, record)
			continue
		}
		candidate, buildErr := r.buildCandidate(index, identity, version, pkg)
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
func (r *Reader) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*ReleaseCandidate, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	index, err := r.source.readIndex(ctx, r.client, r.indexBase)
	if err != nil {
		return nil, err
	}
	version, ok := index.LatestVersion(identity)
	if !ok {
		return nil, fmt.Errorf("%s 索引没有该发布物的正式版本", identity.ID)
	}
	pkg, ok := version.PackageFor(types.ReleasePlatformWindowsX64)
	if !ok {
		return nil, fmt.Errorf("%s 索引没有 %s 平台压缩包", identity.ID, types.ReleasePlatformWindowsX64)
	}
	return r.buildCandidate(index, identity, version, pkg)
}

func (r *Reader) buildCandidate(index releasecatalog.Index, identity types.ReleaseArtifactIdentity, version releasecatalog.IndexVersion, pkg releasecatalog.IndexPackage) (*ReleaseCandidate, error) {
	archiveURL, err := r.source.downloadURL(index, pkg, r.downloadBase)
	if err != nil {
		return nil, fmt.Errorf("%s 索引压缩包地址无效：%w", identity.ID, err)
	}
	releaseURL, err := r.source.releaseURL(pkg)
	if err != nil {
		return nil, fmt.Errorf("%s 发行页地址无效：%w", identity.ID, err)
	}
	// 提交编号脱离源码记录仓库无法定位，不构成源码记录事实。
	sourceRepository := strings.TrimSpace(r.source.RecordRepository)
	sourceRevision := strings.TrimSpace(version.SourceRevision)
	if sourceRepository == "" {
		sourceRevision = ""
	}
	return &ReleaseCandidate{
		Artifact:         identity,
		Version:          version.Version,
		PublishedAt:      version.PublishedAt,
		SourceRevision:   sourceRevision,
		SourceRepository: sourceRepository,
		DataVersion:      version.DataVersion,
		Compatibility:    version.Compatibility,
		ReleaseNotes:     strings.TrimSpace(version.ReleaseNotes),
		Source:           r.source.reference(),
		ReleaseURL:       releaseURL,
		ArchiveURL:       archiveURL,
		SizeBytes:        pkg.SizeBytes,
		SHA256:           pkg.SHA256,
	}, nil
}
