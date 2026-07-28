package everything

import (
	"context"
	"strings"

	"eucli-box/pkg/types"
)

func Execute(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	config, err := loadConfig(input.ToolBodyDirectory)
	if err != nil {
		return failure("load everything config", err, nil)
	}
	request, err := parseRequest(input, config)
	if err != nil {
		return failure("parse everything request", err, nil)
	}
	metadata := requestMetadata(request)
	provider, err := resolveSearchProvider(config, input)
	if err != nil {
		return failure("resolve Everything provider", err, metadata)
	}
	metadata["provider"] = provider.ID
	metadata["executableSource"] = provider.ExecutableSource
	metadata["runtimeSource"] = provider.RuntimeSource
	var lock *runtimeLock
	if usesBundledRuntime(provider) {
		lock, err = acquireBundledRuntimeLock(ctx, input.ToolDataDirectory, config, request)
		if err != nil {
			return failure("lock bundled Everything runtime", err, metadata)
		}
		defer lock.Release()
		request, err = ensureBundledRuntime(ctx, input.ToolDataDirectory, config, provider, request)
		if err != nil {
			return failure("prepare bundled Everything runtime", err, metadata)
		}
		metadata["instanceName"] = request.InstanceName
		if requiresBundledWindowsService(request) {
			metadata["serviceName"] = bundledServiceName(request.InstanceName)
		}
	}
	response, err := searchEverything(ctx, provider.ESExecutable, request)
	metadata["durationMs"] = response.DurationMs
	metadata["resultsCount"] = len(response.Results)
	if err != nil {
		return failure("execute everything search", err, metadata)
	}
	content, truncated := formatContent(response, request)
	metadata["truncated"] = truncated
	return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: content, Metadata: metadata}
}

func requestMetadata(request searchRequest) map[string]any {
	metadata := map[string]any{
		"query":          request.Query,
		"scopePath":      request.ScopePath,
		"scopePaths":     request.ScopePaths,
		"scopeMode":      request.ScopeMode,
		"instanceName":   request.InstanceName,
		"maxResults":     request.MaxResults,
		"timeoutMs":      request.TimeoutMs,
		"maxOutputChars": request.MaxOutputChars,
	}
	if strings.TrimSpace(request.Description) != "" {
		metadata["description"] = request.Description
	}
	return metadata
}

func usesBundledRuntime(provider selectedProvider) bool {
	return provider.Bundled
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
