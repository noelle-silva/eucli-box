package aiimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"eucli-box/pkg/types"
	networkrequest "eucli-box/src/network-request-system"
)

// 图片与请求体的命名上限：失控的响应不能撑爆内存或会话附件。
const (
	maxRemoteImageBytes  = 25 << 20
	maxRequestImageBytes = 16 << 20
	remoteImageTimeout   = 60 * time.Second
)

// promptImage 是一张已归一化的输入图片：MIME 与原始字节。
type promptImage struct {
	Mime   string
	Base64 string
	DataURL string
	Bytes  []byte
}

var (
	embeddedDataURLPattern  = regexp.MustCompile(`(?i)data:image/(png|jpeg|jpg|webp|gif);base64,[A-Za-z0-9+/=_\-\r\n\t ]+`)
	embeddedHTTPURLPattern  = regexp.MustCompile(`(?i)https?://[^\s"'<>]+`)
	bareBase64Pattern       = regexp.MustCompile(`^[A-Za-z0-9+/=_\-\r\n\t ]+$`)
)

// normalizeImageInput 把 data URL 或裸 base64 归一为图片字节与 MIME。
func normalizeImageInput(input string) (promptImage, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return promptImage{}, errors.New("图片数据为空")
	}
	mime := ""
	if strings.HasPrefix(strings.ToLower(value), "data:image/") {
		header, payload, ok := strings.Cut(strings.TrimPrefix(value, "data:"), ",")
		if !ok || strings.TrimSpace(payload) == "" {
			return promptImage{}, errors.New("图片 data URL 无效")
		}
		mediaType, suffix, ok := strings.Cut(header, ";")
		if !ok || !strings.EqualFold(strings.TrimSpace(suffix), "base64") {
			return promptImage{}, errors.New("图片 data URL 必须是 base64")
		}
		mime = strings.ToLower(strings.TrimSpace(mediaType))
		value = payload
	}
	clean := strings.NewReplacer(" ", "", "\n", "", "\r", "", "\t", "", "-", "+", "_", "/").Replace(value)
	switch len(clean) % 4 {
	case 2:
		clean += "=="
	case 3:
		clean += "="
	case 1:
		return promptImage{}, errors.New("图片 base64 无效")
	}
	payload, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		return promptImage{}, errors.New("图片 base64 无效")
	}
	if len(payload) > maxRequestImageBytes {
		return promptImage{}, fmt.Errorf("图片超过大小上限（%d 字节）", maxRequestImageBytes)
	}
	if mime == "" {
		mime = inferKnownImageMime(payload)
	}
	mime = normalizeImageMime(mime)
	if mime == "" {
		mime = "image/png"
	}
	return promptImage{Mime: mime, Base64: base64.StdEncoding.EncodeToString(payload), DataURL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(payload), Bytes: payload}, nil
}

