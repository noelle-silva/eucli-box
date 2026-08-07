package types

import "time"

type EucliBoxCompatibility struct {
	MinimumVersion          string `json:"minimumVersion"`
	MaximumVersionExclusive string `json:"maximumVersionExclusive"`
}

type CompatibilityStatus struct {
	Compatible                    bool                  `json:"compatible"`
	Reason                        string                `json:"reason,omitempty"`
	CurrentEucliBoxVersion        string                `json:"currentEucliBoxVersion,omitempty"`
	RequiredEucliBoxCompatibility EucliBoxCompatibility `json:"requiredEucliBoxCompatibility"`
}

type EucliBoxRelease struct {
	Version     string `json:"version"`
	DataVersion string `json:"dataVersion"`
}

type EucliBoxReleaseInfo struct {
	Version             string               `json:"version"`
	DataVersion         string               `json:"dataVersion"`
	ClientCompatibility *CompatibilityStatus `json:"clientCompatibility,omitempty"`
}

const (
	ReleaseArtifactKindBox    = "eucli-box"
	ReleaseArtifactKindTool   = "tool"
	ReleaseArtifactKindPlugin = "plugin"

	ReleasePlatformWindowsX64 = "windows-x64"

	ReleaseCheckStatusNotChecked = "not_checked"
	ReleaseCheckStatusChecking   = "checking"
	ReleaseCheckStatusCompleted  = "completed"
	ReleaseCheckStatusFailed     = "failed"
)

type ReleaseArtifactIdentity struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type OfficialReleaseSource struct {
	Kind       string `json:"kind"`
	Repository string `json:"repository"`
	Owner      string `json:"owner"`
	Name       string `json:"name"`
	Ref        string `json:"ref,omitempty"`
}

type ReleaseSourceRecord struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Recorded   bool   `json:"recorded"`
}

type ReleaseFileRecord struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type ReleaseExternalAsset struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Version     string `json:"version"`
	PackagePath string `json:"packagePath"`
	FileCount   int    `json:"fileCount"`
	TreeSHA256  string `json:"treeSha256"`
}

type ReleaseProductRecord struct {
	SchemaVersion    int                     `json:"schemaVersion"`
	Artifact         ReleaseArtifactIdentity `json:"artifact"`
	Version          string                  `json:"version"`
	Platform         string                  `json:"platform"`
	OfficialSource   string                  `json:"officialSource"`
	Compatibility    *EucliBoxCompatibility  `json:"eucliBoxCompatibility,omitempty"`
	Source           ReleaseSourceRecord     `json:"source"`
	DataVersion      string                  `json:"dataVersion,omitempty"`
	ExternalAssets   []ReleaseExternalAsset  `json:"externalAssets,omitempty"`
	VerificationOnly bool                    `json:"verificationOnly,omitempty"`
}

type ReleaseManifest struct {
	SchemaVersion    int                     `json:"schemaVersion"`
	Artifact         ReleaseArtifactIdentity `json:"artifact"`
	Version          string                  `json:"version"`
	Platform         string                  `json:"platform"`
	TagName          string                  `json:"tagName"`
	OfficialSource   string                  `json:"officialSource"`
	Compatibility    *EucliBoxCompatibility  `json:"eucliBoxCompatibility,omitempty"`
	Source           ReleaseSourceRecord     `json:"source"`
	DataVersion      string                  `json:"dataVersion,omitempty"`
	ExternalAssets   []ReleaseExternalAsset  `json:"externalAssets,omitempty"`
	VerificationOnly bool                    `json:"verificationOnly,omitempty"`
	Archive          ReleaseFileRecord       `json:"archive"`
	Files            []ReleaseFileRecord     `json:"files"`
}

type ReleaseCheckResult struct {
	Artifact          ReleaseArtifactIdentity   `json:"artifact"`
	Source            OfficialReleaseSource     `json:"source"`
	Installed         bool                      `json:"installed"`
	CurrentVersion    string                    `json:"currentVersion,omitempty"`
	LatestVersion     string                    `json:"latestVersion,omitempty"`
	Status            string                    `json:"status"`
	CheckedAt         time.Time                 `json:"checkedAt,omitempty"`
	PublishedAt       time.Time                 `json:"publishedAt,omitempty"`
	IndexUpdatedAt    time.Time                 `json:"indexUpdatedAt,omitempty"`
	UpdateAvailable   bool                      `json:"updateAvailable"`
	ReleaseURL        string                    `json:"releaseUrl,omitempty"`
	ReleaseNotes      string                    `json:"releaseNotes,omitempty"`
	DownloadSize      int64                     `json:"downloadSize,omitempty"`
	Compatibility     *CompatibilityStatus      `json:"compatibility,omitempty"`
	AffectedArtifacts []ReleaseArtifactIdentity `json:"affectedArtifacts,omitempty"`
	FailureReason     string                    `json:"failureReason,omitempty"`
}

