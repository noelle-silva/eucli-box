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

// normalized 归一化读取配置；缺省值只在此处定义一次。
func (c Config) normalized() (Config, error) {
	if c.Client == nil {
		c.Client = &http.Client{Timeout: defaultTimeout}
	}
	c.IndexBase = strings.TrimRight(strings.TrimSpace(c.IndexBase), "/")
	if c.IndexBase == "" {
		c.IndexBase = defaultIndexBase
	}
	c.DownloadBase = strings.TrimRight(strings.TrimSpace(c.DownloadBase), "/")
	if c.Timeout == 0 {
		c.Timeout = defaultTimeout
	}
	if c.Timeout < 0 {
		return Config{}, fmt.Errorf("发行来源读取超时不能为负数")
	}
	return c, nil
}

// Checker 按类别持有内嵌官方来源的读取器；自身不保存任何检查结果。
type Checker struct {
	readers map[string]*Reader
}

// New 装配官方来源读取器；官方来源是内嵌固定事实，不从注册表读取。
func New(config Config) (*Checker, error) {
	sources, err := releasecatalog.LoadSources()
	if err != nil {
		return nil, err
	}
	recordRepository, err := sources.RecordRepository()
	if err != nil {
		return nil, err
	}
	readers := make(map[string]*Reader, len(sources.Sources))
	for _, kind := range []string{types.ReleaseArtifactKindTool, types.ReleaseArtifactKindPlugin} {
		official, err := sources.SourceFor(kind)
		if err != nil {
			return nil, err
		}
		repository, err := releasecatalog.ParseRepository(official.Repository)
		if err != nil {
			return nil, err
		}
		reader, err := NewSourceReader(IndexSource{
			Name:             types.ReleaseSourceOfficialName,
			Kind:             types.ReleaseSourceKindGithub,
			Repository:       repository.Address,
			Ref:              official.Ref,
			RecordRepository: recordRepository,
		}, config)
		if err != nil {
			return nil, err
		}
		readers[kind] = reader
	}
	return &Checker{readers: readers}, nil
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

// ListCandidates 按分类读取官方索引内该分类全部实际发布物的候选事实。
func (c *Checker) ListCandidates(ctx context.Context, kind string) ([]CandidateRecord, error) {
	reader, ok := c.readers[strings.TrimSpace(kind)]
	if !ok {
		return []CandidateRecord{}, nil
	}
	return reader.ListCandidates(ctx, kind)
}

// LatestCandidate 读取单个发布物的最新候选，供安装与更新系统使用。
func (c *Checker) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*ReleaseCandidate, error) {
	reader, ok := c.readers[strings.TrimSpace(identity.Kind)]
	if !ok {
		return nil, fmt.Errorf("发布物类别 %q 没有内嵌官方来源", identity.Kind)
	}
	return reader.LatestCandidate(ctx, identity)
}
