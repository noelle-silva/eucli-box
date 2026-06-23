package agentruntime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

const (
	compactCommandToken = "/compact"
	compactCommandName  = "compact"
)

type contextCompressionPlan struct {
	previousSummary          *types.Message
	messagesToCompress       []types.Message
	compressedUntilMessageID string
	retainRecentMessages     int
	summaryVersion           int
}

func isCompactRunRequest(request types.RunRequest) bool {
	command, hasCommand, _ := parseRuntimeSlashCommand(request.Message)
	return hasCommand && command == compactCommandToken
}

func validateRunSlashCommand(request types.RunRequest) error {
	command, hasCommand, unsupportedArgs := parseRuntimeSlashCommand(request.Message)
	if unsupportedArgs {
		return runtimeInvalid("slash command arguments are not supported", nil)
	}
	if !hasCommand {
		return nil
	}
	if command != compactCommandToken {
		return runtimeInvalid("unknown slash command", nil)
	}
	if len(request.Attachments) > 0 {
		return runtimeInvalid("/compact does not accept attachments", nil)
	}
	return nil
}

func parseRuntimeSlashCommand(message string) (string, bool, bool) {
	text := strings.TrimLeft(message, " \t\r\n")
	if !strings.HasPrefix(text, "/") {
		return "", false, false
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", false, false
	}
	return fields[0], true, len(fields) > 1
}

func prepareCompactRunSession(session types.Session, request types.RunRequest) (types.Session, types.Message, error) {
	if strings.TrimSpace(request.SessionID) == "" {
		return types.Session{}, types.Message{}, runtimeInvalid("当前没有可压缩的会话", nil)
	}
	if len(session.Messages) == 0 {
		return types.Session{}, types.Message{}, runtimeInvalid("当前没有可压缩的会话", nil)
	}
	anchorID := strings.TrimSpace(request.ParentMessageID)
	if anchorID == "" {
		anchorID = lastMessageIDInBranch(session.Messages, activeSessionBranchID(session))
	}
	if anchorID == "" {
		return types.Session{}, types.Message{}, runtimeInvalid("当前没有可压缩的会话", nil)
	}
	return sessionContextThroughMessage(session, anchorID)
}

func (s *system) continueCompactRun(ctx context.Context, record *runRecord, contextSession types.Session) {
	if err := ctx.Err(); err != nil {
		s.cancelRunRecord(context.Background(), record, record.session)
		return
	}
	config, err := s.storage.LoadContextCompressionConfig(ctx)
	if err != nil {
		s.failCommandRun(record, runtimeStorageFailed("failed to load context compression config", err))
		return
	}
	if !types.HasCompleteModelCoordinate(config.Coordinate) {
		s.failCommandRun(record, runtimeInvalid("请先在设置 > AI 微服务中配置上下文压缩模型", nil))
		return
	}
	plan, err := buildContextCompressionPlan(contextSession, config.RetainRecentMessages)
	if err != nil {
		s.failCommandRun(record, err)
		return
	}
	response, err := s.providers.Complete(ctx, types.ModelRequest{Coordinate: config.Coordinate, Temperature: config.Temperature, Messages: contextCompressionPromptMessages(plan)})
	if err != nil {
		if ctx.Err() != nil {
			s.cancelRunRecord(context.Background(), record, record.session)
			return
		}
		s.failCommandRun(record, runtimeProviderFailed("failed to complete context compression", err))
		return
	}
	summary, err := normalizeContextCompressionSummary(response.Content)
	if err != nil {
		s.failCommandRun(record, err)
		return
	}
	appendContextCompressionMessages(record, plan, summary)
	if err := s.setRunMessageIDs(record.runID, record.inputMessageID, record.lastMessageID); err != nil {
		s.failCommandRun(record, err)
		return
	}
	if err := s.saveRunSession(ctx, record, types.RunStatusCompleted); err != nil {
		s.failCommandRun(record, err)
		return
	}
	state, err := s.updateRun(record.runID, types.RunStatusCompleted, "")
	if err != nil {
		return
	}
	s.publish(record.runID, "run_completed", state)
}

func (s *system) failCommandRun(record *runRecord, err error) {
	reason, payload := runFailureFromError(err, "")
	state, updateErr := s.updateRunWithError(record.runID, types.RunStatusFailed, reason, payload)
	if updateErr != nil {
		s.publish(record.runID, "run_failed", types.RunState{ID: record.runID, Status: types.RunStatusFailed, Reason: reason, Error: cloneErrorPayload(payload)})
		return
	}
	s.publish(record.runID, "run_failed", state)
}

