package agentruntime

import (
	"context"
	"strings"

	"eucli-box/pkg/types"
)

// imageSendDecision 是一张图片发往模型的发送决策：占位文字非空表示
// 不发送图片本体只发占位文字；否则按 UsePreview 选择小副本或原图。
type imageSendDecision struct {
	Placeholder string
	UsePreview  bool
}

type imageOccurrenceKey struct {
	MessageIndex    int
	AttachmentIndex int
}

// conversationImagePlan 是本次请求的图片发送计划：按消息与附件位置
// 定位每一张图片的发送决策。
type conversationImagePlan struct {
	decisions map[imageOccurrenceKey]imageSendDecision
}

func (plan *conversationImagePlan) decisionFor(messageIndex int, attachmentIndex int) imageSendDecision {
	if plan == nil {
		return imageSendDecision{}
	}
	return plan.decisions[imageOccurrenceKey{MessageIndex: messageIndex, AttachmentIndex: attachmentIndex}]
}

// buildConversationImagePlan 统计上下文中的全部图片，按对话顺序从后往前
// 应用两项预算：历史预算决定保留张数，超出的以占位文字替代；原图预算
// 决定保留者中最近的若干张以原图发送，其余用小副本。多版本存图关闭或
// 单项预算关闭时，对应限制不生效。
func (s *system) buildConversationImagePlan(ctx context.Context, messages []types.Message) (*conversationImagePlan, error) {
	config, err := s.storage.LoadConversationImageConfig(ctx)
	if err != nil {
		return nil, runtimeStorageFailed("failed to load conversation image config", err)
	}
	type occurrence struct {
		key        imageOccurrenceKey
		attachment types.MessageAttachment
	}
	occurrences := []occurrence{}
	for messageIndex, message := range messages {
		for attachmentIndex, attachment := range messageImageAttachments(message) {
			occurrences = append(occurrences, occurrence{key: imageOccurrenceKey{MessageIndex: messageIndex, AttachmentIndex: attachmentIndex}, attachment: attachment})
		}
	}
	decisions := make(map[imageOccurrenceKey]imageSendDecision, len(occurrences))
	if len(occurrences) == 0 {
		return &conversationImagePlan{decisions: decisions}, nil
	}
	historyBudget := config.EffectiveHistoryBudgetCount()
	originalBudget := config.EffectiveOriginalBudgetCount()
	total := len(occurrences)
	for index, item := range occurrences {
		positionFromEnd := total - 1 - index
		if historyBudget > 0 && positionFromEnd >= historyBudget {
			decisions[item.key] = imageSendDecision{Placeholder: imageBudgetPlaceholderText(item.attachment.ID)}
			continue
		}
		usePreview := config.MultiVersionEnabled && strings.TrimSpace(item.attachment.PreviewPath) != ""
		if usePreview && originalBudget > 0 && positionFromEnd < originalBudget {
			usePreview = false
		}
		decisions[item.key] = imageSendDecision{UsePreview: usePreview}
	}
	return &conversationImagePlan{decisions: decisions}, nil
}

// imageBudgetPlaceholderText 是超出图片预算的占位文字：注明图片 ID，
// 让模型知道这张图存在、可通过会话图片工具按 ID 取回。
func imageBudgetPlaceholderText(attachmentID string) string {
	return "此图片超出此会话的图片预算上限，图片 ID：" + strings.TrimSpace(attachmentID)
}

// messageImageAttachments 收集一条消息上可发送的图片附件；
// 发送计划与图片装载两条链路共用同一过滤规则，位置一一对应。
func messageImageAttachments(message types.Message) []types.MessageAttachment {
	attachments := []types.MessageAttachment{}
	for _, attachment := range message.Attachments {
		if attachment.Kind != "image" || strings.TrimSpace(attachment.Path) == "" {
			continue
		}
		attachments = append(attachments, attachment)
	}
	return attachments
}
