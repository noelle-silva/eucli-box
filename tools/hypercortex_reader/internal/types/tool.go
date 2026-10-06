package types

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

type ToolExecutionOutput struct {
	Status   ToolStatus     `json:"status"`
	Content  string         `json:"content"`
	Error    string         `json:"error,omitempty"`
	Metadata map[string]any `json:"metadata"`
}

type ToolStatus string

const (
	ToolStatusSuccess   ToolStatus = "success"
	ToolStatusFailed    ToolStatus = "failed"
	ToolStatusDenied    ToolStatus = "denied"
	ToolStatusCancelled ToolStatus = "cancelled"
)
