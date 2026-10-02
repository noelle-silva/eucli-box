package webfetch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"golang.org/x/net/html/charset"
)

// fetchResult 是一次成功抓取的结果：最终网址、状态码与解码后的正文。
type fetchResult struct {
	URL        string
	StatusCode int
	Kind       bodyKind
	Content    string
	Truncated  bool
	// Profile 是本次使用的浏览器身份名称。
	Profile string
	// Attempts 是实际发起的请求次数。
	Attempts int
}

// fetch 校验网址，使用带浏览器身份与 TLS 指纹的客户端发起请求；
// 被封锁时按退避策略换一套身份重试。
func fetch(ctx context.Context, rawURL string, timeoutMs int, config Config) (fetchResult, error) {
	current, err := validateFetchURL(rawURL)
	if err != nil {
		return fetchResult{}, err
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	maxAttempts := config.MaxRetries + 1
	var lastResult fetchResult
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		profile := selectBrowserProfile(rng)
		result, err := requestOnce(ctx, current, timeoutMs, config, profile)
		if err != nil {
			lastErr = err
			if attempt < maxAttempts {
				if waitErr := sleep(ctx, retryDelay(config, attempt, 0)); waitErr != nil {
					return fetchResult{}, waitErr
				}
				continue
			}
			return fetchResult{}, err
		}
		result.Attempts = attempt
		if isBlockedStatus(result.StatusCode) && attempt < maxAttempts {
			lastResult = result
			if waitErr := sleep(ctx, retryDelay(config, attempt, 0)); waitErr != nil {
				return fetchResult{}, waitErr
			}
			continue
		}
		return result, nil
	}
	if lastErr != nil {
		return fetchResult{}, lastErr
	}
	lastResult.Attempts = maxAttempts
	return lastResult, nil
}

// requestOnce 用一套浏览器身份发起一次请求，读取、限字节、分类并解码响应体。
func requestOnce(ctx context.Context, target *url.URL, timeoutMs int, config Config, profile browserProfile) (fetchResult, error) {
	client, err := newClient(config, profile, timeoutMs)
	if err != nil {
		return fetchResult{}, fmt.Errorf("build http client: %w", err)
	}
	request, err := fhttp.NewRequestWithContext(ctx, fhttp.MethodGet, target.String(), nil)
	if err != nil {
		return fetchResult{}, fmt.Errorf("build request: %w", err)
	}
	applyProfileHeaders(request, profile)
	response, err := client.Do(request)
	if err != nil {
		return fetchResult{}, fmt.Errorf("fetch %s: %w", target.Host, err)
	}
	return readBody(response, profile.Name, config)
}

// newClient 构造带 TLS 指纹的客户端：应用浏览器指纹、超时、代理与跳转上限。
func newClient(config Config, profile browserProfile, timeoutMs int) (tls_client.HttpClient, error) {
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutMilliseconds(timeoutMs),
		tls_client.WithClientProfile(profile.TLS),
		tls_client.WithRandomTLSExtensionOrder(),
		tls_client.WithCookieJar(tls_client.NewCookieJar()),
		tls_client.WithCustomRedirectFunc(redirectPolicy(config.MaxRedirects)),
	}
	if proxyURL := strings.TrimSpace(config.ProxyURL); proxyURL != "" {
		options = append(options, tls_client.WithProxyUrl(proxyURL))
	}
	return tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
}

// applyProfileHeaders 把浏览器身份的请求头与顺序写到请求上，
// 并补上从搜索引擎进入的 Referer。
func applyProfileHeaders(request *fhttp.Request, profile browserProfile) {
	for key, value := range profile.Headers {
		request.Header.Set(key, value)
	}
	if request.Header.Get("Referer") == "" {
		request.Header.Set("Referer", "https://www.google.com/")
	}
	if len(profile.HeaderOrder) > 0 {
		request.Header[fhttp.HeaderOrderKey] = append([]string(nil), profile.HeaderOrder...)
	}
	request.Header[fhttp.PHeaderOrderKey] = []string{":method", ":authority", ":scheme", ":path"}
}

// redirectPolicy 限制自动跟随的跳转次数，避免陷入跳转环。
func redirectPolicy(maxRedirects int) func(*fhttp.Request, []*fhttp.Request) error {
	return func(_ *fhttp.Request, via []*fhttp.Request) error {
		if len(via) > maxRedirects {
			return fmt.Errorf("exceeded the maximum of %d redirects", maxRedirects)
		}
		return nil
	}
}

// readBody 读取、限字节、分类并解码最终响应体。
func readBody(response *fhttp.Response, profileName string, config Config) (fetchResult, error) {
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
		Profile:    profileName,
	}, nil
}

// readCapped 最多读取 maxResponseBytes 字节。声明长度超过上限时立即拒绝；
// 流式读取超过上限时截断并如实标记，不把已读到的有界内容丢掉。
func readCapped(response *fhttp.Response, config Config) ([]byte, bool, error) {
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
