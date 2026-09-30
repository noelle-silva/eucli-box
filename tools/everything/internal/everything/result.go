package everything

import (
	"fmt"
	"strings"
)

// formatContent 把搜索结果整理为稳定的 Markdown，并按字符上限截断。
func formatContent(payload searchResultPayload, request searchRequest) (string, bool) {
	var builder strings.Builder
	builder.WriteString("## Everything Search Results\n\n")
	builder.WriteString(fmt.Sprintf("Query: `%s`  \n", payload.Query))
	builder.WriteString(fmt.Sprintf("Results: `%d`\n", len(payload.Results)))
	if scope := strings.TrimSpace(payload.ScopePath); scope != "" {
		builder.WriteString(fmt.Sprintf("Scope: %s\n", inlineCode(scope)))
	}
	builder.WriteString("\n")
	if len(payload.Results) == 0 {
		builder.WriteString("No local files matched the query.\n")
		return truncateWithState(builder.String(), request.MaxOutputChars)
	}
	builder.WriteString("### Results\n\n")
	for index, result := range payload.Results {
		builder.WriteString(fmt.Sprintf("%d. %s\n", index+1, inlineCode(result.FullPath)))
		details := []string{}
		if strings.TrimSpace(result.Kind) != "" {
			details = append(details, "kind: "+result.Kind)
		}
		if strings.TrimSpace(result.Size) != "" {
			details = append(details, "size: "+result.Size)
		}
		if strings.TrimSpace(result.ModifiedAt) != "" {
			details = append(details, "modified: "+result.ModifiedAt)
		}
		if len(details) > 0 {
			builder.WriteString("   ")
			builder.WriteString(strings.Join(details, " | "))
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	return truncateWithState(builder.String(), request.MaxOutputChars)
}

// truncateWithState 按字符上限截断，并报告是否发生截断。
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

// truncateRunes 按字符截断（不切断多字节字符），并报告是否发生截断。
func truncateRunes(text string, limit int) (string, bool) {
	if limit <= 0 {
		return "", text != ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text, false
	}
	return string(runes[:limit]), true
}

func inlineCode(text string) string {
	if strings.Contains(text, "`") {
		return text
	}
	return "`" + text + "`"
}