func buildContextCompressionPlan(contextSession types.Session, retainRecentMessages int) (contextCompressionPlan, error) {
	if retainRecentMessages <= 0 {
		retainRecentMessages = types.DefaultContextCompressionRetainRecentMessages
	}
	latestSummaryIndex := latestCompressionSummaryIndex(contextSession.Messages)
	startIndex := 0
	version := 1
	var previousSummary *types.Message
	if latestSummaryIndex >= 0 {
		summary := contextSession.Messages[latestSummaryIndex]
		previousSummary = &summary
		startIndex = latestSummaryIndex + 1
		if summary.Control != nil && summary.Control.SummaryVersion > 0 {
			version = summary.Control.SummaryVersion + 1
		} else {
			version = 2
		}
	}
	realMessages := realContextMessages(contextSession.Messages[startIndex:])
	if len(realMessages) <= retainRecentMessages {
		if previousSummary != nil {
			return contextCompressionPlan{}, runtimeInvalid("距离上次压缩后没有新增可压缩内容", nil)
		}
		return contextCompressionPlan{}, runtimeInvalid("没有足够的历史内容可压缩", nil)
	}
	toCompress := realMessages[:len(realMessages)-retainRecentMessages]
	if len(toCompress) == 0 {
		return contextCompressionPlan{}, runtimeInvalid("没有足够的历史内容可压缩", nil)
	}
	return contextCompressionPlan{previousSummary: previousSummary, messagesToCompress: toCompress, compressedUntilMessageID: toCompress[len(toCompress)-1].ID, retainRecentMessages: retainRecentMessages, summaryVersion: version}, nil
}

func compactedContextSession(contextSession types.Session) types.Session {
	latestSummaryIndex := latestCompressionSummaryIndex(contextSession.Messages)
	if latestSummaryIndex < 0 {
		return contextSession
	}
	summary := contextSession.Messages[latestSummaryIndex]
	retain := types.DefaultContextCompressionRetainRecentMessages
	if summary.Control != nil && summary.Control.RetainRecentMessages > 0 {
		retain = summary.Control.RetainRecentMessages
	}
	recent := realContextMessages(contextSession.Messages[latestSummaryIndex+1:])
	if len(recent) > retain {
		recent = recent[len(recent)-retain:]
	}
	contextSession.Messages = append([]types.Message{summary}, recent...)
	return contextSession
}

func latestCompressionSummaryIndex(messages []types.Message) int {
	for index := len(messages) - 1; index >= 0; index-- {
		if isCompressionSummaryMessage(messages[index]) {
			return index
		}
	}
	return -1
}

func isCompressionSummaryMessage(message types.Message) bool {
	return message.Type == types.MessageTypeSystemControl && message.Control != nil && message.Control.Kind == types.MessageControlKindCompressionSummary
}

func isSystemControlMessage(message types.Message) bool {
	return message.Type == types.MessageTypeSystemControl
}

func realContextMessages(messages []types.Message) []types.Message {
	real := make([]types.Message, 0, len(messages))
	for _, message := range messages {
		if isSystemControlMessage(message) {
			continue
		}
		switch strings.TrimSpace(message.Type) {
		case "user", "assistant", "tool", "tool_request", "tool_confirmation", "failure":
			real = append(real, message)
		}
	}
	return real
}

func contextCompressionPromptMessages(plan contextCompressionPlan) []types.PromptMessage {
	now := time.Now().UTC()
	return []types.PromptMessage{
		{Role: "system", Content: types.DefaultContextCompressionSystemPrompt, Order: 0, CreatedAt: now, UpdatedAt: now},
		{Role: "user", Content: contextCompressionUserPrompt(plan), Order: 1, CreatedAt: now, UpdatedAt: now},
	}
}

func contextCompressionUserPrompt(plan contextCompressionPlan) string {
	oldSummary := "无旧摘要"
	if plan.previousSummary != nil && strings.TrimSpace(plan.previousSummary.Content) != "" {
		oldSummary = strings.TrimSpace(plan.previousSummary.Content)
	}
	return strings.TrimSpace(fmt.Sprintf("旧摘要：\n%s\n\n待压缩对话：\n%s\n\n输出要求：\n只输出更新后的摘要；不要输出解释；不要输出 Markdown 代码围栏；不要重复总结后续会原样保留的最近消息。", oldSummary, contextCompressionTranscript(plan.messagesToCompress)))
}

