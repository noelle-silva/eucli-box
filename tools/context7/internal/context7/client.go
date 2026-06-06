package context7

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"eucli-box/pkg/types"
	networkrequest "eucli-box/src/network-request-system"
)

type apiError struct {
	Error       string `json:"error"`
	Message     string `json:"message"`
	RedirectURL string `json:"redirectUrl"`
}

func searchLibraries(ctx context.Context, network networkrequest.System, config Config, request lookupRequest, input types.ToolExecutionInput, metadata map[string]any) types.ToolExecutionOutput {
	endpoint, err := withQuery(config.SearchEndpoint, map[string]string{"libraryName": request.LibraryName, "query": request.Query, "fast": strconv.FormatBool(request.Fast)})
	if err != nil {
		return failure("build context7 search request", err, metadata)
	}
	body, statusCode, durationMs, err := doGET(ctx, network, endpoint, request, config, input)
	metadata["statusCode"] = statusCode
	metadata["durationMs"] = durationMs
	if err != nil {
		return failure("execute context7 search request", err, metadata)
	}
	var response searchResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return failure("decode context7 search response", err, metadata)
	}
	metadata["resultsCount"] = len(response.Results)
	metadata["searchFilterApplied"] = response.SearchFilterApplied
	content, truncated := formatSearchContent(response, request)
	metadata["truncated"] = truncated
	return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: content, Metadata: metadata}
}

func queryDocs(ctx context.Context, network networkrequest.System, config Config, request lookupRequest, input types.ToolExecutionInput, metadata map[string]any) types.ToolExecutionOutput {
	endpoint, err := withQuery(config.ContextEndpoint, map[string]string{"libraryId": request.LibraryID, "query": request.Query, "type": "json", "fast": strconv.FormatBool(request.Fast)})
	if err != nil {
		return failure("build context7 docs request", err, metadata)
	}
	body, statusCode, durationMs, err := doGET(ctx, network, endpoint, request, config, input)
	metadata["statusCode"] = statusCode
	metadata["durationMs"] = durationMs
	if err != nil {
		return failure("execute context7 docs request", err, metadata)
	}
	var response docsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return failure("decode context7 docs response", err, metadata)
	}
	metadata["codeSnippets"] = len(response.CodeSnippets)
	metadata["infoSnippets"] = len(response.InfoSnippets)
	content, truncated := formatDocsContent(response, request)
	metadata["truncated"] = truncated
	return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: content, Metadata: metadata}
}

func doGET(ctx context.Context, network networkrequest.System, endpoint string, request lookupRequest, config Config, input types.ToolExecutionInput) ([]byte, int, int64, error) {
	headers := map[string]string{}
	apiKey, err := apiKey(config, input)
	if err != nil {
		return nil, 0, 0, err
	}
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}
	resp, err := network.Do(ctx, types.HTTPRequest{Method: http.MethodGet, URL: endpoint, Headers: headers, Timeout: time.Duration(request.TimeoutMs) * time.Millisecond})
	if err != nil {
		return nil, 0, 0, fmt.Errorf("context7 request failed: %w", err)
	}
	durationMs := resp.Duration.Milliseconds()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, resp.StatusCode, durationMs, context7HTTPError(resp.StatusCode, resp.Body)
	}
	return resp.Body, resp.StatusCode, durationMs, nil
}

func apiKey(config Config, input types.ToolExecutionInput) (string, error) {
	if keyName := strings.TrimSpace(config.APIKeyUserConfig); keyName != "" {
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
	if envName := strings.TrimSpace(config.APIKeyEnv); envName != "" {
		if key := strings.TrimSpace(os.Getenv(envName)); key != "" {
			return key, nil
		}
	}
	if config.AnonymousAllowed {
		return "", nil
	}
	return "", fmt.Errorf("context7 requires API key via userConfig.%s or %s", config.APIKeyUserConfig, config.APIKeyEnv)
}

func withQuery(endpoint string, params map[string]string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func context7HTTPError(statusCode int, body []byte) error {
	var parsed apiError
	if err := json.Unmarshal(body, &parsed); err == nil && (parsed.Message != "" || parsed.Error != "") {
		message := firstNonEmpty(parsed.Message, parsed.Error)
		if parsed.RedirectURL != "" {
			message = message + " redirectUrl=" + parsed.RedirectURL
		}
		return fmt.Errorf("context7 returned status %d: %s", statusCode, message)
	}
	return fmt.Errorf("context7 returned status %d: %s", statusCode, truncateText(string(body), 500))
}
