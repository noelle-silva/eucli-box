package types

type ToolBinary struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	Path   string `json:"path"`
}

// ToolRequestKindWarmup marks a tool request as a warm-up: the host asks the
// tool to prepare itself; no user arguments are executed. An empty request
// kind is the ordinary execution request, so execution payloads carry no
// request-kind field at all.
const ToolRequestKindWarmup = "warmup"

type ToolExecutionInput struct {
	ActionID             string                `json:"actionId"`
	ToolName             string                `json:"toolName"`
	Arguments            map[string]any        `json:"arguments"`
	UserConfig           map[string]any        `json:"userConfig"`
	DefaultConfig        map[string]any        `json:"defaultConfig"`
	ToolBodyDirectory    string                `json:"toolBodyDirectory"`
	ToolDataDirectory    string                `json:"toolDataDirectory"`
	HostWorkingDirectory string                `json:"hostWorkingDirectory"`
	Workspace            *ToolWorkspaceContext `json:"workspace,omitempty"`
	TimeoutMs            int64                 `json:"timeoutMs,omitempty"`
	RequestKind          string                `json:"requestKind,omitempty"`
}

// IsToolWarmupRequest reports whether a tool input is a warm-up request.
func IsToolWarmupRequest(input ToolExecutionInput) bool {
	return input.RequestKind == ToolRequestKindWarmup
}

type ToolExecutionOutput struct {
	Status   ToolStatus     `json:"status"`
	Content  string         `json:"content"`
	Error    string         `json:"error,omitempty"`
	Metadata map[string]any `json:"metadata"`
}

type ToolStatus string

const (
	ToolStatusSuccess ToolStatus = "success"
	ToolStatusFailed  ToolStatus = "failed"
	ToolStatusDenied  ToolStatus = "denied"
)
