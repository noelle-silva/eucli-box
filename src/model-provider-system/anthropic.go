package modelprovider

import (
	"encoding/json"
	"time"

	"eucli-box/pkg/types"
)

type anthropicAdapter struct{}

type anthropicModelListResponse struct {
	Data []struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"data"`
}

type anthropicCompleteResponse struct {
	ID      string `json:"id"`
	Content []struct {
		Type  string         `json:"type"`
		Text  string         `json:"text"`
		ID    string         `json:"id"`
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
	} `json:"content"`
}

func (anthropicAdapter) BuildListModelsRequest(provider types.Provider, timeout int64) (types.HTTPRequest, error) {
	return types.HTTPRequest{
		Method: "GET",
		URL:    joinURL(provider.BaseURL, "/models"),
		Headers: map[string]string{
			"x-api-key":         provider.Key,
			"anthropic-version": "2023-06-01",
		},
		Timeout: time.Duration(timeout),
	}, nil
}

func (anthropicAdapter) ParseListModelsResponse(response types.HTTPResponse) ([]types.ModelInfo, error) {
	if err := requireSuccess(response); err != nil {
		return nil, err
	}
	var payload anthropicModelListResponse
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return nil, providerParseFailed("failed to parse Anthropic model list", err)
	}
	models := make([]types.ModelInfo, 0, len(payload.Data))
	for _, model := range payload.Data {
		name := model.DisplayName
		if name == "" {
			name = model.ID
		}
		if model.ID != "" {
			models = append(models, types.ModelInfo{ID: model.ID, Name: name})
		}
	}
	return models, nil
}

func (anthropicAdapter) BuildCompleteRequest(provider types.Provider, request types.ModelRequest, timeout int64) (types.HTTPRequest, error) {
	messages, systemText, err := anthropicMessages(request.Messages)
	if err != nil {
		return types.HTTPRequest{}, err
	}
	body := map[string]any{
		"model":       request.Coordinate.ModelID,
		"messages":    messages,
		"temperature": request.Temperature,
		"max_tokens":  4096,
	}
	if systemText != "" {
		body["system"] = systemText
	}
	if len(request.Tools) > 0 {
		body["tools"] = anthropicTools(request.Tools)
	}
	payload, err := encodeJSONBody(body)
	if err != nil {
		return types.HTTPRequest{}, err
	}
	return types.HTTPRequest{
		Method: "POST",
		URL:    joinURL(provider.BaseURL, "/messages"),
		Headers: map[string]string{
			"x-api-key":         provider.Key,
			"anthropic-version": "2023-06-01",
		},
		BodyKind: types.HTTPBodyJSON,
		Body:     payload,
		Timeout:  time.Duration(timeout),
	}, nil
}

func (anthropicAdapter) ParseCompleteResponse(response types.HTTPResponse) (types.ModelResponse, error) {
	if err := requireSuccess(response); err != nil {
		return types.ModelResponse{}, err
	}
	var payload anthropicCompleteResponse
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return types.ModelResponse{}, providerParseFailed("failed to parse Anthropic completion", err)
	}
	createdAt := time.Now().UTC()
	result := types.ModelResponse{ID: payload.ID, Raw: response.Body, CreatedAt: createdAt}
	for _, item := range payload.Content {
		switch item.Type {
		case "text":
			if result.Content == "" {
				result.Content = item.Text
			} else {
				result.Content += "\n" + item.Text
			}
		case "tool_use":
			result.ToolIntents = append(result.ToolIntents, types.ToolIntent{ID: item.ID, ToolName: item.Name, Arguments: item.Input, CreatedAt: createdAt})
		}
	}
	return result, nil
}

func anthropicMessages(messages []types.PromptMessage) ([]map[string]string, string, error) {
	converted := make([]map[string]string, 0, len(messages))
	systemText := ""
	for _, message := range messages {
		switch message.Role {
		case "system":
			if systemText == "" {
				systemText = message.Content
			} else {
				systemText += "\n\n" + message.Content
			}
		case "user", "assistant":
			converted = append(converted, map[string]string{"role": message.Role, "content": message.Content})
		default:
			return nil, "", providerInvalid("Anthropic messages only support system, user, and assistant roles", nil)
		}
	}
	return converted, systemText, nil
}

func anthropicTools(tools []types.ToolDefinition) []map[string]any {
	converted := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		converted = append(converted, map[string]any{
			"name":         tool.Name,
			"description":  tool.Description,
			"input_schema": toolSchema(tool.InputSchema),
		})
	}
	return converted
}
