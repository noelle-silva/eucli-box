// Package everything 是 everything 工具的业务动作：
// 通过 Everything 应用的开放接口，搜索本机文件并整理结果。
package everything

import (
	"context"
	"time"

	"eucli-box/pkg/types"
)

// Execute 执行一次工具动作。
func Execute(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	if err := ctx.Err(); err != nil {
		return failure("tool execution cancelled", err, nil)
	}
	config, err := loadConfig(input.ToolBodyDirectory)
	if err != nil {
		return failure("load everything config", err, nil)
	}
	request, err := parseSearchRequest(input, config)
	if err != nil {
		return failure("parse everything request", err, nil)
	}
	connection, err := loadUserConfig(input)
	if err != nil {
		return failure("load everything connection config", err, nil)
	}
	client, err := newRPCClient(connection.Endpoint, connection.Key, requestTimeout(request, config))
	if err != nil {
		return failure("create everything client", err, nil)
	}

	metadata := requestMetadata(request)
	startedAt := time.Now()
	payload, err := searchEverything(ctx, client, request)
	metadata["durationMs"] = int64(time.Since(startedAt) / time.Millisecond)
	if err != nil {
		return failure("execute everything search", err, metadata)
	}
	metadata["resultsCount"] = len(payload.Results)
	content, truncated := formatContent(payload, request)
	metadata["truncated"] = truncated
	return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: content, Metadata: metadata}
}

// requestMetadata 给出这次搜索的可见事实。
func requestMetadata(request searchRequest) map[string]any {
	metadata := map[string]any{
		"query":          request.Query,
		"scopePath":      request.ScopePath,
		"maxResults":     request.MaxResults,
		"timeoutMs":      request.TimeoutMs,
		"maxOutputChars": request.MaxOutputChars,
	}
	if request.Description != "" {
		metadata["description"] = request.Description
	}
	return metadata
}

// failure 是统一的失败输出：如实说明失败原因，不返回假成功；
// 失败同样携带机器可读错误码（如有）。
func failure(scope string, err error, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	errorMessage := scope
	if err != nil {
		errorMessage = scope + ": " + err.Error()
	}
	if code := errorCode(err); code != "" {
		metadata["code"] = code
	}
	metadata["error"] = errorMessage
	return types.ToolExecutionOutput{Status: types.ToolStatusFailed, Content: errorMessage, Error: errorMessage, Metadata: metadata}
}
