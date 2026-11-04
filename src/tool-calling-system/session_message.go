package toolcalling

import (
	"context"
	"encoding/json"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
)

// sendSessionMessage 把工具的消息意图交给宿主实现在日常通道上发送：
// 发送、等待角色停下、取回角色最新消息都在实现方内部完成，
// 能力层只做转发与回传，不复制任何发送逻辑。
func (c *capabilitySession) sendSessionMessage(ctx context.Context, payload json.RawMessage) toolcontrol.CapabilityResult {
	var request types.ToolMessageRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return capabilityFailed("会话消息请求格式无效")
	}
	if c.messages == nil {
		return capabilityFailed("宿主未接入会话消息能力")
	}
	result, err := c.messages.DeliverToolMessage(ctx, request)
	if err != nil {
		return capabilityFailed(err.Error())
	}
	return capabilitySuccess(result)
}
