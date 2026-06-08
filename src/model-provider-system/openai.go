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
			Content          string          `json:"content"`
			ReasoningContent string          `json:"reasoning_content"`
			Reasoning        json.RawMessage `json:"reasoning"`
			ToolCalls        []struct {
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
			Content          string          `json:"content"`
			ReasoningContent string          `json:"reasoning_content"`
			Reasoning        json.RawMessage `json:"reasoning"`
			ToolCalls        []struct {
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
	sse           *sseParser
	onEvent       types.ModelStreamHandler
	id            string
	content       strings.Builder
	reasoning     strings.Builder
	reasoningData string
	toolCalls     map[int]*openAIToolCallBuilder
	createdAt     time.Time
}

type openAIReasoningPayload struct {
	EncryptedContent string `json:"encrypted_content"`
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
		"model":    request.Coordinate.ModelID,
		"messages": messages,
	}
	if request.ReasoningEffort != "" {
		body["reasoning_effort"] = openAIReasoningEffort(request.ReasoningEffort)
	} else {
		body["temperature"] = request.Temperature
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

func openAIReasoningEffort(effort types.ReasoningEffort) string {
	switch types.NormalizeReasoningEffort(effort, types.DefaultReasoningEffort) {
	case types.ReasoningEffortVeryLow:
		return "minimal"
	case types.ReasoningEffortLow:
		return "low"
	case types.ReasoningEffortHigh:
		return "high"
	case types.ReasoningEffortVeryHigh:
		return "xhigh"
	default:
		return "medium"
	}
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
	result := types.ModelResponse{ID: p.id, Content: p.content.String(), Reasoning: p.reasoning.String(), ReasoningSource: "op", ReasoningData: p.reasoningData, Raw: response.Body, CreatedAt: p.createdAt}
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
		result.ToolIntents = append(result.ToolIntents, types.ToolIntent{ID: id, ToolName: builder.Name, Arguments: args, Source: types.ToolCallSourceNative, Raw: argsRaw, CreatedAt: result.CreatedAt})
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
		reasoningDelta := choice.Delta.ReasoningContent
		reasoningData := ""
		if text, data, err := parseOpenAIReasoningPayload(choice.Delta.Reasoning); err != nil {
			return providerParseFailed("failed to parse OpenAI stream reasoning payload", err)
		} else {
			if reasoningDelta == "" {
				reasoningDelta = text
			}
			reasoningData = data
		}
		if reasoningDelta != "" {
			p.reasoning.WriteString(reasoningDelta)
		}
		if reasoningData != "" {
			p.reasoningData = reasoningData
		}
		if (reasoningDelta != "" || reasoningData != "") && p.onEvent != nil {
			if err := p.onEvent(types.ModelStreamEvent{Type: types.ModelStreamEventReasoningDelta, ReasoningDelta: reasoningDelta, Reasoning: p.reasoning.String(), ReasoningSource: "op", ReasoningData: p.reasoningData, CreatedAt: time.Now().UTC()}); err != nil {
				return err
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
	result.Reasoning = message.ReasoningContent
	text, data, err := parseOpenAIReasoningPayload(message.Reasoning)
	if err != nil {
		return types.ModelResponse{}, providerParseFailed("failed to parse OpenAI reasoning payload", err)
	}
	if result.Reasoning == "" {
		result.Reasoning = text
	}
	result.ReasoningData = data
	if strings.TrimSpace(result.Reasoning) != "" || strings.TrimSpace(result.ReasoningData) != "" {
		result.ReasoningSource = "op"
	}
	for _, toolCall := range message.ToolCalls {
		args, err := parseToolArguments(toolCall.Function.Arguments)
		if err != nil {
			return types.ModelResponse{}, err
		}
		result.ToolIntents = append(result.ToolIntents, types.ToolIntent{ID: toolCall.ID, ToolName: toolCall.Function.Name, Arguments: args, Source: types.ToolCallSourceNative, Raw: toolCall.Function.Arguments, CreatedAt: result.CreatedAt})
	}
	return result, nil
}

func parseOpenAIReasoningPayload(raw json.RawMessage) (string, string, error) {
	if len(raw) == 0 {
		return "", "", nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", "", nil
	}
	if strings.HasPrefix(trimmed, "\"") {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", "", err
		}
		return text, "", nil
	}
	var payload openAIReasoningPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", err
	}
	return "", strings.TrimSpace(payload.EncryptedContent), nil
}

func openAIMessages(messages []types.PromptMessage) ([]map[string]any, error) {
	converted := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		nativeToolParts := promptNativeToolParts(message)
		textProtocolResultParts := promptTextProtocolToolResultParts(message)
		if message.Role == "assistant" && len(nativeToolParts) > 0 {
			if err := requireToolResults(nativeToolParts); err != nil {
				return nil, err
			}
			assistant := openAIAssistantMessage(message)
			toolCalls := make([]map[string]any, 0, len(nativeToolParts))
			for _, part := range nativeToolParts {
				arguments, err := toolArgumentsJSON(part)
				if err != nil {
					return nil, err
				}
				toolCalls = append(toolCalls, map[string]any{"id": part.CallID, "type": "function", "function": map[string]any{"name": part.ToolName, "arguments": arguments}})
			}
			assistant["tool_calls"] = toolCalls
			converted = append(converted, assistant)
			for _, part := range nativeToolParts {
				if part.Result == nil {
					continue
				}
				converted = append(converted, map[string]any{"role": "tool", "tool_call_id": part.CallID, "content": toolResultText(part)})
			}
			converted = appendUserTextObservation(converted, textProtocolToolResultsText(textProtocolResultParts))
			continue
		}
		if message.Role == "assistant" {
			converted = append(converted, openAIAssistantMessage(message))
		} else {
			converted = append(converted, map[string]any{"role": message.Role, "content": openAIMessageContent(message)})
		}
		if message.Role == "assistant" {
			converted = appendUserTextObservation(converted, textProtocolToolResultsText(textProtocolResultParts))
		}
	}
	return converted, nil
}

func openAIAssistantMessage(message types.PromptMessage) map[string]any {
	assistant := map[string]any{"role": "assistant", "content": openAIMessageContent(message)}
	if reasoning := promptReasoningText(message); reasoning != "" {
		assistant["reasoning_content"] = reasoning
	}
	if part, ok := firstPromptReasoningPartForSource(message, "op"); ok {
		if data := strings.TrimSpace(part.Data); data != "" {
			assistant["reasoning"] = map[string]any{"encrypted_content": data}
		}
	}
	return assistant
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
