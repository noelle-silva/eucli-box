package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"eucli-box/pkg/types"
)

const maxRequestBodyBytes = 64 << 20

func decodeJSON[T any](r *http.Request) (T, error) {
	var value T
	payload, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes+1))
	if err != nil {
		return value, gatewayInvalid("failed to read request body", err)
	}
	if int64(len(payload)) > maxRequestBodyBytes {
		return value, gatewayInvalid("request body is too large", nil)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, gatewayInvalid("request body is invalid json", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return value, gatewayInvalid("request body must contain only one json value", err)
	}
	return value, nil
}

func pathValue(r *http.Request, key string) (string, error) {
	value := r.PathValue(key)
	if value == "" {
		return "", gatewayInvalid("path value is required", nil)
	}
	return value, nil
}

func validateRunRequest(request types.RunRequest) error {
	if strings.TrimSpace(request.RoleID) == "" {
		return gatewayInvalid("roleId is required", nil)
	}
	hasAttachments := len(request.Attachments) > 0
	hasMessage := strings.TrimSpace(request.Message) != "" || hasAttachments
	hasUserMessageID := strings.TrimSpace(request.UserMessageID) != ""
	if hasMessage == hasUserMessageID {
		return gatewayInvalid("exactly one of message or userMessageId is required", nil)
	}
	if hasUserMessageID && strings.TrimSpace(request.ParentMessageID) != "" {
		return gatewayInvalid("parentMessageId cannot be combined with userMessageId", nil)
	}
	if hasUserMessageID && hasAttachments {
		return gatewayInvalid("attachments cannot be combined with userMessageId", nil)
	}
	if hasUserMessageID && strings.TrimSpace(request.SessionID) == "" {
		return gatewayInvalid("sessionId is required when userMessageId is provided", nil)
	}
	if strings.TrimSpace(request.ParentMessageID) != "" && strings.TrimSpace(request.SessionID) == "" {
		return gatewayInvalid("sessionId is required when parentMessageId is provided", nil)
	}
	return nil
}

func validateRole(role types.Role) error {
	if strings.TrimSpace(role.ID) == "" {
		return gatewayInvalid("role id is required", nil)
	}
	if strings.TrimSpace(role.Name) == "" {
		return gatewayInvalid("role name is required", nil)
	}
	return nil
}

func validateProvider(provider types.Provider) error {
	if strings.TrimSpace(provider.ID) == "" {
		return gatewayInvalid("provider id is required", nil)
	}
	if strings.TrimSpace(provider.Name) == "" {
		return gatewayInvalid("provider name is required", nil)
	}
	if strings.TrimSpace(provider.BaseURL) == "" {
		return gatewayInvalid("provider baseUrl is required", nil)
	}
	if strings.TrimSpace(provider.Key) == "" {
		return gatewayInvalid("provider key is required", nil)
	}
	if provider.Protocol == "" {
		return gatewayInvalid("provider protocol is required", nil)
	}
	return nil
}

func validateTool(tool types.ToolDefinition) error {
	if strings.TrimSpace(tool.ID) == "" {
		return gatewayInvalid("tool id is required", nil)
	}
	if strings.TrimSpace(tool.Name) == "" {
		return gatewayInvalid("tool name is required", nil)
	}
	if strings.TrimSpace(tool.Description) == "" {
		return gatewayInvalid("tool description is required", nil)
	}
	return nil
}

func validateConfirmation(confirmation types.ToolConfirmation) error {
	if strings.TrimSpace(confirmation.DecisionID) == "" {
		return gatewayInvalid("confirmation decisionId is required", nil)
	}
	return nil
}

func validateSession(routeRoleID string, session types.Session) error {
	if strings.TrimSpace(routeRoleID) == "" {
		return gatewayInvalid("roleID is required", nil)
	}
	if strings.TrimSpace(session.RoleID) == "" {
		return gatewayInvalid("session roleId is required", nil)
	}
	if strings.TrimSpace(session.RoleID) != strings.TrimSpace(routeRoleID) {
		return gatewayInvalid("session roleId does not match route roleID", nil)
	}
	if strings.TrimSpace(session.ID) == "" {
		return gatewayInvalid("session id is required", nil)
	}
	return nil
}
