package roleprompt

import (
	"context"
	"strings"

	"eucli-box/pkg/types"
)

func validateModelConfig(ctx context.Context, providers ProviderSystem, config types.ModelConfig) error {
	coordinateKind := strings.TrimSpace(config.Coordinate.Kind)
	if coordinateKind == "model_group" || strings.TrimSpace(config.Coordinate.GroupID) != "" {
		if strings.TrimSpace(config.Coordinate.GroupID) == "" {
			return roleModelInvalid("model group id is required", nil)
		}
	} else if strings.TrimSpace(config.Coordinate.ProviderID) == "" {
		return roleModelInvalid("provider id is required", nil)
	}
	if strings.TrimSpace(config.Coordinate.ModelID) == "" {
		return roleModelInvalid("model id is required", nil)
	}
	if _, _, err := providers.ResolveModel(ctx, config.Coordinate); err != nil {
		return roleModelInvalid("role model coordinate is not available", err)
	}
	return nil
}
