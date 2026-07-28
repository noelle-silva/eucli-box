package zhihusearch

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
		return failure("load zhihu_search config", err, nil)
	}
	request, err := parseRequest(input, config)
	if err != nil {
		return failure("parse zhihu_search request", err, nil)
	}
	network, err := networkrequest.NewSystem(networkrequest.Config{DefaultTimeout: time.Duration(config.Limits.DefaultTimeoutMs) * time.Millisecond, MaxTimeout: time.Duration(config.Limits.MaxTimeoutMs) * time.Millisecond, UserAgent: "eucli-box-zhihu-search/1.0"})
	if err != nil {
		return failure("initialize zhihu_search network", err, map[string]any{"searchType": request.SearchType})
	}
	apiResponse, statusCode, durationMs, err := callZhihu(ctx, network, config, request, input)
	items := normalizeItems(apiResponse, request.SearchType)
	sources := buildSources(items)
	metadata := map[string]any{
		"searchType":     request.SearchType,
		"query":          request.Query,
		"resultsCount":   len(items),
		"statusCode":     statusCode,
		"durationMs":     durationMs,
		"count":          request.Count,
		"maxOutputChars": request.MaxOutputChars,
		"code":           apiResponse.Code,
		"apiMessage":     apiResponse.Message,
		"sources":        sources,
		"items":          items,
	}
	if strings.TrimSpace(request.Description) != "" {
		metadata["description"] = request.Description
	}
	if err != nil {
		return failure("execute zhihu_search request", err, metadata)
	}
	content, truncated := formatContent(request.SearchType, apiResponse.Message, items, request)
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
