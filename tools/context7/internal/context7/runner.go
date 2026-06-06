package context7

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
	networkrequest "eucli-box/src/network-request-system"
)

func Execute(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	config, err := loadConfig(input.ToolDirectory)
	if err != nil {
		return failure("load context7 config", err, nil)
	}
	request, err := parseRequest(input, config)
	if err != nil {
		return failure("parse context7 request", err, nil)
	}
	network, err := networkrequest.NewSystem(networkrequest.Config{DefaultTimeout: time.Duration(config.Limits.DefaultTimeoutMs) * time.Millisecond, MaxTimeout: time.Duration(config.Limits.MaxTimeoutMs) * time.Millisecond, UserAgent: "eucli-box-context7/1.0"})
	if err != nil {
		return failure("initialize context7 network", err, map[string]any{"action": request.Action})
	}
	metadata := map[string]any{
		"action":         request.Action,
		"query":          request.Query,
		"fast":           request.Fast,
		"maxOutputChars": request.MaxOutputChars,
	}
	if strings.TrimSpace(request.Description) != "" {
		metadata["description"] = request.Description
	}
	if request.Action == actionSearch {
		metadata["libraryName"] = request.LibraryName
		return searchLibraries(ctx, network, config, request, input, metadata)
	}
	metadata["libraryId"] = request.LibraryID
	return queryDocs(ctx, network, config, request, input, metadata)
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
