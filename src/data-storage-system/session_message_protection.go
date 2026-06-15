package datastorage

import (
	"strings"

	"eucli-box/pkg/types"
)

func protectedSessionMessageMutationReason(session types.Session, messageID string) string {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return ""
	}
	message, ok := storedSessionMessageByID(session, messageID)
	if !ok {
		return ""
	}
	if message.Type == types.MessageTypeSystemControl {
		return "压缩标记和摘要不能被普通编辑或删除"
	}
	if compressedSourceMessageIDs(session)[messageID] {
		return "已被上下文摘要覆盖的历史消息不能被普通编辑或删除"
	}
	return ""
}

func protectedSessionMessageSubtreeMutationReason(session types.Session, rootMessageID string) string {
	rootMessageID = strings.TrimSpace(rootMessageID)
	if rootMessageID == "" {
		return ""
	}
	children := map[string][]string{}
	for _, message := range session.Messages {
		parentID := strings.TrimSpace(message.ParentMessageID)
		if parentID == "" {
			continue
		}
		children[parentID] = append(children[parentID], message.ID)
	}
	seen := map[string]struct{}{}
	stack := []string{rootMessageID}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		current = strings.TrimSpace(current)
		if current == "" {
			continue
		}
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		if reason := protectedSessionMessageMutationReason(session, current); reason != "" {
			return reason
		}
		stack = append(stack, children[current]...)
	}
	return ""
}

func compressedSourceMessageIDs(session types.Session) map[string]bool {
	protected := map[string]bool{}
	byID := storedSessionMessagesByID(session.Messages)
	for _, message := range session.Messages {
		if message.Type != types.MessageTypeSystemControl || message.Control == nil || message.Control.Kind != types.MessageControlKindCompressionSummary {
			continue
		}
		untilID := strings.TrimSpace(message.Control.CompressedUntilMessageID)
		if untilID == "" {
			continue
		}
		current, ok := byID[untilID]
		seen := map[string]struct{}{}
		for ok && strings.TrimSpace(current.ID) != "" {
			if _, exists := seen[current.ID]; exists {
				break
			}
			seen[current.ID] = struct{}{}
			if current.Type == types.MessageTypeSystemControl && current.Control != nil && current.Control.Kind == types.MessageControlKindCompressionSummary {
				break
			}
			protected[current.ID] = true
			parentID := strings.TrimSpace(current.ParentMessageID)
			if parentID == "" {
				break
			}
			current, ok = byID[parentID]
		}
	}
	return protected
}

func storedSessionMessagesByID(messages []types.Message) map[string]types.Message {
	byID := map[string]types.Message{}
	for _, message := range messages {
		messageID := strings.TrimSpace(message.ID)
		if messageID == "" {
			continue
		}
		byID[messageID] = message
	}
	return byID
}
