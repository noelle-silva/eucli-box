package aiassist

import "strings"

func normalizeGeneratedChatTitle(raw string) string {
	value := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	lines := []string{}
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	value = strings.TrimSpace(lines[0])
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(value, "标题:"), "标题："), "会话标题："))
	value = strings.Trim(value, "\"'`“”‘’")
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) > 80 {
		value = string([]rune(value)[:80])
	}
	return strings.TrimSpace(value)
}

func buildChatTranscriptForTitle(content []string) string {
	transcript := strings.TrimSpace(strings.Join(content, "\n\n"))
	if transcript == "" {
		return ""
	}
	userContent := "请为以下聊天记录生成一个简短标题：\n\n" + transcript
	if len([]rune(userContent)) > 16000 {
		runes := []rune(userContent)
		userContent = string(runes[len(runes)-16000:])
	}
	return strings.TrimSpace(userContent)
}

func normalizeGeneratedMermaidCode(raw string) string {
	value := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if strings.Contains(value, "```") {
		blocks := strings.Split(value, "```")
		for _, block := range blocks {
			block = strings.TrimSpace(block)
			if block == "" {
				continue
			}
			if strings.HasPrefix(strings.ToLower(block), "mermaid") {
				parts := strings.SplitN(block, "\n", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1])
				}
			}
		}
	}
	return value
}

func replaceMermaidFenceOnce(markdown string, oldCode string, newCode string) (string, bool) {
	target := normalizeMermaidSource(oldCode)
	replacement := normalizeMermaidSource(newCode)
	if target == "" || replacement == "" {
		return markdown, false
	}
	for _, block := range scanMarkdownFenceBlocks(markdown) {
		if !isMermaidFenceLanguage(block.language) || normalizeMermaidSource(block.body) != target {
			continue
		}
		eol := block.eol
		if eol == "" {
			eol = "\n"
		}
		body := strings.ReplaceAll(replacement, "\n", eol)
		return markdown[:block.bodyStart] + body + eol + markdown[block.bodyEnd:], true
	}
	return markdown, false
}

func normalizeMermaidSource(source string) string {
	value := strings.ReplaceAll(source, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.TrimSpace(value)
}

func isMermaidFenceLanguage(language string) bool {
	switch language {
	case "mermaid", "flowchart", "graph":
		return true
	default:
		return false
	}
}

type markdownFenceBlock struct {
	language  string
	body      string
	bodyStart int
	bodyEnd   int
	eol       string
}

func scanMarkdownFenceBlocks(markdown string) []markdownFenceBlock {
	blocks := []markdownFenceBlock{}
	for lineStart := 0; lineStart < len(markdown); {
		textEnd, nextLine := markdownLineBounds(markdown, lineStart)
		mark, count, info, ok := parseMarkdownFenceOpen(markdown[lineStart:textEnd])
		if !ok {
			lineStart = nextLine
			continue
		}
		eol := markdown[textEnd:nextLine]
		bodyStart := nextLine
		cursor := nextLine
		closed := false
		for cursor < len(markdown) {
			closeTextEnd, closeNextLine := markdownLineBounds(markdown, cursor)
			if isMarkdownFenceClose(markdown[cursor:closeTextEnd], mark, count) {
				blocks = append(blocks, markdownFenceBlock{
					language:  markdownFenceLanguage(info),
					body:      markdown[bodyStart:cursor],
					bodyStart: bodyStart,
					bodyEnd:   cursor,
					eol:       eol,
				})
				lineStart = closeNextLine
				closed = true
				break
			}
			cursor = closeNextLine
		}
		if !closed {
			break
		}
	}
	return blocks
}

func markdownLineBounds(value string, start int) (int, int) {
	offset := strings.IndexByte(value[start:], '\n')
	if offset < 0 {
		return len(value), len(value)
	}
	textEnd := start + offset
	nextLine := textEnd + 1
	if textEnd > start && value[textEnd-1] == '\r' {
		textEnd--
	}
	return textEnd, nextLine
}

func parseMarkdownFenceOpen(line string) (byte, int, string, bool) {
	rest, ok := stripMarkdownFenceIndent(line)
	if !ok || len(rest) < 3 {
		return 0, 0, "", false
	}
	mark := rest[0]
	if mark != '`' && mark != '~' {
		return 0, 0, "", false
	}
	count := 0
	for count < len(rest) && rest[count] == mark {
		count++
	}
	if count < 3 {
		return 0, 0, "", false
	}
	info := strings.TrimSpace(rest[count:])
	if mark == '`' && strings.Contains(info, "`") {
		return 0, 0, "", false
	}
	return mark, count, info, true
}

func isMarkdownFenceClose(line string, mark byte, count int) bool {
	rest, ok := stripMarkdownFenceIndent(line)
	if !ok {
		return false
	}
	run := 0
	for run < len(rest) && rest[run] == mark {
		run++
	}
	if run < count {
		return false
	}
	return strings.TrimSpace(rest[run:]) == ""
}

func stripMarkdownFenceIndent(line string) (string, bool) {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return "", false
	}
	return line[indent:], true
}

func markdownFenceLanguage(info string) string {
	fields := strings.Fields(info)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(fields[0])
}
