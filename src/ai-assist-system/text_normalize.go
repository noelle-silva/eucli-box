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
	src := strings.ReplaceAll(markdown, "\r\n", "\n")
	oldTrim := strings.TrimSpace(oldCode)
	newTrim := strings.TrimSpace(newCode)
	if oldTrim == "" || newTrim == "" {
		return markdown, false
	}
	needle := "```mermaid\n" + oldTrim + "\n```"
	replacement := "```mermaid\n" + newTrim + "\n```"
	if strings.Contains(src, needle) {
		return strings.Replace(src, needle, replacement, 1), true
	}
	return markdown, false
}