func contextCompressionTranscript(messages []types.Message) string {
	blocks := make([]string, 0, len(messages))
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			content = strings.TrimSpace(message.Reason)
		}
		for _, attachment := range message.Attachments {
			name := strings.TrimSpace(attachment.Name)
			if name == "" {
				name = "附件"
			}
			if attachment.Kind == "image" {
				content += "\n[图片附件：" + name + "]"
				continue
			}
			if strings.TrimSpace(attachment.Text) != "" {
				content += "\n[文本附件：" + name + "]\n" + strings.TrimSpace(attachment.Text)
			}
		}
		if content == "" {
			content = "（空消息）"
		}
		blocks = append(blocks, fmt.Sprintf("[%s] %s\n%s", message.ID, contextCompressionMessageLabel(message), content))
	}
	return strings.Join(blocks, "\n\n---\n\n")
}

func contextCompressionMessageLabel(message types.Message) string {
	switch strings.TrimSpace(message.Type) {
	case "assistant":
		return "助手"
	case "tool":
		name := strings.TrimSpace(message.ToolName)
		if name == "" {
			name = "工具"
		}
		return "工具结果：" + name
	case "tool_request":
		return "工具请求"
	case "tool_confirmation":
		return "工具确认"
	case "failure":
		return "运行失败"
	default:
		return "用户"
	}
}

func normalizeContextCompressionSummary(raw string) (string, error) {
	summary := strings.TrimSpace(raw)
	if strings.HasPrefix(summary, "```") && strings.HasSuffix(summary, "```") {
		lines := strings.Split(summary, "\n")
		if len(lines) >= 2 {
			summary = strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
		}
	}
	if summary == "" {
		return "", runtimeInvalid("generated context summary is empty", nil)
	}
	return summary, nil
}

func appendContextCompressionMessages(record *runRecord, plan contextCompressionPlan, summary string) {
	boundary := systemControlMessage("已执行 /compact，上下文已压缩。", types.MessageControl{Kind: types.MessageControlKindCompressionBoundary, CommandName: compactCommandName, Source: "manual", SourceText: compactCommandToken, RetainRecentMessages: plan.retainRecentMessages, CompressedUntilMessageID: plan.compressedUntilMessageID, SummaryVersion: plan.summaryVersion})
	appendRunMessage(record, boundary)
	boundaryID := record.messageParent.ID
	record.inputMessageID = boundaryID
	previousSummaryID := ""
	if plan.previousSummary != nil {
		previousSummaryID = plan.previousSummary.ID
	}
	summaryMessage := systemControlMessage(summary, types.MessageControl{Kind: types.MessageControlKindCompressionSummary, CommandName: compactCommandName, Source: "manual", SourceText: compactCommandToken, RetainRecentMessages: plan.retainRecentMessages, PreviousSummaryMessageID: previousSummaryID, CompressedUntilMessageID: plan.compressedUntilMessageID, SummaryVersion: plan.summaryVersion})
	appendRunMessage(record, summaryMessage)
}

func systemControlMessage(content string, control types.MessageControl) types.Message {
	now := time.Now().UTC()
	return types.Message{ID: utils.NewID("message"), Type: types.MessageTypeSystemControl, Content: content, Control: &control, CreatedAt: now, UpdatedAt: now}
}

func compressionSummaryPromptContent(message types.Message) string {
	content := strings.TrimSpace(message.Content)
	if content == "" {
		return ""
	}
	return "以下是此前会话的压缩摘要，用于延续上下文：\n\n" + content
}

func (s *system) hasActiveRunInDependencyPath(record *runRecord) bool {
	if record == nil || strings.TrimSpace(record.session.ID) == "" || len(record.dependencyIDs) == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, other := range s.runs {
		if other == nil || other.runID == record.runID {
			continue
		}
		if strings.TrimSpace(other.roleID) != strings.TrimSpace(record.roleID) || strings.TrimSpace(other.state.SessionID) != strings.TrimSpace(record.session.ID) {
			continue
		}
		if !isActiveRunStatus(other.state.Status) {
			continue
		}
		if _, ok := record.dependencyIDs[strings.TrimSpace(other.anchorMessageID)]; ok {
			return true
		}
		for messageID := range other.dependencyIDs {
			if _, ok := record.dependencyIDs[strings.TrimSpace(messageID)]; ok {
				return true
			}
		}
	}
	return false
}
