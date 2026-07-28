package types

import (
	"strings"
	"time"
)

const (
	ToolAvailabilityActive      = "active"
	ToolAvailabilityUnavailable = "unavailable"
)

type ToolDefinition struct {
	ID                        string                `json:"id"`
	Name                      string                `json:"name"`
	Description               string                `json:"description"`
	Version                   string                `json:"version"`
	EucliBoxCompatibility     EucliBoxCompatibility `json:"eucliBoxCompatibility"`
	Compatibility             CompatibilityStatus   `json:"compatibility"`
	Status                    string                `json:"status,omitempty"`
	StatusMessage             string                `json:"statusMessage,omitempty"`
	PromptDescription         string                `json:"promptDescription,omitempty"`
	PromptDescriptionOverride string                `json:"promptDescriptionOverride,omitempty"`
	DefaultInvocationMode     ToolInvocationMode    `json:"defaultInvocationMode,omitempty"`
	Type                      string                `json:"type"`
	InputSchema               map[string]any        `json:"inputSchema,omitempty"`
	UserConfigSchema          map[string]any        `json:"userConfigSchema,omitempty"`
	UserConfig                map[string]any        `json:"userConfig,omitempty"`
	DefaultConfig             map[string]any        `json:"defaultConfig,omitempty"`
	BodyDirectory             string                `json:"bodyDirectory,omitempty"`
	DataDirectory             string                `json:"dataDirectory,omitempty"`
	Binaries                  []ToolBinary          `json:"binaries,omitempty"`
	CreatedAt                 time.Time             `json:"createdAt"`
	UpdatedAt                 time.Time             `json:"updatedAt"`
}

type ToolUserSettings struct {
	UserConfig                map[string]any `json:"userConfig"`
	PromptDescriptionOverride string         `json:"promptDescriptionOverride,omitempty"`
	UpdatedAt                 time.Time      `json:"updatedAt,omitempty"`
}

func ToolPromptDescription(tool ToolDefinition) string {
	if override := tool.PromptDescriptionOverride; strings.TrimSpace(override) != "" {
		return override
	}
	if promptDescription := tool.PromptDescription; strings.TrimSpace(promptDescription) != "" {
		return promptDescription
	}
	return tool.Description
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
	ToolBodyDirectory    string         `json:"toolBodyDirectory"`
	ToolDataDirectory    string         `json:"toolDataDirectory"`
	HostWorkingDirectory string         `json:"hostWorkingDirectory"`
}

type ToolExecutionOutput struct {
	Status   ToolStatus     `json:"status"`
	Content  string         `json:"content"`
	Error    string         `json:"error,omitempty"`
	Metadata map[string]any `json:"metadata"`
}

type ToolSummary struct {
	ID                    string                `json:"id"`
	Name                  string                `json:"name"`
	Description           string                `json:"description"`
	Version               string                `json:"version,omitempty"`
	EucliBoxCompatibility EucliBoxCompatibility `json:"eucliBoxCompatibility"`
	Compatibility         CompatibilityStatus   `json:"compatibility"`
	Status                string                `json:"status,omitempty"`
	StatusMessage         string                `json:"statusMessage,omitempty"`
	Type                  string                `json:"type"`
	UpdatedAt             time.Time             `json:"updatedAt"`
}

type ToolIntent struct {
	ID             string             `json:"id"`
	ToolName       string             `json:"toolName"`
	Arguments      map[string]any     `json:"arguments,omitempty"`
	InvocationMode ToolInvocationMode `json:"invocationMode,omitempty"`
	Source         string             `json:"source,omitempty"`
	Raw            string             `json:"raw,omitempty"`
	CreatedAt      time.Time          `json:"createdAt"`
}

type ToolInvocationMode string

const (
	ToolInvocationModeSync  ToolInvocationMode = "sync"
	ToolInvocationModeAsync ToolInvocationMode = "async"
)

func NormalizeToolInvocationMode(mode ToolInvocationMode) ToolInvocationMode {
	switch CleanToolInvocationMode(mode) {
	case ToolInvocationModeAsync:
		return ToolInvocationModeAsync
	default:
		return ToolInvocationModeSync
	}
}

func CleanToolInvocationMode(mode ToolInvocationMode) ToolInvocationMode {
	return ToolInvocationMode(strings.TrimSpace(string(mode)))
}

func ValidToolInvocationMode(mode ToolInvocationMode) bool {
	switch CleanToolInvocationMode(mode) {
	case "", ToolInvocationModeSync, ToolInvocationModeAsync:
		return true
	default:
		return false
	}
}

func ValidExplicitToolInvocationMode(mode ToolInvocationMode) bool {
	switch CleanToolInvocationMode(mode) {
	case ToolInvocationModeSync, ToolInvocationModeAsync:
		return true
	default:
		return false
	}
}

const (
	ToolCallSourceNative       = "native"
	ToolCallSourceTextProtocol = "text_protocol"
)

type ToolAction struct {
	ID             string             `json:"id"`
	ToolName       string             `json:"toolName"`
	Arguments      map[string]any     `json:"arguments,omitempty"`
	InvocationMode ToolInvocationMode `json:"invocationMode,omitempty"`
	Source         string             `json:"source,omitempty"`
	Raw            string             `json:"raw,omitempty"`
	CreatedAt      time.Time          `json:"createdAt"`
}

type ToolRunPlan struct {
	ID             string              `json:"id"`
	RoleID         string              `json:"roleId,omitempty"`
	Action         ToolAction          `json:"action"`
	Tool           ToolDefinition      `json:"tool"`
	InvocationMode ToolInvocationMode  `json:"invocationMode,omitempty"`
	Decision       PermissionDecision  `json:"decision"`
	WorkspaceFence *ToolWorkspaceFence `json:"workspaceFence,omitempty"`
	PlanStatus     ToolPlanStatus      `json:"planStatus"`
	Executable     string              `json:"executable,omitempty"`
	CreatedAt      time.Time           `json:"createdAt"`
}

type ToolWorkspaceFence struct {
	WorkspaceID           string                   `json:"workspaceId"`
	RegisteredDirectories []WorkspaceDirectory     `json:"registeredDirectories,omitempty"`
	Paths                 []ToolWorkspaceFencePath `json:"paths,omitempty"`
	RequiresConfirmation  bool                     `json:"requiresConfirmation"`
}

type ToolWorkspaceFencePath struct {
	Argument              string `json:"argument"`
	RawPath               string `json:"rawPath"`
	AbsolutePath          string `json:"absolutePath"`
	WithinWorkspace       bool   `json:"withinWorkspace"`
	MatchedDirectoryAlias string `json:"matchedDirectoryAlias,omitempty"`
	Reason                string `json:"reason,omitempty"`
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
	ID        string         `json:"id"`
	ActionID  string         `json:"actionId"`
	ToolName  string         `json:"toolName"`
	Status    string         `json:"status"`
	Reason    string         `json:"reason"`
	Details   map[string]any `json:"details,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}

const (
	PermissionStatusAllowed           = "allowed"
	PermissionStatusDenied            = "denied"
	PermissionStatusNeedsConfirmation = "needs_confirmation"
)
