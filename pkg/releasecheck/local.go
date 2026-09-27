package releasecheck

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

// LocalCandidate 从货架（用户注册的目录）的货品清单与压缩包
// 读取单个货架候选；货品与官方源使用同一货品格式。
type LocalCandidate struct {
	ManifestPath string
	ArchivePath  string
}

// NewLocalCandidate 构造本地候选读取器；路径可为空，会在使用时报告。
func NewLocalCandidate(manifestPath string, archivePath string) *LocalCandidate {
	return &LocalCandidate{
		ManifestPath: strings.TrimSpace(manifestPath),
		ArchivePath:  strings.TrimSpace(archivePath),
	}
}

// LatestCandidate 从本地货品清单构造候选，并核对压缩包文件与清单一致。
func (s *LocalCandidate) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*ReleaseCandidate, error) {
	if err := s.validateInputs(); err != nil {
		return nil, err
	}
	manifest, err := s.readManifest(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateLocalManifest(manifest, identity); err != nil {
		return nil, err
	}
	if filepath.Base(filepath.Clean(s.ArchivePath)) != manifest.Archive.Name {
		return nil, errors.New("本地货品压缩包文件名与发行清单不一致")
	}
	size, sha256, err := recordForFile(s.ArchivePath)
	if err != nil {
		return nil, fmt.Errorf("读取本地货品压缩包失败：%w", err)
	}
	if size != manifest.Archive.Size || !strings.EqualFold(sha256, manifest.Archive.SHA256) {
		return nil, errors.New("本地货品压缩包大小或摘要与发行清单不一致")
	}
	return &ReleaseCandidate{
		Artifact:         identity,
		Version:          manifest.Version,
		PublishedAt:      time.Time{},
		SourceRevision:   manifest.Source.Commit,
		SourceRepository: manifest.Source.Repository,
		DataVersion:      manifest.DataVersion,
		Compatibility:    manifest.Compatibility,
		OfficialSource:   manifest.OfficialSource,
		ReleaseURL:       "",
		ArchiveURL:       s.ArchivePath,
		SizeBytes:        size,
		SHA256:           sha256,
		Local:            true,
	}, nil
}

func (s *LocalCandidate) validateInputs() error {
	if strings.TrimSpace(s.ManifestPath) == "" || strings.TrimSpace(s.ArchivePath) == "" {
		return errors.New("本地货品资料不完整")
	}
	return nil
}

func (s *LocalCandidate) readManifest(ctx context.Context) (types.ReleaseManifest, error) {
	payload, err := readRegularFile(ctx, s.ManifestPath)
	if err != nil {
		return types.ReleaseManifest{}, err
	}
	manifest, err := release.DecodeReleaseManifest(payload)
	if err != nil {
		return types.ReleaseManifest{}, fmt.Errorf("本地货品清单无效：%w", err)
	}
	return manifest, nil
}

func validateLocalManifest(manifest types.ReleaseManifest, identity types.ReleaseArtifactIdentity) error {
	if manifest.Artifact != identity || manifest.Platform != types.ReleasePlatformWindowsX64 {
		return errors.New("本地货品不是本平台的目标发布物")
	}
	if strings.TrimSpace(manifest.Source.Commit) == "" {
		return errors.New("本地货品缺少当前源码记录")
	}
	return nil
}

// LocalSourceReader 从单类别货架按发布物构造候选；货架根下直接是
// <id>/<version>/，版本目录内放货品清单与压缩包。
type LocalSourceReader struct {
	root string
	kind string
}

// NewLocalSourceReader 构造单类别货架候选读取器。
func NewLocalSourceReader(packageRoot string, kind string) (*LocalSourceReader, error) {
	kind = strings.TrimSpace(kind)
	if kind != types.ReleaseArtifactKindTool && kind != types.ReleaseArtifactKindPlugin {
		return nil, fmt.Errorf("货架不支持发布物类别 %q", kind)
	}
	root, err := filepath.Abs(strings.TrimSpace(packageRoot))
	if err != nil || strings.TrimSpace(packageRoot) == "" {
		return nil, errors.New("货架路径无效")
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("货架路径读取失败：%w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("货架路径不是目录")
	}
	return &LocalSourceReader{root: root, kind: kind}, nil
}

// LatestCandidate 读取目标发布物在货架上的最高版本候选；只服务本货架类别。
func (s *LocalSourceReader) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*ReleaseCandidate, error) {
	if err := s.validateIdentity(identity); err != nil {
		return nil, err
	}
	candidates, err := s.localCandidates(ctx, identity)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("货架没有目标发布物 %s 的候选成品", identity.ID)
	}
	sort.SliceStable(candidates, func(i int, j int) bool {
		left, leftErr := release.CompareVersions(candidates[i].version, candidates[j].version)
		if leftErr != nil {
			return candidates[i].version < candidates[j].version
		}
		return left > 0
	})
	return localCandidate(ctx, identity, candidates[0])
}

