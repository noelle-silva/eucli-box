// Package sessionimage 是 session_image 工具的业务动作实现：
// 单一职责——把会话中既有的图片按 ID 引用进本次回复。
package sessionimage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"eucli-box/tools/session_image/internal/toolcontrol"
	"eucli-box/tools/session_image/internal/types"
)

// SessionService 是工具访问宿主会话能力的窄接口：由控制通道客户端实现；
// 独立运行（无宿主控制通道）时为 nil，会话相关动作明确失败。
type SessionService interface {
	Request(ctx context.Context, capability string, access string, payload any) (toolcontrol.CapabilityResult, error)
}

// Execute 执行一次 session_image 工具动作。
func Execute(ctx context.Context, input types.ToolExecutionInput, session SessionService) types.ToolExecutionOutput {
	if err := ctx.Err(); err != nil {
		return failure("tool execution cancelled", err, nil)
	}
	imageID := strings.TrimSpace(stringValue(input.Arguments["imageId"]))
	if imageID == "" {
		return failure("parse session_image request", errors.New("imageId is required"), nil)
	}
	if err := validateAttachmentID(imageID); err != nil {
		return failure("parse session_image request", err, nil)
	}
	service, err := sessionServiceOrFail(session)
	if err != nil {
		return failure("parse session_image request", err, nil)
	}
	result, err := service.Request(ctx, types.ToolCapabilitySessionAttachments, types.ToolCapabilityAccessReference, types.SessionAttachmentReferenceRequest{AttachmentID: imageID})
	if err != nil {
		return failure("引用会话图片失败", err, nil)
	}
	if err := capabilityFailure(result); err != nil {
		return failure("引用会话图片失败", err, nil)
	}
	payload, err := json.Marshal(result.Payload)
	if err != nil {
		return failure("会话图片引用响应无效", err, nil)
	}
	var info types.SessionAttachmentInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return failure("会话图片引用响应无效", err, nil)
	}
	name := strings.TrimSpace(info.Name)
	if name == "" {
		name = "图片"
	}
	content := fmt.Sprintf("已把会话图片 %s（id: %s）引用到本次回复中。", name, info.ID)
	return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: content, Metadata: map[string]any{"imageId": info.ID, "name": name}}
}

// attachmentIDPattern 是会话附件 ID 的标准形态：att-<时间戳>-<序号>。
var attachmentIDPattern = regexp.MustCompile(`^att-\d+-\d+$`)

// validateAttachmentID 在本地校验附件 ID 形态：把「格式不合法」与
// 「不存在」区分开，避免排查时朝错误方向走。
func validateAttachmentID(imageID string) error {
	if attachmentIDPattern.MatchString(imageID) {
		return nil
	}
	if imageID != strings.ToLower(imageID) {
		return fmt.Errorf("图片 ID 格式不合法: %s（ID 区分大小写，请使用清单中的原始小写 ID）", imageID)
	}
	return fmt.Errorf("图片 ID 格式不合法: %s（正确形态形如 att-1790722702891563400-6352，请核对是否完整）", imageID)
}

// sessionServiceOrFail 取会话能力服务；独立运行（无宿主控制通道）时明确失败。
func sessionServiceOrFail(session SessionService) (SessionService, error) {
	if session == nil {
		return nil, errors.New("当前执行不在宿主会话中，无法引用会话图片")
	}
	return session, nil
}

// capabilityFailure 把能力响应中的拒绝/失败状态翻译为错误；
// 未授权时透传宿主的标准拒绝文案。
func capabilityFailure(result toolcontrol.CapabilityResult) error {
	switch result.Status {
	case toolcontrol.CapabilityStatusSuccess:
		return nil
	case toolcontrol.CapabilityStatusDenied:
		if strings.TrimSpace(result.Error) != "" {
			return errors.New(result.Error)
		}
		return errors.New(toolcontrol.CapabilityDeniedMessage)
	default:
		if strings.TrimSpace(result.Error) != "" {
			return errors.New(result.Error)
		}
		return errors.New("宿主能力请求失败")
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func failure(scope string, err error, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	message := scope
	if err != nil {
		message = scope + ": " + err.Error()
	}
	metadata["error"] = message
	return types.ToolExecutionOutput{
		Status:   types.ToolStatusFailed,
		Content:  message,
		Error:    message,
		Metadata: metadata,
	}
}
