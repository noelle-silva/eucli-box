package types

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
	Version string `json:"version"`
}

type EucliBoxReleaseInfo struct {
	Version             string               `json:"version"`
	ClientCompatibility *CompatibilityStatus `json:"clientCompatibility,omitempty"`
}
