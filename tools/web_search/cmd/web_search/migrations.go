package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"eucli-box/tools/web_search/internal/datamigration"
)

// init 登记本发布物的逐级数据迁移步骤：数据版本升高时在此追加新步骤，历史步骤保留。
func init() {
	for _, step := range []datamigration.Step{
		dataMigrationStep("1.0.0-to-1.1.0", "1.0.0", "1.1.0"),
		dataMigrationStep("1.1.0-to-1.2.0", "1.1.0", "1.2.0"),
	} {
		if err := datamigration.Register(step); err != nil {
			panic(fmt.Sprintf("登记数据迁移步骤失败：%v", err))
		}
	}
}

// dataMigrationStep 构造一级迁移：在 state/ 下写入本级迁移记录并核对。
func dataMigrationStep(id string, from string, to string) datamigration.Step {
	return datamigration.Step{
		ID:          id,
		FromVersion: from,
		ToVersion:   to,
		Scope:       []string{"state"},
		Precheck:    dataMigrationPrecheck,
		Apply: func(ctx context.Context, dataDir string) error {
			return dataMigrationApply(ctx, dataDir, from, to)
		},
		Verify: func(ctx context.Context, dataDir string) error {
			return dataMigrationVerify(ctx, dataDir, from, to)
		},
	}
}

// dataMigrationPrecheck 检查迁移开始条件：数据目录可用。
func dataMigrationPrecheck(ctx context.Context, dataDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(dataDir)
	if err != nil {
		return fmt.Errorf("数据目录不可用：%w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("数据目录不是目录：%s", dataDir)
	}
	return nil
}

// dataMigrationApply 在 state/ 下写入本级迁移记录。
func dataMigrationApply(ctx context.Context, dataDir string, from string, to string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stateDir := filepath.Join(dataDir, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	record := map[string]any{
		"fromVersion": from,
		"toVersion":   to,
		"appliedAt":   time.Now().UTC().Format(time.RFC3339Nano),
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stateDir, "migration-record.json"), append(payload, '\n'), 0o644)
}

// dataMigrationVerify 核对迁移结果：迁移记录存在且内容为本级迁移。
func dataMigrationVerify(ctx context.Context, dataDir string, from string, to string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload, err := os.ReadFile(filepath.Join(dataDir, "state", "migration-record.json"))
	if err != nil {
		return err
	}
	var record struct {
		FromVersion string `json:"fromVersion"`
		ToVersion   string `json:"toVersion"`
	}
	if err := json.Unmarshal(payload, &record); err != nil {
		return err
	}
	if record.FromVersion != from || record.ToVersion != to {
		return fmt.Errorf("迁移记录不符：%s -> %s", record.FromVersion, record.ToVersion)
	}
	return nil
}
