package webfetch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

// fetchResult 是一次成功抓取的结果：最终网址、状态码与解码后的正文。
type fetchResult struct {
	URL        string
	StatusCode int
	Kind       bodyKind
	Content    string
	Truncated  bool
}

// fetch 校验网址，发起一次请求并解码响应；跳转由客户端自动跟随。
//
// 请求基于系统默认传输（http.DefaultTransport），自动遵循
// HTTP_PROXY/HTTPS_PROXY/NO_PROXY 环境变量，不自行解析地址。
func fetch(ctx context.Context, rawURL string, timeoutMs int, config Config) (fetchResult, error) {
	current, err := validateFetchURL(rawURL)
	if err != nil {
		return fetchResult{}, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, current.String(), nil)
	if err != nil {
		return fetchResult{}, fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("User-Agent", config.UserAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml,text/*;q=0.9,application/json;q=0.8")
	client := &http.Client{
		Transport:     http.DefaultTransport,
		CheckRedirect: redirectPolicy(config.MaxRedirects),
	}
	response, err := client.Do(request)
	if err != nil {
		return fetchResult{}, fmt.Errorf("fetch %s: %w", current.Host, err)
	}
	return readBody(response, config)
}

// redirectPolicy 限制自动跟随的跳转次数，避免陷入跳转环。
func redirectPolicy(maxRedirects int) func(*http.Request, []*http.Request) error {
	return func(_ *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return fmt.Errorf("exceeded the maximum of %d redirects", maxRedirects)
		}
		return nil
	}
}

// readBody 读取、限字节、分类并解码最终响应体。
func readBody(response *http.Response, config Config) (fetchResult, error) {
	finalURL := response.Request.URL.String()
	contentType := response.Header.Get("Content-Type")
	kind, ok := classifyContentType(contentType)
	if !ok {
		_ = response.Body.Close()
		if contentType == "" {
			contentType = "unknown"
		}
		return fetchResult{}, fmt.Errorf("unsupported content type %q", contentType)
	}
	body, truncatedByBytes, err := readCapped(response, config)
	if err != nil {
		return fetchResult{}, err
	}
	decoded, err := decodeBody(body, contentType)
	if err != nil {
		return fetchResult{}, err
	}
	truncatedByChars := false
	if len([]rune(decoded)) > config.MaxBodyChars {
		decoded = string([]rune(decoded)[:config.MaxBodyChars])
		truncatedByChars = true
	}
	return fetchResult{
		URL:        finalURL,
		StatusCode: response.StatusCode,
		Kind:       kind,
		Content:    decoded,
		Truncated:  truncatedByBytes || truncatedByChars,
	}, nil
}

// readCapped 最多读取 maxResponseBytes 字节。声明长度超过上限时立即拒绝；
// 流式读取超过上限时截断并如实标记，不把已读到的有界内容丢掉。
func readCapped(response *http.Response, config Config) ([]byte, bool, error) {
	defer response.Body.Close()
	if declared := response.Header.Get("Content-Length"); declared != "" {
		if length, err := parseContentLength(declared); err == nil && length > int64(config.MaxResponseBytes) {
			return nil, false, fmt.Errorf("response exceeds the maximum of %d bytes", config.MaxResponseBytes)
		}
	}
	limited := io.LimitReader(response.Body, int64(config.MaxResponseBytes)+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, false, fmt.Errorf("read response body: %w", err)
	}
	if len(body) > config.MaxResponseBytes {
		return body[:config.MaxResponseBytes], true, nil
	}
	return body, false, nil
}

// decodeBody 依据内容类型声明的编码解码正文，缺省按 UTF-8；
// charset 会同时参考响应头与网页内的声明。
func decodeBody(body []byte, contentType string) (string, error) {
	decoded, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return "", fmt.Errorf("unsupported charset in %q", contentType)
	}
	result, err := io.ReadAll(decoded)
	if err != nil {
		return "", fmt.Errorf("decode response body: %w", err)
	}
	return string(result), nil
}

func parseContentLength(value string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(value), 10, 64)
}
