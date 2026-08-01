package releasecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

const maxReleasePages = 100

type githubRelease struct {
	TagName    string               `json:"tag_name"`
	Draft      bool                 `json:"draft"`
	Prerelease bool                 `json:"prerelease"`
	HTMLURL    string               `json:"html_url"`
	Body       string               `json:"body"`
	Assets     []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type releaseCandidate struct {
	release  githubRelease
	manifest types.ReleaseManifest
}

func (c *Checker) readRepositoryReleases(ctx context.Context, source types.OfficialReleaseSource) ([]githubRelease, error) {
	source = normalizedSource(c.catalog, source)
	all := make([]githubRelease, 0)
	for page := 1; page <= maxReleasePages; page++ {
		endpoint := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100&page=%d", c.apiBaseURL, url.PathEscape(source.Owner), url.PathEscape(source.Name), page)
		payload, status, err := c.read(ctx, endpoint, "application/vnd.github+json")
		if err != nil {
			return nil, fmt.Errorf("读取 %s 官方发行失败：%w", source.Repository, err)
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("读取 %s 官方发行失败：GitHub 返回 %d", source.Repository, status)
		}
		var pageReleases []githubRelease
		if err := json.Unmarshal(payload, &pageReleases); err != nil {
			return nil, fmt.Errorf("读取 %s 官方发行失败：返回资料无效", source.Repository)
		}
		all = append(all, pageReleases...)
		if len(pageReleases) < 100 {
			return all, nil
		}
	}
	return nil, fmt.Errorf("读取 %s 官方发行失败：发行记录页数超过限制", source.Repository)
}

func (c *Checker) latestCandidate(ctx context.Context, source types.OfficialReleaseSource, identity types.ReleaseArtifactIdentity, releases []githubRelease) (*releaseCandidate, error) {
	var latest *releaseCandidate
	for _, item := range releases {
		if item.Draft || item.Prerelease {
			continue
		}
		version, belongs := versionFromTag(identity, strings.TrimSpace(item.TagName))
		if !belongs {
			continue
		}
		if release.ValidateVersion(version) != nil {
			return nil, fmt.Errorf("%s 官方发行 %q 使用了无效正式版本", identity.ID, item.TagName)
		}
		manifestAsset, err := uniqueManifestAsset(item.Assets)
		if err != nil {
			return nil, fmt.Errorf("%s 官方发行 %q 资料异常：%w", identity.ID, item.TagName, err)
		}
		if err := c.validateAssetURL(source, manifestAsset.BrowserDownloadURL); err != nil {
			return nil, fmt.Errorf("%s 官方发行 %q 的清单地址无效：%w", identity.ID, item.TagName, err)
		}
		payload, status, err := c.read(ctx, manifestAsset.BrowserDownloadURL, "application/octet-stream")
		if err != nil || status != http.StatusOK {
			if err == nil {
				err = fmt.Errorf("远端返回 %d", status)
			}
			return nil, fmt.Errorf("%s 官方发行 %q 的清单无法读取：%w", identity.ID, item.TagName, err)
		}
		manifest, err := release.DecodeReleaseManifest(payload)
		if err != nil {
			return nil, fmt.Errorf("%s 官方发行 %q 的清单无效：%w", identity.ID, item.TagName, err)
		}
		if manifest.VerificationOnly || !manifest.Source.Recorded {
			return nil, fmt.Errorf("%s 官方发行 %q 使用了仅供验证的成品", identity.ID, item.TagName)
		}
		if err := release.ValidateManifestIdentity(manifest, identity, item.TagName, source.Repository); err != nil {
			return nil, fmt.Errorf("%s 官方发行 %q 身份异常：%w", identity.ID, item.TagName, err)
		}
		if manifest.Version != version {
			return nil, fmt.Errorf("%s 官方发行 %q 的版本与标签不一致", identity.ID, item.TagName)
		}
		archiveAsset, err := namedAsset(item.Assets, manifest.Archive.Name)
		if err != nil {
			return nil, fmt.Errorf("%s 官方发行 %q 的成品文件异常：%w", identity.ID, item.TagName, err)
		}
		if archiveAsset.Size != manifest.Archive.Size {
			return nil, fmt.Errorf("%s 官方发行 %q 的成品大小与清单不一致", identity.ID, item.TagName)
		}
		candidate := &releaseCandidate{release: item, manifest: manifest}
		if latest == nil {
			latest = candidate
			continue
		}
		order, err := release.CompareVersions(manifest.Version, latest.manifest.Version)
		if err != nil {
			return nil, err
		}
		if order > 0 {
			latest = candidate
		} else if order == 0 && manifest.Archive.SHA256 != latest.manifest.Archive.SHA256 {
			return nil, fmt.Errorf("%s 同一版本存在不同成品内容", identity.ID)
		}
	}
	return latest, nil
}

func (c *Checker) read(ctx context.Context, endpoint string, accept string) ([]byte, int, error) {
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Accept", accept)
	request.Header.Set("User-Agent", "eucli-box-release-check/1.0")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, response.StatusCode, err
	}
	return payload, response.StatusCode, nil
}

func versionFromTag(identity types.ReleaseArtifactIdentity, tag string) (string, bool) {
	prefix := ""
	switch identity.Kind {
	case types.ReleaseArtifactKindBox:
		prefix = "v"
	case types.ReleaseArtifactKindTool, types.ReleaseArtifactKindPlugin:
		prefix = identity.ID + "/v"
	default:
		return "", false
	}
	if !strings.HasPrefix(tag, prefix) {
		return "", false
	}
	return strings.TrimPrefix(tag, prefix), true
}

func uniqueManifestAsset(assets []githubReleaseAsset) (githubReleaseAsset, error) {
	matches := make([]githubReleaseAsset, 0, 1)
	for _, asset := range assets {
		if strings.HasSuffix(strings.ToLower(strings.TrimSpace(asset.Name)), ".manifest.json") {
			matches = append(matches, asset)
		}
	}
	if len(matches) != 1 {
		return githubReleaseAsset{}, fmt.Errorf("必须且只能包含一份 .manifest.json")
	}
	if strings.TrimSpace(matches[0].BrowserDownloadURL) == "" || matches[0].Size <= 0 {
		return githubReleaseAsset{}, fmt.Errorf("发行清单下载资料无效")
	}
	return matches[0], nil
}

func namedAsset(assets []githubReleaseAsset, name string) (githubReleaseAsset, error) {
	matches := make([]githubReleaseAsset, 0, 1)
	for _, asset := range assets {
		if asset.Name == name {
			matches = append(matches, asset)
		}
	}
	if len(matches) != 1 {
		return githubReleaseAsset{}, fmt.Errorf("必须且只能包含文件 %s", strconv.Quote(name))
	}
	if strings.TrimSpace(matches[0].BrowserDownloadURL) == "" || matches[0].Size <= 0 {
		return githubReleaseAsset{}, fmt.Errorf("成品下载资料无效")
	}
	return matches[0], nil
}

func (c *Checker) validateAssetURL(source types.OfficialReleaseSource, value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("下载地址无效")
	}
	if strings.HasPrefix(c.apiBaseURL, "http://127.0.0.1:") {
		apiURL, _ := url.Parse(c.apiBaseURL)
		if parsed.Scheme == apiURL.Scheme && parsed.Host == apiURL.Host {
			return nil
		}
		return fmt.Errorf("隔离检查地址越过本次官方来源服务")
	}
	prefix := strings.TrimSuffix(source.Repository, "/") + "/releases/download/"
	if !strings.HasPrefix(value, prefix) {
		return fmt.Errorf("下载地址不是固定官方仓库")
	}
	return nil
}