type localCandidateFile struct {
	version  string
	manifest string
	archive  string
}

func (s *LocalSourceReader) localCandidates(ctx context.Context, identity types.ReleaseArtifactIdentity) ([]localCandidateFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	versionsDir := filepath.Join(s.root, identity.ID)
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	result := make([]localCandidateFile, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		version := entry.Name()
		if err := release.ValidateVersion(version); err != nil {
			continue
		}
		candidate, ok := locateLocalArtifact(filepath.Join(versionsDir, version))
		if !ok {
			continue
		}
		result = append(result, candidate)
	}
	return result, nil
}

// validateIdentity 拒绝不属于本货架类别的发布物。
func (s *LocalSourceReader) validateIdentity(identity types.ReleaseArtifactIdentity) error {
	if strings.TrimSpace(identity.Kind) != s.kind {
		return fmt.Errorf("货架类别 %q 不服务发布物类别 %q", s.kind, identity.Kind)
	}
	return nil
}

// locateLocalArtifact 在版本目录内定位唯一的 zip 与清单对。
func locateLocalArtifact(dir string) (localCandidateFile, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return localCandidateFile{}, false
	}
	var manifest, archive string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if strings.HasSuffix(name, ".manifest.json") {
			manifest = filepath.Join(dir, name)
		} else if strings.HasSuffix(name, ".zip") {
			archive = filepath.Join(dir, name)
		}
	}
	if manifest != "" && archive != "" {
		return localCandidateFile{version: filepath.Base(dir), manifest: manifest, archive: archive}, true
	}
	return localCandidateFile{}, false
}

func localCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity, file localCandidateFile) (*ReleaseCandidate, error) {
	reader := NewLocalCandidate(file.manifest, file.archive)
	return reader.LatestCandidate(ctx, identity)
}

// LocalShelfItem 是货架上一个可下载物的清单事实。
type LocalShelfItem struct {
	Artifact types.ReleaseArtifactIdentity
	Candidate *ReleaseCandidate
}

// LocalShelf 是货架的只读能力：按发布物读候选，以及枚举全部货品。
type LocalShelf interface {
	CandidateReader
	List(ctx context.Context) ([]LocalShelfItem, error)
}

// List 枚举货架上本类别拥有有效候选的全部货品，每个货品给出最高版本候选。
func (s *LocalSourceReader) List(ctx context.Context) ([]LocalShelfItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return []LocalShelfItem{}, nil
		}
		return nil, err
	}
	items := make([]LocalShelfItem, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		identity := types.ReleaseArtifactIdentity{Kind: s.kind, ID: entry.Name()}
		candidates, err := s.localCandidates(ctx, identity)
		if err != nil {
			return nil, err
		}
		if len(candidates) == 0 {
			continue
		}
		sort.SliceStable(candidates, func(i int, j int) bool {
			left, leftErr := release.CompareVersions(candidates[i].version, candidates[j].version)
			if leftErr != nil {
				return candidates[i].version < candidates[j].version
			}
			return left > 0
		})
		candidate, err := localCandidate(ctx, identity, candidates[0])
		if err != nil {
			continue
		}
		items = append(items, LocalShelfItem{Artifact: identity, Candidate: candidate})
	}
	sort.Slice(items, func(i int, j int) bool {
		return items[i].Artifact.ID < items[j].Artifact.ID
	})
	return items, nil
}

// LocalSource 从已核对候选打包源事实；货架货品使用清单记录的真实来源事实。
func LocalSource(candidate *ReleaseCandidate) (release.ArtifactPackageSource, error) {
	if candidate == nil {
		return release.ArtifactPackageSource{}, errors.New("本地候选为空")
	}
	product := types.ReleaseProductRecord{
		SchemaVersion:  release.ReleaseManifestSchemaVersion,
		Artifact:       candidate.Artifact,
		Version:        candidate.Version,
		Platform:       types.ReleasePlatformWindowsX64,
		OfficialSource: candidate.OfficialSource,
		Compatibility:  candidate.Compatibility,
		Source:         types.ReleaseSourceRecord{Repository: candidate.SourceRepository, Commit: candidate.SourceRevision},
		DataVersion:    candidate.DataVersion,
	}
	source := release.ArtifactPackageSource{
		Artifact:   candidate.Artifact,
		Product:    product,
		ArchiveURL: candidate.ArchiveURL,
		SizeBytes:  candidate.SizeBytes,
		SHA256:     candidate.SHA256,
		Local:      true,
	}
	if err := release.ValidateArtifactPackageSource(source); err != nil {
		return release.ArtifactPackageSource{}, err
	}
	return source, nil
}

func recordForFile(path string) (int64, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, "", err
	}
	if !info.Mode().IsRegular() {
		return 0, "", errors.New("不是普通文件")
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()
	hasher := sha256.New()
	written, err := io.Copy(hasher, file)
	if err != nil {
		return 0, "", err
	}
	return written, hex.EncodeToString(hasher.Sum(nil)), nil
}

func readRegularFile(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
