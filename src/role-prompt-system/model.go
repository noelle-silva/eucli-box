package roleprompt

import (
	"context"
	"strings"

	"eucli-box/pkg/types"
)

// validateModelConfig 校验角色模型配置：候选池非空、默认候选存在且唯一、
// 每个候选坐标都真实可解析。任何一项不成立即快速失败，不静默剔除。
func validateModelConfig(ctx context.Context, providers ProviderSystem, config types.ModelConfig) error {
	if config.Temperature < 0 || config.Temperature > 2 {
		return roleModelInvalid("temperature must be between 0 and 2", nil)
	}
	if len(config.Candidates) == 0 {
		return roleModelInvalid("role model candidates are required", nil)
	}
	seenIDs := make(map[string]struct{}, len(config.Candidates))
	seenKeys := make(map[string]struct{}, len(config.Candidates))
	for _, candidate := range config.Candidates {
		id := strings.TrimSpace(candidate.ID)
		if id == "" {
			return roleModelInvalid("role model candidate id is required", nil)
		}
		if _, exists := seenIDs[id]; exists {
			return roleModelInvalid("role model candidate id is duplicated", nil)
		}
		seenIDs[id] = struct{}{}
		key := types.ModelCandidateKey(candidate.Coordinate)
		if _, exists := seenKeys[key]; exists {
			return roleModelInvalid("role model candidate coordinate is duplicated", nil)
		}
		seenKeys[key] = struct{}{}
		if err := validateModelCandidateCoordinate(ctx, providers, candidate.Coordinate); err != nil {
			return err
		}
	}
	if _, ok := config.DefaultCandidate(); !ok {
		return roleModelInvalid("role default model candidate is required", nil)
	}
	return nil
}

// validateModelCandidateCoordinate 校验单个候选坐标的形态与可解析性。
func validateModelCandidateCoordinate(ctx context.Context, providers ProviderSystem, coordinate types.ModelCoordinate) error {
	if strings.TrimSpace(coordinate.Kind) == "model_group" || strings.TrimSpace(coordinate.GroupID) != "" {
		if strings.TrimSpace(coordinate.GroupID) == "" {
			return roleModelInvalid("model group id is required", nil)
		}
	} else if strings.TrimSpace(coordinate.ProviderID) == "" {
		return roleModelInvalid("provider id is required", nil)
	}
	if strings.TrimSpace(coordinate.ModelID) == "" {
		return roleModelInvalid("model id is required", nil)
	}
	if _, _, err := providers.ResolveModel(ctx, coordinate); err != nil {
		return roleModelInvalid("role model candidate coordinate is not available", err)
	}
	return nil
}
