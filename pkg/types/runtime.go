package types

import "time"

type Message struct {
	ID              string              `json:"id"`
	Type            string              `json:"type"`
	Content         string              `json:"content"`
	Attachments     []MessageAttachment `json:"attachments,omitempty"`
	ParentMessageID string              `json:"parentMessageId,omitempty"`
	BranchID        string              `json:"branchId,omitempty"`
	ToolID          string              `json:"toolId,omitempty"`
	ToolName        string              `json:"toolName,omitempty"`
	Reason          string              `json:"reason,omitempty"`
	CreatedAt       time.Time           `json:"createdAt"`
	UpdatedAt       time.Time           `json:"updatedAt"`
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
	Stream          bool            `json:"stream,omitempty"`
}

type RunState struct {
	ID             string    `json:"id"`
	RoleID         string    `json:"roleId"`
	SessionID      string    `json:"sessionId"`
	InputMessageID string    `json:"inputMessageId,omitempty"`
	LastMessageID  string    `json:"lastMessageId,omitempty"`
	Stream         bool      `json:"stream,omitempty"`
	Status         RunStatus `json:"status"`
	Reason         string    `json:"reason,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
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

type RunEvent struct {
	ID        string    `json:"id"`
	RunID     string    `json:"runId"`
	Type      string    `json:"type"`
	Payload   any       `json:"payload,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}
