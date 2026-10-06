package agentruntime

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

// eventSubscriber 是一个订阅者的独立投递缓冲：
// 发布者只把事件写进缓冲，永不阻塞、永不失败；
// 独立的投递 goroutine 负责把缓冲里的增量合并后送给消费者。
// 消费者再慢也不会被删除或断开，最多是缓冲里的变化被合并。
type eventSubscriber struct {
	out  chan types.RunEvent
	done chan struct{}

	mu      sync.Mutex
	pending []types.RunEvent
	closed  bool
	signal  chan struct{}
}

func newEventSubscriber() *eventSubscriber {
	return &eventSubscriber{
		out:    make(chan types.RunEvent, 64),
		done:   make(chan struct{}),
		signal: make(chan struct{}, 1),
	}
}

// enqueue 把事件写入缓冲；同一条消息的相邻增量在入缓冲时即合并，避免缓冲无界膨胀。
func (sub *eventSubscriber) enqueue(event types.RunEvent) {
	sub.mu.Lock()
	if sub.closed {
		sub.mu.Unlock()
		return
	}
	if isMergeableDelta(event) && !deltaHasReset(event) {
		if n := len(sub.pending); n > 0 {
			last := &sub.pending[n-1]
			if isMergeableDelta(*last) && last.Type == event.Type && deltaMessageID(*last) == deltaMessageID(event) {
				*last = mergeDeltaEvents(*last, event)
				sub.mu.Unlock()
				sub.notify()
				return
			}
		}
	}
	sub.pending = append(sub.pending, event)
	sub.mu.Unlock()
	sub.notify()
}

func (sub *eventSubscriber) notify() {
	select {
	case sub.signal <- struct{}{}:
	default:
	}
}

func (sub *eventSubscriber) run() {
	defer close(sub.out)
	for {
		select {
		case <-sub.signal:
		case <-sub.done:
			return
		}
		for {
			sub.mu.Lock()
			if len(sub.pending) == 0 {
				sub.mu.Unlock()
				break
			}
			event := sub.pending[0]
			sub.pending = sub.pending[1:]
			sub.mu.Unlock()

			select {
			case sub.out <- event:
			case <-sub.done:
				return
			}
		}
	}
}

func (sub *eventSubscriber) close() {
	sub.mu.Lock()
	if sub.closed {
		sub.mu.Unlock()
		return
	}
	sub.closed = true
	sub.mu.Unlock()
	close(sub.done)
}

// isMergeableDelta 判断事件是否是可按消息合并的增量事件。
func isMergeableDelta(event types.RunEvent) bool {
	return event.Type == "assistant_message_delta"
}

func deltaMessageID(event types.RunEvent) string {
	if delta, ok := event.Payload.(types.RunMessageDelta); ok {
		return delta.MessageID
	}
	return ""
}

// deltaHasReset 表示该增量要求整体替换，不能被并入前一条增量。
func deltaHasReset(event types.RunEvent) bool {
	if delta, ok := event.Payload.(types.RunMessageDelta); ok {
		return delta.ContentReset || delta.ReasoningReset
	}
	return false
}

// mergeDeltaEvents 把后到的增量并入前一条同消息增量：正文与思考内容累加，状态取最新。
func mergeDeltaEvents(base types.RunEvent, next types.RunEvent) types.RunEvent {
	baseDelta, baseOK := base.Payload.(types.RunMessageDelta)
	nextDelta, nextOK := next.Payload.(types.RunMessageDelta)
	if !baseOK || !nextOK {
		return next
	}
	baseDelta.ContentDelta += nextDelta.ContentDelta
	baseDelta.ReasoningDelta += nextDelta.ReasoningDelta
	if nextDelta.ReasoningSource != "" {
		baseDelta.ReasoningSource = nextDelta.ReasoningSource
	}
	if nextDelta.ReasoningSignature != "" {
		baseDelta.ReasoningSignature = nextDelta.ReasoningSignature
	}
	if nextDelta.ReasoningData != "" {
		baseDelta.ReasoningData = nextDelta.ReasoningData
	}
	if nextDelta.Status != "" {
		baseDelta.Status = nextDelta.Status
	}
	if len(nextDelta.PartsDelta) > 0 {
		baseDelta.PartsDelta = append(baseDelta.PartsDelta, nextDelta.PartsDelta...)
	}
	baseDelta.CreatedAt = nextDelta.CreatedAt
	base.Payload = baseDelta
	return base
}

