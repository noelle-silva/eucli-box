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
	localBoxStatusWaitingStop     = "waiting_stop"
	localBoxStatusSwitching       = "switching"
	localBoxStatusRestoring       = "restoring"
)

// 业务端更新与恢复错误码：分类固定，限制必须有名字、有分类、有解释。
const (
	// LOCAL_BOX_UPDATE_FAILED：更新流程失败（下载、准备、切换、启动验收等环节）。
	localBoxErrorUpdateFailed = "LOCAL_BOX_UPDATE_FAILED"
	// LOCAL_BOX_UPDATE_BLOCKED：真实工作保护，用户稍后可重试；不下载、不留等待任务。
	localBoxErrorUpdateBlocked = "LOCAL_BOX_UPDATE_BLOCKED"
	// LOCAL_BOX_RESTORE_FAILED：恢复上一版失败，保留现场，停止重试。
	localBoxErrorRestoreFailed = "LOCAL_BOX_RESTORE_FAILED"
	// LOCAL_BOX_DATA_UNSAFE：数据状态无法确认或恢复失败，不启动任何版本，需要人工处理。
	localBoxErrorDataUnsafe = "LOCAL_BOX_DATA_UNSAFE"
	// LOCAL_BOX_MIGRATION_RECOVERED：信息说明类；程序更换成功、数据迁移未完成已恢复，下次启动重试迁移。
	localBoxInfoMigrationRecovered = "LOCAL_BOX_MIGRATION_RECOVERED"
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
	TargetVersion  string                        `json:"targetVersion"`
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
