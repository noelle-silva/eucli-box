package everything

import (
	"fmt"
	"strings"
)

func formatContent(response searchResponse, request searchRequest) (string, bool) {
	var builder strings.Builder
	builder.WriteString("## Everything Search Results\n\n")
	builder.WriteString(fmt.Sprintf("Query: `%s`  \n", response.Query))
	builder.WriteString(fmt.Sprintf("Results: `%d` of limit `%d`\n", len(response.Results), response.Limit))
	if scope := scopeSummary(response); scope != "" {
		builder.WriteString(fmt.Sprintf("Scope: %s\n", scope))
	}
	if strings.TrimSpace(response.InstanceName) != "" {
		builder.WriteString(fmt.Sprintf("Instance: `%s`\n", response.InstanceName))
	}
	builder.WriteString("\n")
	if len(response.Results) == 0 {
		builder.WriteString("No local files matched the query.\n")
		return truncateWithState(builder.String(), request.MaxOutputChars)
	}
	builder.WriteString("### Results\n\n")
	for index, result := range response.Results {
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

func scopeSummary(response searchResponse) string {
	if response.ScopeMode == scopeModeAllLocalDrives && len(response.ScopePaths) > 0 {
		return "all local drives: " + strings.Join(inlineCodeList(response.ScopePaths), ", ")
	}
	if strings.TrimSpace(response.ScopePath) != "" {
		return inlineCode(response.ScopePath)
	}
	return ""
}

func inlineCodeList(values []string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, inlineCode(value))
	}
	return items
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

func inlineCode(text string) string {
	if strings.Contains(text, "`") {
		return text
	}
	return "`" + text + "`"
}
