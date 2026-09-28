package toolcalling

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"eucli-box/pkg/datapaths"
	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

// toolRequestKindMigration 是宿主以数据迁移模式唤起工具的请求种类。
const toolRequestKindMigration = "migration"

// toolDataDirectory 返回工具在数据区的长期数据目录；
// ID 安全校验与数据布局分别复用工具 ID 规则与数据区路径事实源。
func (s *system) toolDataDirectory(toolID string) (string, error) {
	toolID, err := cleanToolID(toolID)
	if err != nil {
		return "", err
	}
	return filepath.Join(datapaths.ToolDataDir(s.config.DataRoot), toolID), nil
}

// migrateToolData 是工具装载前的数据版本关卡：以专门的迁移模式唤起新版本工具，
// 让它对工具自己的数据完成检查与逐级迁移；只有数据未变化或迁移成功才允许继续激活。
// 它不切换版本、不写工具程序事实，只回报迁移是否通过。
func (s *system) migrateToolData(ctx context.Context, toolID string, prepared release.PreparedProgram, dataDir string) error {
	definition, executable, err := s.preparedToolExecutable(prepared)
	if err != nil {
		return err
	}
	input, err := json.Marshal(types.ToolExecutionInput{
		ActionID:             "data-migration",
		ToolName:             definition.ID,
		Arguments:            map[string]any{},
		DefaultConfig:        definition.DefaultConfig,
		ToolBodyDirectory:    prepared.Directory,
		ToolDataDirectory:    dataDir,
		HostWorkingDirectory: dataDir,
		RequestKind:          toolRequestKindMigration,
	})
	if err != nil {
		return toolExecutionInvalid("failed to encode data migration input", err)
	}
	outcome := s.executeToolProcess(ctx, definition.ID, executable, prepared.Directory, input, nil, nil)
	if outcome.FailureKind != "" {
		message := "tool data migration failed: " + outcome.FailureKind
		if outcome.FailureError != nil {
			message += ": " + outcome.FailureError.Error()
		}
		return toolExecutionInvalid(message, outcome.FailureError)
	}
	if outcome.ExitError != nil {
		message := outcome.ExitError.Error()
		if stderr := strings.TrimSpace(string(outcome.Stderr)); stderr != "" {
			message += ": " + stderr
		}
		return toolExecutionInvalid("tool data migration failed: "+message, outcome.ExitError)
	}
	if outcome.FailureError != nil {
		return toolExecutionInvalid("tool data migration failed: "+outcome.FailureError.Error(), outcome.FailureError)
	}
	var output types.ToolExecutionOutput
	if err := json.Unmarshal(bytes.TrimSpace(outcome.Stdout), &output); err != nil {
		return toolExecutionInvalid("tool data migration output is not valid json", err)
	}
	if output.Status != types.ToolStatusSuccess {
		message := strings.TrimSpace(output.Error)
		if message == "" {
			message = strings.TrimSpace(output.Content)
		}
		if message == "" {
			message = "status " + string(output.Status)
		}
		return toolExecutionInvalid("tool data migration failed: "+message, nil)
	}
	state, _ := output.Metadata["migrationState"].(string)
	switch state {
	case types.DataMigrationStateDataUnchanged, types.DataMigrationStateMigrated:
		return nil
	case types.DataMigrationStateRecovered:
		return toolExecutionInvalid("tool data migration did not complete; data was restored to its previous version", nil)
	default:
		return toolExecutionInvalid("tool data migration returned an unknown state: "+state, nil)
	}
}
