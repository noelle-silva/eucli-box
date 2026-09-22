package types

const (
	// 托管参数：启动时机。
	SystemPluginStartBoot = "boot"
	SystemPluginStartLazy = "lazy"

	// 托管参数：崩溃重启策略。
	SystemPluginRestartOnFailure = "on-failure"
	SystemPluginRestartNever     = "never"

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

// SystemPluginCapability 是插件在清单中声明的能力；占位符取值是第一种能力。
type SystemPluginCapability struct {
	Type       string                             `json:"type"`
	Interfaces []SystemPluginPlaceholderInterface `json:"interfaces,omitempty"`
}

// SystemPluginHosting 是插件的托管参数：启动时机、常驻、崩溃重启、限时停机。
type SystemPluginHosting struct {
	Start         string `json:"start"`
	Resident      bool   `json:"resident"`
	Restart       string `json:"restart"`
	StopTimeoutMs int64  `json:"stopTimeoutMs"`
}

type SystemPluginManifest struct {
	ProtocolVersion       int                      `json:"protocolVersion"`
	ID                    string                   `json:"id"`
	Name                  string                   `json:"name"`
	Description           string                   `json:"description"`
	Version               string                   `json:"version"`
	EucliBoxCompatibility EucliBoxCompatibility    `json:"eucliBoxCompatibility"`
	Hosting               SystemPluginHosting      `json:"hosting"`
	Binaries              []SystemPluginBinary     `json:"binaries"`
	ConfigSchema          map[string]any           `json:"configSchema,omitempty"`
	Capabilities          []SystemPluginCapability `json:"capabilities,omitempty"`
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
	Hosting               SystemPluginHosting                    `json:"hosting"`
	Status                string                                 `json:"status"`
	StatusMessage         string                                 `json:"statusMessage,omitempty"`
	Installed             bool                                   `json:"installed,omitempty"`
	CurrentVersion        string                                 `json:"currentVersion,omitempty"`
	InstallStatus         string                                 `json:"installStatus,omitempty"`
	InstallPhase          string                                 `json:"installPhase,omitempty"`
	OperationID           string                                 `json:"operationId,omitempty"`
	Active                bool                                   `json:"active,omitempty"`
	Enabled               bool                                   `json:"enabled"`
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
	Hosting               SystemPluginHosting   `json:"hosting"`
	Status                string                `json:"status"`
	StatusMessage         string                `json:"statusMessage,omitempty"`
	Installed             bool                  `json:"installed,omitempty"`
	CurrentVersion        string                `json:"currentVersion,omitempty"`
	InstallStatus         string                `json:"installStatus,omitempty"`
	InstallPhase          string                `json:"installPhase,omitempty"`
	OperationID           string                `json:"operationId,omitempty"`
	Active                bool                  `json:"active,omitempty"`
	Enabled               bool                  `json:"enabled"`
}

type SystemPluginAvailablePlaceholderInterface struct {
	PluginID             string `json:"pluginId"`
	PluginName           string `json:"pluginName"`
	InterfaceID          string `json:"interfaceId"`
	InterfaceDescription string `json:"interfaceDescription"`
	PlaceholderName      string `json:"placeholderName"`
	Disabled             bool   `json:"disabled,omitempty"`
}

type SystemPluginCreatePlaceholderRequest struct {
	PluginID    string `json:"pluginId"`
	InterfaceID string `json:"interfaceId"`
}

// SystemPluginPlaceholderSource 是一次按来源点名的取值请求：
// 只向该接口的所属插件发起调用，未引用的插件与接口不会被联系。
type SystemPluginPlaceholderSource struct {
	PluginID    string `json:"pluginId"`
	InterfaceID string `json:"interfaceId"`
	Name        string `json:"name,omitempty"`
}

type SystemPluginPlaceholderValue struct {
	PluginID    string `json:"pluginId"`
	InterfaceID string `json:"interfaceId"`
	Name        string `json:"name"`
	Value       string `json:"value"`
}
