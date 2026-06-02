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
	NewCompleteStreamParser(onEvent types.ModelStreamHandler) completeStreamParser
}

type completeStreamParser interface {
	Accept(data []byte) error
	Finish(response types.HTTPResponse) (types.ModelResponse, error)
}

type sseEvent struct {
	Event string
	Data  string
}

type sseParser struct {
	buffer    string
	eventName string
	dataLines []string
	onEvent   func(event sseEvent) error
}

func newSSEParser(onEvent func(event sseEvent) error) *sseParser {
	return &sseParser{onEvent: onEvent}
}

func (p *sseParser) Accept(data []byte) error {
	p.buffer += string(data)
	for {
		index := strings.IndexByte(p.buffer, '\n')
		if index < 0 {
			return nil
		}
		line := strings.TrimSuffix(p.buffer[:index], "\r")
		p.buffer = p.buffer[index+1:]
		if err := p.acceptLine(line); err != nil {
			return err
		}
	}
}

func (p *sseParser) Finish() error {
	if strings.TrimSpace(p.buffer) != "" {
		if err := p.acceptLine(strings.TrimSuffix(p.buffer, "\r")); err != nil {
			return err
		}
	}
	p.buffer = ""
	return p.dispatch()
}

func (p *sseParser) acceptLine(line string) error {
	if line == "" {
		return p.dispatch()
	}
	if strings.HasPrefix(line, ":") {
		return nil
	}
	name, value, ok := strings.Cut(line, ":")
	if !ok {
		return nil
	}
	value = strings.TrimPrefix(value, " ")
	switch name {
	case "event":
		p.eventName = value
	case "data":
		p.dataLines = append(p.dataLines, value)
	}
	return nil
}

func (p *sseParser) dispatch() error {
	if len(p.dataLines) == 0 {
		p.eventName = ""
		return nil
	}
	event := sseEvent{Event: p.eventName, Data: strings.Join(p.dataLines, "\n")}
	p.eventName = ""
	p.dataLines = nil
	if p.onEvent == nil {
		return nil
	}
	return p.onEvent(event)
}

func sortInts(values []int) {
	for i := 1; i < len(values); i++ {
		value := values[i]
		j := i - 1
		for j >= 0 && values[j] > value {
			values[j+1] = values[j]
			j--
		}
		values[j+1] = value
	}
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
