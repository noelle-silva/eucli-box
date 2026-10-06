package types

import (
	"strings"
	"time"
)

const DefaultSessionTitle = "新聊天"

const sessionTitleMaxRunes = 80

// NormalizeSessionTitle 把任意文本规范为合法的会话标题：
// 压缩空白、按上限截断、空文本回落默认标题。
func NormalizeSessionTitle(title string) string {
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		return DefaultSessionTitle
	}
	runes := []rune(title)
	if len(runes) > sessionTitleMaxRunes {
		return strings.TrimSpace(string(runes[:sessionTitleMaxRunes]))
	}
	return title
}

const MessagePartDisplayHideResult = "hideResult"

type Message struct {
	ID              string              `json:"id"`
	Type            string              `json:"type"`
	SpeakerRoleID   string              `json:"speakerRoleId,omitempty"`
	Content         string              `json:"content"`
	Control         *MessageControl     `json:"control,omitempty"`
	Error           *ErrorPayload       `json:"error,omitempty"`
	Parts           []MessagePart       `json:"parts,omitempty"`
	Attachments     []MessageAttachment `json:"attachments,omitempty"`
	ParentMessageID string              `json:"parentMessageId,omitempty"`
	BranchID        string              `json:"branchId,omitempty"`
	ToolID          string              `json:"toolId,omitempty"`
	ToolName        string              `json:"toolName,omitempty"`
	// AsyncToolTaskID 把异步结果消息与其来源任务绑定，作为回灌幂等的第二道防线。
	AsyncToolTaskID string    `json:"asyncToolTaskId,omitempty"`
	Reason          string    `json:"reason,omitempty"`
	TokenEstimate   int       `json:"tokenEstimate,omitempty"`
	ModelDurationMs int64     `json:"modelDurationMs,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

const (
	MessageTypeSystemControl   = "system_control"
	MessageTypeAsyncToolResult = "async_tool_result"

	MessageControlKindCompressionBoundary = "compression_boundary"
	MessageControlKindCompressionSummary  = "compression_summary"
)

type MessageControl struct {
	Kind                     string `json:"kind"`
	CommandName              string `json:"commandName,omitempty"`
	Source                   string `json:"source,omitempty"`
	SourceText               string `json:"sourceText,omitempty"`
	RetainRecentMessages     int    `json:"retainRecentMessages,omitempty"`
	PreviousSummaryMessageID string `json:"previousSummaryMessageId,omitempty"`
	CompressedUntilMessageID string `json:"compressedUntilMessageId,omitempty"`
	SummaryVersion           int    `json:"summaryVersion,omitempty"`
}

type ErrorPayload struct {
	Code    string          `json:"code,omitempty"`
	Message string          `json:"message"`
	System  string          `json:"system,omitempty"`
	Details any             `json:"details,omitempty"`
	Cause   *ErrorPayload   `json:"cause,omitempty"`
	Causes  []*ErrorPayload `json:"causes,omitempty"`
}

type RunRetryInfo struct {
	Attempt     int           `json:"attempt"`
	MaxAttempts int           `json:"maxAttempts"`
	RetryAt     time.Time     `json:"retryAt"`
	DelayMs     int           `json:"delayMs"`
	Message     string        `json:"message,omitempty"`
	Failure     *ErrorPayload `json:"failure,omitempty"`
}

type MessagePart struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Text       string          `json:"text,omitempty"`
	Source     string          `json:"source,omitempty"`
	Signature  string          `json:"signature,omitempty"`
	Data       string          `json:"data,omitempty"`
	Raw        string          `json:"raw,omitempty"`
	CallID     string          `json:"callId,omitempty"`
	ToolName   string          `json:"toolName,omitempty"`
	Input      map[string]any  `json:"input,omitempty"`
	State      string          `json:"state,omitempty"`
	Decision   *ToolDecision   `json:"decision,omitempty"`
	Result     *ToolPartResult `json:"result,omitempty"`
	Display    map[string]any  `json:"display,omitempty"`
	DurationMs int64           `json:"durationMs,omitempty"`
	CreatedAt  time.Time       `json:"createdAt,omitempty"`
	UpdatedAt  time.Time       `json:"updatedAt,omitempty"`
}

func (part MessagePart) IsToolResultHidden() bool {
	return messagePartDisplayTruthy(part.Display, MessagePartDisplayHideResult)
}

func messagePartDisplayTruthy(display map[string]any, key string) bool {
	if len(display) == 0 {
		return false
	}
	switch value := display[key].(type) {
	case bool:
		return value
	case string:
		return value == "true"
	default:
		return false
	}
}

type SessionMessagePatch struct {
	Content *string        `json:"content,omitempty"`
	Parts   *[]MessagePart `json:"parts,omitempty"`
}

type SessionSettingsPatch struct {
	StreamEnabled   *bool            `json:"streamEnabled,omitempty"`
	ReasoningEffort *string          `json:"reasoningEffort,omitempty"`
	ModelOverride   *ModelCoordinate `json:"modelOverride,omitempty"`
}

type SessionMessageSave struct {
	Session       Session                   `json:"session"`
	MetadataPatch map[string]string         `json:"metadataPatch,omitempty"`
	Writes        []SessionMessageWrite     `json:"writes,omitempty"`
	Deletes       []SessionMessageDelete    `json:"deletes,omitempty"`
	Conditions    []SessionMessageCondition `json:"conditions,omitempty"`
	Status        RunStatus                 `json:"status"`
}

type SessionMessageWrite struct {
	Message  Message  `json:"message"`
	Expected *Message `json:"expected,omitempty"`
}

type SessionMessageDelete struct {
	MessageID string   `json:"messageId"`
	Expected  *Message `json:"expected,omitempty"`
}

type SessionMessageCondition struct {
	MessageID string   `json:"messageId"`
	Expected  *Message `json:"expected,omitempty"`
}

type ToolDecision struct {
	ID        string    `json:"id"`
	ActionID  string    `json:"actionId"`
	ToolName  string    `json:"toolName"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

type ToolPartResult struct {
	ID         string         `json:"id"`
	ActionID   string         `json:"actionId"`
	ToolName   string         `json:"toolName"`
	Status     ToolStatus     `json:"status"`
	Content    string         `json:"content,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Error      string         `json:"error,omitempty"`
	DurationMs int64          `json:"durationMs,omitempty"`
	CreatedAt  time.Time      `json:"createdAt,omitempty"`
}

type MessageAttachment struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
	Mime string `json:"mime,omitempty"`
	Path string `json:"path,omitempty"`
	// PreviewPath 是小副本相对路径：发往模型的请求体默认使用它；
	// 空值表示没有小副本（多版本存图关闭或未生成）。
	PreviewPath string `json:"previewPath,omitempty"`
	// CallID 是产出该附件的工具调用标识；用户消息附件不带该值。
	CallID string `json:"callId,omitempty"`
}

type RunAttachment struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Mime    string `json:"mime,omitempty"`
	DataURL string `json:"dataUrl,omitempty"`
}

type Session struct {
	ID             string            `json:"id"`
	RoleID         string            `json:"roleId"`
	GroupID        string            `json:"groupId,omitempty"`
	WorkspaceID    string            `json:"workspaceId,omitempty"`
	Title          string            `json:"title"`
	Status         string            `json:"status"`
	Messages       []Message         `json:"messages"`
	AsyncToolTasks []AsyncToolTask   `json:"asyncToolTasks,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
	LastActive     time.Time         `json:"lastActive"`
}

type AsyncToolTaskStatus string

const (
	AsyncToolTaskStatusPending   AsyncToolTaskStatus = "pending"
	AsyncToolTaskStatusRunning   AsyncToolTaskStatus = "running"
	AsyncToolTaskStatusSucceeded AsyncToolTaskStatus = "succeeded"
	AsyncToolTaskStatusFailed    AsyncToolTaskStatus = "failed"
	AsyncToolTaskStatusCompleted AsyncToolTaskStatus = "completed"
)

type RunContinuation struct {
	Stream             *bool            `json:"stream,omitempty"`
	ReasoningEffort    ReasoningEffort  `json:"reasoningEffort,omitempty"`
	ModelOverride      *ModelCoordinate `json:"modelOverride,omitempty"`
	HookPromptMode     string           `json:"hookPromptMode,omitempty"`
	HookPromptPresetID string           `json:"hookPromptPresetId,omitempty"`
}

type AsyncToolTask struct {
	ID                 string              `json:"id"`
	RunID              string              `json:"runId,omitempty"`
	RoleID             string              `json:"roleId,omitempty"`
	GroupID            string              `json:"groupId,omitempty"`
	WorkspaceID        string              `json:"workspaceId,omitempty"`
	SessionID          string              `json:"sessionId"`
	AssistantMessageID string              `json:"assistantMessageId,omitempty"`
	TaskName           string              `json:"taskName"`
	ToolName           string              `json:"toolName"`
	Status             AsyncToolTaskStatus `json:"status"`
	Continuation       RunContinuation     `json:"continuation,omitempty"`
	Action             ToolAction          `json:"action"`
	Plan               ToolRunPlan         `json:"plan,omitempty"`
	Result             *ToolResult         `json:"result,omitempty"`
	Error              string              `json:"error,omitempty"`
	SubmittedAt        time.Time           `json:"submittedAt"`
	StartedAt          time.Time           `json:"startedAt,omitempty"`
	FinishedAt         time.Time           `json:"finishedAt,omitempty"`
	CompletedAt        time.Time           `json:"completedAt,omitempty"`
	// InjectionClaimedAt 是跨运行回灌认领标记：非零表示已有运行认领了该任务的回灌权。
	// 认领与完成都是单调终态标记，迟到的运行期副本不得抹掉它们。
	InjectionClaimedAt  time.Time `json:"injectionClaimedAt,omitempty"`
	InjectionClaimRunID string    `json:"injectionClaimRunId,omitempty"`
}

// MergeAsyncToolTask 合并同一任务的两个副本，保护单调的终态标记：
// 已完成或已认领的权威副本，不被缺少这些标记的迟到副本覆盖。
func MergeAsyncToolTask(current AsyncToolTask, incoming AsyncToolTask) AsyncToolTask {
	if strings.TrimSpace(incoming.ID) == "" {
		return current
	}
	if !current.CompletedAt.IsZero() && incoming.CompletedAt.IsZero() {
		return current
	}
	if !current.InjectionClaimedAt.IsZero() && incoming.InjectionClaimedAt.IsZero() && incoming.CompletedAt.IsZero() {
		merged := incoming
		merged.InjectionClaimedAt = current.InjectionClaimedAt
		merged.InjectionClaimRunID = current.InjectionClaimRunID
		return merged
	}
	return incoming
}

// MergeAsyncToolTasks 以任务 ID 为键合并两组任务，保持首次出现顺序。
func MergeAsyncToolTasks(current []AsyncToolTask, incoming []AsyncToolTask) []AsyncToolTask {
	if len(current) == 0 && len(incoming) == 0 {
		return nil
	}
	byID := map[string]AsyncToolTask{}
	order := []string{}
	add := func(task AsyncToolTask) {
		id := strings.TrimSpace(task.ID)
		if id == "" {
			return
		}
		if existing, ok := byID[id]; ok {
			byID[id] = MergeAsyncToolTask(existing, task)
			return
		}
		order = append(order, id)
		byID[id] = task
	}
	for _, task := range current {
		add(task)
	}
	for _, task := range incoming {
		add(task)
	}
	result := make([]AsyncToolTask, 0, len(order))
	for _, id := range order {
		result = append(result, byID[id])
	}
	return result
}

type AsyncToolTaskQuery struct {
	RoleID      string `json:"roleId,omitempty"`
	GroupID     string `json:"groupId,omitempty"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	SessionID   string `json:"sessionId,omitempty"`
}

type SessionSummary struct {
	ID                 string    `json:"id"`
	RoleID             string    `json:"roleId"`
	GroupID            string    `json:"groupId,omitempty"`
	WorkspaceID        string    `json:"workspaceId,omitempty"`
	Title              string    `json:"title"`
	LastMessagePreview string    `json:"lastMessagePreview"`
	Status             string    `json:"status"`
	UpdatedAt          time.Time `json:"updatedAt"`
	LastActive         time.Time `json:"lastActive"`
}

type RunStatus string

const (
	RunStatusCreated             RunStatus = "created"
	RunStatusRunning             RunStatus = "running"
	RunStatusWaitingConfirmation RunStatus = "waiting_confirmation"
	RunStatusCompleted           RunStatus = "completed"
	RunStatusFailed              RunStatus = "failed"
	RunStatusCancelled           RunStatus = "cancelled"
)

type RunRequest struct {
	RoleID             string           `json:"roleId"`
	GroupID            string           `json:"groupId,omitempty"`
	WorkspaceID        string           `json:"workspaceId,omitempty"`
	SessionID          string           `json:"sessionId"`
	Message            string           `json:"message"`
	Attachments        []RunAttachment  `json:"attachments,omitempty"`
	ParentMessageID    string           `json:"parentMessageId,omitempty"`
	UserMessageID      string           `json:"userMessageId,omitempty"`
	ContextMessageID   string           `json:"contextMessageId,omitempty"`
	ModelOverride      *ModelCoordinate `json:"modelOverride,omitempty"`
	ReasoningEffort    ReasoningEffort  `json:"reasoningEffort,omitempty"`
	HookPromptMode     string           `json:"hookPromptMode,omitempty"`
	HookPromptPresetID string           `json:"hookPromptPresetId,omitempty"`
	Stream             *bool            `json:"stream,omitempty"`
}

type RunState struct {
	ID                   string        `json:"id"`
	RoleID               string        `json:"roleId"`
	GroupID              string        `json:"groupId,omitempty"`
	WorkspaceID          string        `json:"workspaceId,omitempty"`
	SessionID            string        `json:"sessionId"`
	InputMessageID       string        `json:"inputMessageId,omitempty"`
	LastMessageID        string        `json:"lastMessageId,omitempty"`
	DependencyMessageIDs []string      `json:"dependencyMessageIds,omitempty"`
	Stream               bool          `json:"stream,omitempty"`
	Status               RunStatus     `json:"status"`
	Reason               string        `json:"reason,omitempty"`
	Retry                *RunRetryInfo `json:"retry"`
	Error                *ErrorPayload `json:"error,omitempty"`
	CreatedAt            time.Time     `json:"createdAt"`
	UpdatedAt            time.Time     `json:"updatedAt"`
}

// RunMessageDelta 只承载某条助手消息本次新增的变化：
// 正文增量、思考增量，以及消息诞生所需的标识。前端把它叠加到本地消息上。
type RunMessageDelta struct {
	RunID              string        `json:"runId"`
	RoleID             string        `json:"roleId"`
	GroupID            string        `json:"groupId,omitempty"`
	WorkspaceID        string        `json:"workspaceId,omitempty"`
	SessionID          string        `json:"sessionId"`
	MessageID          string        `json:"messageId"`
	ParentMessageID    string        `json:"parentMessageId,omitempty"`
	BranchID           string        `json:"branchId,omitempty"`
	SpeakerRoleID      string        `json:"speakerRoleId,omitempty"`
	MessageType        string        `json:"messageType,omitempty"`
	MessageCreatedAt   time.Time     `json:"messageCreatedAt"`
	Stream             bool          `json:"stream,omitempty"`
	Status             RunStatus     `json:"status,omitempty"`
	ContentDelta       string        `json:"contentDelta,omitempty"`
	ContentReset       bool          `json:"contentReset,omitempty"`
	ReasoningDelta     string        `json:"reasoningDelta,omitempty"`
	ReasoningReset     bool          `json:"reasoningReset,omitempty"`
	ReasoningSource    string        `json:"reasoningSource,omitempty"`
	ReasoningSignature string        `json:"reasoningSignature,omitempty"`
	ReasoningData      string        `json:"reasoningData,omitempty"`
	PartsDelta         []MessagePart `json:"partsDelta,omitempty"`
	CreatedAt          time.Time     `json:"createdAt"`
}

type RunAssistantMessageUpdate struct {
	RunID       string        `json:"runId"`
	RoleID      string        `json:"roleId"`
	GroupID     string        `json:"groupId,omitempty"`
	WorkspaceID string        `json:"workspaceId,omitempty"`
	SessionID   string        `json:"sessionId"`
	Stream      bool          `json:"stream,omitempty"`
	Status      RunStatus     `json:"status,omitempty"`
	Reason      string        `json:"reason,omitempty"`
	Retry       *RunRetryInfo `json:"retry"`
	Error       *ErrorPayload `json:"error,omitempty"`
	Message     Message       `json:"message"`
	CreatedAt   time.Time     `json:"createdAt"`
}

type RunEvent struct {
	ID          string    `json:"id"`
	RunID       string    `json:"runId"`
	GroupID     string    `json:"groupId,omitempty"`
	WorkspaceID string    `json:"workspaceId,omitempty"`
	Type        string    `json:"type"`
	Payload     any       `json:"payload,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ToolOutputUpdateEventPayload carries live tool output progress to clients.
type ToolOutputUpdateEventPayload struct {
	RunID       string    `json:"runId"`
	RoleID      string    `json:"roleId,omitempty"`
	GroupID     string    `json:"groupId,omitempty"`
	WorkspaceID string    `json:"workspaceId,omitempty"`
	SessionID   string    `json:"sessionId"`
	MessageID   string    `json:"messageId,omitempty"`
	CallID      string    `json:"callId"`
	ToolName    string    `json:"toolName,omitempty"`
	Bytes       uint64    `json:"bytes"`
	Preview     string    `json:"preview"`
	CreatedAt   time.Time `json:"createdAt"`
}
