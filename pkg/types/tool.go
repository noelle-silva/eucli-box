package types

import "time"

type ToolDefinition struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	PromptDescription string         `json:"promptDescription,omitempty"`
	Type              string         `json:"type"`
	InputSchema       map[string]any `json:"inputSchema,omitempty"`
	UserConfig        map[string]any `json:"userConfig,omitempty"`
	DefaultConfig     map[string]any `json:"defaultConfig,omitempty"`
	Directory         string         `json:"directory,omitempty"`
	Binaries          []ToolBinary   `json:"binaries,omitempty"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
}

type ToolBinary struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	Path   string `json:"path"`
}

type ToolExecutionInput struct {
	ActionID             string         `json:"actionId"`
	ToolName             string         `json:"toolName"`
	Arguments            map[string]any `json:"arguments"`
	UserConfig           map[string]any `json:"userConfig"`
	DefaultConfig        map[string]any `json:"defaultConfig"`
	ToolDirectory        string         `json:"toolDirectory"`
	HostWorkingDirectory string         `json:"hostWorkingDirectory"`
}

type ToolExecutionOutput struct {
	Status   ToolStatus     `json:"status"`
	Content  string         `json:"content"`
	Error    string         `json:"error,omitempty"`
	Metadata map[string]any `json:"metadata"`
}

type ToolSummary struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Type        string    `json:"type"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ToolIntent struct {
	ID        string         `json:"id"`
	ToolName  string         `json:"toolName"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Source    string         `json:"source,omitempty"`
	Raw       string         `json:"raw,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}

const (
	ToolCallSourceNative       = "native"
	ToolCallSourceTextProtocol = "text_protocol"
)

type ToolAction struct {
	ID        string         `json:"id"`
	ToolName  string         `json:"toolName"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Source    string         `json:"source,omitempty"`
	Raw       string         `json:"raw,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}

type ToolRunPlan struct {
	ID         string             `json:"id"`
	Action     ToolAction         `json:"action"`
	Tool       ToolDefinition     `json:"tool"`
	Decision   PermissionDecision `json:"decision"`
	PlanStatus ToolPlanStatus     `json:"planStatus"`
	Executable string             `json:"executable,omitempty"`
	CreatedAt  time.Time          `json:"createdAt"`
}

type ToolPlanStatus string

const (
	ToolPlanStatusReady             ToolPlanStatus = "ready"
	ToolPlanStatusNeedsConfirmation ToolPlanStatus = "needs_confirmation"
	ToolPlanStatusDenied            ToolPlanStatus = "denied"
)

type ToolStatus string

const (
	ToolStatusSuccess   ToolStatus = "success"
	ToolStatusFailed    ToolStatus = "failed"
	ToolStatusDenied    ToolStatus = "denied"
	ToolStatusCancelled ToolStatus = "cancelled"
)

type ToolResult struct {
	ID        string         `json:"id"`
	ActionID  string         `json:"actionId"`
	ToolName  string         `json:"toolName"`
	Status    ToolStatus     `json:"status"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Error     string         `json:"error,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}

type ToolConfirmation struct {
	ID         string    `json:"id"`
	DecisionID string    `json:"decisionId"`
	Approved   bool      `json:"approved"`
	Reason     string    `json:"reason,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type PermissionDecision struct {
	ID        string    `json:"id"`
	ActionID  string    `json:"actionId"`
	ToolName  string    `json:"toolName"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
}

const (
	PermissionStatusAllowed           = "allowed"
	PermissionStatusDenied            = "denied"
	PermissionStatusNeedsConfirmation = "needs_confirmation"
)
