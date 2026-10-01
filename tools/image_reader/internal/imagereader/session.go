package imagereader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
)

// SessionService 是工具访问宿主会话能力的窄接口：由控制通道客户端实现；
// 独立运行（无宿主控制通道）时为 nil，写入动作明确失败。
type SessionService interface {
	Request(ctx context.Context, capability string, access string, payload any) (toolcontrol.CapabilityResult, error)
}

// writeSessionImage 经会话附件写入能力把图片挂回会话；落盘由宿主代办。
func writeSessionImage(ctx context.Context, session SessionService, name string, imageDataURL string) (types.SessionAttachmentInfo, error) {
	if session == nil {
		return types.SessionAttachmentInfo{}, errors.New("当前执行不在宿主会话中，无法把图片加载进会话")
	}
	result, err := session.Request(ctx, types.ToolCapabilitySessionAttachments, types.ToolCapabilityAccessWrite, types.SessionAttachmentWriteRequest{Name: name, DataURL: imageDataURL})
	if err != nil {
		return types.SessionAttachmentInfo{}, fmt.Errorf("写入会话图片失败: %w", err)
	}
	if err := capabilityFailure(result); err != nil {
		return types.SessionAttachmentInfo{}, err
	}
	payload, err := json.Marshal(result.Payload)
	if err != nil {
		return types.SessionAttachmentInfo{}, fmt.Errorf("会话图片写入响应无效: %w", err)
	}
	var info types.SessionAttachmentInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return types.SessionAttachmentInfo{}, fmt.Errorf("会话图片写入响应无效: %w", err)
	}
	return info, nil
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
