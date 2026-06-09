package types

import "testing"

func TestEstimateMessageTokenCountDoesNotDoubleCountTextPart(t *testing.T) {
	message := Message{Content: "abcd", Parts: []MessagePart{{Type: "text", Text: "abcd"}}}

	if got := EstimateMessageTokenCount(message); got != 1 {
		t.Fatalf("token estimate = %d, want 1", got)
	}
}

func TestEstimateMessageTokenCountIncludesTextAttachment(t *testing.T) {
	message := Message{
		Content:     "abcd",
		Attachments: []MessageAttachment{{Kind: "txt", Text: "abcdefgh"}, {Kind: "image", Path: "sessions/role/session/attachments/att/image.png"}},
	}

	if got := EstimateMessageTokenCount(message); got != 3 {
		t.Fatalf("token estimate = %d, want 3", got)
	}
}

func TestEstimateMessageTokenCountSkipsTextProtocolToolRawRequest(t *testing.T) {
	message := Message{
		Content: "abcd",
		Parts: []MessagePart{{
			Type:     "tool",
			Source:   ToolCallSourceTextProtocol,
			Raw:      "this raw request is already inside content",
			ToolName: "tool",
			Input:    map[string]any{"command": "pwd"},
			Result:   &ToolPartResult{Status: ToolStatusSuccess, Content: "abcd"},
		}},
	}

	if got := EstimateMessageTokenCount(message); got != 5 {
		t.Fatalf("token estimate = %d, want 5", got)
	}
}
