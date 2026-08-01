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
