// Package webfetch 实现 web_fetch 工具的业务动作：获取一个 HTTP(S) 网址的内容。
//
// 它只培育几个基础根动作：读取输入、读取运行时配置、校验网址、构造带浏览器身份
// 与 TLS 指纹的客户端、发起一次有界请求、被封锁时换身份退避重试、解码正文、
// 把网页渲染为 Markdown、输出工具协议结果。复杂行为由这些根动作协作涌现，
// 不在工具内预设任何「抓取管理器」。
package webfetch

import (
	"context"
	"strings"

	"eucli-box/tools/web_fetch/internal/types"
)

// Execute 执行一次抓取动作。
func Execute(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	if err := ctx.Err(); err != nil {
		return failure("tool execution cancelled", err, nil)
	}
	config, err := loadConfig(input.ToolBodyDirectory)
	if err != nil {
		return failure("load web_fetch config", err, nil)
	}
	request, err := parseRequest(input, config)
	if err != nil {
		return failure("parse web_fetch request", err, nil)
	}
	result, err := fetch(ctx, request.URL, request.TimeoutMs, config)
	metadata := map[string]any{
		"url":            request.URL,
		"maxOutputChars": request.MaxOutputChars,
	}
	if strings.TrimSpace(request.Description) != "" {
		metadata["description"] = request.Description
	}
	if err != nil {
		return failure("execute web_fetch request", err, metadata)
	}
	content, truncated := renderResult(result, request.MaxOutputChars)
	metadata["finalUrl"] = result.URL
	metadata["statusCode"] = result.StatusCode
	metadata["truncated"] = truncated
	metadata["profile"] = result.Profile
	metadata["attempts"] = result.Attempts
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
