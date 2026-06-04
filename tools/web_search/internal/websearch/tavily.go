package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
	networkrequest "eucli-box/src/network-request-system"
)

func callTavily(ctx context.Context, network networkrequest.System, provider ProviderConfig, request searchRequest, input types.ToolExecutionInput) (searchResponse, error) {
	apiKey, err := providerAPIKey(provider, input)
	if err != nil {
		return searchResponse{}, err
	}
	payload := map[string]any{
		"api_key":                    apiKey,
		"query":                      request.Query,
		"max_results":                request.MaxResults,
		"search_depth":               request.SearchDepth,
		"topic":                      request.Topic,
		"include_answer":             request.IncludeAnswer,
		"include_images":             request.IncludeImages,
		"include_image_descriptions": request.IncludeImageDescriptions,
	}
	if request.IncludeRawContent != "" {
		payload["include_raw_content"] = request.IncludeRawContent
	}
	if request.Country != "" {
		payload["country"] = strings.ToLower(request.Country)
	}
	if request.StartDate != "" || request.EndDate != "" {
		if request.StartDate != "" {
			payload["start_date"] = request.StartDate
		}
		if request.EndDate != "" {
			payload["end_date"] = request.EndDate
		}
	} else if request.TimeRange != "" {
		payload["time_range"] = request.TimeRange
	}
	body, statusCode, durationMs, err := doJSONRequest(ctx, network, provider, request, nil, payload)
	if err != nil {
		return searchResponse{}, err
	}
	var raw tavilyResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return searchResponse{}, fmt.Errorf("decode tavily response: %w", err)
	}
	results := make([]searchResult, 0, len(raw.Results))
	for _, item := range raw.Results {
		results = append(results, searchResult{Title: item.Title, URL: item.URL, Snippet: item.Content, Content: firstNonEmpty(item.RawContent, item.Content), Score: item.scorePtr()})
	}
	metadata := map[string]any{}
	if raw.Query != "" {
		metadata["providerQuery"] = raw.Query
	}
	if len(raw.Images) > 0 {
		metadata["images"] = raw.Images
	}
	return searchResponse{Provider: provider.ID, Query: request.Query, Answer: raw.Answer, Results: results, Metadata: metadata, StatusCode: statusCode, DurationMs: durationMs}, nil
}

type tavilyResponse struct {
	Query   string         `json:"query"`
	Answer  string         `json:"answer"`
	Images  []any          `json:"images"`
	Results []tavilyResult `json:"results"`
}

type tavilyResult struct {
	Title      string  `json:"title"`
	URL        string  `json:"url"`
	Content    string  `json:"content"`
	RawContent string  `json:"raw_content"`
	Score      float64 `json:"score"`
}

func (r tavilyResult) scorePtr() *float64 {
	if r.Score == 0 {
		return nil
	}
	score := r.Score
	return &score
}
