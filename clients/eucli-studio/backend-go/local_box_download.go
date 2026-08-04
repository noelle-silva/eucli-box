package main

import (
	"context"
	"encoding/json"
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
	state.LatestVersion = candidate.Manifest.Version
	state.ReleaseNotes = candidate.ReleaseNotes
	state.DownloadSize = candidate.Manifest.Archive.Size
	if !state.Installed {
		state.Status = localBoxStatusReadyToInstall
	}
	m.publishState(state)
	return state, nil
}

func (m *localBoxManager) downloadAndVerify(ctx context.Context, candidate *releasecheck.ReleaseCandidate, workDir string, state localBoxState) (types.ReleaseManifest, localBoxState, error) {
	downloadDir := filepath.Join(workDir, "download")
	extractedDir := filepath.Join(workDir, "extracted")
	if m.source == nil {
		return types.ReleaseManifest{}, state, localBoxOperationFailure("LOCAL_BOX_RELEASE_UNAVAILABLE", errors.New("业务端候选读取能力未初始化"))
	}
	state.Status = localBoxStatusDownloading
	state.Progress = localBoxProgress{Phase: localBoxStatusDownloading, TotalBytes: candidate.ManifestSize}
	m.publishState(state)
	if err := writeLocalBoxInstallWorkState(workDir, state); err != nil {
		return types.ReleaseManifest{}, state, localBoxOperationFailure("LOCAL_BOX_DOWNLOAD_FAILED", err)
	}
	manifest, err := m.source.AcquireArtifacts(ctx, candidate, downloadDir, func(progress localBoxProgress) {
		state.Progress = progress
		state.Progress.ReceivedBytes = progress.ReceivedBytes
		if progress.TotalBytes > 0 {
			state.Progress.TotalBytes = progress.TotalBytes
		}
		_ = writeLocalBoxInstallWorkState(workDir, state)
		m.publishState(state)
	})
	if err != nil {
		return types.ReleaseManifest{}, state, localBoxOperationFailure(localBoxErrorCodeFrom(err, "LOCAL_BOX_PACKAGE_INVALID"), err)
	}
	state.Status = localBoxStatusVerifying
	state.Progress.Phase = localBoxStatusVerifying
	state.Progress.ReceivedBytes = 0
	m.publishState(state)
	if err := writeLocalBoxInstallWorkState(workDir, state); err != nil {
		return types.ReleaseManifest{}, state, localBoxOperationFailure("LOCAL_BOX_PACKAGE_INVALID", err)
	}
	if err := release.ExtractArchive(release.ExtractArchiveOptions{ArchivePath: filepath.Join(downloadDir, manifest.Archive.Name), TargetDir: extractedDir}); err != nil {
		return types.ReleaseManifest{}, state, localBoxOperationFailure("LOCAL_BOX_PACKAGE_INVALID", err)
	}
	if _, err := release.ValidateExtractedPackage(release.ValidateExtractedPackageOptions{Directory: extractedDir, Manifest: manifest}); err != nil {
		return types.ReleaseManifest{}, state, localBoxOperationFailure("LOCAL_BOX_PACKAGE_INVALID", err)
	}
	return manifest, state, nil
}

func sameReleaseManifest(left types.ReleaseManifest, right types.ReleaseManifest) bool {
	leftPayload, leftErr := json.Marshal(left)
	rightPayload, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftPayload) == string(rightPayload)
}

func localBoxInstallRecordFromManifest(manifest types.ReleaseManifest, paths localBoxPaths, installIdentity string, dataIdentity string, source localBoxSourceKind) localBoxInstallRecord {
	return localBoxInstallRecord{
		SchemaVersion: 1, Artifact: types.ReleaseArtifactIdentity{Kind: localBoxArtifactID, ID: localBoxArtifactID},
		Source: string(source), Version: manifest.Version, Platform: manifest.Platform, InstallIdentity: installIdentity,
		DataIdentity: dataIdentity, ProgramDir: paths.programDir, DataDir: paths.dataDir, RuntimeDir: paths.runtimeDir,
	}
}

func ensureInstallData(paths localBoxPaths) (localrun.DataIdentityRecord, error) {
	if err := os.MkdirAll(paths.rootDir, 0o700); err != nil {
		return localrun.DataIdentityRecord{}, fmt.Errorf("建立业务端安装目录失败：%w", err)
	}
	return localrun.EnsureDataIdentity(paths.dataDir)
}
