package roleprompt

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) SaveRole(ctx context.Context, role types.Role) error {
	if err := validateRole(ctx, s.providers, role); err != nil {
		return err
	}
	// 描述是给人看的备注：只做首尾空白规范化，不参与校验，也不进提示词。
	role.Description = strings.TrimSpace(role.Description)
	role.HookPromptPresetID = strings.TrimSpace(role.HookPromptPresetID)
	now := time.Now().UTC()
	if role.CreatedAt.IsZero() {
		role.CreatedAt = now
	}
	role.UpdatedAt = now
	if err := s.storage.SaveRole(ctx, role); err != nil {
		return roleStorageFailed("failed to save role", err)
	}
	return nil
}

func (s *system) LoadRole(ctx context.Context, roleID string) (types.Role, error) {
	if strings.TrimSpace(roleID) == "" {
		return types.Role{}, roleInvalid("role id is required", nil)
	}
	role, err := s.storage.LoadRole(ctx, roleID)
	if err != nil {
		return types.Role{}, roleNotFound("failed to load role", err)
	}
	return role, nil
}

func (s *system) ListRoles(ctx context.Context) ([]types.RoleSummary, error) {
	roles, err := s.storage.ListRoles(ctx)
	if err != nil {
		return nil, roleStorageFailed("failed to list roles", err)
	}
	return roles, nil
}

func (s *system) ReorderRoles(ctx context.Context, roleIDs []string) error {
	if err := s.storage.ReorderRoles(ctx, roleIDs); err != nil {
		return roleStorageFailed("failed to reorder roles", err)
	}
	return nil
}

func (s *system) DeleteRole(ctx context.Context, roleID string) error {
	if strings.TrimSpace(roleID) == "" {
		return roleInvalid("role id is required", nil)
	}
	if err := s.storage.DeleteRole(ctx, roleID); err != nil {
		return roleStorageFailed("failed to delete role", err)
	}
	return nil
}

// validateRole 校验角色的完整定义：结构与本地约束，外加模型坐标可用性。
// 只在保存角色时使用：模型可用性必须在这里落定，运行期不再重复校验。
func validateRole(ctx context.Context, providers ProviderSystem, role types.Role) error {
	if err := validateRoleShape(role); err != nil {
		return err
	}
	return validateModelConfig(ctx, providers, role.ModelConfig)
}

// validateRoleShape 只校验角色自身的结构与本地约束：纯内存判断，不碰模型供应商，也没有任何副作用。
func validateRoleShape(role types.Role) error {
	if strings.TrimSpace(role.ID) == "" {
		return roleInvalid("role id is required", nil)
	}
	if strings.TrimSpace(role.Name) == "" {
		return roleInvalid("role name is required", nil)
	}
	if err := validatePrompts(role.Prompts); err != nil {
		return err
	}
	return validateToolPolicy(role.ToolPolicy)
}
