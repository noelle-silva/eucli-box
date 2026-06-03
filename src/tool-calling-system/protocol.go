package toolcalling

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

const (
	toolRequestStartMarker = "<<<TOOL_REQUEST>>>"
	toolRequestEndMarker   = "<<<END_TOOL_REQUEST>>>"
)

var toolRequestLinePattern = regexp.MustCompile(`^\[([A-Za-z0-9_-]+)\]:[ \t]?(.*)$`)

func (s *system) ParseTextToolRequests(ctx context.Context, content string) (string, []types.ToolIntent, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, toolProtocolInvalid("TOOL_REQUEST_CANCELLED: text tool request parsing was cancelled", err)
	}
	return parseTextToolRequests(content)
}

func (s *system) VisibleTextToolContent(ctx context.Context, content string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", toolProtocolInvalid("TOOL_REQUEST_CANCELLED: text tool request display filtering was cancelled", err)
	}
	return visibleTextToolContent(content), nil
}

func parseTextToolRequests(content string) (string, []types.ToolIntent, error) {
	lines := protocolLines(content)
	outside := make([]string, 0, len(lines))
	intents := []types.ToolIntent{}
	inBlock := false
	inFence := false
	sawBlock := false
	blockStartLine := 0
	blockLines := []string{}

	for index, line := range lines {
		lineNumber := index + 1
		marker := strings.TrimSpace(line)
		if !inBlock {
			if isMarkdownFenceLine(line) {
				inFence = !inFence
				outside = append(outside, line)
				continue
			}
			if inFence {
				outside = append(outside, line)
				continue
			}
			switch marker {
			case toolRequestStartMarker:
				inBlock = true
				sawBlock = true
				blockStartLine = lineNumber
				blockLines = []string{line}
			case toolRequestEndMarker:
				return "", nil, protocolError("TOOL_REQUEST_MISSING_START", lineNumber, "found end marker without a matching start marker")
			default:
				outside = append(outside, line)
			}
			continue
		}

		blockLines = append(blockLines, line)
		switch marker {
		case toolRequestStartMarker:
			return "", nil, protocolError("TOOL_REQUEST_NESTED_BLOCK", lineNumber, "found a start marker before the previous tool request was closed")
		case toolRequestEndMarker:
			intent, err := parseToolRequestBlock(blockLines, blockStartLine)
			if err != nil {
				return "", nil, err
			}
			intents = append(intents, intent)
			inBlock = false
			blockStartLine = 0
			blockLines = nil
		}
	}
	if inBlock {
		return "", nil, protocolError("TOOL_REQUEST_MISSING_END", blockStartLine, "start marker was not closed before the end of the model response")
	}
	if !sawBlock {
		return content, nil, nil
	}
	return normalizeProtocolContent(outside), intents, nil
}

func parseToolRequestBlock(blockLines []string, startLine int) (types.ToolIntent, error) {
	entries := make([]toolRequestEntry, 0, len(blockLines))
	seen := map[string]int{}
	for index := 1; index < len(blockLines)-1; index++ {
		lineNumber := startLine + index
		line := strings.TrimSpace(blockLines[index])
		if line == "" {
			continue
		}
		entry, err := parseToolRequestLine(line, lineNumber)
		if err != nil {
			return types.ToolIntent{}, err
		}
		if firstLine, ok := seen[entry.key]; ok {
			return types.ToolIntent{}, protocolError("TOOL_REQUEST_DUPLICATE_KEY", lineNumber, fmt.Sprintf("key %q was already declared at line %d", entry.key, firstLine))
		}
		seen[entry.key] = lineNumber
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return types.ToolIntent{}, protocolError("TOOL_REQUEST_EMPTY_BLOCK", startLine, "tool request block has no key-value lines")
	}
	if entries[0].key != "tool" {
		return types.ToolIntent{}, protocolError("TOOL_REQUEST_NO_TOOL_NAME", entries[0].lineNumber, "first key inside a tool request block must be [tool]")
	}
	toolName := strings.TrimSpace(entries[0].value)
	if toolName == "" {
		return types.ToolIntent{}, protocolError("TOOL_REQUEST_EMPTY_NAME", entries[0].lineNumber, "tool name is required but was empty")
	}
	arguments := make(map[string]any, len(entries)-1)
	for _, entry := range entries[1:] {
		arguments[entry.key] = strings.TrimSpace(entry.value)
	}
	return types.ToolIntent{ID: utils.NewID("tool-intent"), ToolName: toolName, Arguments: arguments, Source: types.ToolCallSourceTextProtocol, Raw: strings.Join(blockLines, "\n"), CreatedAt: time.Now().UTC()}, nil
}

func parseToolRequestLine(line string, lineNumber int) (toolRequestEntry, error) {
	matches := toolRequestLinePattern.FindStringSubmatch(line)
	if len(matches) != 3 {
		return toolRequestEntry{}, protocolError("TOOL_REQUEST_INVALID_KEY_VALUE", lineNumber, "line must use [key]: value format")
	}
	key := strings.TrimSpace(matches[1])
	if key == "" {
		return toolRequestEntry{}, protocolError("TOOL_REQUEST_INVALID_KEY", lineNumber, "key cannot be empty")
	}
	return toolRequestEntry{key: key, value: matches[2], lineNumber: lineNumber}, nil
}

func protocolLines(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Split(normalized, "\n")
}

func normalizeProtocolContent(lines []string) string {
	content := strings.TrimSpace(strings.Join(lines, "\n"))
	for strings.Contains(content, "\n\n\n") {
		content = strings.ReplaceAll(content, "\n\n\n", "\n\n")
	}
	return content
}

func visibleTextToolContent(content string) string {
	lines := protocolLines(content)
	outside := make([]string, 0, len(lines))
	inBlock := false
	inFence := false
	sawBlock := false
	for _, line := range lines {
		marker := strings.TrimSpace(line)
		if !inBlock {
			if isMarkdownFenceLine(line) {
				inFence = !inFence
				outside = append(outside, line)
				continue
			}
			if inFence {
				outside = append(outside, line)
				continue
			}
		}
		if !inBlock && marker == toolRequestStartMarker {
			inBlock = true
			sawBlock = true
			continue
		}
		if inBlock {
			if marker == toolRequestEndMarker {
				inBlock = false
			}
			continue
		}
		outside = append(outside, line)
	}
	if !sawBlock {
		return trimPartialStartMarkerLine(content)
	}
	return trimPartialStartMarkerLine(normalizeProtocolContent(outside))
}

func isMarkdownFenceLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

func trimPartialStartMarkerLine(content string) string {
	lineStart := strings.LastIndex(content, "\n") + 1
	lastLine := strings.TrimSpace(content[lineStart:])
	if lastLine == "" {
		return content
	}
	for length := 1; length < len(toolRequestStartMarker); length++ {
		if lastLine == toolRequestStartMarker[:length] {
			return strings.TrimSpace(content[:lineStart])
		}
	}
	return content
}

func protocolError(kind string, lineNumber int, message string) error {
	return toolProtocolInvalid(fmt.Sprintf("%s at line %d: %s", kind, lineNumber, message), nil)
}

type toolRequestEntry struct {
	key        string
	value      string
	lineNumber int
}
