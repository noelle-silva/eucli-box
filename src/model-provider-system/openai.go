package modelprovider

import (
	"encoding/json"
	"time"

	"eucli-box/pkg/types"
)

type openAIAdapter struct{}

type openAIModelListResponse struct {
	Data []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"data"`
}

type openAICompleteResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

func (openAIAdapter) BuildListModelsRequest(provider types.Provider, timeout int64) (types.HTTPRequest, error) {
	return types.HTTPRequest{
		Method:  "GET",
		URL:     joinURL(provider.BaseURL, "/models"),
		Headers: map[string]string{"Authorization": "Bearer " + provider.Key},
		Timeout: time.Duration(timeout),
	}, nil
}

func (openAIAdapter) ParseListModelsResponse(response types.HTTPResponse) ([]types.ModelInfo, error) {
	if err := requireSuccess(response); err != nil {
		return nil, err
	}
	var payload openAIModelListResponse
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return nil, providerParseFailed("failed to parse OpenAI model list", err)
	}
	models := make([]types.ModelInfo, 0, len(payload.Data))
	for _, model := range payload.Data {
		name := model.Name
		if name == "" {
			name = model.ID
		}
		if model.ID != "" {
			models = append(models, types.ModelInfo{ID: model.ID, Name: name})
		}
	}
	return models, nil
}

func (openAIAdapter) BuildCompleteRequest(provider types.Provider, request types.ModelRequest, timeout int64) (types.HTTPRequest, error) {
	body := map[string]any{
		"model":       request.Coordinate.ModelID,
		"messages":    openAIMessages(request.Messages),
		"temperature": request.Temperature,
	}
	if len(request.Tools) > 0 {
		body["tools"] = openAITools(request.Tools)
	}
	payload, err := encodeJSONBody(body)
	if err != nil {
		return types.HTTPRequest{}, err
	}
	return types.HTTPRequest{
		Method:   "POST",
		URL:      joinURL(provider.BaseURL, "/chat/completions"),
		Headers:  map[string]string{"Authorization": "Bearer " + provider.Key},
		BodyKind: types.HTTPBodyJSON,
		Body:     payload,
		Timeout:  time.Duration(timeout),
	}, nil
}

func (openAIAdapter) ParseCompleteResponse(response types.HTTPResponse) (types.ModelResponse, error) {
	if err := requireSuccess(response); err != nil {
		return types.ModelResponse{}, err
	}
	var payload openAICompleteResponse
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return types.ModelResponse{}, providerParseFailed("failed to parse OpenAI completion", err)
	}
	result := types.ModelResponse{ID: payload.ID, Raw: response.Body, CreatedAt: time.Now().UTC()}
	if len(payload.Choices) == 0 {
		return result, nil
	}
	message := payload.Choices[0].Message
	result.Content = message.Content
	for _, toolCall := range message.ToolCalls {
		args, err := parseToolArguments(toolCall.Function.Arguments)
		if err != nil {
			return types.ModelResponse{}, err
		}
		result.ToolIntents = append(result.ToolIntents, types.ToolIntent{ID: toolCall.ID, ToolName: toolCall.Function.Name, Arguments: args, Raw: toolCall.Function.Arguments, CreatedAt: result.CreatedAt})
	}
	return result, nil
}

func openAIMessages(messages []types.PromptMessage) []map[string]string {
	converted := make([]map[string]string, 0, len(messages))
	for _, message := range messages {
		converted = append(converted, map[string]string{"role": message.Role, "content": message.Content})
	}
	return converted
}

func openAITools(tools []types.ToolDefinition) []map[string]any {
	converted := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		converted = append(converted, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  toolSchema(tool.InputSchema),
			},
		})
	}
	return converted
}
