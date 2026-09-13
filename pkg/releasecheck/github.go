package releasecheck

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"eucli-box/pkg/release"
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
	// Local 标记候选来自本地商店货架（本地源）。
	Local bool
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
