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
	Capabilities              []ToolCapability      `json:"capabilities,omitempty"`
	InputSchema               map[string]any        `json:"inputSchema,omitempty"`
	UserConfigSchema          map[string]any        `json:"userConfigSchema,omitempty"`
	UserConfig                map[string]any        `json:"userConfig,omitempty"`
	CapabilityGrants          map[string]bool       `json:"capabilityGrants,omitempty"`
	DefaultConfig             map[string]any        `json:"defaultConfig,omitempty"`
	BodyDirectory             string                `json:"bodyDirectory,omitempty"`
	DataDirectory             string                `json:"dataDirectory,omitempty"`
	// ConfigFiles 是工具配置区文件的运行时读取视图，只由设置页读取链路填充，
	// 不随工具定义落盘。
	ConfigFiles []ToolConfigFile `json:"configFiles,omitempty"`
	Binaries    []ToolBinary     `json:"binaries,omitempty"`
	CreatedAt   time.Time        `json:"createdAt"`
	UpdatedAt   time.Time        `json:"updatedAt"`
}

// ToolOutputUpdate is a live output progress for one tool call, relayed by the
// tool over the control channel during execution.
type ToolOutputUpdate struct {
	CallID   string `json:"callId"`
	ToolName string `json:"toolName,omitempty"`
	Bytes    uint64 `json:"bytes"`
	Preview  string `json:"preview"`
}

type ToolUserSettings struct {
	UserConfig                map[string]any  `json:"userConfig"`
	PromptDescriptionOverride string          `json:"promptDescriptionOverride,omitempty"`
	CapabilityGrants          map[string]bool `json:"capabilityGrants,omitempty"`
	// ConfigFiles 是随保存请求一起提交的配置区文件写入意图；
	// 文件内容落 config/ 文件区，不写进 settings.json。
	ConfigFiles []ToolConfigFileWrite `json:"configFiles,omitempty"`
	UpdatedAt   time.Time             `json:"updatedAt,omitempty"`
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
	// WorkspaceID 是声明了工作区身份能力时注入的当前工作区标识；
	// 不在工作区会话里就没有这个字段。
	WorkspaceID string `json:"workspaceId,omitempty"`
	TimeoutMs   int64  `json:"timeoutMs,omitempty"`
	RequestKind string `json:"requestKind,omitempty"`
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

// ToolStopResult reports how many active executions of one tool were stopped.
type ToolStopResult struct {
	Terminated int `json:"terminated"`
}

type ToolIntent struct {
	ID             string             `json:"id"`
	ToolName       string             `json:"toolName"`
	Arguments      map[string]any     `json:"arguments,omitempty"`
	InvocationMode ToolInvocationMode `json:"invocationMode,omitempty"`
	Raw            string             `json:"raw,omitempty"`
	// ArgumentError 记录该次调用参数文本无法解析的原因；
	// 非空表示单点参数错误，由运行时包装为该次调用的失败结果，不升级为整轮失败。
	ArgumentError string    `json:"argumentError,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
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

type ToolAction struct {
	ID string `json:"id"`
	// CallRef 是运行期为本次工具调用分配的内部唯一标识，用于在助手消息上
	// 定位该调用独占的工具部件；模型给的 ID 只作对账标记，同一响应内可重复。
	CallRef        string             `json:"callRef,omitempty"`
	ToolName       string             `json:"toolName"`
	Arguments      map[string]any     `json:"arguments,omitempty"`
	InvocationMode ToolInvocationMode `json:"invocationMode,omitempty"`
	Raw            string             `json:"raw,omitempty"`
	CreatedAt      time.Time          `json:"createdAt"`
}

// ToolRunScope 是一次工具执行的会话身份：能力服务以此访问会话资源。
// RunID 是发起这次工具调用的运行：外部确认要摆回这条运行上，才有人能看见。
type ToolRunScope struct {
	RunID       string `json:"runId,omitempty"`
	RoleID      string `json:"roleId,omitempty"`
	GroupID     string `json:"groupId,omitempty"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	SessionID   string `json:"sessionId,omitempty"`
}

type ToolRunPlan struct {
	ID string `json:"id"`
	// RoleID 是权限裁决的主体标识（与 Scope.RoleID 同源的历史字段）；
	// 执行身份与能力服务一律以 Scope 为唯一来源。
	RoleID         string              `json:"roleId,omitempty"`
	Scope          ToolRunScope        `json:"scope,omitempty"`
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
	ID                  string              `json:"id"`
	ActionID            string              `json:"actionId"`
	ToolName            string              `json:"toolName"`
	Status              ToolStatus          `json:"status"`
	Content             string              `json:"content"`
	Metadata            map[string]any      `json:"metadata,omitempty"`
	Error               string              `json:"error,omitempty"`
	ProducedAttachments []MessageAttachment `json:"producedAttachments,omitempty"`
	DurationMs          int64               `json:"durationMs,omitempty"`
	CreatedAt           time.Time           `json:"createdAt"`
}

// 宿主补进正文的两类事实各有专属标签：工具执行收尾与异步任务回灌。
// 专属标签名把「宿主说的事实」与「工具自己的输出」在语法上分开，
// 模型读到不会把两者混成同一份数据，工具输出里自带的标签也不会被误认。
const (
	ToolExecutionMarkerTag = "eucli-box-tool-execution"
	ToolAsyncResultTag     = "eucli-box-async-tool-result"
)

// 宿主替工具收尾时写进标签的说明：一句话说清是谁、因为什么、最后怎么了。
// 说明统一英文，避免同一份正文里中英混排。
const (
	// 取消：停止前已经有输出，产出留在上面。
	ToolExecutionNoticeCancelledAfterOutput = "User stopped this execution. The output above was produced before the stop."
	// 取消：一个字都没来得及输出。
	ToolExecutionNoticeCancelledWithoutOutput = "User stopped this execution before any output was produced."
	// 取消：当时正等着用户确认。
	ToolExecutionNoticeCancelledDuringConfirmation = "User stopped this execution while it was awaiting confirmation."
	// 收口：整轮被停止时这个工具还没有结果。
	ToolExecutionNoticeRunCancelled = "The run was cancelled before this tool produced a result."
	// 收口：整轮失败时这个工具还没有结果。
	ToolExecutionNoticeRunFailed = "The run failed before this tool produced a result."
	// 受理：任务已转交后台。
	ToolExecutionNoticeAsyncAccepted = "Task accepted. The result will be delivered when it finishes."
	// 后台任务所在进程结束，任务没跑完。
	ToolExecutionNoticeAsyncProcessGone = "The background task process ended before the task finished."
	// 失败与拒绝的公共开头，具体原因接在冒号后面。
	ToolExecutionNoticeFailedPrefix = "Execution failed: "
	ToolExecutionNoticeDeniedPrefix = "Tool call denied: "
)

// ToolExecutionStateAccepted 是异步任务受理时宿主写的收尾状态：
// 它不是工具结果状态，而是「这次调用已转交后台」的运行期事实。
const ToolExecutionStateAccepted = "accepted"

// ToolExecutionNoticeFailed 把失败原因接在统一开头后面；没有原因时只留开头。
func ToolExecutionNoticeFailed(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return strings.TrimSuffix(ToolExecutionNoticeFailedPrefix, ": ")
	}
	return ToolExecutionNoticeFailedPrefix + reason
}

