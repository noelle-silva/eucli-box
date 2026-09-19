package releasepublish

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

// DelistInput 是一次正式下架的目标：Version 为空表示下架整个发布物（全部版本）。
type DelistInput struct {
	Artifact types.ReleaseArtifactIdentity
	Version  string
}

// DelistResult 是一次正式下架的结果事实。
type DelistResult struct {
	Artifact        types.ReleaseArtifactIdentity `json:"artifact"`
	Version         string                        `json:"version,omitempty"`
	Repository      string                        `json:"repository"`
	IndexRemoved    bool                          `json:"indexRemoved"`
	RemovedVersions []string                      `json:"removedVersions"`
	RemovedReleases []DelistedRelease             `json:"removedReleases"`
	RemovedTags     []string                      `json:"removedTags"`
}

// DelistedRelease 是本次下架删除的一份远端发行。
type DelistedRelease struct {
	TagName string `json:"tagName"`
	ID      int64  `json:"releaseId"`
	HTMLURL string `json:"releaseUrl"`
}

// Delist 把一个发布物或其中一个正式版本从官方来源彻底下架：
// 先移除统一版本索引记录，再删除对应远端发行与标签，最后复核三方均已清空。
// 目标允许不在当前白名单中（下架的正是被移除的旧发布物）；
// 对已经部分下架的对象重跑会补齐残留，不会破坏既有资料。
func (p *Publisher) Delist(ctx context.Context, source types.OfficialReleaseSource, input DelistInput) (DelistResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if p.token == "" {
		return DelistResult{}, fmt.Errorf("缺少 GitHub 发布凭据")
	}
	if err := releasecatalog.ValidateArtifactIdentity(input.Artifact); err != nil {
		return DelistResult{}, fmt.Errorf("下架目标无效：%w", err)
	}
	version := strings.TrimSpace(input.Version)
	if version != "" {
		if err := release.ValidateFormalVersion(version); err != nil {
			return DelistResult{}, fmt.Errorf("下架版本必须是三段正式版本：%w", err)
		}
	}
	result := DelistResult{Artifact: input.Artifact, Version: version, Repository: source.Repository}

	index, indexSHA, err := p.readIndexContents(ctx, source)
	if err != nil {
		return result, err
	}
	updatedIndex, removedVersions, err := removeDelistedVersions(index, input.Artifact, version)
	if err != nil {
		return result, err
	}

	releases, err := p.releasesForDelist(ctx, source, input.Artifact, version)
	if err != nil {
		return result, err
	}
	refs, err := p.tagRefsForDelist(ctx, source, input.Artifact, version)
	if err != nil {
		return result, err
	}
	if len(removedVersions) == 0 && len(releases) == 0 && len(refs) == 0 {
		return result, fmt.Errorf("官方索引与远端发行均不存在 %s，无需下架", delistDescription(input.Artifact, version))
	}

	if len(removedVersions) > 0 {
		payload, err := encodeIndex(updatedIndex)
		if err != nil {
			return result, err
		}
		if err := p.putIndexContents(ctx, source, indexSHA, payload); err != nil {
			return result, fmt.Errorf("移除官方索引记录失败（远端发行尚未删除）：%w", err)
		}
		result.IndexRemoved = true
		result.RemovedVersions = removedVersions
	}
	for _, item := range releases {
		if err := p.deleteRelease(ctx, source, item.ID); err != nil {
			return result, err
		}
		result.RemovedReleases = append(result.RemovedReleases, DelistedRelease{TagName: item.TagName, ID: item.ID, HTMLURL: item.HTMLURL})
	}
	for _, ref := range refs {
		if err := p.deleteTagRef(ctx, source, ref); err != nil {
			return result, err
		}
		result.RemovedTags = append(result.RemovedTags, ref)
	}
	if err := p.verifyDelisted(ctx, source, input.Artifact, version); err != nil {
		return result, err
	}
	return result, nil
}

// removeDelistedVersions 在索引副本上移除目标版本；Version 为空表示移除整个发布物。
// 返回更新后的索引与被移除的版本清单；目标不在索引中时原样返回，交由调用方按远端事实决定成败。
func removeDelistedVersions(index releasecatalog.Index, identity types.ReleaseArtifactIdentity, version string) (releasecatalog.Index, []string, error) {
	for position := range index.Artifacts {
		artifact := index.Artifacts[position]
		if artifact.Kind != identity.Kind || artifact.ID != identity.ID {
			continue
		}
		removed := make([]string, 0, len(artifact.Versions))
		kept := make([]releasecatalog.IndexVersion, 0, len(artifact.Versions))
		for _, item := range artifact.Versions {
			if version == "" || item.Version == version {
				removed = append(removed, item.Version)
				continue
			}
			kept = append(kept, item)
		}
		if len(removed) == 0 {
			return index, nil, nil
		}
		updated := index
		updated.Artifacts = append([]releasecatalog.IndexArtifact(nil), index.Artifacts...)
		if len(kept) == 0 {
			updated.Artifacts = append(updated.Artifacts[:position], updated.Artifacts[position+1:]...)
		} else {
			artifact.Versions = kept
			updated.Artifacts[position] = artifact
		}
		if len(updated.Artifacts) == 0 {
			return releasecatalog.Index{}, nil, fmt.Errorf("不能下架官方索引中的最后一个发布物")
		}
		updated.UpdatedAt = time.Now().UTC()
		if err := releasecatalog.ValidateIndex(updated); err != nil {
			return releasecatalog.Index{}, nil, fmt.Errorf("下架后的统一版本索引无效：%w", err)
		}
		return updated, removed, nil
	}
	return index, nil, nil
}

