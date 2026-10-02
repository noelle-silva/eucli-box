package webfetch

import (
	"fmt"
	"net/url"
	"strings"
)

// maxURLLength 是允许的请求网址最大长度。
const maxURLLength = 2048

// bodyKind 是本工具能解码的正文种类。
type bodyKind string

const (
	bodyKindHTML bodyKind = "html"
	bodyKindText bodyKind = "text"
)

// validateFetchURL 解析网址并执行与网络无关的传输限制：
// 只接受 http 与 https，不接受网址内嵌账号密码，并限制网址长度。
func validateFetchURL(input string) (*url.URL, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, fmt.Errorf("url must be a non-empty string")
	}
	if len(trimmed) > maxURLLength {
		return nil, fmt.Errorf("url exceeds the maximum length of %d", maxURLLength)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %s", trimmed)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported url scheme %q (only http and https are allowed)", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("invalid url: %s", trimmed)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("credentials in urls are not allowed")
	}
	return parsed, nil
}

// classifyContentType 把响应的内容类型归为可解码的正文种类；
// 二进制等不支持的类型返回空。
func classifyContentType(contentType string) (bodyKind, bool) {
	mime := strings.ToLower(strings.TrimSpace(contentType))
	if index := strings.IndexByte(mime, ';'); index >= 0 {
		mime = strings.TrimSpace(mime[:index])
	}
	switch {
	case mime == "text/html" || mime == "application/xhtml+xml":
		return bodyKindHTML, true
	case strings.HasPrefix(mime, "text/"):
		return bodyKindText, true
	case mime == "application/json" || mime == "application/xml":
		return bodyKindText, true
	case strings.HasSuffix(mime, "+json") || strings.HasSuffix(mime, "+xml"):
		return bodyKindText, true
	default:
		return "", false
	}
}
