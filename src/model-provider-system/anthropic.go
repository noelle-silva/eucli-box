package modelprovider

import (
	"encoding/json"
	"strconv"
	"strings"
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
		Type      string         `json:"type"`
		Text      string         `json:"text"`
		Thinking  string         `json:"thinking"`
		Signature string         `json:"signature"`
		Data      string         `json:"data"`
		ID        string         `json:"id"`
		Name      string         `json:"name"`
		Input     map[string]any `json:"input"`
	} `json:"content"`
}

type anthropicStreamPayload struct {
	Type    string `json:"type"`
	Message struct {
		ID string `json:"id"`
	} `json:"message"`
	Index        int `json:"index"`
	ContentBlock struct {
		Type      string         `json:"type"`
		Text      string         `json:"text"`
		Thinking  string         `json:"thinking"`
		Signature string         `json:"signature"`
		Data      string         `json:"data"`
		ID        string         `json:"id"`
		Name      string         `json:"name"`
		Input     map[string]any `json:"input"`
	} `json:"content_block"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		Signature   string `json:"signature"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
}

type anthropicToolUseBuilder struct {
	ID          string
	Name        string
	Input       map[string]any
	PartialJSON strings.Builder
}

type anthropicStreamParser struct {
	sse                *sseParser
	onEvent            types.ModelStreamHandler
	id                 string
	content            strings.Builder
	reasoning          strings.Builder
	reasoningSignature string
	reasoningData      string
	toolUses           map[int]*anthropicToolUseBuilder
	createdAt          time.Time
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
		"model":      request.Coordinate.ModelID,
		"messages":   messages,
		"max_tokens": 4096,
	}
	if request.ReasoningEffort != "" {
		body["thinking"] = map[string]any{"type": "enabled", "budget_tokens": anthropicThinkingBudgetTokens(request.ReasoningEffort)}
	} else {
		body["temperature"] = request.Temperature
	}
	if request.Stream {
		body["stream"] = true
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
			"Content-Type":      "application/json",
		},
		BodyKind: types.HTTPBodyJSON,
		Body:     payload,
		Timeout:  time.Duration(timeout),
	}, nil
}

func anthropicThinkingBudgetTokens(effort types.ReasoningEffort) int {
	switch types.NormalizeReasoningEffort(effort, types.DefaultReasoningEffort) {
	case types.ReasoningEffortVeryLow:
		return 1024
	case types.ReasoningEffortLow:
		return 1536
	case types.ReasoningEffortHigh:
		return 3072
	case types.ReasoningEffortVeryHigh:
		return 3584
	default:
		return 2048
	}
}

func (anthropicAdapter) NewCompleteStreamParser(onEvent types.ModelStreamHandler) completeStreamParser {
	parser := &anthropicStreamParser{onEvent: onEvent, toolUses: map[int]*anthropicToolUseBuilder{}, createdAt: time.Now().UTC()}
	parser.sse = newSSEParser(parser.acceptEvent)
	return parser
}

func (p *anthropicStreamParser) Accept(data []byte) error {
	return p.sse.Accept(data)
}

func (p *anthropicStreamParser) Finish(response types.HTTPResponse) (types.ModelResponse, error) {
	if err := requireSuccess(response); err != nil {
		return types.ModelResponse{}, err
	}
	if err := p.sse.Finish(); err != nil {
		return types.ModelResponse{}, err
	}
	result := types.ModelResponse{ID: p.id, Content: p.content.String(), Reasoning: p.reasoning.String(), ReasoningSource: "an", ReasoningSignature: p.reasoningSignature, ReasoningData: p.reasoningData, Raw: response.Body, CreatedAt: p.createdAt}
	indexes := make([]int, 0, len(p.toolUses))
	for index := range p.toolUses {
		indexes = append(indexes, index)
	}
	sortInts(indexes)
	for _, index := range indexes {
		builder := p.toolUses[index]
		if builder == nil || strings.TrimSpace(builder.Name) == "" {
			continue
		}
		argsRaw := strings.TrimSpace(builder.PartialJSON.String())
		args := builder.Input
		if argsRaw != "" {
			parsed, err := parseToolArguments(argsRaw)
			if err != nil {
				return types.ModelResponse{}, err
			}
			args = parsed
		} else if args == nil {
			var err error
			args, err = parseToolArguments(argsRaw)
			if err != nil {
				return types.ModelResponse{}, err
			}
		}
		id := strings.TrimSpace(builder.ID)
		if id == "" {
			id = "tool-use-" + strconv.Itoa(index)
		}
		result.ToolIntents = append(result.ToolIntents, types.ToolIntent{ID: id, ToolName: builder.Name, Arguments: args, Source: types.ToolCallSourceNative, Raw: argsRaw, CreatedAt: result.CreatedAt})
	}
	return result, nil
}

