package aiimage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"eucli-box/tools/ai-image/internal/types"
	networkrequest "eucli-box/tools/ai-image/internal/networkrequest"
)

// generationRequest 是一次生成动作的全部输入。
type generationRequest struct {
	Prompt          string
	ReferenceImages []promptImage
}

// generationResult 是一次生成动作的产出：图片 data URL 与请求事实。
type generationResult struct {
	ImageDataURL string
	ProtocolKind string
	StatusCode   int
	DurationMs   int64
}

// callProvider 按运营商协议执行一次生图请求。
func callProvider(ctx context.Context, network networkrequest.System, configRootDir string, provider providerEntry, request generationRequest, timeoutMs int64) (generationResult, error) {
	timeout, err := requestTimeout(timeoutMs)
	if err != nil {
		return generationResult{}, err
	}
	switch provider.Protocol {
	case protocolImages:
		return callImagesProtocol(ctx, network, provider, request, timeout)
	case protocolImagesEdits:
		return callImagesEditsProtocol(ctx, network, provider, request, timeout)
	case protocolChat:
		return callChatProtocol(ctx, network, provider, request, timeout)
	case protocolAdapter:
		return callAdapterProtocol(ctx, network, configRootDir, provider, request, timeout)
	default:
		return generationResult{}, fmt.Errorf("运营商 %s 的协议不受支持: %q", provider.ID, provider.Protocol)
	}
}

// 请求时限的取值范围：缺省 120 秒；显式值必须在 5 秒到 1 小时之间，
// 越界直接参数错误，不做静默钳制。
const (
	defaultRequestTimeout = 120 * time.Second
	minRequestTimeoutMs   = 5000
	maxRequestTimeoutMs   = 3600000
)

// requestTimeout 解析请求时限：0/缺省取 120 秒；越界返回参数错误并回显范围。
func requestTimeout(timeoutMs int64) (time.Duration, error) {
	if timeoutMs == 0 {
		return defaultRequestTimeout, nil
	}
	if timeoutMs < minRequestTimeoutMs || timeoutMs > maxRequestTimeoutMs {
		return 0, permanentError(
			fmt.Errorf("timeoutMs 超出取值范围（%d-%d 毫秒，缺省 %d）", minRequestTimeoutMs, maxRequestTimeoutMs, defaultRequestTimeout.Milliseconds()),
			actionFixTimeout,
		)
	}
	return time.Duration(timeoutMs) * time.Millisecond, nil
}

// images 协议：POST {baseUrl}/images/generations，JSON 体 {model, prompt, n:1}。
func callImagesProtocol(ctx context.Context, network networkrequest.System, provider providerEntry, request generationRequest, timeout time.Duration) (generationResult, error) {
	model, err := resolveModel(provider, "")
	if err != nil {
		return generationResult{}, err
	}
	body, err := marshalNoHTMLEscape(map[string]any{"model": model, "prompt": request.Prompt, "n": 1})
	if err != nil {
		return generationResult{}, err
	}
	headers := map[string]string{"Authorization": "Bearer " + provider.APIKey, "Content-Type": "application/json"}
	response, retried, err := doGenerationRequest(ctx, network, http.MethodPost, provider.BaseURL+"/images/generations", headers, types.HTTPBodyJSON, body, timeout)
	if err != nil {
		return generationResult{}, err
	}
	if err := requireSuccess(response, retried); err != nil {
		return generationResult{}, err
	}
	imageDataURL, err := parseImageSource(ctx, network, response.Body, "")
	if err != nil {
		return generationResult{}, err
	}
	return generationResult{ImageDataURL: imageDataURL, ProtocolKind: protocolImages, StatusCode: response.StatusCode, DurationMs: response.Duration.Milliseconds()}, nil
}

// images-edits 协议：POST {baseUrl}/images/edits，multipart（model、prompt、
// 参考图 image[]）；无参考图明确失败。
func callImagesEditsProtocol(ctx context.Context, network networkrequest.System, provider providerEntry, request generationRequest, timeout time.Duration) (generationResult, error) {
	if len(request.ReferenceImages) == 0 {
		return generationResult{}, fmt.Errorf("运营商 %s 使用 images-edits 协议，必须提供参考图", provider.ID)
	}
	model, err := resolveModel(provider, "")
	if err != nil {
		return generationResult{}, err
	}
	parts := []multipartPart{
		{Name: "model", Value: model},
		{Name: "prompt", Value: request.Prompt},
	}
	for index, image := range request.ReferenceImages {
		parts = append(parts, multipartPart{
			Name:        "image[]",
			Filename:    fmt.Sprintf("ref-%d.%s", index+1, extFromMime(image.Mime)),
			ContentType: image.Mime,
			Bytes:       image.Bytes,
		})
	}
	body, contentType, err := buildMultipartFormData(parts)
	if err != nil {
		return generationResult{}, err
	}
	headers := map[string]string{"Authorization": "Bearer " + provider.APIKey, "Content-Type": contentType}
	response, retried, err := doGenerationRequest(ctx, network, http.MethodPost, provider.BaseURL+"/images/edits", headers, types.HTTPBodyBytes, body, timeout)
	if err != nil {
		return generationResult{}, err
	}
	if err := requireSuccess(response, retried); err != nil {
		return generationResult{}, err
	}
	imageDataURL, err := parseImageSource(ctx, network, response.Body, "")
	if err != nil {
		return generationResult{}, err
	}
	return generationResult{ImageDataURL: imageDataURL, ProtocolKind: protocolImagesEdits, StatusCode: response.StatusCode, DurationMs: response.Duration.Milliseconds()}, nil
}