func (s *system) Subscribe(ctx context.Context) (<-chan types.RunEvent, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, runtimeInvalid("subscription context is cancelled", err)
	}
	sub := newEventSubscriber()
	s.mu.Lock()
	s.subscribers[sub] = struct{}{}
	s.mu.Unlock()
	go sub.run()
	unsubscribe := func() {
		s.removeSubscriber(sub)
	}
	go func() {
		<-ctx.Done()
		s.removeSubscriber(sub)
	}()
	return sub.out, unsubscribe, nil
}

// eventSource 是一条事件自带的身份：事件归属由事件源自带，不依赖运行记录是否仍在内存，
// 因此运行记录可以安全回收，晚到的异步事件仍能确定自己的运行、群组与工作区。
type eventSource struct {
	runID       string
	groupID     string
	workspaceID string
}

func eventSourceFromRecord(record *runRecord) eventSource {
	if record == nil {
		return eventSource{}
	}
	return eventSource{runID: record.runID, groupID: record.groupID, workspaceID: record.workspaceID}
}

func eventSourceFromAsyncTask(task types.AsyncToolTask) eventSource {
	return eventSource{runID: task.RunID, groupID: task.GroupID, workspaceID: task.WorkspaceID}
}

func eventSourceFromToolOutput(output toolOutputContext) eventSource {
	return eventSource{runID: output.runID, groupID: output.groupID, workspaceID: output.workspaceID}
}

func (s *system) publish(source eventSource, eventType string, payload any) {
	event := types.RunEvent{ID: utils.NewID("event"), RunID: source.runID, GroupID: source.groupID, WorkspaceID: source.workspaceID, Type: eventType, Payload: payload, CreatedAt: time.Now().UTC()}
	s.mu.Lock()
	subs := make([]*eventSubscriber, 0, len(s.subscribers))
	for sub := range s.subscribers {
		subs = append(subs, sub)
	}
	s.mu.Unlock()
	for _, sub := range subs {
		sub.enqueue(event)
	}
}

func (s *system) publishAssistantMessageUpdate(record *runRecord) {
	message, ok := currentRunAssistantMessage(record)
	if !ok {
		return
	}
	s.recordPublishedMessage(record, message)
	s.publishRunMessageUpdate(record, "assistant_message_update", message)
}

// publishedMessageState 是某个助手消息上一次已发布内容的基线；
// 流式增量通过与它求差得到，保证“同一份变化只发一次”。
type publishedMessageState struct {
	content   string
	reasoning string
	signature string
	data      string
	parts     map[string]types.MessagePart
}

func (s *system) recordPublishedMessage(record *runRecord, message types.Message) {
	mid := strings.TrimSpace(message.ID)
	if mid == "" || record == nil {
		return
	}
	s.publishedMu.Lock()
	if s.publishedMessages == nil {
		s.publishedMessages = map[string]map[string]publishedMessageState{}
	}
	byRun := s.publishedMessages[record.runID]
	if byRun == nil {
		byRun = map[string]publishedMessageState{}
		s.publishedMessages[record.runID] = byRun
	}
	byRun[mid] = publishedMessageSnapshot(message)
	s.publishedMu.Unlock()
}

func publishedMessageSnapshot(message types.Message) publishedMessageState {
	parts := map[string]types.MessagePart{}
	// 工具片段会被原地改写，基线必须深拷贝，否则比对会因别名而漏报变化。
	for _, part := range cloneRunMessageParts(message.Parts) {
		if part.Type != "tool" {
			continue
		}
		if id := strings.TrimSpace(part.ID); id != "" {
			parts[id] = part
		}
	}
	reasoning, signature, data := assistantReasoningFields(message)
	return publishedMessageState{content: message.Content, reasoning: reasoning, signature: signature, data: data, parts: parts}
}

