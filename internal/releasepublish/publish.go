package releasepublish

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

const (
	defaultAPIBaseURL = "https://api.github.com"
	requestTimeout    = 10 * time.Minute
	maxResponseSize   = 8 << 20
)

type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

type Config struct {
	Client     HTTPDoer
	APIBaseURL string
	Token      string
}

type Publisher struct {
	catalog    releasecatalog.Catalog
	client     HTTPDoer
	apiBaseURL string
	token      string
}

type PublishInput struct {
	Manifest     types.ReleaseManifest
	ArchivePath  string
	ManifestPath string
	NotesPath    string
}

type Result struct {
	Artifact   types.ReleaseArtifactIdentity `json:"artifact"`
	Version    string                        `json:"version"`
	TagName    string                        `json:"tagName"`
	Repository string                        `json:"repository"`
	ReleaseID  int64                         `json:"releaseId"`
	ReleaseURL string                        `json:"releaseUrl"`
	Assets     []types.ReleaseFileRecord     `json:"assets"`
}

func New(config Config) (*Publisher, error) {
	catalog, err := releasecatalog.Load()
	if err != nil {
		return nil, err
	}
	apiBaseURL := strings.TrimRight(strings.TrimSpace(config.APIBaseURL), "/")
	if apiBaseURL == "" {
		apiBaseURL = defaultAPIBaseURL
	}
	if apiBaseURL != defaultAPIBaseURL && !strings.HasPrefix(apiBaseURL, "http://127.0.0.1:") {
		return nil, fmt.Errorf("GitHub 发布 API 必须使用固定官方地址")
	}
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	return &Publisher{catalog: catalog, client: client, apiBaseURL: apiBaseURL, token: strings.TrimSpace(config.Token)}, nil
}

func (p *Publisher) Publish(ctx context.Context, input PublishInput) (result Result, err error) {
	if ctx == nil {
		return Result{}, fmt.Errorf("发布上下文不能为空")
	}
	if p.token == "" {
		return Result{}, fmt.Errorf("缺少 GitHub 发布凭据")
	}
	prepared, err := p.prepareInput(input)
	if err != nil {
		return Result{}, err
	}
	if existing, findErr := p.findReleaseByTag(ctx, prepared.source, input.Manifest.TagName); findErr != nil {
		return Result{}, findErr
	} else if existing != nil {
		return Result{}, fmt.Errorf("官方来源已经存在相同发布物和版本：%s", input.Manifest.TagName)
	}

	draft, err := p.createDraft(ctx, prepared.source, input.Manifest, prepared.notes)
	if err != nil {
		return Result{}, err
	}
	result = Result{
		Artifact:   input.Manifest.Artifact,
		Version:    input.Manifest.Version,
		TagName:    input.Manifest.TagName,
		Repository: prepared.source.Repository,
		ReleaseID:  draft.ID,
		ReleaseURL: draft.HTMLURL,
	}
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w；未公开发行已保留：%s", err, result.ReleaseURL)
		}
	}()

	for _, asset := range prepared.assets {
		if _, err = p.uploadAsset(ctx, draft.UploadURL, asset); err != nil {
			return result, err
		}
	}
	remoteDraft, err := p.getRelease(ctx, prepared.source, draft.ID)
	if err != nil {
		return result, err
	}
	if err = validateRemoteRelease(remoteDraft, input.Manifest.TagName, prepared.notes, true, prepared.assets); err != nil {
		return result, err
	}
	if err = p.verifyRemoteAssets(ctx, remoteDraft, prepared.assets); err != nil {
		return result, err
	}
	if _, err = p.setDraftState(ctx, prepared.source, draft.ID, false); err != nil {
		return result, err
	}
	publicRelease, err := p.getRelease(ctx, prepared.source, draft.ID)
	if err != nil {
		return result, err
	}
	if err = validateRemoteRelease(publicRelease, input.Manifest.TagName, prepared.notes, false, prepared.assets); err != nil {
		return result, err
	}
	if err = p.verifyRemoteAssets(ctx, publicRelease, prepared.assets); err != nil {
		return result, err
	}
	result.ReleaseURL = publicRelease.HTMLURL
	result.Assets = make([]types.ReleaseFileRecord, 0, len(prepared.assets))
	for _, asset := range prepared.assets {
		result.Assets = append(result.Assets, asset.Record)
	}
	return result, nil
}

