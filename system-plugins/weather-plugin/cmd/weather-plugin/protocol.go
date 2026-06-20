package main

type pluginRequest struct {
	Action        string         `json:"action"`
	UserConfig    map[string]any `json:"userConfig"`
	DefaultConfig map[string]any `json:"defaultConfig"`
}

type pluginResponse struct {
	Status string            `json:"status"`
	Values map[string]string `json:"values,omitempty"`
	Error  string            `json:"error,omitempty"`
}

const (
	actionResolvePlaceholders = "resolve_placeholders"
	weatherDetailInterfaceID  = "weather-detail"
	weatherBriefInterfaceID   = "weather-brief"
)