// chat 协议：POST {baseUrl}/chat/completions，体 {model, messages,
// temperature:0.2}；有参考图时 user 内容为 text + image_url 数组。
func callChatProtocol(ctx context.Context, network networkrequest.System, provider providerEntry, request generationRequest, timeout time.Duration) (generationResult, error) {
	model, err := resolveModel(provider, "")
	if err != nil {
		return generationResult{}, err
	}
	content := any(request.Prompt)
	if len(request.ReferenceImages) > 0 {
		items := []map[string]any{{"type": "text", "text": request.Prompt}}
		for _, image := range request.ReferenceImages {
			items = append(items, map[string]any{"type": "image_url", "image_url": map[string]any{"url": image.DataURL}})
		}
		content = items
	}
	messages := []map[string]any{}
	if prompt := strings.TrimSpace(provider.ChatSystemPrompt); prompt != "" {
		messages = append(messages, map[string]any{"role": "system", "content": prompt})
	}
	messages = append(messages, map[string]any{"role": "user", "content": content})
	body, err := marshalNoHTMLEscape(map[string]any{"model": model, "messages": messages, "temperature": 0.2})
	if err != nil {
		return generationResult{}, err
	}
	headers := map[string]string{"Authorization": "Bearer " + provider.APIKey, "Content-Type": "application/json"}
	response, retried, err := doGenerationRequest(ctx, network, http.MethodPost, provider.BaseURL+"/chat/completions", headers, types.HTTPBodyJSON, body, timeout)
	if err != nil {
		return generationResult{}, err
	}
	if err := requireSuccess(response, retried); err != nil {
		return generationResult{}, err
	}
	imageDataURL, err := parseImageSource(ctx, network, response.Body, "")
	if err != nil {
		return generationResult{}, err
	}
	return generationResult{ImageDataURL: imageDataURL, ProtocolKind: protocolChat, StatusCode: response.StatusCode, DurationMs: response.Duration.Milliseconds()}, nil
}

// callAdapterProtocol 按声明式适配文件拼请求、取结果。
func callAdapterProtocol(ctx context.Context, network networkrequest.System, configRootDir string, provider providerEntry, request generationRequest, timeout time.Duration) (generationResult, error) {
	model, err := resolveModel(provider, "")
	if err != nil {
		return generationResult{}, err
	}
	adapter, err := loadAdapter(configRootDir, provider.Adapter)
	if err != nil {
		return generationResult{}, err
	}
	renderCtx := adapterRenderContext{APIKey: provider.APIKey, Model: model, Prompt: request.Prompt, Images: request.ReferenceImages}
	requestURL := provider.BaseURL + "/" + strings.TrimLeft(renderAdapterString(adapter.Request.Path, renderCtx), "/")
	headers := map[string]string{}
	for key, value := range adapter.Request.Headers {
		headers[key] = renderAdapterString(value, renderCtx)
	}
	var body []byte
	bodyKind := types.HTTPBodyNone
	if len(adapter.Request.JSON) > 0 {
		body, err = renderAdapterJSON(adapter.Request.JSON, renderCtx)
		if err != nil {
			return generationResult{}, err
		}
		bodyKind = types.HTTPBodyJSON
		if _, ok := headers["Content-Type"]; !ok {
			headers["Content-Type"] = "application/json"
		}
	} else {
		parts := make([]multipartPart, 0, len(adapter.Request.Form.Fields)+len(request.ReferenceImages))
		for _, key := range sortedStringKeys(adapter.Request.Form.Fields) {
			parts = append(parts, multipartPart{Name: key, Value: renderAdapterString(adapter.Request.Form.Fields[key], renderCtx)})
		}
		for index, image := range request.ReferenceImages {
			parts = append(parts, multipartPart{
				Name:        adapter.Request.Form.Images.Field,
				Filename:    renderFormFilename(adapter.Request.Form.Images.Filename, index+1, extFromMime(image.Mime)),
				ContentType: image.Mime,
				Bytes:       image.Bytes,
			})
		}
		multipartBody, contentType, err := buildMultipartFormData(parts)
		if err != nil {
			return generationResult{}, err
		}
		body = multipartBody
		bodyKind = types.HTTPBodyBytes
		headers["Content-Type"] = contentType
	}
	response, retried, err := doGenerationRequest(ctx, network, adapter.Request.Method, requestURL, headers, bodyKind, body, timeout)
	if err != nil {
		return generationResult{}, err
	}
	if err := requireSuccess(response, retried); err != nil {
		return generationResult{}, err
	}
	imageDataURL, err := parseImageSource(ctx, network, response.Body, adapter.Response.ImagePath)
	if err != nil {
		return generationResult{}, err
	}
	return generationResult{ImageDataURL: imageDataURL, ProtocolKind: protocolAdapter, StatusCode: response.StatusCode, DurationMs: response.Duration.Milliseconds()}, nil
}