type preparedInput struct {
	source types.OfficialReleaseSource
	notes  string
	assets []localAsset
}

type localAsset struct {
	Path   string
	Record types.ReleaseFileRecord
}

func (p *Publisher) prepareInput(input PublishInput) (preparedInput, error) {
	manifest := input.Manifest
	if err := release.ValidateReleaseManifest(manifest); err != nil {
		return preparedInput{}, err
	}
	if manifest.VerificationOnly || !manifest.Source.Recorded {
		return preparedInput{}, fmt.Errorf("仅供验证或未进入源码记录的成品不能正式发布")
	}
	if !p.catalog.Contains(manifest.Artifact) {
		return preparedInput{}, fmt.Errorf("发布物不在正式白名单中")
	}
	source, err := p.catalog.SourceFor(manifest.Artifact.Kind)
	if err != nil {
		return preparedInput{}, err
	}
	expectedTag, err := releasecatalog.TagName(manifest.Artifact, manifest.Version)
	if err != nil {
		return preparedInput{}, err
	}
	if err := release.ValidateManifestIdentity(manifest, manifest.Artifact, expectedTag, source.Repository); err != nil {
		return preparedInput{}, err
	}
	archivePath, archivePayload, err := readRegularFile(input.ArchivePath, manifest.Archive.Name)
	if err != nil {
		return preparedInput{}, err
	}
	if err := release.ValidateArchiveDigest(manifest, archivePayload); err != nil {
		return preparedInput{}, err
	}
	manifestPath, manifestPayload, err := readRegularFile(input.ManifestPath, strings.TrimSuffix(manifest.Archive.Name, ".zip")+".manifest.json")
	if err != nil {
		return preparedInput{}, err
	}
	decoded, err := release.DecodeReleaseManifest(manifestPayload)
	if err != nil {
		return preparedInput{}, err
	}
	if !reflect.DeepEqual(decoded, manifest) {
		return preparedInput{}, fmt.Errorf("待上传发行清单与已验收成品不一致")
	}
	_, notesPayload, err := readRegularFile(input.NotesPath, "release-notes.md")
	if err != nil {
		return preparedInput{}, err
	}
	notes := strings.TrimSpace(string(notesPayload))
	if notes == "" {
		return preparedInput{}, fmt.Errorf("正式发行说明不能为空")
	}
	manifestRecord := types.ReleaseFileRecord{Name: filepath.Base(manifestPath), Size: int64(len(manifestPayload)), SHA256: release.SHA256(manifestPayload)}
	assets := []localAsset{{Path: archivePath, Record: manifest.Archive}, {Path: manifestPath, Record: manifestRecord}}
	return preparedInput{source: source, notes: notes, assets: assets}, nil
}

func readRegularFile(path string, expectedName string) (string, []byte, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil || strings.TrimSpace(path) == "" {
		return "", nil, fmt.Errorf("待发布文件路径无效")
	}
	if filepath.Base(absolute) != expectedName {
		return "", nil, fmt.Errorf("待发布文件名必须为 %s", expectedName)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", nil, err
	}
	if info.IsDir() {
		return "", nil, fmt.Errorf("待发布文件不能是目录")
	}
	payload, err := os.ReadFile(absolute)
	if err != nil {
		return "", nil, err
	}
	return absolute, payload, nil
}

