package modelprovider

import (
	"context"
	"encoding/json"
	"testing"

	"eucli-box/pkg/types"
)

func runAnthropicComplete(t *testing.T, request types.ModelRequest) map[string]any {
	t.Helper()
	if request.Coordinate.ProviderID == "" {
		request.Coordinate = types.ModelCoordinate{ProviderID: "anthropic-main", ModelID: "claude-3-5-sonnet"}
	}
	storage := newFakeProviderStorage()
	storage.providers["anthropic-main"] = testAnthropicProvider()
	network := &fakeNetwork{response: types.HTTPResponse{StatusCode: 200, Body: []byte(`{"id":"msg-1","content":[{"type":"text","text":"done"}]}`)}}
	system := newTestProviderSystem(t, network, storage)
	if _, err := system.Complete(context.Background(), request); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(network.lastRequest.Body, &body); err != nil {
		t.Fatalf("request body is invalid json: %v", err)
	}
	return body
}

func countAnthropicCacheControls(value any) int {
	switch item := value.(type) {
	case map[string]any:
		count := 0
		if _, ok := item["cache_control"]; ok {
			count++
		}
		for _, child := range item {
			count += countAnthropicCacheControls(child)
		}
		return count
	case []any:
		count := 0
		for _, child := range item {
			count += countAnthropicCacheControls(child)
		}
		return count
	default:
		return 0
	}
}

func requireAnthropicEphemeralCacheControl(t *testing.T, value any, label string) {
	t.Helper()
	block, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s is not a content block: %#v", label, value)
	}
	cacheControl, ok := block["cache_control"].(map[string]any)
	if !ok || cacheControl["type"] != "ephemeral" {
		t.Fatalf("%s cache_control = %#v", label, block["cache_control"])
	}
}

func TestCompleteAnthropicMarksSystemToolsAndLastMessage(t *testing.T) {
	body := runAnthropicComplete(t, types.ModelRequest{
		Messages: []types.PromptMessage{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "hi"},
		},
		Tools: []types.ToolDefinition{
			{Name: "first", Description: "first tool", InputSchema: map[string]any{"type": "object"}},
			{Name: "second", Description: "second tool", InputSchema: map[string]any{"type": "object"}},
		},
	})

	systemBlocks, ok := body["system"].([]any)
	if !ok || len(systemBlocks) != 1 {
		t.Fatalf("system = %#v", body["system"])
	}
	systemBlock := systemBlocks[0].(map[string]any)
	if systemBlock["type"] != "text" || systemBlock["text"] != "You are helpful" {
		t.Fatalf("system block = %#v", systemBlock)
	}
	requireAnthropicEphemeralCacheControl(t, systemBlock, "system block")

	toolBlocks, ok := body["tools"].([]any)
	if !ok || len(toolBlocks) != 2 {
		t.Fatalf("tools = %#v", body["tools"])
	}
	firstTool := toolBlocks[0].(map[string]any)
	secondTool := toolBlocks[1].(map[string]any)
	if firstTool["name"] != "first" || secondTool["name"] != "second" {
		t.Fatalf("tool order changed: %#v", toolBlocks)
	}
	if _, ok := firstTool["cache_control"]; ok {
		t.Fatalf("first tool should not carry cache_control: %#v", firstTool)
	}
	requireAnthropicEphemeralCacheControl(t, secondTool, "last tool")

	messages, ok := body["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %#v", body["messages"])
	}
	lastMessage := messages[len(messages)-1].(map[string]any)
	contentBlocks, ok := lastMessage["content"].([]any)
	if !ok || len(contentBlocks) != 1 {
		t.Fatalf("last message content = %#v", lastMessage["content"])
	}
	textBlock := contentBlocks[0].(map[string]any)
	if textBlock["type"] != "text" || textBlock["text"] != "hi" {
		t.Fatalf("last message block = %#v", textBlock)
	}
	requireAnthropicEphemeralCacheControl(t, textBlock, "last message block")

	if count := countAnthropicCacheControls(body); count != 3 {
		t.Fatalf("cache_control count = %d, want 3", count)
	}
}

func TestCompleteAnthropicSkipsCacheBreakpointsForEmptySections(t *testing.T) {
	body := runAnthropicComplete(t, types.ModelRequest{
		Messages: []types.PromptMessage{{Role: "user", Content: "hi"}},
	})

	if _, ok := body["system"]; ok {
		t.Fatalf("system should be absent: %#v", body["system"])
	}
	if _, ok := body["tools"]; ok {
		t.Fatalf("tools should be absent: %#v", body["tools"])
	}
	if count := countAnthropicCacheControls(body); count != 1 {
		t.Fatalf("cache_control count = %d, want 1", count)
	}
}

func TestCompleteAnthropicSkipsMessageBreakpointWhenNoMessages(t *testing.T) {
	body := runAnthropicComplete(t, types.ModelRequest{
		Messages: []types.PromptMessage{{Role: "system", Content: "You are helpful"}},
		Tools:    []types.ToolDefinition{{Name: "search", Description: "search", InputSchema: map[string]any{"type": "object"}}},
	})

	messages, ok := body["messages"].([]any)
	if !ok || len(messages) != 0 {
		t.Fatalf("messages = %#v", body["messages"])
	}
	if count := countAnthropicCacheControls(body); count != 2 {
		t.Fatalf("cache_control count = %d, want 2", count)
	}
}

