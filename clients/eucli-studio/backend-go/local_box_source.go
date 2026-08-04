package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

// localBoxSourceKind 是业务端成品来源类别。
// 官方来源读取固定官方发行；开发来源读取当前源码制作出的本地成品。
// 来源只能由开发体验启动入口显式开启，不能根据构建方式或文件存在猜测。
type localBoxSourceKind string

const (
	localBoxSourceOfficial    localBoxSourceKind = "official"
	localBoxSourceDevelopment localBoxSourceKind = "development"
)

// 开发来源对应的进程环境变量，只由开发体验启动入口设置。
const (
	devSourceEnvironment   = "EUCLI_DEV_BOX_SOURCE"
	devManifestEnvironment = "EUCLI_DEV_BOX_MANIFEST"
	devArchiveEnvironment  = "EUCLI_DEV_BOX_ARCHIVE"
	devBoxRootEnvironment  = "EUCLI_DEV_BOX_BOX_ROOT"
	devSourceEnabled       = "1"
)

func (kind localBoxSourceKind) valid() bool {
	return kind == localBoxSourceOfficial || kind == localBoxSourceDevelopment
}

func (kind localBoxSourceKind) normalize() localBoxSourceKind {
	if !kind.valid() {
		return localBoxSourceOfficial
	}
	return kind
}

// localBoxArtifactSource 是业务端成品来源的统一接口。
// 来源只负责两件事：报告当前候选、把候选的清单和压缩包文件带到本次安装下载目录。
// 清单解析、压缩包核对、安全解包、安装、启动、登记、连接和退出由安装流程统一完成。
type localBoxArtifactSource interface {
	Kind() localBoxSourceKind
	LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error)
	AcquireArtifacts(ctx context.Context, candidate *releasecheck.ReleaseCandidate, downloadDir string, onProgress func(localBoxProgress)) (types.ReleaseManifest, error)
}

// officialArtifactSource 从固定官方发行读取业务端候选和成品文件。
type officialArtifactSource struct {
	checker *releasecheck.Checker
}

func (s *officialArtifactSource) Kind() localBoxSourceKind {
	return localBoxSourceOfficial
}

func (s *officialArtifactSource) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error) {
	if s.checker == nil {
		return nil, errors.New("官方业务端候选读取能力未初始化")
	}
	return s.checker.LatestCandidate(ctx, identity)
}

func (s *officialArtifactSource) AcquireArtifacts(ctx context.Context, candidate *releasecheck.ReleaseCandidate, downloadDir string, onProgress func(localBoxProgress)) (types.ReleaseManifest, error) {
	return acquireOfficialArtifacts(ctx, candidate, downloadDir, onProgress)
}

// acquireOfficialArtifacts 从候选的远端地址下载清单和压缩包并完成文件级核对。
// 官方来源和隔离验证使用的候选都通过远端地址提供文件，共用这一取得能力。
func acquireOfficialArtifacts(ctx context.Context, candidate *releasecheck.ReleaseCandidate, downloadDir string, onProgress func(localBoxProgress)) (types.ReleaseManifest, error) {
	if candidate == nil {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_RELEASE_UNAVAILABLE", errors.New("官方候选为空"))
	}
	if err := os.MkdirAll(downloadDir, 0o700); err != nil {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_DOWNLOAD_FAILED", err)
	}
	manifestName := strings.TrimSuffix(candidate.Manifest.Archive.Name, ".zip") + ".manifest.json"
	manifestPath := filepath.Join(downloadDir, manifestName)
	manifestClient := &localBoxProgressClient{
		base: &http.Client{Timeout: 15 * time.Minute},
		onRead: func(received int64) {
			if onProgress != nil {
				onProgress(localBoxProgress{Phase: localBoxStatusDownloading, ReceivedBytes: received, TotalBytes: candidate.ManifestSize})
			}
		},
	}
	if _, err := release.DownloadFileUnchecked(ctx, release.DownloadFileOptions{
		Client: manifestClient, URL: candidate.ManifestURL, TargetPath: manifestPath, ExpectedName: manifestName,
		ExpectedSize: candidate.ManifestSize, MaxBytes: candidate.ManifestSize + 1,
	}); err != nil {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_DOWNLOAD_FAILED", err)
	}
	manifestPayload, err := os.ReadFile(manifestPath)
	if err != nil {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_DOWNLOAD_FAILED", err)
	}
	manifest, err := release.DecodeReleaseManifest(manifestPayload)
	if err != nil {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_PACKAGE_INVALID", err)
	}
	if !sameReleaseManifest(manifest, candidate.Manifest) {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_PACKAGE_INVALID", errors.New("下载的发行清单与官方候选不一致"))
	}
	archiveClient := &localBoxProgressClient{
		base: &http.Client{Timeout: 15 * time.Minute},
		onRead: func(received int64) {
			if onProgress != nil {
				onProgress(localBoxProgress{Phase: localBoxStatusVerifying, ReceivedBytes: received, TotalBytes: manifest.Archive.Size})
			}
		},
	}
	archivePath := filepath.Join(downloadDir, manifest.Archive.Name)
	archiveRecord, err := release.DownloadFile(ctx, release.DownloadFileOptions{
		Client: archiveClient, URL: candidate.ArchiveURL, TargetPath: archivePath, ExpectedName: manifest.Archive.Name,
		ExpectedSize: manifest.Archive.Size, ExpectedSHA256: manifest.Archive.SHA256,
		MaxBytes: manifest.Archive.Size + 1,
	})
	if err != nil {
		code := "LOCAL_BOX_DOWNLOAD_FAILED"
		if errors.Is(err, release.ErrDownloadSizeMismatch) || errors.Is(err, release.ErrDownloadDigestMismatch) {
			code = "LOCAL_BOX_PACKAGE_INVALID"
		}
		return types.ReleaseManifest{}, localBoxOperationFailure(code, err)
	}
	if archiveRecord != manifest.Archive {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_PACKAGE_INVALID", errors.New("下载成品与发行清单不一致"))
	}
	return manifest, nil
}

