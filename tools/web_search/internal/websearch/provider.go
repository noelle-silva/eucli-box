package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"eucli-box/pkg/types"
	networkrequest "eucli-box/src/network-request-system"
)

type searchResponse struct {
	Provider   string
	Query      string
	Answer     string
	Results    []searchResult
	Metadata   map[string]any
	StatusCode int
	DurationMs int64
}

type searchResult struct {
	Title   string
	URL     string
	Snippet string
	Content string
	Score   *float64
}

func callProvider(ctx context.Context, network networkrequest.System, provider ProviderConfig, request searchRequest, input types.ToolExecutionInput) (searchResponse, error) {
	if request.MaxResults > provider.MaxResults {
		return searchResponse{}, fmt.Errorf("maxResults must be between 1 and %d for provider %q", provider.MaxResults, provider.ID)
	}
	switch provider.Kind {
	case providerKindTavily:
		return callTavily(ctx, network, provider, request, input)
	case providerKindAnySearch:
		return callAnySearch(ctx, network, provider, request, input)
	default:
		return searchResponse{}, fmt.Errorf("provider %q kind %q is unsupported", provider.ID, provider.Kind)
	}
}

func doJSONRequest(ctx context.Context, network networkrequest.System, provider ProviderConfig, request searchRequest, headers map[string]string, payload any) ([]byte, int, int64, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("encode %s request: %w", provider.ID, err)
	}
	if headers == nil {
		headers = map[string]string{}
	}
	headers["Content-Type"] = "application/json"
	resp, err := network.Do(ctx, types.HTTPRequest{Method: http.MethodPost, URL: provider.Endpoint, Headers: headers, BodyKind: types.HTTPBodyJSON, Body: body, Timeout: time.Duration(request.TimeoutMs) * time.Millisecond})
	if err != nil {
		return nil, 0, 0, fmt.Errorf("%s request failed: %w", provider.ID, err)
	}
	durationMs := resp.Duration.Milliseconds()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, resp.StatusCode, durationMs, fmt.Errorf("%s returned status %d: %s", provider.ID, resp.StatusCode, truncateText(string(resp.Body), 500))
	}
	return resp.Body, resp.StatusCode, durationMs, nil
}

func providerAPIKey(provider ProviderConfig, input types.ToolExecutionInput) (string, error) {
	if keyName := strings.TrimSpace(provider.APIKeyUserConfig); keyName != "" {
		if value, ok := input.UserConfig[keyName]; ok && value != nil {
			key, err := stringValue(value, keyName)
			if err != nil {
				return "", err
			}
			if key != "" {
				return key, nil
			}
		}
	}
	if envName := strings.TrimSpace(provider.APIKeyEnv); envName != "" {
		if key := strings.TrimSpace(os.Getenv(envName)); key != "" {
			return key, nil
		}
	}
	if provider.AnonymousAllowed {
		return "", nil
	}
	return "", fmt.Errorf("provider %q requires API key via userConfig.%s or %s", provider.ID, provider.APIKeyUserConfig, provider.APIKeyEnv)
}
