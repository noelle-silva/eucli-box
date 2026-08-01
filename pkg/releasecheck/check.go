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
	defaultAPIBaseURL = "https://api.github.com"
	defaultTimeout    = 15 * time.Second
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
	Client     HTTPDoer
	APIBaseURL string
	Timeout    time.Duration
	Now        func() time.Time
	Token      string
}

type Checker struct {
	catalog    releasecatalog.Catalog
	client     HTTPDoer
	apiBaseURL string
	timeout    time.Duration
	now        func() time.Time
	token      string
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
		catalog:    catalog,
		client:     config.Client,
		apiBaseURL: config.APIBaseURL,
		timeout:    config.Timeout,
		now:        config.Now,
		token:      strings.TrimSpace(config.Token),
	}, nil
}

func (c *Checker) Check(ctx context.Context, installed []InstalledArtifact, currentBoxVersion string) types.ReleaseCheckSnapshot {
	return c.CheckOnly(ctx, installed, currentBoxVersion, c.catalog.Artifacts)
}

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
	results := make([]types.ReleaseCheckResult, 0, len(requestedSet))
	for _, source := range c.catalog.Sources {
		if !sourceRequested(source.Kind, requestedSet) {
			continue
		}
		releases, err := c.readRepositoryReleases(ctx, source)
		for _, artifact := range c.catalog.Artifacts {
			if artifact.Kind != source.Kind {
				continue
			}
			if _, requested := requestedSet[identityKey(artifact)]; !requested {
				continue
			}
			installedArtifact, isInstalled := installedByIdentity[identityKey(artifact)]
			result := types.ReleaseCheckResult{
				Artifact:       artifact,
				Source:         normalizedSource(c.catalog, source),
				Installed:      isInstalled,
				CurrentVersion: installedArtifact.Version,
				Status:         types.ReleaseCheckStatusCompleted,
				CheckedAt:      started,
			}
			if err != nil {
				result.Status = types.ReleaseCheckStatusFailed
				result.FailureReason = err.Error()
				results = append(results, result)
				continue
			}
			candidate, candidateErr := c.latestCandidate(ctx, source, artifact, releases)
			if candidateErr != nil {
				result.Status = types.ReleaseCheckStatusFailed
				result.FailureReason = candidateErr.Error()
				results = append(results, result)
				continue
			}
			if candidate != nil {
				result.LatestVersion = candidate.manifest.Version
				result.ReleaseURL = candidate.release.HTMLURL
				result.ReleaseNotes = strings.TrimSpace(candidate.release.Body)
				result.DownloadSize = candidate.manifest.Archive.Size
				if isInstalled {
					order, compareErr := release.CompareVersions(candidate.manifest.Version, installedArtifact.Version)
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
				if candidate.manifest.Compatibility != nil && strings.TrimSpace(currentBoxVersion) != "" {
					status := release.AssessEucliBoxCompatibility(candidate.manifest.Version, currentBoxVersion, *candidate.manifest.Compatibility)
					result.Compatibility = &status
				}
			}
			results = append(results, result)
		}
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

func sourceRequested(kind string, requested map[string]struct{}) bool {
	for key := range requested {
		if strings.HasPrefix(key, kind+":") {
			return true
		}
	}
	return false
}