type ReleaseCheckSnapshot struct {
	Status        string               `json:"status"`
	StartedAt     time.Time            `json:"startedAt,omitempty"`
	CheckedAt     time.Time            `json:"checkedAt,omitempty"`
	Results       []ReleaseCheckResult `json:"results"`
	FailureReason string               `json:"failureReason,omitempty"`
}

// ArtifactInstallState 表示单个发布物（工具或插件）当前对外展示的整体安装/更新状态。
// Status 与 Phase 是两个互不替代的事实，各自只使用自己的词表，不得混用。
type ArtifactInstallState struct {
	OperationID    string                   `json:"operationId,omitempty"`
	Artifact       ReleaseArtifactIdentity  `json:"artifact"`
	Installed      bool                     `json:"installed"`
	CurrentVersion string                   `json:"currentVersion,omitempty"`
	TargetVersion  string                   `json:"targetVersion,omitempty"`
	Status         string                   `json:"status"`
	Phase          string                   `json:"phase,omitempty"`
	Progress       ReleaseOperationProgress `json:"progress,omitempty"`
	Error          ReleaseOperationError    `json:"error,omitempty"`
}

type ReleaseOperationProgress struct {
	ReceivedBytes int64 `json:"receivedBytes"`
	TotalBytes    int64 `json:"totalBytes"`
}

type ReleaseOperationError struct {
	Code    string `json:"code,omitempty"`
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

// ArtifactActivityState 表示发布物当前的真实活动状态，用于展示阻止原因。
type ArtifactActivityState struct {
	Artifact       ReleaseArtifactIdentity `json:"artifact"`
	Active         bool                    `json:"active"`
	ActiveRequests int                     `json:"activeRequests"`
	Updating       bool                    `json:"updating"`
	Reason         string                  `json:"reason,omitempty"`
}

const (
	ArtifactStatusNotInstalled     = "not_installed"
	ArtifactStatusCheckingRelease  = "checking_release"
	ArtifactStatusReadyToInstall   = "ready_to_install"
	ArtifactStatusReadyToUpdate    = "ready_to_update"
	ArtifactStatusDownloading      = "downloading"
	ArtifactStatusVerifying        = "verifying"
	ArtifactStatusCheckingActivity = "checking_activity"
	ArtifactStatusPreparing        = "preparing"
	ArtifactStatusSwitching        = "switching"
	ArtifactStatusStarting         = "starting"
	ArtifactStatusActive           = "active"
	ArtifactStatusUnavailable      = "unavailable"
	ArtifactStatusFailed           = "failed"
	ArtifactStatusBlocked          = "blocked"
	ArtifactStatusRestoring        = "restoring"
)

const (
	ArtifactPhaseCandidate    = "candidate"
	ArtifactPhaseCompatibility = "compatibility"
	ArtifactPhaseActivity     = "activity"
	ArtifactPhaseDownload     = "download"
	ArtifactPhaseManifest     = "manifest"
	ArtifactPhaseArchive      = "archive"
	ArtifactPhasePackage      = "package"
	ArtifactPhasePrepare      = "prepare"
	ArtifactPhaseSwitch       = "switch"
	ArtifactPhaseProbe        = "probe"
	ArtifactPhaseRestore      = "restore"
	ArtifactPhaseRefresh      = "refresh"
)

// 固定错误码：工具和插件后台、网关、客户端统一使用，不得各自造同义名称。
const (
	ArtifactErrorNotInstalled       = "ARTIFACT_NOT_INSTALLED"
	ArtifactErrorReleaseUnavailable = "ARTIFACT_RELEASE_UNAVAILABLE"
	ArtifactErrorCandidateMismatch  = "ARTIFACT_CANDIDATE_MISMATCH"
	ArtifactErrorCompatibility      = "ARTIFACT_COMPATIBILITY_FAILED"
	ArtifactErrorDownloadFailed     = "ARTIFACT_DOWNLOAD_FAILED"
	ArtifactErrorManifestInvalid    = "ARTIFACT_MANIFEST_INVALID"
	ArtifactErrorPackageInvalid     = "ARTIFACT_PACKAGE_INVALID"
	ArtifactErrorPathInvalid        = "ARTIFACT_PATH_INVALID"
	ArtifactErrorToolActive         = "TOOL_ACTIVE"
	ArtifactErrorPluginActive       = "PLUGIN_ACTIVE"
	ArtifactErrorUpdateInProgress   = "ARTIFACT_UPDATE_IN_PROGRESS"
	ArtifactErrorPrepareFailed      = "ARTIFACT_PREPARE_FAILED"
	ArtifactErrorSwitchFailed       = "ARTIFACT_SWITCH_FAILED"
	ArtifactErrorProbeFailed        = "ARTIFACT_PROBE_FAILED"
	ArtifactErrorRestoreFailed      = "ARTIFACT_RESTORE_FAILED"
	ArtifactErrorStateUnknown       = "ARTIFACT_STATE_UNKNOWN"
	ArtifactErrorDataChanged        = "ARTIFACT_DATA_CHANGED"
)