// ToolExecutionNoticeDenied 把拒绝原因接在统一开头后面；没有原因时只留开头。
func ToolExecutionNoticeDenied(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return strings.TrimSuffix(ToolExecutionNoticeDeniedPrefix, ": ")
	}
	return ToolExecutionNoticeDeniedPrefix + reason
}

// xmlTextEscaper 转义文本节点与属性值里的 XML 元字符；
// 工具输出里出现的尖括号与引号因此不会把标记结构撑破。
var xmlTextEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&apos;",
)

// ToolExecutionMarker 生成一段宿主收尾标记：
// state 属性写明终局状态，文本节点写明具体是什么情况；
// 没有说明时输出自闭合标签。
func ToolExecutionMarker(state string, detail string) string {
	open := "<" + ToolExecutionMarkerTag + ` state="` + escapeXMLText(strings.TrimSpace(state)) + `"`
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return open + " />"
	}
	return open + ">" + escapeXMLText(detail) + "</" + ToolExecutionMarkerTag + ">"
}

// AppendToolExecutionMarker 把收尾标记追加到已产出内容的末尾。
// 已产出内容原样保留（只去掉尾部换行），标记另起一段；
// 没有产出内容时，正文就是标记本身。
func AppendToolExecutionMarker(content string, state string, detail string) string {
	marker := ToolExecutionMarker(state, detail)
	content = strings.TrimRight(content, "\r\n")
	if strings.TrimSpace(content) == "" {
		return marker
	}
	return content + "\n\n" + marker
}

// WrapToolAsyncResult 用身份标签把回灌正文整段包起来：
// 属性声明任务身份，标签中间是给人和模型读的整段说明。
// 正文原样嵌入、不做转义——正文里可能已经带了宿主收尾标记，
// 转义会把内层标记变成死文本，读的人就看不出它属于谁了。
func WrapToolAsyncResult(body string, taskID string, toolName string, status string) string {
	open := "<" + ToolAsyncResultTag +
		` task-id="` + escapeXMLText(strings.TrimSpace(taskID)) + `"` +
		` tool="` + escapeXMLText(strings.TrimSpace(toolName)) + `"` +
		` status="` + escapeXMLText(strings.TrimSpace(status)) + `"`
	body = strings.TrimRight(body, "\r\n")
	if strings.TrimSpace(body) == "" {
		return open + " />"
	}
	return open + ">\n" + body + "\n</" + ToolAsyncResultTag + ">"
}

func escapeXMLText(value string) string {
	return xmlTextEscaper.Replace(value)
}

type ToolConfirmation struct {
	ID         string `json:"id"`
	DecisionID string `json:"decisionId"`
	Approved   bool   `json:"approved"`
	// RememberForSession 表示用户选择“本会话内始终同意”：
	// 本次放行后把该工具写入会话放行清单，本会话内后续调用直接执行。
	RememberForSession bool      `json:"rememberForSession,omitempty"`
	Reason             string    `json:"reason,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
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
