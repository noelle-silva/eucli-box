package types

import "time"

const DefaultSessionTitle = "新聊天"

const (
	MessagePartDisplayHideInvocation = "hideInvocation"
	MessagePartDisplayHideResult     = "hideResult"
)

type Message struct {
	ID              string              `json:"id"`
	Type            string              `json:"type"`
	Content         string              `json:"content"`
	Error           *ErrorPayload       `json:"error,omitempty"`
	Parts           []MessagePart       `json:"parts,omitempty"`
	Attachments     []MessageAttachment `json:"attachments,omitempty"`
	ParentMessageID string              `json:"parentMessageId,omitempty"`
	BranchID        string              `json:"branchId,omitempty"`
	ToolID          string              `json:"toolId,omitempty"`
	ToolName        string              `json:"toolName,omitempty"`
	Reason          string              `json:"reason,omitempty"`
	CreatedAt       time.Time           `json:"createdAt"`
	UpdatedAt       time.Time           `json:"updatedAt"`
}

type ErrorPayload struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	System  string `json:"system,omitempty"`
	Details any    `json:"details,omitempty"`
}

type MessagePart struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Source    string          `json:"source,omitempty"`
	Raw       string          `json:"raw,omitempty"`
	CallID    string          `json:"callId,omitempty"`
	ToolName  string          `json:"toolName,omitempty"`
	Input     map[string]any  `json:"input,omitempty"`
	State     string          `json:"state,omitempty"`
	Decision  *ToolDecision   `json:"decision,omitempty"`
	Result    *ToolPartResult `json:"result,omitempty"`
	Display   map[string]any  `json:"display,omitempty"`
	CreatedAt time.Time       `json:"createdAt,omitempty"`
	UpdatedAt time.Time       `json:"updatedAt,omitempty"`
}

func (part MessagePart) IsToolInvocationHidden() bool {
	return messagePartDisplayTruthy(part.Display, MessagePartDisplayHideInvocation)
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
	ID        string         `json:"id"`
	ActionID  string         `json:"actionId"`
	ToolName  string         `json:"toolName"`
	Status    ToolStatus     `json:"status"`
	Content   string         `json:"content,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Error     string         `json:"error,omitempty"`
	CreatedAt time.Time      `json:"createdAt,omitempty"`
}

type MessageAttachment struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Mime    string `json:"mime,omitempty"`
	Path    string `json:"path,omitempty"`
	Lang    string `json:"lang,omitempty"`
	Text    string `json:"text,omitempty"`
	FullLen int    `json:"fullLen,omitempty"`
	SendLen int    `json:"sendLen,omitempty"`
	SendPct int    `json:"sendPct,omitempty"`
}

type RunAttachment struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Mime    string `json:"mime,omitempty"`
	DataURL string `json:"dataUrl,omitempty"`
	Lang    string `json:"lang,omitempty"`
	Text    string `json:"text,omitempty"`
	FullLen int    `json:"fullLen,omitempty"`
	SendLen int    `json:"sendLen,omitempty"`
	SendPct int    `json:"sendPct,omitempty"`
}

type Session struct {
	ID         string            `json:"id"`
	RoleID     string            `json:"roleId"`
	Title      string            `json:"title"`
	Status     string            `json:"status"`
	Messages   []Message         `json:"messages"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
	LastActive time.Time         `json:"lastActive"`
}

type SessionSummary struct {
	ID         string    `json:"id"`
	RoleID     string    `json:"roleId"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	UpdatedAt  time.Time `json:"updatedAt"`
	LastActive time.Time `json:"lastActive"`
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
	RoleID          string          `json:"roleId"`
	SessionID       string          `json:"sessionId"`
	Message         string          `json:"message"`
	Attachments     []RunAttachment `json:"attachments,omitempty"`
	ParentMessageID string          `json:"parentMessageId,omitempty"`
	UserMessageID   string          `json:"userMessageId,omitempty"`
	ReasoningEffort ReasoningEffort `json:"reasoningEffort,omitempty"`
	Stream          bool            `json:"stream,omitempty"`
}

type RunState struct {
	ID                   string        `json:"id"`
	RoleID               string        `json:"roleId"`
	SessionID            string        `json:"sessionId"`
	InputMessageID       string        `json:"inputMessageId,omitempty"`
	LastMessageID        string        `json:"lastMessageId,omitempty"`
	DependencyMessageIDs []string      `json:"dependencyMessageIds,omitempty"`
	Stream               bool          `json:"stream,omitempty"`
	Status               RunStatus     `json:"status"`
	Reason               string        `json:"reason,omitempty"`
	Error                *ErrorPayload `json:"error,omitempty"`
	CreatedAt            time.Time     `json:"createdAt"`
	UpdatedAt            time.Time     `json:"updatedAt"`
}

type RunStreamDelta struct {
	RunID           string    `json:"runId"`
	RoleID          string    `json:"roleId"`
	SessionID       string    `json:"sessionId"`
	MessageID       string    `json:"messageId"`
	ParentMessageID string    `json:"parentMessageId,omitempty"`
	BranchID        string    `json:"branchId,omitempty"`
	ContentDelta    string    `json:"contentDelta"`
	Content         string    `json:"content"`
	CreatedAt       time.Time `json:"createdAt"`
}

type RunAssistantMessageUpdate struct {
	RunID     string        `json:"runId"`
	RoleID    string        `json:"roleId"`
	SessionID string        `json:"sessionId"`
	Stream    bool          `json:"stream,omitempty"`
	Status    RunStatus     `json:"status,omitempty"`
	Reason    string        `json:"reason,omitempty"`
	Error     *ErrorPayload `json:"error,omitempty"`
	Message   Message       `json:"message"`
	CreatedAt time.Time     `json:"createdAt"`
}

type RunEvent struct {
	ID        string    `json:"id"`
	RunID     string    `json:"runId"`
	Type      string    `json:"type"`
	Payload   any       `json:"payload,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}
