package agentruntime

import (
	"context"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

func (s *system) handleToolIntent(ctx context.Context, record *runRecord, intent types.ToolIntent) (types.ToolResult, error) {
	results, err := s.handleToolIntents(ctx, record, []types.ToolIntent{intent})
	if err != nil {
		return types.ToolResult{}, err
	}
	if len(results) != 1 {
		return types.ToolResult{}, runtimeStateInvalid("tool result count mismatch", nil)
	}
	return results[0], nil
}

func newRuntimeID(prefix string) string {
	return utils.NewID(prefix)
}
