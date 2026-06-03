package modelprovider

import (
	"encoding/json"
	"strconv"
	"strings"
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

type openAIStreamResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

type openAIToolCallBuilder struct {
	ID        string
	Name      string
	Arguments strings.Builder
}

type openAIStreamParser struct {
	sse       *sseParser
	onEvent   types.ModelStreamHandler
	id        string
	content   strings.Builder
	toolCalls map[int]*openAIToolCallBuilder
	createdAt time.Time
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
	messages, err := openAIMessages(request.Messages)
	if err != nil {
		return types.HTTPRequest{}, err
	}
	body := map[string]any{
		"model":       request.Coordinate.ModelID,
		"messages":    messages,
		"temperature": request.Temperature,
	}
	if request.Stream {
		body["stream"] = true
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
		Headers:  map[string]string{"Authorization": "Bearer " + provider.Key, "Content-Type": "application/json"},
		BodyKind: types.HTTPBodyJSON,
		Body:     payload,
		Timeout:  time.Duration(timeout),
	}, nil
}

func (openAIAdapter) NewCompleteStreamParser(onEvent types.ModelStreamHandler) completeStreamParser {
	parser := &openAIStreamParser{onEvent: onEvent, toolCalls: map[int]*openAIToolCallBuilder{}, createdAt: time.Now().UTC()}
	parser.sse = newSSEParser(parser.acceptEvent)
	return parser
}

func (p *openAIStreamParser) Accept(data []byte) error {
	return p.sse.Accept(data)
}

func (p *openAIStreamParser) Finish(response types.HTTPResponse) (types.ModelResponse, error) {
	if err := requireSuccess(response); err != nil {
		return types.ModelResponse{}, err
	}
	if err := p.sse.Finish(); err != nil {
		return types.ModelResponse{}, err
	}
	result := types.ModelResponse{ID: p.id, Content: p.content.String(), Raw: response.Body, CreatedAt: p.createdAt}
	indexes := make([]int, 0, len(p.toolCalls))
	for index := range p.toolCalls {
		indexes = append(indexes, index)
	}
	sortInts(indexes)
	for _, index := range indexes {
		builder := p.toolCalls[index]
		if builder == nil || strings.TrimSpace(builder.Name) == "" {
			continue
		}
		argsRaw := builder.Arguments.String()
		args, err := parseToolArguments(argsRaw)
		if err != nil {
			return types.ModelResponse{}, err
		}
		id := strings.TrimSpace(builder.ID)
		if id == "" {
			id = "tool-call-" + strconv.Itoa(index)
		}
		result.ToolIntents = append(result.ToolIntents, types.ToolIntent{ID: id, ToolName: builder.Name, Arguments: args, Raw: argsRaw, CreatedAt: result.CreatedAt})
	}
	return result, nil
}

func (p *openAIStreamParser) acceptEvent(event sseEvent) error {
	if strings.TrimSpace(event.Data) == "[DONE]" {
		return nil
	}
	var payload openAIStreamResponse
	if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
		return providerParseFailed("failed to parse OpenAI stream event", err)
	}
	if p.id == "" {
		p.id = payload.ID
	}
	for _, choice := range payload.Choices {
		if choice.Delta.Content != "" {
			p.content.WriteString(choice.Delta.Content)
			if p.onEvent != nil {
				if err := p.onEvent(types.ModelStreamEvent{Type: types.ModelStreamEventContentDelta, ContentDelta: choice.Delta.Content, Content: p.content.String(), CreatedAt: time.Now().UTC()}); err != nil {
					return err
				}
			}
		}
		for _, toolCall := range choice.Delta.ToolCalls {
			builder := p.toolCalls[toolCall.Index]
			if builder == nil {
				builder = &openAIToolCallBuilder{}
				p.toolCalls[toolCall.Index] = builder
			}
			if toolCall.ID != "" {
				builder.ID = toolCall.ID
			}
			if toolCall.Function.Name != "" {
				builder.Name = toolCall.Function.Name
			}
			if toolCall.Function.Arguments != "" {
				builder.Arguments.WriteString(toolCall.Function.Arguments)
			}
		}
	}
	return nil
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

func openAIMessages(messages []types.PromptMessage) ([]map[string]any, error) {
	converted := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		toolParts := promptToolParts(message)
		if message.Role == "assistant" && len(toolParts) > 0 {
			if err := requireToolResults(toolParts); err != nil {
				return nil, err
			}
			assistant := map[string]any{"role": "assistant", "content": openAIMessageContent(message)}
			toolCalls := make([]map[string]any, 0, len(toolParts))
			for _, part := range toolParts {
				arguments, err := toolArgumentsJSON(part)
				if err != nil {
					return nil, err
				}
				toolCalls = append(toolCalls, map[string]any{"id": part.CallID, "type": "function", "function": map[string]any{"name": part.ToolName, "arguments": arguments}})
			}
			assistant["tool_calls"] = toolCalls
			converted = append(converted, assistant)
			for _, part := range toolParts {
				if part.Result == nil {
					continue
				}
				converted = append(converted, map[string]any{"role": "tool", "tool_call_id": part.CallID, "content": toolResultText(part)})
			}
			continue
		}
		converted = append(converted, map[string]any{"role": message.Role, "content": openAIMessageContent(message)})
	}
	return converted, nil
}

func openAIMessageContent(message types.PromptMessage) any {
	if len(message.Images) == 0 {
		return message.Content
	}
	parts := []map[string]any{}
	if strings.TrimSpace(message.Content) != "" {
		parts = append(parts, map[string]any{"type": "text", "text": message.Content})
	}
	for _, image := range message.Images {
		dataURL := strings.TrimSpace(image.DataURL)
		if dataURL == "" {
			continue
		}
		parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL}})
	}
	if len(parts) == 0 {
		return message.Content
	}
	return parts
}

func openAITools(tools []types.ToolDefinition) []map[string]any {
	converted := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		converted = append(converted, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": modelToolDescription(tool),
				"parameters":  toolSchema(tool.InputSchema),
			},
		})
	}
	return converted
}
