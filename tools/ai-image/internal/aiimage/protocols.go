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

	"eucli-box/pkg/types"
	networkrequest "eucli-box/src/network-request-system"
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
	timeout := requestTimeout(timeoutMs)
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

// requestTimeout 归一请求时限：缺省 120 秒，下限 5 秒，上限 1 小时。
func requestTimeout(timeoutMs int64) time.Duration {
	if timeoutMs <= 0 {
		return 120 * time.Second
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond
	if timeout < 5*time.Second {
		return 5 * time.Second
	}
	if timeout > time.Hour {
		return time.Hour
	}
	return timeout
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
	response, err := doRequest(ctx, network, http.MethodPost, provider.BaseURL+"/images/generations", headers, types.HTTPBodyJSON, body, timeout)
	if err != nil {
		return generationResult{}, err
	}
	if err := requireSuccess(response); err != nil {
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
	response, err := doRequest(ctx, network, http.MethodPost, provider.BaseURL+"/images/edits", headers, types.HTTPBodyBytes, body, timeout)
	if err != nil {
		return generationResult{}, err
	}
	if err := requireSuccess(response); err != nil {
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
	response, err := doRequest(ctx, network, http.MethodPost, provider.BaseURL+"/chat/completions", headers, types.HTTPBodyJSON, body, timeout)
	if err != nil {
		return generationResult{}, err
	}
	if err := requireSuccess(response); err != nil {
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
	response, err := doRequest(ctx, network, adapter.Request.Method, requestURL, headers, bodyKind, body, timeout)
	if err != nil {
		return generationResult{}, err
	}
	if err := requireSuccess(response); err != nil {
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

// requireSuccess 判定响应状态；失败时保留 HTTP 状态码并携带上游错误信息，
// 让调用方能区分参数类错误（4xx）与服务类错误（5xx）。
func requireSuccess(response types.HTTPResponse) error {
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	message := fmt.Sprintf("运营商请求失败：HTTP %d", response.StatusCode)
	if upstream := upstreamErrorMessage(response.Body); upstream != "" {
		message += "：" + upstream
	}
	return errors.New(message)
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
