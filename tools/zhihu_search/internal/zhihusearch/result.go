package zhihusearch

import (
	"fmt"
	"strings"
)

type normalizedItem struct {
	Title        string `json:"title"`
	URL          string `json:"url"`
	AuthorName   string `json:"authorName"`
	Summary      string `json:"summary"`
	VoteUpCount  int    `json:"voteUpCount,omitempty"`
	CommentCount int    `json:"commentCount,omitempty"`
	EditTime     any    `json:"editTime,omitempty"`
}

type sourceItem struct {
	Rank       int    `json:"rank"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	Snippet    string `json:"snippet"`
	AuthorName string `json:"authorName"`
}

func normalizeItems(raw apiResponse, searchType string) []normalizedItem {
	items := make([]normalizedItem, 0, len(raw.Data.Items))
	for _, item := range raw.Data.Items {
		normalized := normalizedItem{Title: item.Title, URL: item.URL, AuthorName: item.AuthorName, Summary: item.ContentText, EditTime: item.EditTime}
		if searchType == searchTypeZhihu {
			normalized.VoteUpCount = item.VoteUpCount
			normalized.CommentCount = item.CommentCount
		}
		items = append(items, normalized)
	}
	return items
}

func buildSources(items []normalizedItem) []sourceItem {
	sources := make([]sourceItem, 0, len(items))
	for index, item := range items {
		sources = append(sources, sourceItem{Rank: index + 1, Title: item.Title, URL: item.URL, Snippet: item.Summary, AuthorName: item.AuthorName})
	}
	return sources
}

func formatContent(searchType string, apiMessage string, items []normalizedItem, request searchRequest) (string, bool) {
	var builder strings.Builder
	if searchType == searchTypeGlobal {
		builder.WriteString("## Zhihu Global Search Results\n\n")
	} else {
		builder.WriteString("## Zhihu Search Results\n\n")
	}
	builder.WriteString(fmt.Sprintf("Query: `%s`  \n", request.Query))
	builder.WriteString(fmt.Sprintf("Status: `%s`  \n", apiMessage))
	builder.WriteString(fmt.Sprintf("Results: `%d`\n\n", len(items)))
	if len(items) == 0 {
		builder.WriteString("No Zhihu results returned.\n")
		return truncateWithState(builder.String(), request.MaxOutputChars)
	}
	for index, item := range items {
		title := firstNonEmpty(item.Title, item.URL, "Untitled")
		if item.URL != "" {
			builder.WriteString(fmt.Sprintf("%d. [%s](%s)\n", index+1, escapeMarkdownLinkText(title), item.URL))
		} else {
			builder.WriteString(fmt.Sprintf("%d. %s\n", index+1, title))
		}
		if item.AuthorName != "" {
			builder.WriteString(fmt.Sprintf("   Author: %s\n", item.AuthorName))
		}
		if searchType == searchTypeZhihu {
			builder.WriteString(fmt.Sprintf("   Votes/Comments: %d / %d\n", item.VoteUpCount, item.CommentCount))
		}
		if editTimeText := editTimeString(item.EditTime); editTimeText != "" {
			builder.WriteString(fmt.Sprintf("   Edit time: %s\n", editTimeText))
		}
		if strings.TrimSpace(item.Summary) != "" {
			builder.WriteString("   ")
			builder.WriteString(strings.TrimSpace(item.Summary))
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

func editTimeString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case float64:
		if typed == 0 {
			return ""
		}
		return fmt.Sprintf("%.0f", typed)
	case int:
		if typed == 0 {
			return ""
		}
		return fmt.Sprint(typed)
	case int64:
		if typed == 0 {
			return ""
		}
		return fmt.Sprint(typed)
	default:
		return fmt.Sprint(typed)
	}
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