// developmentArtifactSource 从开发启动入口提供的本地成品读取候选和成品文件。
// 本地成品必须是本次当前源码制作结果：清单标记仅供验证、身份为业务端、平台为 Windows x64。
type developmentArtifactSource struct {
	manifestPath string
	archivePath  string
}

func newDevelopmentArtifactSource(manifestPath string, archivePath string) *developmentArtifactSource {
	return &developmentArtifactSource{manifestPath: strings.TrimSpace(manifestPath), archivePath: strings.TrimSpace(archivePath)}
}

func (s *developmentArtifactSource) Kind() localBoxSourceKind {
	return localBoxSourceDevelopment
}

func (s *developmentArtifactSource) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error) {
	if err := s.validateInputs(); err != nil {
		return nil, err
	}
	manifest, err := s.readDevelopmentManifest(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateDevelopmentManifest(manifest, identity); err != nil {
		return nil, err
	}
	return &releasecheck.ReleaseCandidate{
		Artifact: identity,
		Manifest: manifest,
		// 开发来源不携带发行网页和远端下载地址；文件取得由 AcquireArtifacts 从本地读取完成。
		ReleaseNotes: readDevelopmentReleaseNotes(ctx, s.manifestPath),
	}, nil
}

func (s *developmentArtifactSource) validateInputs() error {
	if s.manifestPath == "" || s.archivePath == "" {
		return localBoxOperationFailure("LOCAL_BOX_DEV_ARTIFACT_UNAVAILABLE", errors.New("开发成品资料不完整"))
	}
	return nil
}

func (s *developmentArtifactSource) AcquireArtifacts(ctx context.Context, candidate *releasecheck.ReleaseCandidate, downloadDir string, onProgress func(localBoxProgress)) (types.ReleaseManifest, error) {
	if candidate == nil {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_DEV_ARTIFACT_UNAVAILABLE", errors.New("开发候选为空"))
	}
	if err := s.validateInputs(); err != nil {
		return types.ReleaseManifest{}, err
	}
	if err := os.MkdirAll(downloadDir, 0o700); err != nil {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_DEV_ARTIFACT_UNAVAILABLE", err)
	}
	manifest, err := s.readDevelopmentManifest(ctx)
	if err != nil {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_DEV_ARTIFACT_UNAVAILABLE", err)
	}
	if !sameReleaseManifest(manifest, candidate.Manifest) {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_DEV_ARTIFACT_INVALID", errors.New("本地发行清单与开发候选不一致"))
	}
	if filepath.Base(filepath.Clean(s.archivePath)) != manifest.Archive.Name {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_DEV_ARTIFACT_INVALID", errors.New("本地压缩包文件名与发行清单不一致"))
	}
	manifestName := strings.TrimSuffix(manifest.Archive.Name, ".zip") + ".manifest.json"
	targetManifest := filepath.Join(downloadDir, manifestName)
	if err := copyFileWithProgress(ctx, s.manifestPath, targetManifest, 0, onProgress); err != nil {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_DEV_ARTIFACT_UNAVAILABLE", fmt.Errorf("读取本地发行清单失败：%w", err))
	}
	totalBytes, err := regularFileSize(s.archivePath)
	if err != nil {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_DEV_ARTIFACT_UNAVAILABLE", err)
	}
	targetArchive := filepath.Join(downloadDir, manifest.Archive.Name)
	if err := copyFileWithProgress(ctx, s.archivePath, targetArchive, totalBytes, onProgress); err != nil {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_DEV_ARTIFACT_UNAVAILABLE", fmt.Errorf("读取本地压缩包失败：%w", err))
	}
	record, err := releaseRecordForFile(targetArchive)
	if err != nil {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_DEV_ARTIFACT_UNAVAILABLE", err)
	}
	if record != manifest.Archive {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_DEV_ARTIFACT_INVALID", errors.New("本地压缩包大小或摘要与发行清单不一致"))
	}
	return manifest, nil
}

func (s *developmentArtifactSource) readDevelopmentManifest(ctx context.Context) (types.ReleaseManifest, error) {
	payload, err := readRegularFile(ctx, s.manifestPath, "本地发行清单")
	if err != nil {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_DEV_ARTIFACT_UNAVAILABLE", err)
	}
	manifest, err := release.DecodeReleaseManifest(payload)
	if err != nil {
		return types.ReleaseManifest{}, localBoxOperationFailure("LOCAL_BOX_DEV_ARTIFACT_INVALID", fmt.Errorf("本地发行清单无效：%w", err))
	}
	return manifest, nil
}

func validateDevelopmentManifest(manifest types.ReleaseManifest, identity types.ReleaseArtifactIdentity) error {
	if !manifest.VerificationOnly {
		return localBoxOperationFailure("LOCAL_BOX_DEV_ARTIFACT_INVALID", errors.New("本地成品缺少验证或开发标记"))
	}
	if manifest.Artifact != identity || manifest.Platform != types.ReleasePlatformWindowsX64 {
		return localBoxOperationFailure("LOCAL_BOX_DEV_ARTIFACT_INVALID", errors.New("本地成品不是本平台的业务端"))
	}
	if strings.TrimSpace(manifest.Source.Commit) == "" {
		return localBoxOperationFailure("LOCAL_BOX_DEV_ARTIFACT_INVALID", errors.New("本地成品缺少当前源码记录"))
	}
	return nil
}

func readDevelopmentReleaseNotes(ctx context.Context, manifestPath string) string {
	notesPath := filepath.Join(filepath.Dir(manifestPath), "release-notes.md")
	payload, err := readRegularFile(ctx, notesPath, "本地发行说明")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(payload))
}

