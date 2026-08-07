package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"eucli-box/pkg/localrun"
	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

type localBoxOperationError struct {
	code string
	err  error
}

func (e localBoxOperationError) Error() string { return e.code + ": " + e.err.Error() }
func (e localBoxOperationError) Unwrap() error { return e.err }

func localBoxOperationFailure(code string, err error) error {
	if err == nil {
		err = errors.New("未知业务端安装错误")
	}
	return localBoxOperationError{code: code, err: err}
}

func (m *localBoxManager) readLatestCandidate(ctx context.Context, state localBoxState) (localBoxState, error) {
	if m.source == nil {
		m.publishState(state)
		return state, nil
	}
	if !state.Installed {
		state.Status = localBoxStatusCheckingRelease
		m.publishState(state)
	}
	candidate, err := m.source.LatestCandidate(ctx, types.ReleaseArtifactIdentity{Kind: localBoxArtifactID, ID: localBoxArtifactID})
	if err != nil {
		if state.Installed {
			m.publishState(state)
			return state, nil
		}
		failed := localBoxFailure(state, localBoxErrorCodeFrom(err, "LOCAL_BOX_RELEASE_UNAVAILABLE"), err.Error(), localBoxStatusCheckingRelease)
		m.publishState(failed)
		return failed, nil
	}
	if candidate == nil {
		if state.Installed {
			m.publishState(state)
			return state, nil
		}
		failed := localBoxFailure(state, "LOCAL_BOX_RELEASE_UNAVAILABLE", "当前来源没有可安装的业务端成品", localBoxStatusCheckingRelease)
		m.publishState(failed)
		return failed, nil
	}
	state.LatestVersion = candidate.Version
	state.ReleaseNotes = candidate.ReleaseNotes
	state.DownloadSize = candidate.SizeBytes
	if !state.Installed {
		state.Status = localBoxStatusReadyToInstall
	}
	m.publishState(state)
	return state, nil
}

// downloadAndVerify 取得目标压缩包、核对、安全解包并完成包内核对，
// 返回已经完整验收的解包结果；不判断适用范围、不切换当前版本。
func (m *localBoxManager) downloadAndVerify(ctx context.Context, candidate *releasecheck.ReleaseCandidate, workDir string, state localBoxState) (release.ValidatedPackage, localBoxState, error) {
	downloadDir := filepath.Join(workDir, "download")
	extractedDir := filepath.Join(workDir, "extracted")
	if m.source == nil {
		return release.ValidatedPackage{}, state, localBoxOperationFailure("LOCAL_BOX_RELEASE_UNAVAILABLE", errors.New("业务端候选读取能力未初始化"))
	}
	state.Status = localBoxStatusDownloading
	state.Progress = localBoxProgress{Phase: localBoxStatusDownloading, TotalBytes: candidate.SizeBytes}
	m.publishState(state)
	if err := writeLocalBoxInstallWorkState(workDir, state); err != nil {
		return release.ValidatedPackage{}, state, localBoxOperationFailure("LOCAL_BOX_DOWNLOAD_FAILED", err)
	}
	archivePath, err := m.source.AcquireArchive(ctx, candidate, downloadDir, func(progress localBoxProgress) {
		state.Progress = progress
		state.Progress.ReceivedBytes = progress.ReceivedBytes
		if progress.TotalBytes > 0 {
			state.Progress.TotalBytes = progress.TotalBytes
		}
		_ = writeLocalBoxInstallWorkState(workDir, state)
		m.publishState(state)
	})
	if err != nil {
		return release.ValidatedPackage{}, state, localBoxOperationFailure(localBoxErrorCodeFrom(err, "LOCAL_BOX_PACKAGE_INVALID"), err)
	}
	state.Status = localBoxStatusVerifying
	state.Progress.Phase = localBoxStatusVerifying
	state.Progress.ReceivedBytes = 0
	m.publishState(state)
	if err := writeLocalBoxInstallWorkState(workDir, state); err != nil {
		return release.ValidatedPackage{}, state, localBoxOperationFailure("LOCAL_BOX_PACKAGE_INVALID", err)
	}
	expected, err := m.source.ExpectedProduct(ctx, candidate)
	if err != nil {
		return release.ValidatedPackage{}, state, localBoxOperationFailure("LOCAL_BOX_PACKAGE_INVALID", err)
	}
	if err := release.ExtractArchive(release.ExtractArchiveOptions{ArchivePath: archivePath, TargetDir: extractedDir}); err != nil {
		return release.ValidatedPackage{}, state, localBoxOperationFailure("LOCAL_BOX_PACKAGE_INVALID", err)
	}
	validated, err := release.ValidateExtractedPackage(release.ValidateExtractedPackageOptions{Directory: extractedDir, Product: expected})
	if err != nil {
		return release.ValidatedPackage{}, state, localBoxOperationFailure("LOCAL_BOX_PACKAGE_INVALID", err)
	}
	return validated, state, nil
}

func localBoxInstallRecordFromProduct(product types.ReleaseProductRecord, paths localBoxPaths, installIdentity string, dataIdentity string, source localBoxSourceKind) localBoxInstallRecord {
	return localBoxInstallRecord{
		SchemaVersion: 1, Artifact: types.ReleaseArtifactIdentity{Kind: localBoxArtifactID, ID: localBoxArtifactID},
		Source: string(source), Version: product.Version, Platform: product.Platform, InstallIdentity: installIdentity,
		DataIdentity: dataIdentity, ProgramDir: paths.programDir, DataDir: paths.dataDir, RuntimeDir: paths.runtimeDir,
	}
}

func ensureInstallData(paths localBoxPaths) (localrun.DataIdentityRecord, error) {
	if err := os.MkdirAll(paths.rootDir, 0o700); err != nil {
		return localrun.DataIdentityRecord{}, fmt.Errorf("建立业务端安装目录失败：%w", err)
	}
	return localrun.EnsureDataIdentity(paths.dataDir)
}
