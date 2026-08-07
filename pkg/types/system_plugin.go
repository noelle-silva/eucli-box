package types

const (
	SystemPluginLifecyclePersistent      = "persistent"
	SystemPluginLifecycleOnDemand        = "on-demand"
	SystemPluginLifecycleCachedHeartbeat = "cached-heartbeat"

	SystemPluginStatusActive      = "active"
	SystemPluginStatusUnavailable = "unavailable"
)

type SystemPluginBinary struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	Path   string `json:"path"`
}

type SystemPluginPlaceholderInterface struct {
	ID          string `json:"id"`
	DefaultName string `json:"defaultName"`
	Description string `json:"description"`
}

type SystemPluginManifest struct {
	ID                    string                             `json:"id"`
	Name                  string                             `json:"name"`
	Description           string                             `json:"description"`
	Version               string                             `json:"version"`
	EucliBoxCompatibility EucliBoxCompatibility              `json:"eucliBoxCompatibility"`
	LifecycleType         string                             `json:"lifecycleType"`
	HeartbeatIntervalMs   int64                              `json:"heartbeatIntervalMs,omitempty"`
	Binaries              []SystemPluginBinary               `json:"binaries"`
	ConfigSchema          map[string]any                     `json:"configSchema,omitempty"`
	PlaceholderInterfaces []SystemPluginPlaceholderInterface `json:"placeholderInterfaces"`
}

type SystemPluginUserConfig struct {
	UserConfig               map[string]any    `json:"userConfig,omitempty"`
	PlaceholderNameOverrides map[string]string `json:"placeholderNameOverrides,omitempty"`
}

type SystemPluginPlaceholderInterfaceView struct {
	ID            string `json:"id"`
	DefaultName   string `json:"defaultName"`
	EffectiveName string `json:"effectiveName"`
	Description   string `json:"description"`
}

type SystemPluginView struct {
	ID                    string                                 `json:"id"`
	SourceID              string                                 `json:"sourceId"`
	Name                  string                                 `json:"name"`
	Description           string                                 `json:"description"`
	Version               string                                 `json:"version"`
	EucliBoxCompatibility EucliBoxCompatibility                  `json:"eucliBoxCompatibility"`
	Compatibility         CompatibilityStatus                    `json:"compatibility"`
	LifecycleType         string                                 `json:"lifecycleType"`
	Status                string                                 `json:"status"`
	StatusMessage         string                                 `json:"statusMessage,omitempty"`
	Installed             bool                                   `json:"installed,omitempty"`
	CurrentVersion        string                                 `json:"currentVersion,omitempty"`
	InstallStatus         string                                 `json:"installStatus,omitempty"`
	InstallPhase          string                                 `json:"installPhase,omitempty"`
	OperationID           string                                 `json:"operationId,omitempty"`
	Active                bool                                   `json:"active,omitempty"`
	DefaultConfig         map[string]any                         `json:"defaultConfig,omitempty"`
	UserConfig            map[string]any                         `json:"userConfig,omitempty"`
	ConfigSchema          map[string]any                         `json:"configSchema,omitempty"`
	PlaceholderInterfaces []SystemPluginPlaceholderInterfaceView `json:"placeholderInterfaces"`
}

type SystemPluginSummary struct {
	ID                    string                `json:"id"`
	SourceID              string                `json:"sourceId"`
	Name                  string                `json:"name"`
	Description           string                `json:"description"`
	Version               string                `json:"version"`
	EucliBoxCompatibility EucliBoxCompatibility `json:"eucliBoxCompatibility"`
	Compatibility         CompatibilityStatus   `json:"compatibility"`
	LifecycleType         string                `json:"lifecycleType"`
	Status                string                `json:"status"`
	StatusMessage         string                `json:"statusMessage,omitempty"`
	Installed             bool                  `json:"installed,omitempty"`
	CurrentVersion        string                `json:"currentVersion,omitempty"`
	InstallStatus         string                `json:"installStatus,omitempty"`
	InstallPhase          string                `json:"installPhase,omitempty"`
	OperationID           string                `json:"operationId,omitempty"`
	Active                bool                  `json:"active,omitempty"`
}

type SystemPluginAvailablePlaceholderInterface struct {
	PluginID             string `json:"pluginId"`
	PluginName           string `json:"pluginName"`
	InterfaceID          string `json:"interfaceId"`
	InterfaceDescription string `json:"interfaceDescription"`
	PlaceholderName      string `json:"placeholderName"`
}

type SystemPluginCreatePlaceholderRequest struct {
	PluginID    string `json:"pluginId"`
	InterfaceID string `json:"interfaceId"`
}

type SystemPluginPlaceholderValue struct {
	PluginID    string `json:"pluginId"`
	InterfaceID string `json:"interfaceId"`
	Name        string `json:"name"`
	Value       string `json:"value"`
}

type SystemPluginPlaceholderRequest struct {
	Action                string                                 `json:"action"`
	PluginID              string                                 `json:"pluginId"`
	PlaceholderInterfaces []SystemPluginPlaceholderInterfaceView `json:"placeholderInterfaces"`
	UserConfig            map[string]any                         `json:"userConfig,omitempty"`
	DefaultConfig         map[string]any                         `json:"defaultConfig,omitempty"`
	PluginDirectory       string                                 `json:"pluginDirectory"`
	PluginDataDirectory   string                                 `json:"pluginDataDirectory"`
	HostWorkingDirectory  string                                 `json:"hostWorkingDirectory"`
}

type SystemPluginPlaceholderResponse struct {
	Status string            `json:"status"`
	Values map[string]string `json:"values,omitempty"`
	Error  string            `json:"error,omitempty"`
}