type githubRelease struct {
	ID         int64                `json:"id"`
	TagName    string               `json:"tag_name"`
	Draft      bool                 `json:"draft"`
	Prerelease bool                 `json:"prerelease"`
	HTMLURL    string               `json:"html_url"`
	UploadURL  string               `json:"upload_url"`
	Body       string               `json:"body"`
	Assets     []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	URL                string `json:"url"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func (p *Publisher) findReleaseByTag(ctx context.Context, source types.OfficialReleaseSource, tag string) (*githubRelease, error) {
	for page := 1; page <= 100; page++ {
		endpoint := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100&page=%d", p.apiBaseURL, url.PathEscape(source.Owner), url.PathEscape(source.Name), page)
		var releases []githubRelease
		if _, err := p.requestJSON(ctx, http.MethodGet, endpoint, nil, &releases); err != nil {
			return nil, fmt.Errorf("读取官方发行记录失败：%w", err)
		}
		for index := range releases {
			if releases[index].TagName == tag {
				return &releases[index], nil
			}
		}
		if len(releases) < 100 {
			return nil, nil
		}
	}
	return nil, fmt.Errorf("官方发行记录页数超过限制")
}

func (p *Publisher) createDraft(ctx context.Context, source types.OfficialReleaseSource, manifest types.ReleaseManifest, notes string) (githubRelease, error) {
	payload := map[string]any{
		"tag_name":   manifest.TagName,
		"name":       manifest.Artifact.ID + " v" + manifest.Version,
		"body":       notes,
		"draft":      true,
		"prerelease": false,
	}
	if manifest.Artifact.Kind == types.ReleaseArtifactKindBox {
		payload["target_commitish"] = manifest.Source.Commit
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/releases", p.apiBaseURL, url.PathEscape(source.Owner), url.PathEscape(source.Name))
	var created githubRelease
	if _, err := p.requestJSON(ctx, http.MethodPost, endpoint, payload, &created); err != nil {
		return githubRelease{}, fmt.Errorf("建立未公开发行失败：%w", err)
	}
	if created.ID <= 0 || !created.Draft || created.TagName != manifest.TagName || strings.TrimSpace(created.UploadURL) == "" {
		return githubRelease{}, fmt.Errorf("GitHub 没有返回有效的未公开发行")
	}
	return created, nil
}

func (p *Publisher) uploadAsset(ctx context.Context, uploadTemplate string, asset localAsset) (githubReleaseAsset, error) {
	uploadURL := strings.Split(strings.TrimSpace(uploadTemplate), "{")[0]
	parsed, err := url.Parse(uploadURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return githubReleaseAsset{}, fmt.Errorf("GitHub 上传地址无效")
	}
	query := parsed.Query()
	query.Set("name", asset.Record.Name)
	parsed.RawQuery = query.Encode()
	file, err := os.Open(asset.Path)
	if err != nil {
		return githubReleaseAsset{}, err
	}
	defer file.Close()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), file)
	if err != nil {
		return githubReleaseAsset{}, err
	}
	request.ContentLength = asset.Record.Size
	request.Header.Set("Content-Type", "application/octet-stream")
	p.setHeaders(request, "application/vnd.github+json")
	response, err := p.client.Do(request)
	if err != nil {
		return githubReleaseAsset{}, fmt.Errorf("上传 %s 失败：%w", asset.Record.Name, err)
	}
	payload, readErr := readResponse(response, maxResponseSize)
	if readErr != nil {
		return githubReleaseAsset{}, readErr
	}
	if response.StatusCode != http.StatusCreated {
		return githubReleaseAsset{}, fmt.Errorf("上传 %s 失败：GitHub 返回 %d", asset.Record.Name, response.StatusCode)
	}
	var uploaded githubReleaseAsset
	if err := json.Unmarshal(payload, &uploaded); err != nil || uploaded.ID <= 0 || uploaded.Name != asset.Record.Name || uploaded.Size != asset.Record.Size {
		return githubReleaseAsset{}, fmt.Errorf("GitHub 返回的上传结果与 %s 不一致", asset.Record.Name)
	}
	return uploaded, nil
}

func (p *Publisher) getRelease(ctx context.Context, source types.OfficialReleaseSource, id int64) (githubRelease, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/releases/%d", p.apiBaseURL, url.PathEscape(source.Owner), url.PathEscape(source.Name), id)
	var item githubRelease
	if _, err := p.requestJSON(ctx, http.MethodGet, endpoint, nil, &item); err != nil {
		return githubRelease{}, fmt.Errorf("重新读取远端发行失败：%w", err)
	}
	return item, nil
}

func (p *Publisher) setDraftState(ctx context.Context, source types.OfficialReleaseSource, id int64, draft bool) (githubRelease, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/releases/%d", p.apiBaseURL, url.PathEscape(source.Owner), url.PathEscape(source.Name), id)
	var item githubRelease
	if _, err := p.requestJSON(ctx, http.MethodPatch, endpoint, map[string]any{"draft": draft}, &item); err != nil {
		return githubRelease{}, fmt.Errorf("公开正式发行失败：%w", err)
	}
	if item.Draft != draft {
		return githubRelease{}, fmt.Errorf("GitHub 没有按要求更新公开状态")
	}
	return item, nil
}

func validateRemoteRelease(remote githubRelease, tag string, notes string, draft bool, local []localAsset) error {
	if remote.TagName != tag || remote.Draft != draft || remote.Prerelease || strings.TrimSpace(remote.Body) != strings.TrimSpace(notes) {
		return fmt.Errorf("远端发行身份、状态或说明与本地不一致")
	}
	if len(remote.Assets) != len(local) {
		return fmt.Errorf("远端发行文件数量与本地不一致")
	}
	for _, expected := range local {
		matches := 0
		for _, actual := range remote.Assets {
			if actual.Name == expected.Record.Name {
				matches++
				if actual.Size != expected.Record.Size || actual.ID <= 0 || strings.TrimSpace(actual.URL) == "" {
					return fmt.Errorf("远端发行文件 %s 的资料不一致", actual.Name)
				}
			}
		}
		if matches != 1 {
			return fmt.Errorf("远端发行必须且只能包含一份 %s", expected.Record.Name)
		}
	}
	return nil
}

func (p *Publisher) verifyRemoteAssets(ctx context.Context, remote githubRelease, local []localAsset) error {
	for _, expected := range local {
		var actual githubReleaseAsset
		for _, candidate := range remote.Assets {
			if candidate.Name == expected.Record.Name {
				actual = candidate
				break
			}
		}
		payload, err := p.downloadAsset(ctx, actual.URL, expected.Record.Size)
		if err != nil {
			return fmt.Errorf("重新下载 %s 失败：%w", expected.Record.Name, err)
		}
		if int64(len(payload)) != expected.Record.Size || !strings.EqualFold(release.SHA256(payload), expected.Record.SHA256) {
			return fmt.Errorf("重新下载的 %s 与本地已验收文件不一致", expected.Record.Name)
		}
	}
	return nil
}

func (p *Publisher) downloadAsset(ctx context.Context, endpoint string, expectedSize int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	p.setHeaders(request, "application/octet-stream")
	response, err := p.client.Do(request)
	if err != nil {
		return nil, err
	}
	payload, readErr := readResponse(response, expectedSize+1)
	if readErr != nil {
		return nil, readErr
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 返回 %d", response.StatusCode)
	}
	return payload, nil
}

func (p *Publisher) requestJSON(ctx context.Context, method string, endpoint string, body any, target any) (int, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return 0, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	p.setHeaders(request, "application/vnd.github+json")
	response, err := p.client.Do(request)
	if err != nil {
		return 0, err
	}
	payload, readErr := readResponse(response, maxResponseSize)
	if readErr != nil {
		return response.StatusCode, readErr
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, fmt.Errorf("GitHub 返回 %d", response.StatusCode)
	}
	if target != nil {
		if err := json.Unmarshal(payload, target); err != nil {
			return response.StatusCode, fmt.Errorf("GitHub 返回资料无效")
		}
	}
	return response.StatusCode, nil
}

func (p *Publisher) setHeaders(request *http.Request, accept string) {
	request.Header.Set("Accept", accept)
	if p.token != "" {
		request.Header.Set("Authorization", "Bearer "+p.token)
	}
	request.Header.Set("User-Agent", "eucli-box-release-publisher/1.0")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

func readResponse(response *http.Response, limit int64) ([]byte, error) {
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, limit))
	if err != nil {
		return nil, err
	}
	return payload, nil
}