// inferKnownImageMime 按魔数识别图片类型；无法识别返回空串。
func inferKnownImageMime(payload []byte) string {
	switch {
	case len(payload) >= 8 && string(payload[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png"
	case len(payload) >= 3 && payload[0] == 0xff && payload[1] == 0xd8 && payload[2] == 0xff:
		return "image/jpeg"
	case len(payload) >= 12 && string(payload[:4]) == "RIFF" && string(payload[8:12]) == "WEBP":
		return "image/webp"
	case len(payload) >= 6 && (string(payload[:6]) == "GIF87a" || string(payload[:6]) == "GIF89a"):
		return "image/gif"
	default:
		return ""
	}
}

func normalizeImageMime(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/png":
		return "image/png"
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/webp":
		return "image/webp"
	case "image/gif":
		return "image/gif"
	default:
		return ""
	}
}

func extFromMime(mime string) string {
	switch normalizeImageMime(mime) {
	case "image/jpeg":
		return "jpg"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	default:
		return "png"
	}
}

// parseImageSource 从响应体解析图片来源并归一为 data URL：
// 先按内置候选字段与取值路径从 JSON 取源，取不到再扫描响应文本。
func parseImageSource(ctx context.Context, network networkrequest.System, body []byte, imagePath string) (string, error) {
	if imagePath != "" {
		var document any
		if err := json.Unmarshal(body, &document); err != nil {
			return "", fmt.Errorf("响应不是合法 JSON，无法按 imagePath 取图: %w", err)
		}
		source, ok := lookupJSONPath(document, imagePath)
		if !ok {
			return "", fmt.Errorf("响应中没有 imagePath 指向的字段: %s", imagePath)
		}
		return resolveImageSource(ctx, network, source)
	}
	source := extractImageFromResponse(body)
	if source == "" {
		return "", errors.New("响应中没有找到图片数据")
	}
	return resolveImageSource(ctx, network, source)
}

// lookupJSONPath 按点分路径在 JSON 文档中取值；数组下标为十进制数字。
func lookupJSONPath(document any, path string) (string, bool) {
	current := document
	for _, segment := range strings.Split(path, ".") {
		segment = strings.TrimSpace(segment)
		switch typed := current.(type) {
		case map[string]any:
			value, ok := typed[segment]
			if !ok {
				return "", false
			}
			current = value
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(typed) {
				return "", false
			}
			current = typed[index]
		default:
			return "", false
		}
	}
	text, ok := current.(string)
	if !ok {
		return "", false
	}
	return text, true
}

// extractImageFromResponse 扫描响应体：JSON 候选字段优先，其次内嵌文本。
func extractImageFromResponse(body []byte) string {
	var document any
	if err := json.Unmarshal(body, &document); err == nil {
		if source := extractImageFromJSONValue(document, 0); source != "" {
			return source
		}
	}
	return extractImageFromText(string(body))
}

// extractImageFromJSONValue 按候选字段递归找图片源：data[0]、images[0]、
// choices[0].message.content、data_url / b64_json / url 等常见形态。
func extractImageFromJSONValue(value any, depth int) string {
	if depth > 8 || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return extractImageFromText(typed)
	case []any:
		for _, item := range typed {
			if source := extractImageFromJSONValue(item, depth+1); source != "" {
				return source
			}
		}
	case map[string]any:
		if source := extractImageFromCollection(typed["data"], depth); source != "" {
			return source
		}
		if source := extractImageFromCollection(typed["images"], depth); source != "" {
			return source
		}
		if choices, ok := typed["choices"].([]any); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]any); ok {
				if message, ok := choice["message"].(map[string]any); ok {
					if source := extractImageFromJSONValue(message["content"], depth+1); source != "" {
						return source
					}
				}
			}
		}
		for _, key := range []string{"data_url", "dataUrl", "image", "image_data_url"} {
			if source := extractImageFromStringField(typed[key]); source != "" {
				return source
			}
		}
		for _, key := range []string{"b64_png", "b64_json", "b64", "base64", "image_base64", "png_base64"} {
			if source := imageDataURLFromBase64String(stringValue(typed[key])); source != "" {
				return source
			}
		}
		for _, key := range []string{"url", "image_url", "imageUrl", "output_url", "outputUrl"} {
			if source := extractImageFromStringField(typed[key]); source != "" {
				return source
			}
			if source := extractImageFromJSONValue(typed[key], depth+1); source != "" {
				return source
			}
		}
		for _, key := range []string{"content", "text", "message"} {
			if source := extractImageFromJSONValue(typed[key], depth+1); source != "" {
				return source
			}
		}
	}
	return ""
}

func extractImageFromCollection(value any, depth int) string {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return ""
	}
	return extractImageFromJSONValue(items[0], depth+1)
}

func extractImageFromStringField(value any) string {
	text := strings.TrimSpace(stringValue(value))
	if text == "" {
		return ""
	}
	if isHTTPURL(text) || strings.HasPrefix(text, "data:image/") || bareBase64Pattern.MatchString(text) {
		return normalizeImageDataURLOrBase64(text)
	}
	return ""
}

