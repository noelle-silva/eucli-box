package main

import "eucli-box/pkg/types"

const (
	localBoxStatusNotInstalled    = "not_installed"
	localBoxStatusCheckingRelease = "checking_release"
	localBoxStatusReadyToInstall  = "ready_to_install"
	localBoxStatusDownloading     = "downloading"
	localBoxStatusVerifying       = "verifying"
	localBoxStatusInstalling      = "installing"
	localBoxStatusStarting        = "starting"
	localBoxStatusConnected       = "connected"
	localBoxStatusFailed          = "failed"
	localBoxStatusStopping        = "stopping"
	localBoxStatusStopped         = "stopped"
)

const localBoxArtifactID = types.ReleaseArtifactKindBox

type localBoxProgress struct {
	Phase         string `json:"phase"`
	ReceivedBytes int64  `json:"receivedBytes"`
	TotalBytes    int64  `json:"totalBytes"`
}

type localBoxError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Phase   string `json:"phase"`
}

type localBoxState struct {
	Status         string                        `json:"status"`
	Artifact       types.ReleaseArtifactIdentity `json:"artifact"`
	Source         string                        `json:"source"`
	Installed      bool                          `json:"installed"`
	CurrentVersion string                        `json:"currentVersion"`
	LatestVersion  string                        `json:"latestVersion"`
	ReleaseNotes   string                        `json:"releaseNotes"`
	DownloadSize   int64                         `json:"downloadSize"`
	Progress       localBoxProgress              `json:"progress"`
	Error          localBoxError                 `json:"error"`
	Connected      bool                          `json:"connected"`
}

func initialLocalBoxState() localBoxState {
	return localBoxState{
		Status:   localBoxStatusNotInstalled,
		Artifact: types.ReleaseArtifactIdentity{Kind: localBoxArtifactID, ID: localBoxArtifactID},
		Progress: localBoxProgress{},
		Error:    localBoxError{},
	}
}

func localBoxFailure(state localBoxState, code string, message string, phase string) localBoxState {
	state.Status = localBoxStatusFailed
	state.Connected = false
	state.Error = localBoxError{Code: code, Message: message, Phase: phase}
	return state
}
