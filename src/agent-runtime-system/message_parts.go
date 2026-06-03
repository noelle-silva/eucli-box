package agentruntime

import (
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

func activeRunAssistant(record *runRecord) (types.Message, bool) {
	messageID := strings.TrimSpace(record.activeAssistantID)
	if messageID == "" {
		return types.Message{}, false
	}
	message, ok := messageByID(record.session.Messages, messageID)
	if !ok || message.Type != "assistant" {
		return types.Message{}, false
	}
	return message, true
}

func setMessageTextPart(message *types.Message, content string, now time.Time) {
	parts := make([]types.MessagePart, 0, len(message.Parts)+1)
	textAdded := false
	if content != "" {
		for _, part := range message.Parts {
			if part.Type != "text" {
				continue
			}
			part.Text = content
			if strings.TrimSpace(part.ID) == "" {
				part.ID = utils.NewID("part")
			}
			if part.CreatedAt.IsZero() {
				part.CreatedAt = now
			}
			part.UpdatedAt = now
			parts = append(parts, part)
			textAdded = true
			break
		}
		if !textAdded {
			parts = append(parts, types.MessagePart{ID: utils.NewID("part"), Type: "text", Text: content, CreatedAt: now, UpdatedAt: now})
		}
	}
	for _, part := range message.Parts {
		if part.Type == "text" {
			continue
		}
		parts = append(parts, part)
	}
	message.Parts = parts
}

func hasToolParts(message types.Message) bool {
	for _, part := range message.Parts {
		if part.Type == "tool" {
			return true
		}
	}
	return false
}

func upsertRunToolPart(record *runRecord, action types.ToolAction, state string, decision *types.PermissionDecision, result *types.ToolResult) {
	ensureRunAssistantMessage(record)
	callID := strings.TrimSpace(action.ID)
	if callID == "" {
		callID = utils.NewID("tool-call")
	}
	now := time.Now().UTC()
	messageID := strings.TrimSpace(record.activeAssistantID)
	for index := range record.session.Messages {
		if record.session.Messages[index].ID != messageID {
			continue
		}
		upsertMessageToolPart(&record.session.Messages[index], action, callID, state, decision, result, now)
		record.session.Messages[index].UpdatedAt = now
		record.messageParent = record.session.Messages[index]
		record.lastMessageID = record.messageParent.ID
		record.session.UpdatedAt = now
		record.session.LastActive = now
		return
	}
}

func upsertMessageToolPart(message *types.Message, action types.ToolAction, callID string, state string, decision *types.PermissionDecision, result *types.ToolResult, now time.Time) {
	for index := range message.Parts {
		part := &message.Parts[index]
		if part.Type != "tool" || strings.TrimSpace(part.CallID) != callID {
			continue
		}
		applyToolPart(part, action, callID, state, decision, result, now)
		return
	}
	part := types.MessagePart{ID: utils.NewID("part"), Type: "tool", CreatedAt: now}
	applyToolPart(&part, action, callID, state, decision, result, now)
	message.Parts = append(message.Parts, part)
}

func applyToolPart(part *types.MessagePart, action types.ToolAction, callID string, state string, decision *types.PermissionDecision, result *types.ToolResult, now time.Time) {
	part.Type = "tool"
	part.CallID = callID
	part.ToolName = action.ToolName
	part.Input = cloneToolArguments(action.Arguments)
	part.State = state
	part.Decision = toolDecisionPart(decision)
	part.Result = toolResultPart(result)
	part.UpdatedAt = now
	if part.CreatedAt.IsZero() {
		part.CreatedAt = now
	}
}

func cloneToolArguments(arguments map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range arguments {
		result[key] = value
	}
	return result
}

func toolDecisionPart(decision *types.PermissionDecision) *types.ToolDecision {
	if decision == nil {
		return nil
	}
	return &types.ToolDecision{ID: decision.ID, ActionID: decision.ActionID, ToolName: decision.ToolName, Status: decision.Status, Reason: decision.Reason, CreatedAt: decision.CreatedAt}
}

func toolResultPart(result *types.ToolResult) *types.ToolPartResult {
	if result == nil {
		return nil
	}
	metadata := map[string]any{}
	for key, value := range result.Metadata {
		metadata[key] = value
	}
	return &types.ToolPartResult{ID: result.ID, ActionID: result.ActionID, ToolName: result.ToolName, Status: result.Status, Content: result.Content, Metadata: metadata, Error: result.Error, CreatedAt: result.CreatedAt}
}