func (p *anthropicStreamParser) acceptEvent(event sseEvent) error {
	if strings.TrimSpace(event.Data) == "" {
		return nil
	}
	var payload anthropicStreamPayload
	if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
		return providerParseFailed("failed to parse Anthropic stream event", err)
	}
	if p.id == "" {
		p.id = payload.Message.ID
	}
	switch payload.Type {
	case "content_block_start":
		if payload.ContentBlock.Type == "text" && payload.ContentBlock.Text != "" {
			return p.appendText(payload.ContentBlock.Text)
		}
		if payload.ContentBlock.Type == "thinking" {
			p.reasoningSignature = strings.TrimSpace(payload.ContentBlock.Signature)
			p.reasoningData = strings.TrimSpace(payload.ContentBlock.Data)
			return p.appendReasoning(payload.ContentBlock.Thinking)
		}
		if payload.ContentBlock.Type == "tool_use" {
			builder := p.toolUses[payload.Index]
			if builder == nil {
				builder = &anthropicToolUseBuilder{}
				p.toolUses[payload.Index] = builder
			}
			builder.ID = payload.ContentBlock.ID
			builder.Name = payload.ContentBlock.Name
			builder.Input = payload.ContentBlock.Input
		}
	case "content_block_delta":
		switch payload.Delta.Type {
		case "text_delta":
			return p.appendText(payload.Delta.Text)
		case "thinking_delta":
			return p.appendReasoning(payload.Delta.Thinking)
		case "signature_delta":
			p.reasoningSignature += payload.Delta.Signature
			if p.onEvent == nil {
				return nil
			}
			return p.onEvent(types.ModelStreamEvent{Type: types.ModelStreamEventReasoningDelta, Reasoning: p.reasoning.String(), ReasoningSource: "an", ReasoningSignature: p.reasoningSignature, ReasoningData: p.reasoningData, CreatedAt: time.Now().UTC()})
		case "input_json_delta":
			builder := p.toolUses[payload.Index]
			if builder == nil {
				builder = &anthropicToolUseBuilder{}
				p.toolUses[payload.Index] = builder
			}
			builder.PartialJSON.WriteString(payload.Delta.PartialJSON)
		}
	}
	return nil
}

func (p *anthropicStreamParser) appendText(delta string) error {
	if delta == "" {
		return nil
	}
	p.content.WriteString(delta)
	if p.onEvent == nil {
		return nil
	}
	return p.onEvent(types.ModelStreamEvent{Type: types.ModelStreamEventContentDelta, ContentDelta: delta, Content: p.content.String(), CreatedAt: time.Now().UTC()})
}

func (p *anthropicStreamParser) appendReasoning(delta string) error {
	if delta == "" {
		return nil
	}
	p.reasoning.WriteString(delta)
	if p.onEvent == nil {
		return nil
	}
	return p.onEvent(types.ModelStreamEvent{Type: types.ModelStreamEventReasoningDelta, ReasoningDelta: delta, Reasoning: p.reasoning.String(), ReasoningSource: "an", ReasoningSignature: p.reasoningSignature, ReasoningData: p.reasoningData, CreatedAt: time.Now().UTC()})
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
		case "thinking":
			if item.Thinking == "" && item.Signature == "" && item.Data == "" {
				continue
			}
			if result.Reasoning == "" {
				result.Reasoning = item.Thinking
			} else {
				result.Reasoning += item.Thinking
			}
			result.ReasoningSignature = strings.TrimSpace(item.Signature)
			result.ReasoningData = strings.TrimSpace(item.Data)
		case "tool_use":
			result.ToolIntents = append(result.ToolIntents, types.ToolIntent{ID: item.ID, ToolName: item.Name, Arguments: item.Input, Source: types.ToolCallSourceNative, CreatedAt: createdAt})
		}
	}
	if strings.TrimSpace(result.Reasoning) != "" || strings.TrimSpace(result.ReasoningSignature) != "" || strings.TrimSpace(result.ReasoningData) != "" {
		result.ReasoningSource = "an"
	}
	return result, nil
}

