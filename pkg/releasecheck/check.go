package releasecheck

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

const (
	defaultAPIBaseURL  = "https://api.github.com"
	defaultIndexBase   = "https://raw.githubusercontent.com"
	defaultTimeout     = 15 * time.Second
)

type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

type InstalledArtifact struct {
	Artifact      types.ReleaseArtifactIdentity
	Version       string
	Compatibility *types.EucliBoxCompatibility
}

type Config struct {
	Client       HTTPDoer
	APIBaseURL   string
	IndexBase    string
	DownloadBase string
	Timeout      time.Duration
	Now          func() time.Time
	Token        string
}

type Checker struct {
	catalog      releasecatalog.Catalog
	client       HTTPDoer
	apiBaseURL   string
	indexBaseURL string
	downloadBase string
	timeout      time.Duration
	now          func() time.Time
	token        string
}

func New(config Config) (*Checker, error) {
	catalog, err := releasecatalog.Load()
	if err != nil {
		return nil, err
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: defaultTimeout}
	}
	config.APIBaseURL = strings.TrimRight(strings.TrimSpace(config.APIBaseURL), "/")
	if config.APIBaseURL == "" {
		config.APIBaseURL = defaultAPIBaseURL
	}
	if !strings.HasPrefix(config.APIBaseURL, "https://") && !strings.HasPrefix(config.APIBaseURL, "http://127.0.0.1:") {
		return nil, fmt.Errorf("发行检查 API 必须使用 GitHub HTTPS 地址")
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
		return nil, fmt.Errorf("发行检查超时不能为负数")
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Checker{
		catalog:      catalog,
		client:       config.Client,
		apiBaseURL:   config.APIBaseURL,
		indexBaseURL: config.IndexBase,
		downloadBase: config.DownloadBase,
		timeout:      config.Timeout,
		now:          config.Now,
		token:        strings.TrimSpace(config.Token),
	}, nil
}

func (c *Checker) Check(ctx context.Context, installed []InstalledArtifact, currentBoxVersion string) types.ReleaseCheckSnapshot {
	return c.CheckOnly(ctx, installed, currentBoxVersion, c.catalog.Artifacts)
}

// CheckOnly 按请求的发布物类别分别读取对应官方仓库的统一版本索引。
// 每个被请求的类别只读取一次索引；不读取其他类别仓库，不列举 Release，不读取 Release 附属资料。
func (c *Checker) CheckOnly(ctx context.Context, installed []InstalledArtifact, currentBoxVersion string, requested []types.ReleaseArtifactIdentity) types.ReleaseCheckSnapshot {
	started := c.now()
	if ctx == nil {
		ctx = context.Background()
	}
	requestedSet := make(map[string]struct{}, len(requested))
	for _, artifact := range requested {
		if c.catalog.Contains(artifact) {
			requestedSet[identityKey(artifact)] = struct{}{}
		}
	}
	installedByIdentity := normalizeInstalled(installed)

	kindRequested := make(map[string]struct{})
	for _, artifact := range c.catalog.Artifacts {
		if _, requested := requestedSet[identityKey(artifact)]; requested {
			kindRequested[artifact.Kind] = struct{}{}
		}
	}
	kindIndexes := make(map[string]releasecatalog.Index, len(kindRequested))
	kindErrors := make(map[string]error, len(kindRequested))
	for kind := range kindRequested {
		source, err := c.catalog.SourceFor(kind)
		if err != nil {
			kindErrors[kind] = err
			continue
		}
		index, err := c.readIndex(ctx, source)
		if err != nil {
			kindErrors[kind] = err
			continue
		}
		kindIndexes[kind] = index
	}

	results := make([]types.ReleaseCheckResult, 0, len(requestedSet))
	for _, artifact := range c.catalog.Artifacts {
		if _, requested := requestedSet[identityKey(artifact)]; !requested {
			continue
		}
		installedArtifact, isInstalled := installedByIdentity[identityKey(artifact)]
		source, sourceErr := c.catalog.SourceFor(artifact.Kind)
		if sourceErr != nil {
			sourceErr = fmt.Errorf("读取 %s 官方来源失败：%w", artifact.Kind, sourceErr)
		}
		result := types.ReleaseCheckResult{
			Artifact:       artifact,
			Source:         source,
			Installed:      isInstalled,
			CurrentVersion: installedArtifact.Version,
			Status:         types.ReleaseCheckStatusCompleted,
			CheckedAt:      started,
		}
		if sourceErr != nil || kindErrors[artifact.Kind] != nil {
			result.Status = types.ReleaseCheckStatusFailed
			if sourceErr != nil {
				result.FailureReason = sourceErr.Error()
			} else {
				result.FailureReason = kindErrors[artifact.Kind].Error()
			}
			results = append(results, result)
			continue
		}
		index := kindIndexes[artifact.Kind]
		version, ok := index.LatestVersion(artifact)
		if !ok {
			result.Status = types.ReleaseCheckStatusFailed
			result.FailureReason = fmt.Sprintf("%s 官方索引没有该发布物的正式版本", artifact.ID)
			results = append(results, result)
			continue
		}
		pkg, ok := version.PackageFor(types.ReleasePlatformWindowsX64)
		if !ok {
			result.Status = types.ReleaseCheckStatusFailed
			result.FailureReason = fmt.Sprintf("%s 官方索引没有 %s 平台压缩包", artifact.ID, types.ReleasePlatformWindowsX64)
			results = append(results, result)
			continue
		}
		result.LatestVersion = version.Version
		result.PublishedAt = version.PublishedAt
		result.IndexUpdatedAt = index.UpdatedAt
		result.ReleaseNotes = strings.TrimSpace(version.ReleaseNotes)
		result.DownloadSize = pkg.SizeBytes
		if releaseURL, urlErr := releaseTagURL(source, pkg.ReleaseTag); urlErr == nil {
			result.ReleaseURL = releaseURL
		}
		if isInstalled {
			order, compareErr := release.CompareVersions(version.Version, installedArtifact.Version)
			if compareErr != nil {
				result.Status = types.ReleaseCheckStatusFailed
				result.FailureReason = compareErr.Error()
				results = append(results, result)
				continue
			}
			result.UpdateAvailable = order > 0
		} else {
			result.UpdateAvailable = true
		}
		if version.Compatibility != nil && strings.TrimSpace(currentBoxVersion) != "" {
			status := release.AssessEucliBoxCompatibility(version.Version, currentBoxVersion, *version.Compatibility)
			result.Compatibility = &status
		}
		results = append(results, result)
	}
	sort.Slice(results, func(i int, j int) bool {
		if results[i].Artifact.Kind != results[j].Artifact.Kind {
			return results[i].Artifact.Kind < results[j].Artifact.Kind
		}
		return results[i].Artifact.ID < results[j].Artifact.ID
	})
	annotateBoxImpact(results, installedByIdentity)
	status := types.ReleaseCheckStatusCompleted
	for _, result := range results {
		if result.Status == types.ReleaseCheckStatusFailed {
			status = types.ReleaseCheckStatusFailed
			break
		}
	}
	return types.ReleaseCheckSnapshot{Status: status, StartedAt: started, CheckedAt: c.now(), Results: results}
}

