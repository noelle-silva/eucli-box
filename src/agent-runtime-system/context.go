package agentruntime

import (
	"context"
	"errors"
	"log"
	"strings"

	"eucli-box/pkg/itemaggregate"
	"eucli-box/pkg/types"
)

func (s *system) buildRoleContext(ctx context.Context, roleID string, session types.Session) (types.RoleContext, error) {
	tools, err := s.availableTools(ctx, roleID)
	if err != nil {
		return types.RoleContext{}, err
	}
	roleContext, err := s.roles.BuildContext(ctx, roleID, session, tools)
	if err != nil {
		return types.RoleContext{}, runtimeRoleFailed("failed to build role context", err)
	}
	return roleContext, nil
}

// availableTools 是「逐项容错聚合」机制在可用工具清单加载上的实例：
// 单个工具加载失败只降级为该项不可用，聚合其余项照常提供，运行推进不因此失败。
func (s *system) availableTools(ctx context.Context, roleID string) ([]types.ToolDefinition, error) {
	policy, err := s.roles.GetToolPolicy(ctx, roleID)
	if err != nil {
		return nil, runtimeRoleFailed("failed to read role tool policy", err)
	}
	summaries, err := s.tools.ListTools(ctx)
	if err != nil {
		return nil, runtimeToolFailed("failed to list tools", err)
	}
	filter := make(map[string]struct{}, len(policy.Tools))
	for _, tool := range policy.Tools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			continue
		}
		filter[tool] = struct{}{}
	}
	candidates := make([]types.ToolSummary, 0, len(summaries))
	items := make([]itemaggregate.Item, 0, len(summaries))
	for _, summary := range summaries {
		if _, idOk := filter[summary.ID]; !idOk {
			if _, nameOk := filter[summary.Name]; !nameOk {
				continue
			}
		}
		candidates = append(candidates, summary)
		items = append(items, itemaggregate.Item{ID: summary.ID, Name: summary.Name})
	}
	result := itemaggregate.Aggregate(ctx, items, s.toolLoader(candidates))
	for _, outcome := range result.Unavailable() {
		log.Printf("agent-runtime-system: 可用工具加载跳过 %s：%s", outcome.Item.ID, outcome.Reason)
	}
	return result.Available(), nil
}

// toolLoader 把候选摘要绑定为逐项加载处理器：复用下层已有的「不可用」标记，
// 加载失败被聚合隔离为该项不可用，保留在结果中供观测而不丢弃。
func (s *system) toolLoader(candidates []types.ToolSummary) itemaggregate.Processor[types.ToolDefinition] {
	summaries := make(map[string]types.ToolSummary, len(candidates))
	for _, summary := range candidates {
		summaries[summary.ID] = summary
	}
	return func(ctx context.Context, item itemaggregate.Item) (types.ToolDefinition, error) {
		summary := summaries[item.ID]
		if summary.Status == types.ToolAvailabilityUnavailable {
			return types.ToolDefinition{}, errors.New(unavailableToolReason(summary.StatusMessage))
		}
		tool, err := s.tools.LoadTool(ctx, summary.ID)
		if err != nil {
			return types.ToolDefinition{}, err
		}
		if tool.Status == types.ToolAvailabilityUnavailable {
			return types.ToolDefinition{}, errors.New(unavailableToolReason(tool.StatusMessage))
		}
		return tool, nil
	}
}

func unavailableToolReason(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "工具当前不可用"
	}
	return message
}