func anthropicMessages(messages []types.PromptMessage) ([]map[string]any, string, error) {
	converted := make([]map[string]any, 0, len(messages))
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
			nativeToolParts := promptNativeToolParts(message)
			textProtocolResultParts := promptTextProtocolToolResultParts(message)
			if message.Role == "assistant" && len(nativeToolParts) > 0 {
				if err := requireToolResults(nativeToolParts); err != nil {
					return nil, "", err
				}
				content, err := anthropicAssistantToolContent(message, nativeToolParts)
				if err != nil {
					return nil, "", err
				}
				converted = append(converted, map[string]any{"role": "assistant", "content": appendAnthropicReasoningContent(content, message)})
				resultBlocks := anthropicToolResultBlocks(nativeToolParts)
				if len(resultBlocks) > 0 {
					converted = append(converted, map[string]any{"role": "user", "content": resultBlocks})
				}
				converted = appendUserTextObservation(converted, textProtocolToolResultsText(textProtocolResultParts))
				continue
			}
			content, err := anthropicMessageContent(message)
			if err != nil {
				return nil, "", err
			}
			if message.Role == "assistant" {
				content = appendAnthropicReasoningContent(content, message)
			}
			converted = append(converted, map[string]any{"role": message.Role, "content": content})
			if message.Role == "assistant" {
				converted = appendUserTextObservation(converted, textProtocolToolResultsText(textProtocolResultParts))
			}
		default:
			return nil, "", providerInvalid("Anthropic messages only support system, user, and assistant roles", nil)
		}
	}
	return converted, systemText, nil
}

func appendAnthropicReasoningContent(content any, message types.PromptMessage) any {
	thinkingText := promptReasoningText(message)
	carrierPart, hasCarrier := firstPromptReasoningPartForSource(message, "an")
	if thinkingText == "" && !hasCarrier {
		return content
	}
	block := map[string]any{"type": "thinking", "thinking": thinkingText}
	if signature := strings.TrimSpace(carrierPart.Signature); signature != "" {
		block["signature"] = signature
	}
	if data := strings.TrimSpace(carrierPart.Data); data != "" {
		block["data"] = data
	}
	blocks, ok := content.([]map[string]any)
	if ok {
		return append([]map[string]any{block}, blocks...)
	}
	// content is a plain string (正文为空时返回 "")
	// 当正文为空时，thinking 块作为唯一内容块，不额外加 text 块
	if strings.TrimSpace(message.Content) == "" {
		return []map[string]any{block}
	}
	return []map[string]any{block, map[string]any{"type": "text", "text": message.Content}}
}

func anthropicAssistantToolContent(message types.PromptMessage, toolParts []types.MessagePart) ([]map[string]any, error) {
	content := []map[string]any{}
	if strings.TrimSpace(message.Content) != "" {
		content = append(content, map[string]any{"type": "text", "text": message.Content})
	}
	for _, part := range toolParts {
		content = append(content, map[string]any{"type": "tool_use", "id": part.CallID, "name": part.ToolName, "input": part.Input})
	}
	if len(content) == 0 {
		return nil, providerInvalid("assistant tool message has no content", nil)
	}
	return content, nil
}

func anthropicToolResultBlocks(toolParts []types.MessagePart) []map[string]any {
	blocks := []map[string]any{}
	for _, part := range toolParts {
		if part.Result == nil {
			continue
		}
		block := map[string]any{"type": "tool_result", "tool_use_id": part.CallID, "content": toolResultText(part)}
		if part.Result.Status != types.ToolStatusSuccess {
			block["is_error"] = true
		}
		blocks = append(blocks, block)
	}
	return blocks
}

func anthropicMessageContent(message types.PromptMessage) (any, error) {
	if len(message.Images) == 0 {
		return message.Content, nil
	}
	parts := []map[string]any{}
	if strings.TrimSpace(message.Content) != "" {
		parts = append(parts, map[string]any{"type": "text", "text": message.Content})
	}
	for _, image := range message.Images {
		parsed, err := parsePromptImageDataURL(image.DataURL)
		if err != nil {
			return nil, err
		}
		parts = append(parts, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": parsed.MediaType, "data": parsed.Base64}})
	}
	if len(parts) == 0 {
		return message.Content, nil
	}
	return parts, nil
}

func anthropicTools(tools []types.ToolDefinition) []map[string]any {
	converted := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		converted = append(converted, map[string]any{
			"name":         tool.Name,
			"description":  modelToolDescription(tool),
			"input_schema": toolSchema(tool.InputSchema),
		})
	}
	return converted
}
