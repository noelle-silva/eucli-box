package modelprovider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"eucli-box/pkg/types"
)

type protocolAdapter interface {
	BuildListModelsRequest(provider types.Provider, timeout int64) (types.HTTPRequest, error)
	ParseListModelsResponse(response types.HTTPResponse) ([]types.ModelInfo, error)
	BuildCompleteRequest(provider types.Provider, request types.ModelRequest, timeout int64) (types.HTTPRequest, error)
	ParseCompleteResponse(response types.HTTPResponse) (types.ModelResponse, error)
}

func adapterFor(protocol types.ProviderProtocol) (protocolAdapter, error) {
	switch protocol {
	case types.ProviderProtocolOpenAI:
		return openAIAdapter{}, nil
	case types.ProviderProtocolAnthropic:
		return anthropicAdapter{}, nil
	default:
		return nil, providerUnsupportedProtocol("provider protocol is not supported", nil)
	}
}

func joinURL(baseURL string, path string) string {
	return normalizeBaseURL(baseURL) + "/" + strings.TrimLeft(path, "/")
}

func encodeJSONBody(value any) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, providerInvalid("failed to encode provider request", err)
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}

func requireSuccess(response types.HTTPResponse) error {
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	message := fmt.Sprintf("provider service returned status %d", response.StatusCode)
	if len(response.Body) > 0 {
		bodyStr := string(response.Body)
		lines := strings.Split(bodyStr, "\n")
		filtered := make([]string, 0, len(lines))
		for _, line := range lines {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "key") || strings.Contains(lower, "api_key") || strings.Contains(lower, "secret") {
				continue
			}
			filtered = append(filtered, line)
		}
		bodyStr = strings.Join(filtered, "\n")
		if len(bodyStr) > 200 {
			bodyStr = bodyStr[:200] + "..."
		}
		message = message + ": " + bodyStr
	}
	return providerServiceFailed(message, nil)
}

func parseToolArguments(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, providerParseFailed("failed to parse tool arguments", err)
	}
	return args, nil
}

func toolSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return schema
}
