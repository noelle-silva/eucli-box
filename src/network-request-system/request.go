package networkrequest

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

var allowedMethods = map[string]struct{}{
	http.MethodGet:    {},
	http.MethodPost:   {},
	http.MethodPut:    {},
	http.MethodPatch:  {},
	http.MethodDelete: {},
}

func buildRequest(ctx context.Context, req types.HTTPRequest, config Config) (preparedRequest, error) {
	if ctx == nil {
		return preparedRequest{}, invalidRequest("context is required", nil)
	}
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if _, ok := allowedMethods[method]; !ok {
		return preparedRequest{}, invalidRequest("unsupported http method", nil)
	}
	parsedURL, err := url.ParseRequestURI(strings.TrimSpace(req.URL))
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return preparedRequest{}, invalidRequest("invalid request url", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return preparedRequest{}, invalidRequest("unsupported url scheme", nil)
	}
	timeout := req.Timeout
	if timeout == 0 {
		timeout = config.DefaultTimeout
	}
	if timeout < 0 || timeout > config.MaxTimeout {
		return preparedRequest{}, invalidRequest("request timeout is outside allowed range", nil)
	}
	body, contentType, err := buildBody(req.BodyKind, req.Body)
	if err != nil {
		return preparedRequest{}, err
	}
	timedCtx, cancel := context.WithTimeout(ctx, timeout)
	httpReq, err := http.NewRequestWithContext(timedCtx, method, parsedURL.String(), body)
	if err != nil {
		cancel()
		return preparedRequest{}, invalidRequest("failed to construct http request", err)
	}
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}
	if httpReq.Header.Get("User-Agent") == "" {
		httpReq.Header.Set("User-Agent", config.UserAgent)
	}
	if contentType != "" && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	return preparedRequest{request: httpReq, started: time.Now().UTC().UnixNano(), cancel: cancel}, nil
}

func buildBody(kind types.HTTPBodyKind, body []byte) (io.Reader, string, error) {
	if kind == "" {
		kind = types.HTTPBodyNone
	}
	switch kind {
	case types.HTTPBodyNone:
		if len(body) > 0 {
			return nil, "", invalidRequest("body kind none cannot carry a body", nil)
		}
		return nil, "", nil
	case types.HTTPBodyJSON:
		return bytes.NewReader(body), "application/json", nil
	case types.HTTPBodyForm:
		return bytes.NewReader(body), "application/x-www-form-urlencoded", nil
	case types.HTTPBodyText:
		return bytes.NewReader(body), "text/plain", nil
	case types.HTTPBodyBytes:
		return bytes.NewReader(body), "", nil
	default:
		return nil, "", invalidRequest("unsupported body kind", nil)
	}
}
