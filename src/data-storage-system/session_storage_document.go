package datastorage

import (
	"time"

	"eucli-box/pkg/types"
)

type sessionStorageDocument struct {
	ID          string                 `json:"id"`
	RoleID      string                 `json:"roleId"`
	GroupID     string                 `json:"groupId,omitempty"`
	WorkspaceID string                 `json:"workspaceId,omitempty"`
	Title       string                 `json:"title"`
	Status      string                 `json:"status"`
	Messages    []messageStorageRecord `json:"messages"`
	Metadata    map[string]string      `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"createdAt"`
	UpdatedAt   time.Time              `json:"updatedAt"`
	LastActive  time.Time              `json:"lastActive"`
}

type messageStorageRecord struct {
	ID              string                    `json:"id"`
	Type            string                    `json:"type"`
	SpeakerRoleID   string                    `json:"speakerRoleId,omitempty"`
	Content         string                    `json:"content,omitempty"`
	Control         *types.MessageControl     `json:"control,omitempty"`
	Error           *types.ErrorPayload       `json:"error,omitempty"`
	Parts           []types.MessagePart       `json:"parts,omitempty"`
	Attachments     []types.MessageAttachment `json:"attachments,omitempty"`
	ParentMessageID string                    `json:"parentMessageId,omitempty"`
	BranchID        string                    `json:"branchId,omitempty"`
	ToolID          string                    `json:"toolId,omitempty"`
	ToolName        string                    `json:"toolName,omitempty"`
	Reason          string                    `json:"reason,omitempty"`
	TokenEstimate   int                       `json:"tokenEstimate,omitempty"`
	CreatedAt       time.Time                 `json:"createdAt"`
	UpdatedAt       time.Time                 `json:"updatedAt"`
}

func toSessionStorageDocument(session types.Session) sessionStorageDocument {
	messages := make([]messageStorageRecord, 0, len(session.Messages))
	for _, message := range session.Messages {
		messages = append(messages, toMessageStorageRecord(message))
	}
	return sessionStorageDocument{
		ID:          session.ID,
		RoleID:      session.RoleID,
		GroupID:     session.GroupID,
		WorkspaceID: session.WorkspaceID,
		Title:       session.Title,
		Status:      session.Status,
		Messages:    messages,
		Metadata:    session.Metadata,
		CreatedAt:   session.CreatedAt,
		UpdatedAt:   session.UpdatedAt,
		LastActive:  session.LastActive,
	}
}

func toMessageStorageRecord(message types.Message) messageStorageRecord {
	content := message.Content
	if messageTextStoredInParts(message) {
		content = ""
	}
	return messageStorageRecord{
		ID:              message.ID,
		Type:            message.Type,
		SpeakerRoleID:   message.SpeakerRoleID,
		Content:         content,
		Control:         message.Control,
		Error:           message.Error,
		Parts:           message.Parts,
		Attachments:     message.Attachments,
		ParentMessageID: message.ParentMessageID,
		BranchID:        message.BranchID,
		ToolID:          message.ToolID,
		ToolName:        message.ToolName,
		Reason:          message.Reason,
		TokenEstimate:   message.TokenEstimate,
		CreatedAt:       message.CreatedAt,
		UpdatedAt:       message.UpdatedAt,
	}
}

func messageTextStoredInParts(message types.Message) bool {
	if message.Type != "user" && message.Type != "assistant" {
		return false
	}
	return message.Content != "" && textProjectionFromParts(message.Parts) == message.Content
}
