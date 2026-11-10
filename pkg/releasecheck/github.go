package releasecheck

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

// ReleaseCandidate 是已经由统一版本索引确认过的可下载事实；
// 不携带任何来自发行附属清单的信息。
type ReleaseCandidate struct {
	Artifact         types.ReleaseArtifactIdentity
	Version          string
	PublishedAt      time.Time
	SourceRevision   string
	SourceRepository string
	DataVersion      string
	Compatibility    *types.EucliBoxCompatibility
	ReleaseNotes     string
	Source           types.ReleaseSourceRef
	ReleaseURL       string
	ArchiveURL       string
	SizeBytes        int64
	SHA256           string
	// Local 标记候选来自本机货架，取货使用本地文件复制。
	Local bool
}

// PackageSource 把已核对候选转换成可下载事实；不执行网络请求。
func (c ReleaseCandidate) PackageSource() (release.ArtifactPackageSource, error) {
	recorded := strings.TrimSpace(c.SourceRepository) != "" && strings.TrimSpace(c.SourceRevision) != ""
	source := release.ArtifactPackageSource{
		Artifact: c.Artifact,
		Product: types.ReleaseProductRecord{
			SchemaVersion:  release.ReleaseManifestSchemaVersion,
			Artifact:       c.Artifact,
			Version:        c.Version,
			Platform:       types.ReleasePlatformWindowsX64,
			OfficialSource: c.Source.Address(),
			Compatibility:  c.Compatibility,
			Source:         types.ReleaseSourceRecord{Repository: c.SourceRepository, Commit: c.SourceRevision, Recorded: recorded},
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

// releaseTagURL 构造代码托管仓库的发行页地址；静态清单没有发行页。
func releaseTagURL(repository string, tag string) (string, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", nil
	}
	repository = strings.TrimSuffix(strings.TrimSpace(repository), "/")
	parsed, err := url.Parse(repository)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("来源地址无效")
	}
	return fmt.Sprintf("%s/releases/tag/%s", repository, url.PathEscape(tag)), nil
}