func TestCompleteAnthropicMarksLastAvailableBlockSkippingThinking(t *testing.T) {
	body := runAnthropicComplete(t, types.ModelRequest{
		Messages: []types.PromptMessage{{
			Role:    "assistant",
			Content: "answer",
			Parts:   []types.MessagePart{{Type: "reasoning", Text: "think", Source: "an"}},
		}},
	})

	messages := body["messages"].([]any)
	contentBlocks := messages[0].(map[string]any)["content"].([]any)
	if len(contentBlocks) != 2 {
		t.Fatalf("assistant content = %#v", contentBlocks)
	}
	thinkingBlock := contentBlocks[0].(map[string]any)
	if thinkingBlock["type"] != "thinking" {
		t.Fatalf("first block = %#v", thinkingBlock)
	}
	if _, ok := thinkingBlock["cache_control"]; ok {
		t.Fatalf("thinking block should not carry cache_control: %#v", thinkingBlock)
	}
	textBlock := contentBlocks[1].(map[string]any)
	if textBlock["type"] != "text" || textBlock["text"] != "answer" {
		t.Fatalf("last block = %#v", textBlock)
	}
	requireAnthropicEphemeralCacheControl(t, textBlock, "last text block")
}

func TestCompleteAnthropicSearchesBackwardPastThinkingOnlyMessage(t *testing.T) {
	body := runAnthropicComplete(t, types.ModelRequest{
		Messages: []types.PromptMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "", Parts: []types.MessagePart{{Type: "reasoning", Text: "think", Source: "an"}}},
		},
	})

	messages := body["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %#v", messages)
	}
	assistantBlocks := messages[1].(map[string]any)["content"].([]any)
	if len(assistantBlocks) != 1 || assistantBlocks[0].(map[string]any)["type"] != "thinking" {
		t.Fatalf("assistant content = %#v", assistantBlocks)
	}
	if _, ok := assistantBlocks[0].(map[string]any)["cache_control"]; ok {
		t.Fatalf("thinking block should not carry cache_control: %#v", assistantBlocks[0])
	}
	userBlocks := messages[0].(map[string]any)["content"].([]any)
	if len(userBlocks) != 1 {
		t.Fatalf("user content = %#v", userBlocks)
	}
	userText := userBlocks[0].(map[string]any)
	if userText["type"] != "text" || userText["text"] != "hello" {
		t.Fatalf("user block = %#v", userText)
	}
	requireAnthropicEphemeralCacheControl(t, userText, "previous message text block")
}

func TestCompleteAnthropicMarksLastToolResultBlock(t *testing.T) {
	body := runAnthropicComplete(t, types.ModelRequest{
		Messages: []types.PromptMessage{{
			Role:    "assistant",
			Content: "checking",
			Parts: []types.MessagePart{
				{Type: "tool", CallID: "toolu-1", ToolName: "first", Input: map[string]any{}, Result: &types.ToolPartResult{Status: types.ToolStatusSuccess, Content: "ok-1"}},
				{Type: "tool", CallID: "toolu-2", ToolName: "second", Input: map[string]any{}, Result: &types.ToolPartResult{Status: types.ToolStatusSuccess, Content: "ok-2"}},
			},
		}},
	})

	messages := body["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %#v", messages)
	}
	resultBlocks := messages[1].(map[string]any)["content"].([]any)
	if len(resultBlocks) != 2 {
		t.Fatalf("tool result blocks = %#v", resultBlocks)
	}
	firstResult := resultBlocks[0].(map[string]any)
	secondResult := resultBlocks[1].(map[string]any)
	if firstResult["type"] != "tool_result" || secondResult["type"] != "tool_result" {
		t.Fatalf("tool result blocks = %#v", resultBlocks)
	}
	if _, ok := firstResult["cache_control"]; ok {
		t.Fatalf("first tool_result should not carry cache_control: %#v", firstResult)
	}
	requireAnthropicEphemeralCacheControl(t, secondResult, "last tool_result block")
	if count := countAnthropicCacheControls(body); count != 1 {
		t.Fatalf("cache_control count = %d, want 1", count)
	}
}

func TestCompleteAnthropicMarksLastImageBlock(t *testing.T) {
	body := runAnthropicComplete(t, types.ModelRequest{
		Messages: []types.PromptMessage{{
			Role:    "user",
			Content: "look",
			Images:  []types.PromptImage{{DataURL: "data:image/png;base64,iVBORw0KGgo="}},
		}},
	})

	messages := body["messages"].([]any)
	contentBlocks := messages[0].(map[string]any)["content"].([]any)
	if len(contentBlocks) != 2 {
		t.Fatalf("user content = %#v", contentBlocks)
	}
	textBlock := contentBlocks[0].(map[string]any)
	if textBlock["type"] != "text" || textBlock["text"] != "look" {
		t.Fatalf("text block = %#v", textBlock)
	}
	if _, ok := textBlock["cache_control"]; ok {
		t.Fatalf("text block should not carry cache_control: %#v", textBlock)
	}
	imageBlock := contentBlocks[1].(map[string]any)
	if imageBlock["type"] != "image" {
		t.Fatalf("image block = %#v", imageBlock)
	}
	requireAnthropicEphemeralCacheControl(t, imageBlock, "last image block")
}
