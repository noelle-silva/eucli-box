package releasepublish

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

type DownloadResult struct {
	Manifest     types.ReleaseManifest
	ArchivePath  string
	ManifestPath string
	ReleaseURL   string
}

func (p *Publisher) DownloadPublished(ctx context.Context, identity types.ReleaseArtifactIdentity, version string, targetRoot string) (DownloadResult, error) {
	if !p.catalog.Contains(identity) {
		return DownloadResult{}, fmt.Errorf("发布物不在正式白名单中")
	}
	tag, err := releasecatalog.TagName(identity, version)
	if err != nil {
		return DownloadResult{}, err
	}
	source, err := p.catalog.SourceFor(identity.Kind)
	if err != nil {
		return DownloadResult{}, err
	}
	remote, err := p.findReleaseByTag(ctx, source, tag)
	if err != nil {
		return DownloadResult{}, err
	}
	if remote == nil || remote.Draft || remote.Prerelease {
		return DownloadResult{}, fmt.Errorf("官方来源没有公开正式发行 %s", tag)
	}
	manifestAsset, err := uniqueRemoteManifest(remote.Assets)
	if err != nil {
		return DownloadResult{}, err
	}
	manifestPayload, err := p.downloadAsset(ctx, manifestAsset.URL, manifestAsset.Size)
	if err != nil {
		return DownloadResult{}, err
	}
	manifest, err := release.DecodeReleaseManifest(manifestPayload)
	if err != nil {
		return DownloadResult{}, err
	}
	if manifest.VerificationOnly || !manifest.Source.Recorded {
		return DownloadResult{}, fmt.Errorf("远端发行使用了仅供验证的成品")
	}
	if err := release.ValidateManifestIdentity(manifest, identity, tag, source.Repository); err != nil {
		return DownloadResult{}, err
	}
	archiveAsset, err := namedRemoteAsset(remote.Assets, manifest.Archive.Name)
	if err != nil {
		return DownloadResult{}, err
	}
	archivePayload, err := p.downloadAsset(ctx, archiveAsset.URL, archiveAsset.Size)
	if err != nil {
		return DownloadResult{}, err
	}
	if err := release.ValidateArchiveDigest(manifest, archivePayload); err != nil {
		return DownloadResult{}, err
	}
	targetRoot, err = filepath.Abs(strings.TrimSpace(targetRoot))
	if err != nil || strings.TrimSpace(targetRoot) == "" {
		return DownloadResult{}, fmt.Errorf("远端复核目录无效")
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return DownloadResult{}, err
	}
	archivePath := filepath.Join(targetRoot, manifest.Archive.Name)
	manifestPath := filepath.Join(targetRoot, manifestAsset.Name)
	if err := os.WriteFile(archivePath, archivePayload, 0o644); err != nil {
		return DownloadResult{}, err
	}
	if err := os.WriteFile(manifestPath, manifestPayload, 0o644); err != nil {
		return DownloadResult{}, err
	}
	return DownloadResult{Manifest: manifest, ArchivePath: archivePath, ManifestPath: manifestPath, ReleaseURL: remote.HTMLURL}, nil
}

func uniqueRemoteManifest(assets []githubReleaseAsset) (githubReleaseAsset, error) {
	matches := make([]githubReleaseAsset, 0, 1)
	for _, asset := range assets {
		if strings.HasSuffix(strings.ToLower(asset.Name), ".manifest.json") {
			matches = append(matches, asset)
		}
	}
	if len(matches) != 1 {
		return githubReleaseAsset{}, fmt.Errorf("远端发行必须且只能包含一份发行清单")
	}
	return matches[0], nil
}

func namedRemoteAsset(assets []githubReleaseAsset, name string) (githubReleaseAsset, error) {
	matches := make([]githubReleaseAsset, 0, 1)
	for _, asset := range assets {
		if asset.Name == name {
			matches = append(matches, asset)
		}
	}
	if len(matches) != 1 {
		return githubReleaseAsset{}, fmt.Errorf("远端发行必须且只能包含文件 %s", name)
	}
	return matches[0], nil
}