func PendingSnapshot() types.ReleaseCheckSnapshot {
	return types.ReleaseCheckSnapshot{Status: types.ReleaseCheckStatusNotChecked, Results: []types.ReleaseCheckResult{}}
}

func CheckingSnapshot(previous types.ReleaseCheckSnapshot, started time.Time) types.ReleaseCheckSnapshot {
	return types.ReleaseCheckSnapshot{Status: types.ReleaseCheckStatusChecking, StartedAt: started.UTC(), CheckedAt: previous.CheckedAt, Results: append([]types.ReleaseCheckResult(nil), previous.Results...)}
}

func normalizeInstalled(installed []InstalledArtifact) map[string]InstalledArtifact {
	result := make(map[string]InstalledArtifact, len(installed))
	for _, item := range installed {
		item.Artifact.Kind = strings.TrimSpace(item.Artifact.Kind)
		item.Artifact.ID = strings.TrimSpace(item.Artifact.ID)
		item.Version = strings.TrimSpace(item.Version)
		if item.Artifact.Kind == "" || item.Artifact.ID == "" || release.ValidateVersion(item.Version) != nil {
			continue
		}
		result[identityKey(item.Artifact)] = item
	}
	return result
}

func annotateBoxImpact(results []types.ReleaseCheckResult, installed map[string]InstalledArtifact) {
	for index := range results {
		result := &results[index]
		if result.Artifact.Kind != types.ReleaseArtifactKindBox || !result.UpdateAvailable || release.ValidateVersion(result.LatestVersion) != nil {
			continue
		}
		for _, item := range installed {
			if item.Artifact.Kind != types.ReleaseArtifactKindTool && item.Artifact.Kind != types.ReleaseArtifactKindPlugin || item.Compatibility == nil {
				continue
			}
			status := release.AssessEucliBoxCompatibility(item.Version, result.LatestVersion, *item.Compatibility)
			if !status.Compatible {
				result.AffectedArtifacts = append(result.AffectedArtifacts, item.Artifact)
			}
		}
		sort.Slice(result.AffectedArtifacts, func(i int, j int) bool {
			if result.AffectedArtifacts[i].Kind != result.AffectedArtifacts[j].Kind {
				return result.AffectedArtifacts[i].Kind < result.AffectedArtifacts[j].Kind
			}
			return result.AffectedArtifacts[i].ID < result.AffectedArtifacts[j].ID
		})
	}
}

func normalizedSource(catalog releasecatalog.Catalog, source types.OfficialReleaseSource) types.OfficialReleaseSource {
	normalized, err := catalog.SourceFor(source.Kind)
	if err == nil {
		return normalized
	}
	return source
}

func identityKey(identity types.ReleaseArtifactIdentity) string {
	return identity.Kind + ":" + identity.ID
}