// extractImageFromText 从文本中找内嵌 data URL、图片 URL 或长 base64。
func extractImageFromText(text string) string {
	value := strings.TrimSpace(text)
	if value == "" {
		return ""
	}
	if match := embeddedDataURLPattern.FindString(value); match != "" {
		return normalizeImageDataURLOrBase64(match)
	}
	maybeJSON := stripCodeFences(value)
	if maybeJSON != value || strings.HasPrefix(maybeJSON, "{") || strings.HasPrefix(maybeJSON, "[") {
		var nested any
		if err := json.Unmarshal([]byte(maybeJSON), &nested); err == nil {
			if source := extractImageFromJSONValue(nested, 0); source != "" {
				return source
			}
		}
	}
	if source := extractHTTPImageURLFromText(value); source != "" {
		return source
	}
	if len(value) > 200 && bareBase64Pattern.MatchString(value) {
		return imageDataURLFromBase64String(value)
	}
	return ""
}

func stripCodeFences(text string) string {
	value := strings.TrimSpace(text)
	if !strings.HasPrefix(value, "```") {
		return value
	}
	start := strings.Index(value, "\n")
	end := strings.LastIndex(value, "```")
	if start >= 0 && end > start {
		return strings.TrimSpace(value[start+1 : end])
	}
	return value
}

func extractHTTPImageURLFromText(text string) string {
	for _, match := range embeddedHTTPURLPattern.FindAllString(text, -1) {
		candidate := normalizeHTTPURL(match)
		if candidate == "" {
			continue
		}
		parsed, err := url.Parse(candidate)
		if err != nil {
			continue
		}
		if isImageFileName(parsed.Path) {
			return candidate
		}
	}
	return ""
}

func isHTTPURL(input string) bool {
	return normalizeHTTPURL(input) != ""
}

func normalizeHTTPURL(input string) string {
	value := strings.TrimSpace(input)
	value = strings.TrimRight(value, ".,;:!?)>]}。！，、；：？）】》”’")
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return value
}

func normalizeImageDataURLOrBase64(input string) string {
	value := strings.TrimSpace(input)
	if value == "" {
		return ""
	}
	if source := normalizeHTTPURL(value); source != "" {
		return source
	}
	if strings.HasPrefix(value, "data:image/") {
		image, err := normalizeImageInput(value)
		if err != nil {
			return value
		}
		return image.DataURL
	}
	return imageDataURLFromBase64String(value)
}

func imageDataURLFromBase64String(input string) string {
	image, err := normalizeImageInput(input)
	if err != nil {
		return ""
	}
	return image.DataURL
}

// resolveImageSource 把取到的源归一为 data URL：URL 下载、base64 转码、
// data URL 原样。
func resolveImageSource(ctx context.Context, network networkrequest.System, source string) (string, error) {
	value := strings.TrimSpace(source)
	if value == "" {
		return "", errors.New("图片数据为空")
	}
	if imageURL := normalizeHTTPURL(value); imageURL != "" {
		return fetchRemoteImageDataURL(ctx, network, imageURL)
	}
	dataURL := normalizeImageDataURLOrBase64(value)
	if !strings.HasPrefix(dataURL, "data:image/") {
		return "", errors.New("图片数据无效")
	}
	return dataURL, nil
}

// fetchRemoteImageDataURL 经宿主网络系统下载远程图片并转 data URL。
func fetchRemoteImageDataURL(ctx context.Context, network networkrequest.System, imageURL string) (string, error) {
	response, err := network.Do(ctx, types.HTTPRequest{Method: http.MethodGet, URL: imageURL, BodyKind: types.HTTPBodyNone, Timeout: remoteImageTimeout})
	if err != nil {
		return "", fmt.Errorf("下载远程图片失败: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("下载远程图片失败：HTTP %d", response.StatusCode)
	}
	payload := response.Body
	if len(payload) > maxRemoteImageBytes {
		return "", fmt.Errorf("远程图片过大（超过 %d 字节）", maxRemoteImageBytes)
	}
	mime := inferKnownImageMime(payload)
	if mime == "" {
		return "", errors.New("远程 URL 返回的不是可识别图片")
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(payload), nil
}

// trimResponseBody 返回不超过上限的响应体文本，供错误信息使用。
func trimResponseBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 500 {
		return text[:500]
	}
	return text
}

// isImageFileName 判定文件名是否带图片扩展名。
func isImageFileName(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp", ".gif"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// marshalNoHTMLEscape 以不转义 HTML 的方式编码 JSON。
func marshalNoHTMLEscape(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buffer.Bytes()), nil
}
