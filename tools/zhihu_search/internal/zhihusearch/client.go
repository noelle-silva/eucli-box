package zhihusearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"eucli-box/pkg/types"
	networkrequest "eucli-box/src/network-request-system"
)

type apiResponse struct {
	Code    int          `json:"Code"`
	Message string       `json:"Message"`
	Data    responseData `json:"Data"`
}

type responseData struct {
	Items []apiItem `json:"Items"`
}

type apiItem struct {
	Title        string `json:"Title"`
	URL          string `json:"Url"`
	AuthorName   string `json:"AuthorName"`
	ContentText  string `json:"ContentText"`
	VoteUpCount  int    `json:"VoteUpCount"`
	CommentCount int    `json:"CommentCount"`
	EditTime     any    `json:"EditTime"`
}

func callZhihu(ctx context.Context, network networkrequest.System, config Config, request searchRequest, input types.ToolExecutionInput) (apiResponse, int, int64, error) {
	secret, err := accessSecret(config, input)
	if err != nil {
		return apiResponse{}, 0, 0, err
	}
	endpoint, err := searchEndpoint(config, request)
	if err != nil {
		return apiResponse{}, 0, 0, err
	}
	requestURL, err := searchURL(endpoint, request)
	if err != nil {
		return apiResponse{}, 0, 0, err
	}
	resp, err := network.Do(ctx, types.HTTPRequest{
		Method: http.MethodGet,
		URL:    requestURL,
		Headers: map[string]string{
			"Authorization":       "Bearer " + secret,
			"X-Request-Timestamp": fmt.Sprint(time.Now().Unix()),
		},
		BodyKind: types.HTTPBodyNone,
		Timeout:  time.Duration(request.TimeoutMs) * time.Millisecond,
	})
	if err != nil {
		return apiResponse{}, 0, 0, fmt.Errorf("zhihu request failed: %w", err)
	}
	durationMs := resp.Duration.Milliseconds()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return apiResponse{}, resp.StatusCode, durationMs, fmt.Errorf("zhihu returned status %d: %s", resp.StatusCode, truncateText(string(resp.Body), 500))
	}
	var raw apiResponse
	if err := json.Unmarshal(resp.Body, &raw); err != nil {
		return apiResponse{}, resp.StatusCode, durationMs, fmt.Errorf("decode zhihu response: %w", err)
	}
	if raw.Code != 0 {
		return raw, resp.StatusCode, durationMs, fmt.Errorf("zhihu returned code %d: %s", raw.Code, raw.Message)
	}
	return raw, resp.StatusCode, durationMs, nil
}

func accessSecret(config Config, input types.ToolExecutionInput) (string, error) {
	if keyName := strings.TrimSpace(config.APIKeyUserConfig); keyName != "" {
		if value, ok := input.UserConfig[keyName]; ok && value != nil {
			secret, err := stringValue(value, keyName)
			if err != nil {
				return "", err
			}
			if secret != "" {
				return secret, nil
			}
		}
	}
	if envName := strings.TrimSpace(config.APIKeyEnv); envName != "" {
		if secret := strings.TrimSpace(os.Getenv(envName)); secret != "" {
			return secret, nil
		}
	}
	return "", fmt.Errorf("zhihu_search requires API key via userConfig.%s or %s", config.APIKeyUserConfig, config.APIKeyEnv)
}

func searchEndpoint(config Config, request searchRequest) (string, error) {
	if request.SearchType == searchTypeZhihu && request.ZhihuSearchURL != "" {
		return request.ZhihuSearchURL, nil
	}
	if request.SearchType == searchTypeGlobal && request.GlobalSearchURL != "" {
		return request.GlobalSearchURL, nil
	}
	endpoint := strings.TrimSpace(config.Endpoints[request.SearchType])
	if endpoint == "" {
		return "", fmt.Errorf("endpoint %q is not configured", request.SearchType)
	}
	if isAbsoluteURL(endpoint) {
		return endpoint, nil
	}
	return strings.TrimRight(request.BaseURL, "/") + endpoint, nil
}

func searchURL(endpoint string, request searchRequest) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("search endpoint must be an absolute URL")
	}
	params := parsed.Query()
	params.Set("Query", request.Query)
	params.Set("Count", fmt.Sprint(request.Count))
	parsed.RawQuery = params.Encode()
	return parsed.String(), nil
}
