package context7

import (
	"fmt"
	"strings"
)

type searchResponse struct {
	Results             []libraryResult `json:"results"`
	SearchFilterApplied bool            `json:"searchFilterApplied"`
}

type libraryResult struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Branch         string   `json:"branch"`
	LastUpdateDate string   `json:"lastUpdateDate"`
	State          string   `json:"state"`
	TotalTokens    int      `json:"totalTokens"`
	TotalSnippets  int      `json:"totalSnippets"`
	Stars          int      `json:"stars"`
	TrustScore     float64  `json:"trustScore"`
	BenchmarkScore float64  `json:"benchmarkScore"`
	Versions       []string `json:"versions"`
}

type docsResponse struct {
	CodeSnippets []codeSnippet       `json:"codeSnippets"`
	InfoSnippets []infoSnippet       `json:"infoSnippets"`
	Rules        map[string][]string `json:"rules"`
}

type codeSnippet struct {
	CodeTitle       string        `json:"codeTitle"`
	CodeDescription string        `json:"codeDescription"`
	CodeLanguage    string        `json:"codeLanguage"`
	CodeTokens      int           `json:"codeTokens"`
	CodeID          string        `json:"codeId"`
	PageTitle       string        `json:"pageTitle"`
	CodeList        []codeExample `json:"codeList"`
	IsDynamic       bool          `json:"isDynamic"`
	SourceFile      string        `json:"sourceFile"`
}

type codeExample struct {
	Language string `json:"language"`
	Code     string `json:"code"`
}

type infoSnippet struct {
	PageID        string `json:"pageId"`
	Breadcrumb    string `json:"breadcrumb"`
	Content       string `json:"content"`
	ContentTokens int    `json:"contentTokens"`
}

func formatSearchContent(response searchResponse, request lookupRequest) (string, bool) {
	var builder strings.Builder
	builder.WriteString("## Context7 Library Search\n\n")
	builder.WriteString(fmt.Sprintf("Library name: `%s`  \n", request.LibraryName))
	builder.WriteString(fmt.Sprintf("Query: `%s`\n\n", request.Query))
	if len(response.Results) == 0 {
		builder.WriteString("No libraries returned.\n")
		return truncateWithState(builder.String(), request.MaxOutputChars)
	}
	builder.WriteString("### Results\n\n")
	for index, result := range response.Results {
		title := firstNonEmpty(result.Title, result.ID, "Untitled")
		builder.WriteString(fmt.Sprintf("%d. %s", index+1, title))
		if result.ID != "" {
			builder.WriteString(fmt.Sprintf(" (`%s`)", result.ID))
		}
		builder.WriteString("\n")
		writeIndentedLine(&builder, "Description", result.Description)
		writeIndentedLine(&builder, "State", result.State)
		writeIndentedLine(&builder, "Branch", result.Branch)
		writeIndentedLine(&builder, "Last update", result.LastUpdateDate)
		if result.TotalSnippets > 0 {
			builder.WriteString(fmt.Sprintf("   Snippets: %d\n", result.TotalSnippets))
		}
		if result.TotalTokens > 0 {
			builder.WriteString(fmt.Sprintf("   Tokens: %d\n", result.TotalTokens))
		}
		if result.Stars > 0 {
			builder.WriteString(fmt.Sprintf("   Stars: %d\n", result.Stars))
		}
		if result.TrustScore > 0 {
			builder.WriteString(fmt.Sprintf("   Trust score: %s\n", formatScore(result.TrustScore)))
		}
		if result.BenchmarkScore > 0 {
			builder.WriteString(fmt.Sprintf("   Benchmark score: %.1f\n", result.BenchmarkScore))
		}
		if len(result.Versions) > 0 {
			builder.WriteString(fmt.Sprintf("   Versions: %s\n", strings.Join(result.Versions, ", ")))
		}
		builder.WriteString("\n")
	}
	return truncateWithState(builder.String(), request.MaxOutputChars)
}

func formatDocsContent(response docsResponse, request lookupRequest) (string, bool) {
	var builder strings.Builder
	builder.WriteString("## Context7 Documentation\n\n")
	builder.WriteString(fmt.Sprintf("Library ID: `%s`  \n", request.LibraryID))
	builder.WriteString(fmt.Sprintf("Query: `%s`\n\n", request.Query))
	if len(response.Rules) > 0 {
		builder.WriteString("### Rules\n\n")
		for group, rules := range response.Rules {
			if len(rules) == 0 {
				continue
			}
			builder.WriteString(fmt.Sprintf("- %s: %s\n", group, strings.Join(rules, " ")))
		}
		builder.WriteString("\n")
	}
	if len(response.CodeSnippets) > 0 {
		builder.WriteString("### Code Snippets\n\n")
		for index, snippet := range response.CodeSnippets {
			title := firstNonEmpty(snippet.CodeTitle, snippet.PageTitle, snippet.CodeID, "Untitled")
			builder.WriteString(fmt.Sprintf("%d. %s\n", index+1, title))
			writeIndentedLine(&builder, "Page", snippet.PageTitle)
			writeIndentedLine(&builder, "Source", firstNonEmpty(snippet.CodeID, snippet.SourceFile))
			writeIndentedLine(&builder, "Description", snippet.CodeDescription)
			if snippet.CodeTokens > 0 {
				builder.WriteString(fmt.Sprintf("   Tokens: %d\n", snippet.CodeTokens))
			}
			for _, example := range snippet.CodeList {
				language := firstNonEmpty(example.Language, snippet.CodeLanguage, "text")
				builder.WriteString("\n")
				builder.WriteString("```" + sanitizeFenceLanguage(language) + "\n")
				builder.WriteString(strings.TrimSpace(example.Code))
				builder.WriteString("\n```\n")
			}
			builder.WriteString("\n")
		}
	}
	if len(response.InfoSnippets) > 0 {
		builder.WriteString("### Info Snippets\n\n")
		for index, snippet := range response.InfoSnippets {
			title := firstNonEmpty(snippet.Breadcrumb, snippet.PageID, "Info")
			builder.WriteString(fmt.Sprintf("%d. %s\n", index+1, title))
			writeIndentedLine(&builder, "Source", snippet.PageID)
			if snippet.ContentTokens > 0 {
				builder.WriteString(fmt.Sprintf("   Tokens: %d\n", snippet.ContentTokens))
			}
			if strings.TrimSpace(snippet.Content) != "" {
				builder.WriteString("\n")
				builder.WriteString(indent(strings.TrimSpace(snippet.Content), "   "))
				builder.WriteString("\n")
			}
			builder.WriteString("\n")
		}
	}
	if len(response.CodeSnippets) == 0 && len(response.InfoSnippets) == 0 {
		builder.WriteString("No documentation snippets returned.\n")
	}
	return truncateWithState(builder.String(), request.MaxOutputChars)
}

func writeIndentedLine(builder *strings.Builder, label string, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	builder.WriteString(fmt.Sprintf("   %s: %s\n", label, strings.TrimSpace(value)))
}

func formatScore(value float64) string {
	if value == float64(int64(value)) {
		return fmt.Sprintf("%.0f", value)
	}
	return fmt.Sprintf("%.1f", value)
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

func indent(text string, prefix string) string {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func sanitizeFenceLanguage(language string) string {
	language = strings.TrimSpace(language)
	if language == "" {
		return "text"
	}
	language = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '+' || r == '#' {
			return r
		}
		return -1
	}, language)
	if language == "" {
		return "text"
	}
	return language
}
