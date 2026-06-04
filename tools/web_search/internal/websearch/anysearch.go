package websearch

import (
	"context"
	"encoding/json"
	"fmt"

	"eucli-box/pkg/types"
	networkrequest "eucli-box/src/network-request-system"
)

func callAnySearch(ctx context.Context, network networkrequest.System, provider ProviderConfig, request searchRequest, input types.ToolExecutionInput) (searchResponse, error) {
	apiKey, err := providerAPIKey(provider, input)
	if err != nil {
		return searchResponse{}, err
	}
	payload := map[string]any{
		"query":       request.Query,
		"max_results": request.MaxResults,
	}
	if request.Domain != "" {
		payload["domain"] = request.Domain
	}
	if request.Tag != "" {
		payload["tag"] = request.Tag
	}
	if len(request.ContentTypes) > 0 {
		payload["content_types"] = request.ContentTypes
	}
	if request.Zone != "" {
		payload["zone"] = request.Zone
	}
	if request.Language != "" {
		payload["language"] = request.Language
	}
	if len(request.Params) > 0 {
		payload["params"] = request.Params
	}
	headers := map[string]string{}
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}
	body, statusCode, durationMs, err := doJSONRequest(ctx, network, provider, request, headers, payload)
	if err != nil {
		return searchResponse{}, err
	}
	var raw anySearchResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return searchResponse{}, fmt.Errorf("decode anysearch response: %w", err)
	}
	if raw.Code != 0 {
		return searchResponse{}, fmt.Errorf("anysearch returned code %d: %s", raw.Code, raw.Message)
	}
	results := make([]searchResult, 0, len(raw.Data.Results))
	for _, item := range raw.Data.Results {
		results = append(results, searchResult{Title: item.Title, URL: item.URL, Snippet: item.Snippet, Content: firstNonEmpty(item.Content, item.Snippet)})
	}
	metadata := map[string]any{}
	for key, value := range raw.Data.Metadata {
		metadata[key] = value
	}
	return searchResponse{Provider: provider.ID, Query: request.Query, Results: results, Metadata: metadata, StatusCode: statusCode, DurationMs: durationMs}, nil
}

type anySearchResponse struct {
	Code    int           `json:"code"`
	Message string        `json:"message"`
	Data    anySearchData `json:"data"`
}

type anySearchData struct {
	Results  []anySearchResult `json:"results"`
	Metadata map[string]any    `json:"metadata"`
}

type anySearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Content string `json:"content"`
}
