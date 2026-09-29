package aiimage

import (
	"errors"
	"strings"
)

const (
	actionGenerate      = "generate"
	actionSessionImages = "session_images"
	actionConfigList    = "config_list"
	actionConfigRead    = "config_read"
	actionConfigWrite   = "config_write"
	actionConfigDelete  = "config_delete"
	actionConfigEdit    = "config_edit"
)

// arguments 是一次工具调用的完整参数视图。
type arguments struct {
	Action          string
	Prompt          string
	Provider        string
	Model           string
	ReferenceImages []string
	TimeoutMs       int64
	File            string
	Content         *string
	Field           string
	Value           any
	HasValue        bool
	Remove          bool
}

// parseArguments 解析模型传入的参数；类型不对时明确失败。
func parseArguments(raw map[string]any) (arguments, error) {
	if raw == nil {
		raw = map[string]any{}
	}
	var args arguments
	args.Action = strings.TrimSpace(stringValue(raw["action"]))
	if args.Action == "" {
		return arguments{}, errors.New("action is required")
	}
	args.Prompt = strings.TrimSpace(stringValue(raw["prompt"]))
	args.Provider = strings.TrimSpace(stringValue(raw["provider"]))
	args.Model = strings.TrimSpace(stringValue(raw["model"]))
	args.File = strings.TrimSpace(stringValue(raw["file"]))
	args.Field = strings.TrimSpace(stringValue(raw["field"]))
	args.Remove = boolValue(raw["remove"])
	args.TimeoutMs = int64Value(raw["timeoutMs"])
	if value, ok := raw["content"]; ok {
		text, ok := value.(string)
		if !ok {
			return arguments{}, errors.New("content must be a string")
		}
		args.Content = &text
	}
	if value, ok := raw["value"]; ok {
		args.Value = value
		args.HasValue = true
	}
	if value, ok := raw["referenceImages"]; ok {
		list, err := stringListValue(value)
		if err != nil {
			return arguments{}, errors.New("referenceImages must be a list of strings")
		}
		args.ReferenceImages = list
	}
	return args, nil
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		return ""
	}
}

func boolValue(value any) bool {
	if typed, ok := value.(bool); ok {
		return typed
	}
	return false
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	default:
		return 0
	}
}

func stringListValue(value any) ([]string, error) {
	items, ok := value.([]any)
	if !ok {
		if list, ok := value.([]string); ok {
			return append([]string(nil), list...), nil
		}
		return nil, errors.New("value is not a list")
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, errors.New("list item is not a string")
		}
		out = append(out, text)
	}
	return out, nil
}