// doRequest 经宿主网络系统发出一次请求。
func doRequest(ctx context.Context, network networkrequest.System, method string, url string, headers map[string]string, bodyKind types.HTTPBodyKind, body []byte, timeout time.Duration) (types.HTTPResponse, error) {
	if network == nil {
		return types.HTTPResponse{}, errors.New("网络请求系统不可用")
	}
	return network.Do(ctx, types.HTTPRequest{Method: method, URL: url, Headers: headers, BodyKind: bodyKind, Body: body, Timeout: timeout})
}

// generationRetryBackoff 是自动重试前的退避等待。
const generationRetryBackoff = 500 * time.Millisecond

// doGenerationRequest 发出一次幂等生图请求：遇连接中断与 502/503/504
// 瞬时故障时退避后自动重试一次；返回响应、是否已重试与最终错误。
func doGenerationRequest(ctx context.Context, network networkrequest.System, method string, url string, headers map[string]string, bodyKind types.HTTPBodyKind, body []byte, timeout time.Duration) (types.HTTPResponse, bool, error) {
	response, err := doRequest(ctx, network, method, url, headers, bodyKind, body, timeout)
	if err != nil {
		if isTransientTransportError(err) {
			if waitErr := sleepWithContext(ctx, generationRetryBackoff); waitErr == nil {
				response, err = doRequest(ctx, network, method, url, headers, bodyKind, body, timeout)
				if err != nil {
					return types.HTTPResponse{}, true, generationTransportError(err)
				}
				return response, true, nil
			}
		}
		return types.HTTPResponse{}, false, generationTransportError(err)
	}
	if isTransientStatus(response.StatusCode) {
		if waitErr := sleepWithContext(ctx, generationRetryBackoff); waitErr == nil {
			retryResponse, retryErr := doRequest(ctx, network, method, url, headers, bodyKind, body, timeout)
			if retryErr != nil {
				return types.HTTPResponse{}, true, generationTransportError(retryErr)
			}
			return retryResponse, true, nil
		}
	}
	return response, false, nil
}

// isTransientTransportError 判定可重试的传输类瞬时故障：连接中断与请求超时。
func isTransientTransportError(err error) bool {
	switch networkErrorCode(err) {
	case "network.connection_lost", "network.timeout":
		return true
	default:
		return false
	}
}

// isTransientStatus 判定可重试的上游瞬时状态：502/503/504。
func isTransientStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// generationTransportError 把传输类错误翻译为带分类的失败：
// 瞬时故障标可重试，参数类标不可重试，其余归入网络检查建议。
func generationTransportError(err error) error {
	switch networkErrorCode(err) {
	case "network.connection_lost", "network.timeout":
		return retryableError(err, actionRetryLater)
	case "network.invalid_request":
		return permanentError(err, actionFixParams)
	default:
		return retryableError(err, actionCheckNetwork)
	}
}

// sleepWithContext 按退避时长等待；上下文取消时立即返回错误。
func sleepWithContext(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// requireSuccess 判定响应状态；失败时保留 HTTP 状态码并携带上游错误信息，
// 并附上失败分类：429 与全部 5xx 可重试（自动重试只覆盖 502/503/504），
// 其余 4xx 为参数类不可重试；自动重试已发生时在信息中标注。
func requireSuccess(response types.HTTPResponse, retried bool) error {
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	message := fmt.Sprintf("运营商请求失败：HTTP %d", response.StatusCode)
	if upstream := upstreamErrorMessage(response.Body); upstream != "" {
		message += "：" + upstream
	}
	if retried {
		message += "（已自动重试 1 次）"
	}
	err := errors.New(message)
	if response.StatusCode >= http.StatusInternalServerError || response.StatusCode == http.StatusTooManyRequests {
		return retryableError(err, actionRetryLater)
	}
	return permanentError(err, actionCheckProvider)
}

// upstreamErrorMessage 从错误响应中取可读信息。
func upstreamErrorMessage(body []byte) string {
	var decoded any
	if err := json.Unmarshal(body, &decoded); err == nil {
		if root, ok := decoded.(map[string]any); ok {
			if errorBox, ok := root["error"].(map[string]any); ok {
				if message := strings.TrimSpace(stringValue(errorBox["message"])); message != "" {
					return message
				}
			}
			if message := strings.TrimSpace(stringValue(root["message"])); message != "" {
				return message
			}
		}
	}
	return trimResponseBody(body)
}

// sortedStringKeys 返回字符串 map 的键排序副本。
func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
