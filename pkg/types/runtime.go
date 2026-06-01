package types

import "time"

type Message struct {
	ID              string    `json:"id"`
	Type            string    `json:"type"`
	Content         string    `json:"content"`
	ParentMessageID string    `json:"parentMessageId,omitempty"`
	BranchID        string    `json:"branchId,omitempty"`
	ToolID          string    `json:"toolId,omitempty"`
	ToolName        string    `json:"toolName,omitempty"`
	Reason          string    `json:"reason,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
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
	RoleID          string `json:"roleId"`
	SessionID       string `json:"sessionId"`
	Message         string `json:"message"`
	ParentMessageID string `json:"parentMessageId,omitempty"`
	UserMessageID   string `json:"userMessageId,omitempty"`
}

type RunState struct {
	ID        string    `json:"id"`
	RoleID    string    `json:"roleId"`
	SessionID string    `json:"sessionId"`
	Status    RunStatus `json:"status"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type RunEvent struct {
	ID        string    `json:"id"`
	RunID     string    `json:"runId"`
	Type      string    `json:"type"`
	Payload   any       `json:"payload,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}