func readRegularFile(ctx context.Context, path string, label string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s不存在：%s", label, path)
		}
		return nil, fmt.Errorf("读取%s失败：%w", label, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s不是普通文件：%s", label, path)
	}
	return os.ReadFile(path)
}

func regularFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if info.IsDir() {
		return 0, errors.New("路径是目录而不是普通文件")
	}
	return info.Size(), nil
}

func releaseRecordForFile(path string) (types.ReleaseFileRecord, error) {
	input, err := os.Open(path)
	if err != nil {
		return types.ReleaseFileRecord{}, err
	}
	defer input.Close()
	digest := sha256.New()
	size, err := io.Copy(digest, input)
	if err != nil {
		return types.ReleaseFileRecord{}, err
	}
	return types.ReleaseFileRecord{Name: filepath.Base(path), Size: size, SHA256: hex.EncodeToString(digest.Sum(nil))}, nil
}

func copyFileWithProgress(ctx context.Context, source string, target string, totalBytes int64, onProgress func(localBoxProgress)) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	output, err := os.CreateTemp(filepath.Dir(target), ".copy-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := output.Name()
	cleanup := func() {
		_ = output.Close()
		_ = os.Remove(temporaryPath)
	}
	defer func() {
		_ = output.Close()
		_ = os.Remove(temporaryPath)
	}()
	progress := &copyProgress{total: totalBytes, onProgress: onProgress}
	written, err := io.Copy(output, io.TeeReader(input, progress))
	if err != nil {
		cleanup()
		return err
	}
	if totalBytes > 0 && written != totalBytes {
		cleanup()
		return fmt.Errorf("复制文件大小不一致")
	}
	if err := output.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := output.Close(); err != nil {
		cleanup()
		return err
	}
	if err := release.ReplaceFileAtomic(temporaryPath, target); err != nil {
		cleanup()
		return err
	}
	return nil
}

type copyProgress struct {
	total      int64
	received   int64
	onProgress func(localBoxProgress)
}

func (progress *copyProgress) Write(buffer []byte) (int, error) {
	count := len(buffer)
	progress.received += int64(count)
	if progress.onProgress != nil {
		progress.onProgress(localBoxProgress{Phase: localBoxStatusVerifying, ReceivedBytes: progress.received, TotalBytes: progress.total})
	}
	return count, nil
}