// releasesForDelist 从官方发行清单中筛出目标发行：指定版本精确匹配，整个发布物按标签命名空间匹配。
func (p *Publisher) releasesForDelist(ctx context.Context, source types.OfficialReleaseSource, identity types.ReleaseArtifactIdentity, version string) ([]githubRelease, error) {
	releases, err := p.listReleases(ctx, source)
	if err != nil {
		return nil, err
	}
	matched := make([]githubRelease, 0, 1)
	for _, item := range releases {
		if delistTagMatches(item.TagName, identity, version) {
			matched = append(matched, item)
		}
	}
	return matched, nil
}

// tagRefsForDelist 从官方标签记录中筛出目标标签：指定版本精确匹配，整个发布物按命名空间匹配。
func (p *Publisher) tagRefsForDelist(ctx context.Context, source types.OfficialReleaseSource, identity types.ReleaseArtifactIdentity, version string) ([]string, error) {
	refs, err := p.matchingTagRefs(ctx, source, identity.ID)
	if err != nil {
		return nil, err
	}
	matched := make([]string, 0, len(refs))
	for _, ref := range refs {
		if delistRefMatches(ref, identity, version) {
			matched = append(matched, ref)
		}
	}
	return matched, nil
}

func delistTagMatches(tag string, identity types.ReleaseArtifactIdentity, version string) bool {
	if version != "" {
		expected, err := releasecatalog.TagName(identity, version)
		if err != nil {
			return false
		}
		return tag == expected
	}
	return strings.HasPrefix(tag, identity.ID+"/")
}

func delistRefMatches(ref string, identity types.ReleaseArtifactIdentity, version string) bool {
	if version != "" {
		expected, err := releasecatalog.TagName(identity, version)
		if err != nil {
			return false
		}
		return ref == "refs/tags/"+expected
	}
	return ref == "refs/tags/"+identity.ID || strings.HasPrefix(ref, "refs/tags/"+identity.ID+"/")
}

// matchingTagRefs 读取官方仓库中以给定前缀开头的全部标签引用。
func (p *Publisher) matchingTagRefs(ctx context.Context, source types.OfficialReleaseSource, prefix string) ([]string, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/git/matching-refs/tags/%s", p.apiBaseURL, url.PathEscape(source.Owner), url.PathEscape(source.Name), url.PathEscape(prefix))
	var items []struct {
		Ref string `json:"ref"`
	}
	if _, err := p.requestJSON(ctx, http.MethodGet, endpoint, nil, &items); err != nil {
		return nil, fmt.Errorf("读取官方标签记录失败：%w", err)
	}
	refs := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Ref) != "" {
			refs = append(refs, item.Ref)
		}
	}
	return refs, nil
}

func (p *Publisher) deleteRelease(ctx context.Context, source types.OfficialReleaseSource, id int64) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/releases/%d", p.apiBaseURL, url.PathEscape(source.Owner), url.PathEscape(source.Name), id)
	if _, err := p.requestJSON(ctx, http.MethodDelete, endpoint, nil, nil); err != nil {
		return fmt.Errorf("删除远端发行 %d 失败：%w", id, err)
	}
	return nil
}

func (p *Publisher) deleteTagRef(ctx context.Context, source types.OfficialReleaseSource, ref string) error {
	name := strings.TrimPrefix(strings.TrimSpace(ref), "refs/")
	if !strings.HasPrefix(name, "tags/") {
		return fmt.Errorf("官方标签 %q 不是标签引用", ref)
	}
	segments := strings.Split(name, "/")
	for position := range segments {
		segments[position] = url.PathEscape(segments[position])
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/git/refs/%s", p.apiBaseURL, url.PathEscape(source.Owner), url.PathEscape(source.Name), strings.Join(segments, "/"))
	if _, err := p.requestJSON(ctx, http.MethodDelete, endpoint, nil, nil); err != nil {
		return fmt.Errorf("删除官方标签 %s 失败：%w", ref, err)
	}
	return nil
}

// verifyDelisted 复核索引、远端发行与标签三方均已不存在目标。
func (p *Publisher) verifyDelisted(ctx context.Context, source types.OfficialReleaseSource, identity types.ReleaseArtifactIdentity, version string) error {
	index, err := p.readIndex(ctx, source)
	if err != nil {
		return fmt.Errorf("下架复核读取官方索引失败：%w", err)
	}
	if indexHasDelistedVersion(index, identity, version) {
		return fmt.Errorf("下架复核发现官方索引仍有 %s", delistDescription(identity, version))
	}
	releases, err := p.releasesForDelist(ctx, source, identity, version)
	if err != nil {
		return err
	}
	if len(releases) > 0 {
		return fmt.Errorf("下架复核发现远端发行仍有 %d 份残留", len(releases))
	}
	refs, err := p.tagRefsForDelist(ctx, source, identity, version)
	if err != nil {
		return err
	}
	if len(refs) > 0 {
		return fmt.Errorf("下架复核发现官方标签仍有 %d 个残留", len(refs))
	}
	return nil
}

func indexHasDelistedVersion(index releasecatalog.Index, identity types.ReleaseArtifactIdentity, version string) bool {
	for _, artifact := range index.Artifacts {
		if artifact.Kind != identity.Kind || artifact.ID != identity.ID {
			continue
		}
		if version == "" {
			return true
		}
		for _, item := range artifact.Versions {
			if item.Version == version {
				return true
			}
		}
	}
	return false
}

func delistDescription(identity types.ReleaseArtifactIdentity, version string) string {
	target := releasecatalog.Target(identity)
	if version == "" {
		return "发布物 " + target
	}
	return target + " v" + version
}
