package datamigration

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"eucli-box/pkg/release"
)

// Step 是一级迁移的登记事实：只声明从哪个数据版本迁到哪个、
// 改变哪些资料、怎样在开始前检查、怎样执行、怎样核对结果。
type Step struct {
	ID          string   // 唯一标识，例如 "1.0.0-to-1.1.0"
	FromVersion string   // 只接受自己明确认识的前一版本
	ToVersion   string   // 只产生明确的下一版本
	Scope       []string // 改变范围与恢复范围：相对数据目录的路径前缀
	Precheck    func(ctx context.Context, dataDir string) error // 开始条件
	Apply       func(ctx context.Context, dataDir string) error // 执行改变
	Verify      func(ctx context.Context, dataDir string) error // 结果核对
}

var registry = map[string]Step{}

// Register 登记一级迁移步骤；登记表是包级唯一登记事实。
// 校验失败返回错误，不 panic。
func Register(step Step) error {
	step.ID = strings.TrimSpace(step.ID)
	if step.ID == "" {
		return migrationInvalid("migration step id is required", nil)
	}
	if _, exists := registry[step.ID]; exists {
		return migrationInvalid(fmt.Sprintf("migration step %s is already registered", step.ID), nil)
	}
	if err := release.ValidateVersion(step.FromVersion); err != nil {
		return migrationInvalid(fmt.Sprintf("migration step %s has invalid from version", step.ID), err)
	}
	if err := release.ValidateVersion(step.ToVersion); err != nil {
		return migrationInvalid(fmt.Sprintf("migration step %s has invalid to version", step.ID), err)
	}
	comparison, err := release.CompareVersions(step.FromVersion, step.ToVersion)
	if err != nil {
		return migrationInvalid(fmt.Sprintf("migration step %s has incomparable versions", step.ID), err)
	}
	if comparison >= 0 {
		return migrationInvalid(fmt.Sprintf("migration step %s must move forward", step.ID), nil)
	}
	normalized := make([]string, 0, len(step.Scope))
	for _, entry := range step.Scope {
		clean, err := normalizeScopeEntry(entry)
		if err != nil {
			return migrationInvalid(fmt.Sprintf("migration step %s has invalid scope", step.ID), err)
		}
		normalized = append(normalized, clean)
	}
	if len(normalized) == 0 {
		return migrationInvalid(fmt.Sprintf("migration step %s must declare a change scope", step.ID), nil)
	}
	if step.Precheck == nil || step.Apply == nil || step.Verify == nil {
		return migrationInvalid(fmt.Sprintf("migration step %s must declare precheck, apply and verify", step.ID), nil)
	}
	registry[step.ID] = Step{
		ID:          step.ID,
		FromVersion: step.FromVersion,
		ToVersion:   step.ToVersion,
		Scope:       normalized,
		Precheck:    step.Precheck,
		Apply:       step.Apply,
		Verify:      step.Verify,
	}
	return nil
}

// registeredSteps 返回已登记步骤的副本，按 FromVersion 升序排列。
func registeredSteps() []Step {
	steps := make([]Step, 0, len(registry))
	for _, step := range registry {
		steps = append(steps, step)
	}
	sort.Slice(steps, func(i int, j int) bool {
		comparison, err := release.CompareVersions(steps[i].FromVersion, steps[j].FromVersion)
		if err != nil {
			return steps[i].FromVersion < steps[j].FromVersion
		}
		return comparison < 0
	})
	return steps
}

// normalizeScopeEntry 把范围条目规范化为以 / 分隔的干净相对路径前缀。
func normalizeScopeEntry(entry string) (string, error) {
	clean, err := validateRelativePath(entry)
	if err != nil {
		return "", err
	}
	return clean, nil
}

// validateRelativePath 校验相对路径干净、不含父级段，并规范化为以 / 分隔。
func validateRelativePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(value) || filepath.IsAbs(filepath.FromSlash(value)) {
		return "", fmt.Errorf("path must be relative: %s", value)
	}
	normalized := strings.ReplaceAll(value, "\\", "/")
	if normalized != strings.Trim(normalized, "/") {
		return "", fmt.Errorf("path must not start or end with a separator: %s", value)
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == "." || part == ".." || part == "" {
			return "", fmt.Errorf("path must not contain parent or empty segments: %s", value)
		}
	}
	return normalized, nil
}
