package types

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

const tokenEstimateCharsPerToken = 4

func EstimateMessageTokenCount(message Message) int {
	chars := 0
	content := strings.TrimSpace(message.Content)
	if content != "" {
		chars += utf8.RuneCountInString(content)
	}

	for _, part := range message.Parts {
		switch strings.TrimSpace(part.Type) {
		case "text":
			if content == "" {
				chars += utf8.RuneCountInString(strings.TrimSpace(part.Text))
			}
		case "reasoning":
			chars += utf8.RuneCountInString(strings.TrimSpace(part.Text))
			chars += utf8.RuneCountInString(strings.TrimSpace(part.Signature))
			chars += utf8.RuneCountInString(strings.TrimSpace(part.Data))
		case "tool":
			isTextProtocolTool := strings.TrimSpace(part.Source) == ToolCallSourceTextProtocol
			if !isTextProtocolTool {
				chars += utf8.RuneCountInString(strings.TrimSpace(part.ToolName))
				chars += mapStringTokenChars(part.Input)
			}
			if part.Result != nil {
				if isTextProtocolTool {
					chars += utf8.RuneCountInString(strings.TrimSpace(part.ToolName))
				}
				chars += utf8.RuneCountInString(strings.TrimSpace(string(part.Result.Status)))
				chars += utf8.RuneCountInString(strings.TrimSpace(part.Result.Content))
				chars += utf8.RuneCountInString(strings.TrimSpace(part.Result.Error))
			}
		}
	}

	for _, attachment := range message.Attachments {
		if strings.TrimSpace(attachment.Kind) == "image" {
			continue
		}
		chars += utf8.RuneCountInString(strings.TrimSpace(attachment.Text))
	}

	if chars <= 0 {
		return 0
	}
	return (chars + tokenEstimateCharsPerToken - 1) / tokenEstimateCharsPerToken
}

func mapStringTokenChars(value map[string]any) int {
	if len(value) == 0 {
		return 0
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return utf8.RuneCount(payload)
}
