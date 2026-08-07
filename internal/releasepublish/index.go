package releasepublish

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

// IndexUpdate 是一次正式发布需要登记进官方仓库统一版本索引的版本记录事实。
type IndexUpdate struct {
	Artifact       types.ReleaseArtifactIdentity
	Version        string
	PublishedAt    time.Time
	SourceRevision string
	DataVersion    string
	Compatibility  *types.EucliBoxCompatibility
	ReleaseNotes   string
	Package        releasecatalog.IndexPackage
}

// UpdateIndex 把一次正式发布登记进官方仓库源码中的统一版本索引并推送。
// 索引文件不存在时创建；已存在时追加版本记录并更新 updatedAt。
// 更新通过 GitHub Contents API 形成一个正式提交，不使用 Release 列表接口。
func (p *Publisher) UpdateIndex(ctx context.Context, source types.OfficialReleaseSource, update IndexUpdate) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateIndexUpdate(update); err != nil {
		return err
	}
	index, sha, err := p.readIndexContents(ctx, source)
	if err != nil {
		return err
	}
	if index.SchemaVersion == 0 {
		index = releasecatalog.Index{SchemaVersion: releasecatalog.IndexSchemaVersion}
	}
	artifactIndex := -1
	for indexAt := range index.Artifacts {
		if index.Artifacts[indexAt].Kind == update.Artifact.Kind && index.Artifacts[indexAt].ID == update.Artifact.ID {
			artifactIndex = indexAt
			break
		}
	}
	if artifactIndex < 0 {
		index.Artifacts = append(index.Artifacts, releasecatalog.IndexArtifact{Kind: update.Artifact.Kind, ID: update.Artifact.ID, Versions: []releasecatalog.IndexVersion{}})
		artifactIndex = len(index.Artifacts) - 1
	}
	artifact := &index.Artifacts[artifactIndex]
	for _, version := range artifact.Versions {
		if version.Version == update.Version {
			return fmt.Errorf("官方索引已经存在相同发布物和版本：%s", update.Version)
		}
	}
	artifact.Versions = append(artifact.Versions, releasecatalog.IndexVersion{
		Version:        update.Version,
		PublishedAt:    update.PublishedAt.UTC(),
		SourceRevision: update.SourceRevision,
		DataVersion:    update.DataVersion,
		Compatibility:  update.Compatibility,
		ReleaseNotes:   strings.TrimSpace(update.ReleaseNotes),
		Packages:       []releasecatalog.IndexPackage{update.Package},
	})
	index.UpdatedAt = time.Now().UTC()
	if err := releasecatalog.ValidateIndex(index); err != nil {
		return fmt.Errorf("更新后的统一版本索引无效：%w", err)
	}
	payload, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("生成统一版本索引失败：%w", err)
	}
	payload = append(payload, '\n')
	return p.putIndexContents(ctx, source, sha, payload)
}

func validateIndexUpdate(update IndexUpdate) error {
	if err := releasecatalog.ValidateIndex(releasecatalog.Index{
		SchemaVersion: releasecatalog.IndexSchemaVersion,
		UpdatedAt:     update.PublishedAt.UTC(),
		Artifacts: []releasecatalog.IndexArtifact{{
			Kind:     update.Artifact.Kind,
			ID:       update.Artifact.ID,
			Versions: []releasecatalog.IndexVersion{{
				Version:        update.Version,
				PublishedAt:    update.PublishedAt.UTC(),
				SourceRevision: update.SourceRevision,
				DataVersion:    update.DataVersion,
				Compatibility:  update.Compatibility,
				ReleaseNotes:   strings.TrimSpace(update.ReleaseNotes),
				Packages:       []releasecatalog.IndexPackage{update.Package},
			}},
		}},
	}); err != nil {
		return fmt.Errorf("本次发布记录无效：%w", err)
	}
	return nil
}

// readIndexContents 读取官方仓库源码中的统一版本索引内容及其 blob 编号。
// 索引不存在时返回空索引和空编号，表示需要新建。
func (p *Publisher) readIndexContents(ctx context.Context, source types.OfficialReleaseSource) (releasecatalog.Index, string, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", p.apiBaseURL, url.PathEscape(source.Owner), url.PathEscape(source.Name), url.PathEscape(releasecatalog.IndexPath), url.QueryEscape(source.Ref))
	var item struct {
		Content  string `json:"content"`
		SHA      string `json:"sha"`
		Encoding string `json:"encoding"`
	}
	status, err := p.requestContentsJSON(ctx, http.MethodGet, endpoint, nil, &item)
	if err != nil {
		if status == http.StatusNotFound {
			return releasecatalog.Index{}, "", nil
		}
		return releasecatalog.Index{}, "", fmt.Errorf("读取官方索引失败：%w", err)
	}
	if !strings.EqualFold(item.Encoding, "base64") || strings.TrimSpace(item.SHA) == "" {
		return releasecatalog.Index{}, "", fmt.Errorf("官方索引远端资料无效")
	}
	payload, err := base64.StdEncoding.DecodeString(item.Content)
	if err != nil {
		return releasecatalog.Index{}, "", fmt.Errorf("官方索引内容无效：%w", err)
	}
	index, err := releasecatalog.DecodeIndex(payload)
	if err != nil {
		return releasecatalog.Index{}, "", err
	}
	return index, item.SHA, nil
}

// putIndexContents 通过 GitHub Contents API 提交统一版本索引变更。
func (p *Publisher) putIndexContents(ctx context.Context, source types.OfficialReleaseSource, sha string, payload []byte) error {
	body := map[string]any{
		"message": "release: update " + releasecatalog.IndexPath,
		"content": base64.StdEncoding.EncodeToString(payload),
		"branch":  source.Ref,
	}
	if strings.TrimSpace(sha) != "" {
		body["sha"] = sha
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/contents/%s", p.apiBaseURL, url.PathEscape(source.Owner), url.PathEscape(source.Name), url.PathEscape(releasecatalog.IndexPath))
	var item struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if _, err := p.requestJSON(ctx, http.MethodPut, endpoint, body, &item); err != nil {
		return fmt.Errorf("提交官方索引失败：%w", err)
	}
	if strings.TrimSpace(item.Commit.SHA) == "" {
		return fmt.Errorf("GitHub 没有返回索引提交结果")
	}
	return nil
}

func (p *Publisher) requestContentsJSON(ctx context.Context, method string, endpoint string, body any, target any) (int, error) {
	payload, err := json.Marshal(body)
	if err != nil && body != nil {
		return 0, err
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(string(payload)))
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
	defer response.Body.Close()
	raw, err := readResponse(response, maxResponseSize)
	if err != nil {
		return response.StatusCode, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, fmt.Errorf("GitHub 返回 %d", response.StatusCode)
	}
	if target != nil {
		if err := json.Unmarshal(raw, target); err != nil {
			return response.StatusCode, fmt.Errorf("GitHub 返回资料无效")
		}
	}
	return response.StatusCode, nil
}
