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

// emptyRequestBody 固定为 {}；前端不能传入 URL、tag、manifest、archive、版本或路径。
type emptyRequestBody struct{}

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
	if strings.TrimSpace(request.GroupID) != "" && strings.TrimSpace(request.WorkspaceID) != "" {
		return gatewayInvalid("groupId cannot be combined with workspaceId", nil)
	}
	hasAttachments := len(request.Attachments) > 0
	hasMessage := strings.TrimSpace(request.Message) != "" || hasAttachments
	hasUserMessageID := strings.TrimSpace(request.UserMessageID) != ""
	hasContextMessageID := strings.TrimSpace(request.ContextMessageID) != ""
	if runInputCount(hasMessage, hasUserMessageID, hasContextMessageID) != 1 {
		return gatewayInvalid("exactly one of message, userMessageId, or contextMessageId is required", nil)
	}
	if err := validateRunSlashCommand(request); err != nil {
		return err
	}
	if (hasUserMessageID || hasContextMessageID) && strings.TrimSpace(request.ParentMessageID) != "" {
		return gatewayInvalid("parentMessageId cannot be combined with userMessageId or contextMessageId", nil)
	}
	if (hasUserMessageID || hasContextMessageID) && hasAttachments {
		return gatewayInvalid("attachments cannot be combined with userMessageId or contextMessageId", nil)
	}
	if (hasUserMessageID || hasContextMessageID) && strings.TrimSpace(request.SessionID) == "" {
		return gatewayInvalid("sessionId is required when userMessageId or contextMessageId is provided", nil)
	}
	if strings.TrimSpace(request.ParentMessageID) != "" && strings.TrimSpace(request.SessionID) == "" {
		return gatewayInvalid("sessionId is required when parentMessageId is provided", nil)
	}
	if effort := types.TrimReasoningEffort(request.ReasoningEffort); effort != "" && !types.IsReasoningEffort(effort) {
		return gatewayInvalid("reasoningEffort is invalid", nil)
	}
	if override := modelOverrideFromRunRequest(request); hasModelOverrideInput(override) {
		if _, ok := types.NormalizeModelOverrideCoordinate(override); !ok {
			return gatewayInvalid("modelOverride is invalid", nil)
		}
	}
	return nil
}

func validateRunSlashCommand(request types.RunRequest) error {
	command, hasCommand, err := parseRunSlashCommand(request.Message)
	if err != nil {
		return gatewayInvalid(err.Error(), err)
	}
	if !hasCommand {
		return nil
	}
	if command != "/compact" {
		return gatewayInvalid("unknown slash command", nil)
	}
	if len(request.Attachments) > 0 {
		return gatewayInvalid("/compact does not accept attachments", nil)
	}
	return nil
}

func parseRunSlashCommand(message string) (string, bool, error) {
	text := strings.TrimLeft(message, " \t\r\n")
	if !strings.HasPrefix(text, "/") {
		return "", false, nil
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", false, nil
	}
	if len(fields) > 1 {
		return fields[0], true, gatewayInvalid("slash command arguments are not supported", nil)
	}
	return fields[0], true, nil
}

func modelOverrideFromRunRequest(request types.RunRequest) types.ModelCoordinate {
	if request.ModelOverride == nil {
		return types.ModelCoordinate{}
	}
	return *request.ModelOverride
}

func hasModelOverrideInput(coordinate types.ModelCoordinate) bool {
	return strings.TrimSpace(coordinate.Kind) != "" || strings.TrimSpace(coordinate.ProviderID) != "" || strings.TrimSpace(coordinate.GroupID) != "" || strings.TrimSpace(coordinate.ModelID) != ""
}

func runInputCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
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

func validateWorkspace(workspace types.Workspace) error {
	if strings.TrimSpace(workspace.ID) == "" {
		return gatewayInvalid("workspace id is required", nil)
	}
	if strings.TrimSpace(workspace.Name) == "" {
		return gatewayInvalid("workspace name is required", nil)
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

func validateGroupSession(routeGroupID string, session types.Session) error {
	if strings.TrimSpace(routeGroupID) == "" {
		return gatewayInvalid("groupID is required", nil)
	}
	if strings.TrimSpace(session.GroupID) == "" {
		return gatewayInvalid("session groupId is required", nil)
	}
	if strings.TrimSpace(session.GroupID) != strings.TrimSpace(routeGroupID) {
		return gatewayInvalid("session groupId does not match route groupID", nil)
	}
	if strings.TrimSpace(session.RoleID) != "" {
		return gatewayInvalid("group session roleId must be empty", nil)
	}
	if strings.TrimSpace(session.ID) == "" {
		return gatewayInvalid("session id is required", nil)
	}
	return nil
}

func validateWorkspaceSession(routeWorkspaceID string, session types.Session) error {
	if strings.TrimSpace(routeWorkspaceID) == "" {
		return gatewayInvalid("workspaceID is required", nil)
	}
	if strings.TrimSpace(session.WorkspaceID) == "" {
		return gatewayInvalid("session workspaceId is required", nil)
	}
	if strings.TrimSpace(session.WorkspaceID) != strings.TrimSpace(routeWorkspaceID) {
		return gatewayInvalid("session workspaceId does not match route workspaceID", nil)
	}
	if strings.TrimSpace(session.RoleID) == "" {
		return gatewayInvalid("workspace session roleId is required", nil)
	}
	if strings.TrimSpace(session.GroupID) != "" {
		return gatewayInvalid("workspace session groupId must be empty", nil)
	}
	if strings.TrimSpace(session.ID) == "" {
		return gatewayInvalid("session id is required", nil)
	}
	return nil
}
