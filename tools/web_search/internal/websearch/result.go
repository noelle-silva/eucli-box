package websearch

import (
	"fmt"
	"strings"
)

func formatContent(response searchResponse, request searchRequest) (string, bool) {
	var builder strings.Builder
	builder.WriteString("## Web Search Results\n\n")
	builder.WriteString(fmt.Sprintf("Provider: `%s`  \n", response.Provider))
	builder.WriteString(fmt.Sprintf("Query: `%s`\n\n", response.Query))
	if strings.TrimSpace(response.Answer) != "" {
		builder.WriteString("### Answer\n\n")
		builder.WriteString(strings.TrimSpace(response.Answer))
		builder.WriteString("\n\n")
	}
	if len(response.Results) == 0 {
		builder.WriteString("No search results returned.\n")
		return truncateWithState(builder.String(), request.MaxOutputChars)
	}
	builder.WriteString("### Results\n\n")
	for index, result := range response.Results {
		title := firstNonEmpty(result.Title, result.URL, "Untitled")
		if result.URL != "" {
			builder.WriteString(fmt.Sprintf("%d. [%s](%s)\n", index+1, escapeMarkdownLinkText(title), result.URL))
		} else {
			builder.WriteString(fmt.Sprintf("%d. %s\n", index+1, title))
		}
		if result.Score != nil {
			builder.WriteString(fmt.Sprintf("   Score: %.4f\n", *result.Score))
		}
		if strings.TrimSpace(result.Snippet) != "" {
			builder.WriteString("   ")
			builder.WriteString(strings.TrimSpace(result.Snippet))
			builder.WriteString("\n")
		}
		if request.IncludeContent && strings.TrimSpace(result.Content) != "" && strings.TrimSpace(result.Content) != strings.TrimSpace(result.Snippet) {
			builder.WriteString("\n")
			builder.WriteString(indent(strings.TrimSpace(result.Content), "   "))
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	return truncateWithState(builder.String(), request.MaxOutputChars)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func truncateWithState(text string, limit int) (string, bool) {
	if limit <= 0 {
		return text, false
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text, false
	}
	return string(runes[:limit]), true
}

func truncateText(text string, limit int) string {
	truncated, _ := truncateWithState(text, limit)
	return truncated
}

func escapeMarkdownLinkText(text string) string {
	return strings.ReplaceAll(text, "]", "\\]")
}

func indent(text string, prefix string) string {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = prefix + line
	}
	return strings.Join(lines, "\n")
}
