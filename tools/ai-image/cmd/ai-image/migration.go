package main

import (
	"context"
	"fmt"
	"strings"

	"eucli-box/tools/ai-image/internal/toolcontrol"
	"eucli-box/tools/ai-image/internal/types"
	"eucli-box/tools/ai-image/internal/datamigration"
)

// migrationRequestKind 是宿主以数据迁移模式唤起工具时的请求种类。
const migrationRequestKind = "migration"

// dataVersion 是工具当前声明的目标数据版本：数据发生变化时升高版本，
// 并在 migrations.go 中登记从旧版本到新版本的迁移步骤。
const dataVersion = "1.0.0"

// runDataMigration 以宿主提供的迁移请求执行工具的数据迁移并如实回报结果；
// 它不执行任何用户动作，只在数据版本不配套时沿已登记步骤逐级迁移。
func runDataMigration(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	dataDir := strings.TrimSpace(input.ToolDataDirectory)
	if dataDir == "" {
		return failedOutput("data migration requires a tool data directory", nil)
	}
	client, err := toolcontrol.AdoptControl(ctx)
	if err != nil {
		return toolcontrol.ControlFailedOutput(err)
	}
	if client != nil {
		defer client.Close()
		go func() { _ = client.Serve(ctx) }()
	}
	session, err := datamigration.Prepare(ctx, dataDir, dataVersion)
	if err != nil {
		return failedOutput("data migration prepare failed", err)
	}
	if err := session.Run(ctx); err != nil {
		return failedOutput("data migration run failed", err)
	}
	outcome := session.Outcome()
	if err := session.Complete(ctx); err != nil {
		return failedOutput("data migration complete failed", err)
	}
	return types.ToolExecutionOutput{
		Status:  types.ToolStatusSuccess,
		Content: fmt.Sprintf("data migration %s (%s -> %s)", outcome.State, outcome.From, outcome.To),
		Metadata: map[string]any{
			"migrationState": string(outcome.State),
			"from":           outcome.From,
			"to":             outcome.To,
		},
	}
}