// publishAssistantMessageDelta 只发布某条助手消息本次新增的变化：
// 正文增量、思考增量、以及发生变化的工具片段。没有变化时不发布。
func (s *system) publishAssistantMessageDelta(record *runRecord) {
	message, ok := currentRunAssistantMessage(record)
	if !ok {
		return
	}
	mid := strings.TrimSpace(message.ID)
	if mid == "" {
		return
	}
	s.publishedMu.Lock()
	if s.publishedMessages == nil {
		s.publishedMessages = map[string]map[string]publishedMessageState{}
	}
	byRun := s.publishedMessages[record.runID]
	if byRun == nil {
		byRun = map[string]publishedMessageState{}
		s.publishedMessages[record.runID] = byRun
	}
	prev := byRun[mid]
	next := publishedMessageSnapshot(message)
	byRun[mid] = next
	s.publishedMu.Unlock()

	contentDelta := ""
	contentReset := false
	if next.content != prev.content {
		if strings.HasPrefix(next.content, prev.content) {
			contentDelta = strings.TrimPrefix(next.content, prev.content)
		} else {
			contentDelta = next.content
			contentReset = true
		}
	}
	reasoningDelta := ""
	reasoningReset := false
	if next.reasoning != prev.reasoning {
		if strings.HasPrefix(next.reasoning, prev.reasoning) {
			reasoningDelta = strings.TrimPrefix(next.reasoning, prev.reasoning)
		} else {
			reasoningDelta = next.reasoning
			reasoningReset = true
		}
	}
	partsDelta := changedToolParts(prev.parts, next.parts)

	signatureChanged := next.signature != prev.signature
	dataChanged := next.data != prev.data
	if contentDelta == "" && reasoningDelta == "" && !contentReset && !reasoningReset && len(partsDelta) == 0 && !signatureChanged && !dataChanged {
		return
	}

	state, _ := s.getRunState(record.runID)
	payload := types.RunMessageDelta{
		RunID:              record.runID,
		RoleID:             record.roleID,
		GroupID:            record.groupID,
		WorkspaceID:        record.workspaceID,
		SessionID:          record.session.ID,
		MessageID:          mid,
		ParentMessageID:    strings.TrimSpace(message.ParentMessageID),
		BranchID:           strings.TrimSpace(message.BranchID),
		SpeakerRoleID:      strings.TrimSpace(message.SpeakerRoleID),
		MessageType:        strings.TrimSpace(message.Type),
		MessageCreatedAt:   message.CreatedAt,
		Stream:             record.stream,
		Status:             state.Status,
		ContentDelta:       contentDelta,
		ContentReset:       contentReset,
		ReasoningDelta:     reasoningDelta,
		ReasoningReset:     reasoningReset,
		ReasoningSource:    assistantReasoningSource(message),
		ReasoningSignature: next.signature,
		ReasoningData:      next.data,
		PartsDelta:         partsDelta,
		CreatedAt:          time.Now().UTC(),
	}
	s.publish(eventSourceFromRecord(record), "assistant_message_delta", payload)
}

