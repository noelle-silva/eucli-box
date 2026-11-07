package toolcalling

import (
	"context"
	"encoding/json"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
)

// relayToolConfirmation 把工具带来的外部待确认交给宿主摆上台面，并把裁决结论原样带回。
// 能力层只做转发：摆在哪里、怎么等人、结论怎么落地，都由宿主实现决定。
func (c *capabilitySession) relayToolConfirmation(ctx context.Context, payload json.RawMessage) toolcontrol.CapabilityResult {
	var fact types.ToolConfirmationFact
	if err := json.Unmarshal(payload, &fact); err != nil {
		return capabilityFailed("确认转呈请求格式无效")
	}
	if c.confirmations == nil {
		return capabilityFailed("宿主未接入确认转呈能力")
	}
	outcome, err := c.confirmations.RelayToolConfirmation(ctx, c.plan, fact)
	if err != nil {
		return capabilityFailed(err.Error())
	}
	return capabilitySuccess(outcome)
}
