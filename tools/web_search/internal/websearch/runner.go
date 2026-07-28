package websearch

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
	networkrequest "eucli-box/src/network-request-system"
)

func Execute(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	config, err := loadConfig(input.ToolBodyDirectory)
	if err != nil {
		return failure("load web_search config", err, nil)
	}
	request, err := parseRequest(input, config)
	if err != nil {
		return failure("parse web_search request", err, nil)
	}
	provider, err := selectProvider(config, request.Provider)
	if err != nil {
		return failure("select web_search provider", err, map[string]any{"provider": effectiveProviderName(config, request.Provider)})
	}
	network, err := networkrequest.NewSystem(networkrequest.Config{DefaultTimeout: time.Duration(config.Limits.DefaultTimeoutMs) * time.Millisecond, MaxTimeout: time.Duration(config.Limits.MaxTimeoutMs) * time.Millisecond, UserAgent: "eucli-box-web-search/1.0"})
	if err != nil {
		return failure("initialize web_search network", err, map[string]any{"provider": provider.ID})
	}
	response, err := callProvider(ctx, network, provider, request, input)
	metadata := map[string]any{
		"provider":       provider.ID,
		"query":          request.Query,
		"resultsCount":   len(response.Results),
		"statusCode":     response.StatusCode,
		"durationMs":     response.DurationMs,
		"maxResults":     request.MaxResults,
		"maxOutputChars": request.MaxOutputChars,
	}
	if strings.TrimSpace(request.Description) != "" {
		metadata["description"] = request.Description
	}
	if len(response.Metadata) > 0 {
		metadata["providerMetadata"] = response.Metadata
	}
	if err != nil {
		return failure("execute web_search request", err, metadata)
	}
	content, truncated := formatContent(response, request)
	metadata["truncated"] = truncated
	return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: content, Metadata: metadata}
}

func failure(scope string, err error, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	errorMessage := scope
	if err != nil {
		errorMessage = scope + ": " + err.Error()
	}
	metadata["error"] = errorMessage
	return types.ToolExecutionOutput{Status: types.ToolStatusFailed, Content: errorMessage, Error: errorMessage, Metadata: metadata}
}
