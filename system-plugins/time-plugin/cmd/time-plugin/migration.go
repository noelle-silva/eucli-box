package main

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"eucli-box/system-plugins/time-plugin/internal/datamigration"
)

// dataMigrationFlag 是宿主以数据迁移模式唤起插件时的启动参数。
const dataMigrationFlag = "--data-migration"

// dataVersion 是插件当前声明的目标数据版本。
const dataVersion = "1.0.0"

// migrationResult 是数据迁移模式向宿主回报的结果事实。
type migrationResult struct {
	Status         string `json:"status"`
	MigrationState string `json:"migrationState,omitempty"`
	From           string `json:"from,omitempty"`
	To             string `json:"to,omitempty"`
	Error          string `json:"error,omitempty"`
}

// isDataMigrationMode 判断插件是否以数据迁移模式启动。
func isDataMigrationMode(args []string) bool {
	return len(args) > 0 && args[0] == dataMigrationFlag
}

// runDataMigration 执行插件的数据迁移，向输出写一行结果 JSON 并返回进程退出码；
// 它不进入常驻服务，只在数据版本不配套时沿已登记步骤逐级迁移，结束即退出。
func runDataMigration(args []string, output io.Writer) int {
	if len(args) != 2 || args[0] != dataMigrationFlag || strings.TrimSpace(args[1]) == "" {
		return reportDataMigration(output, migrationResult{Status: "failed", Error: "data migration requires exactly one data directory argument"})
	}
	ctx := context.Background()
	session, err := datamigration.Prepare(ctx, args[1], dataVersion)
	if err != nil {
		return reportDataMigration(output, migrationResult{Status: "failed", Error: err.Error()})
	}
	if err := session.Run(ctx); err != nil {
		return reportDataMigration(output, migrationResult{Status: "failed", Error: err.Error()})
	}
	outcome := session.Outcome()
	if err := session.Complete(ctx); err != nil {
		return reportDataMigration(output, migrationResult{Status: "failed", Error: err.Error()})
	}
	return reportDataMigration(output, migrationResult{
		Status:         "success",
		MigrationState: string(outcome.State),
		From:           outcome.From,
		To:             outcome.To,
	})
}

// reportDataMigration 写出结果 JSON 并给出退出码：只有迁移流程正常结束才为 0。
func reportDataMigration(output io.Writer, result migrationResult) int {
	if err := json.NewEncoder(output).Encode(result); err != nil {
		return 2
	}
	if result.Status != "success" {
		return 1
	}
	return 0
}