func changedToolParts(prev map[string]types.MessagePart, next map[string]types.MessagePart) []types.MessagePart {
	if len(next) == 0 {
		return nil
	}
	out := make([]types.MessagePart, 0, len(next))
	for id, part := range next {
		if before, ok := prev[id]; ok && reflect.DeepEqual(before, part) {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func assistantReasoningFields(message types.Message) (string, string, string) {
	for _, part := range message.Parts {
		if part.Type != "reasoning" {
			continue
		}
		return part.Text, strings.TrimSpace(part.Signature), strings.TrimSpace(part.Data)
	}
	return "", "", ""
}

func assistantReasoningSource(message types.Message) string {
	for _, part := range message.Parts {
		if part.Type != "reasoning" {
			continue
		}
		return strings.TrimSpace(part.Source)
	}
	return ""
}

func (s *system) publishRunMessageUpdate(record *runRecord, eventType string, message types.Message) {
	state, _ := s.getRunState(record.runID)
	now := time.Now().UTC()
	s.publish(eventSourceFromRecord(record), eventType, types.RunAssistantMessageUpdate{RunID: record.runID, RoleID: record.roleID, GroupID: record.groupID, WorkspaceID: record.workspaceID, SessionID: record.session.ID, Stream: record.stream, Status: state.Status, Reason: state.Reason, Retry: cloneRunRetryInfo(state.Retry), Error: cloneErrorPayload(state.Error), Message: cloneRunMessageSnapshot(message), CreatedAt: now})
}

// toolOutputContext 是工具实时输出广播所需的只读身份快照。
// 工具执行协程在启动前取一份，之后只读它，不再触碰运行的可变状态。
type toolOutputContext struct {
	runID       string
	roleID      string
	groupID     string
	workspaceID string
	sessionID   string
	messageID   string
}

func toolOutputContextFromRun(record *runRecord) toolOutputContext {
	if record == nil {
		return toolOutputContext{}
	}
	return toolOutputContext{
		runID:       record.runID,
		roleID:      record.roleID,
		groupID:     record.groupID,
		workspaceID: record.workspaceID,
		sessionID:   record.session.ID,
		messageID:   strings.TrimSpace(record.activeAssistantID),
	}
}

// publishToolOutputUpdate 向订阅者广播工具运行中的实时输出进展。
func (s *system) publishToolOutputUpdate(output toolOutputContext, entry toolRunEntry, update types.ToolOutputUpdate) {
	if strings.TrimSpace(output.runID) == "" {
		return
	}
	payload := types.ToolOutputUpdateEventPayload{
		RunID:       output.runID,
		RoleID:      output.roleID,
		GroupID:     output.groupID,
		WorkspaceID: output.workspaceID,
		SessionID:   output.sessionID,
		MessageID:   output.messageID,
		CallID:      update.CallID,
		ToolName:    update.ToolName,
		Bytes:       update.Bytes,
		Preview:     update.Preview,
		CreatedAt:   time.Now().UTC(),
	}
	s.publish(eventSourceFromToolOutput(output), "tool_output_update", payload)
}

func currentRunAssistantMessage(record *runRecord) (types.Message, bool) {
	messageID := strings.TrimSpace(record.activeAssistantID)
	if messageID == "" && record.messageParent.Type == "assistant" {
		messageID = strings.TrimSpace(record.messageParent.ID)
	}
	if messageID == "" {
		return types.Message{}, false
	}
	message, ok := messageByID(record.session.Messages, messageID)
	if !ok || message.Type != "assistant" {
		return types.Message{}, false
	}
	return message, true
}

func cloneRunMessageSnapshot(message types.Message) types.Message {
	if message.Control != nil {
		control := *message.Control
		message.Control = &control
	}
	message.Error = cloneErrorPayload(message.Error)
	message.Parts = cloneRunMessageParts(message.Parts)
	if len(message.Attachments) > 0 {
		message.Attachments = append([]types.MessageAttachment(nil), message.Attachments...)
	}
	message.TokenEstimate = types.EstimateMessageTokenCount(message)
	return message
}

func cloneRunMessageParts(parts []types.MessagePart) []types.MessagePart {
	if len(parts) == 0 {
		return nil
	}
	out := make([]types.MessagePart, len(parts))
	for index, part := range parts {
		out[index] = part
		out[index].Input = cloneMapAny(part.Input)
		out[index].Display = cloneMapAny(part.Display)
		if part.Decision != nil {
			decision := *part.Decision
			out[index].Decision = &decision
		}
		if part.Result != nil {
			result := *part.Result
			result.Metadata = cloneMapAny(part.Result.Metadata)
			out[index].Result = &result
		}
	}
	return out
}

func cloneMapAny(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneAny(value)
	}
	return out
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMapAny(typed)
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = cloneAny(item)
		}
		return out
	default:
		return value
	}
}
