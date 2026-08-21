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

// DevelopmentCandidate 从显式开发启动入口提供的本地成品清单与压缩包
// 读取候选；本地成品必须是本次当前源码制作结果，清单带验证标记。
// 开发来源只能由开发启动入口显式开启，不能根据文件存在或构建方式猜测。
type DevelopmentCandidate struct {
	ManifestPath string
	ArchivePath  string
}

// NewDevelopmentCandidate 构造开发候选读取器；路径可为空，会在使用时报告。
func NewDevelopmentCandidate(manifestPath string, archivePath string) *DevelopmentCandidate {
	return &DevelopmentCandidate{
		ManifestPath: strings.TrimSpace(manifestPath),
		ArchivePath:  strings.TrimSpace(archivePath),
	}
}

// LatestCandidate 从本地成品清单构造候选，并核对压缩包文件与清单一致。
func (s *DevelopmentCandidate) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*ReleaseCandidate, error) {
	if err := s.validateInputs(); err != nil {
		return nil, err
	}
	manifest, err := s.readManifest(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateDevelopmentManifest(manifest, identity); err != nil {
		return nil, err
	}
	if filepath.Base(filepath.Clean(s.ArchivePath)) != manifest.Archive.Name {
		return nil, errors.New("开发成品压缩包文件名与发行清单不一致")
	}
	size, sha256, err := recordForFile(s.ArchivePath)
	if err != nil {
		return nil, fmt.Errorf("读取开发成品压缩包失败：%w", err)
	}
	if size != manifest.Archive.Size || !strings.EqualFold(sha256, manifest.Archive.SHA256) {
		return nil, errors.New("开发成品压缩包大小或摘要与发行清单不一致")
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
		Development:      true,
	}, nil
}

func (s *DevelopmentCandidate) validateInputs() error {
	if strings.TrimSpace(s.ManifestPath) == "" || strings.TrimSpace(s.ArchivePath) == "" {
		return errors.New("开发成品资料不完整")
	}
	return nil
}

func (s *DevelopmentCandidate) readManifest(ctx context.Context) (types.ReleaseManifest, error) {
	payload, err := readRegularFile(ctx, s.ManifestPath)
	if err != nil {
		return types.ReleaseManifest{}, err
	}
	manifest, err := release.DecodeReleaseManifest(payload)
	if err != nil {
		return types.ReleaseManifest{}, fmt.Errorf("开发成品清单无效：%w", err)
	}
	return manifest, nil
}

func validateDevelopmentManifest(manifest types.ReleaseManifest, identity types.ReleaseArtifactIdentity) error {
	if !manifest.VerificationOnly {
		return errors.New("开发成品缺少验证或开发标记")
	}
	if manifest.Artifact != identity || manifest.Platform != types.ReleasePlatformWindowsX64 {
		return errors.New("开发成品不是本平台的目标发布物")
	}
	if strings.TrimSpace(manifest.Source.Commit) == "" {
		return errors.New("开发成品缺少当前源码记录")
	}
	return nil
}

// DevelopmentSourceReader 从开发成品目录按发布物构造本地候选；目录内按
// <kind>-<id>/<version>/ 组织，读取其中标记仅供验证的最新成品。
type DevelopmentSourceReader struct {
	root string
}

// NewDevelopmentSourceReader 构造开发源候选读取器。
// 返回 nil 表示开发源未激活（SOURCE 非 "1"），调用方应继续使用官方源。
func NewDevelopmentSourceReader(sourceFlag string, packageRoot string) (*DevelopmentSourceReader, error) {
	if strings.TrimSpace(sourceFlag) != "1" {
		return nil, nil
	}
	root, err := filepath.Abs(strings.TrimSpace(packageRoot))
	if err != nil || strings.TrimSpace(packageRoot) == "" {
		return nil, errors.New("开发工具成品根目录无效")
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("开发工具成品根目录读取失败：%w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("开发工具成品根目录不是目录")
	}
	return &DevelopmentSourceReader{root: root}, nil
}

// LatestCandidate 读取目标发布物的本地最高版本开发成品。
func (s *DevelopmentSourceReader) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*ReleaseCandidate, error) {
	candidates, err := s.developmentCandidates(ctx, identity)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("开发成品目录没有目标发布物 %s 的开发版成品", identity.ID)
	}
	sort.SliceStable(candidates, func(i int, j int) bool {
		left, leftErr := release.CompareVersions(candidates[i].version, candidates[j].version)
		if leftErr != nil {
			return candidates[i].version < candidates[j].version
		}
		return left > 0
	})
	return releasecheckCandidate(ctx, identity, candidates[0])
}

type developmentCandidateFile struct {
	version  string
	manifest string
	archive  string
}

func (s *DevelopmentSourceReader) developmentCandidates(ctx context.Context, identity types.ReleaseArtifactIdentity) ([]developmentCandidateFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	expectedRoot := identity.Kind + "-" + identity.ID
	versionsDir := filepath.Join(s.root, expectedRoot)
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	result := make([]developmentCandidateFile, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		version := entry.Name()
		if err := release.ValidateVersion(version); err != nil {
			continue
		}
		candidate, ok := locateDevelopmentArtifact(filepath.Join(versionsDir, version))
		if !ok {
			continue
		}
		result = append(result, candidate)
	}
	return result, nil
}

// locateDevelopmentArtifact 在版本目录内定位唯一的 zip 与 manifest 对。
func locateDevelopmentArtifact(dir string) (developmentCandidateFile, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return developmentCandidateFile{}, false
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
		return developmentCandidateFile{version: filepath.Base(dir), manifest: manifest, archive: archive}, true
	}
	return developmentCandidateFile{}, false
}

func releasecheckCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity, file developmentCandidateFile) (*ReleaseCandidate, error) {
	reader := NewDevelopmentCandidate(file.manifest, file.archive)
	return reader.LatestCandidate(ctx, identity)
}

// DevelopmentSource 从已核对候选打包源事实；只服务于开发来源，携带验证标记放行。
func DevelopmentSource(candidate *ReleaseCandidate) (release.ArtifactPackageSource, error) {
	if candidate == nil {
		return release.ArtifactPackageSource{}, errors.New("开发候选为空")
	}
	product := types.ReleaseProductRecord{
		SchemaVersion:    release.ReleaseManifestSchemaVersion,
		Artifact:         candidate.Artifact,
		Version:          candidate.Version,
		Platform:         types.ReleasePlatformWindowsX64,
		OfficialSource:   candidate.OfficialSource,
		Compatibility:    candidate.Compatibility,
		Source:           types.ReleaseSourceRecord{Repository: candidate.SourceRepository, Commit: candidate.SourceRevision, Recorded: true},
		DataVersion:      candidate.DataVersion,
		VerificationOnly: true,
	}
	source := release.ArtifactPackageSource{
		Artifact:    candidate.Artifact,
		Product:     product,
		ArchiveURL:  candidate.ArchiveURL,
		SizeBytes:   candidate.SizeBytes,
		SHA256:      candidate.SHA256,
		Development: true,
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
