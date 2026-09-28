package systemplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"

	"eucli-box/pkg/types"
)

// pluginDataMigrationFlag 是宿主以数据迁移模式唤起插件时的启动参数。
const pluginDataMigrationFlag = "--data-migration"

// migratePluginData 是插件装载前的数据版本关卡：以专门的迁移模式单独唤起插件程序，
// 让它对插件自己的数据完成检查与逐级迁移；只有数据未变化或迁移成功才允许继续装载。
// 它不进入常驻服务、不握手，只回报迁移是否通过。
func (s *system) migratePluginData(executable string, directory string, dataDirectory string) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.migrationTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, pluginDataMigrationFlag, dataDirectory)
	cmd.Dir = directory
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := "system plugin data migration failed: " + err.Error()
		if stderrText := strings.TrimSpace(stderr.String()); stderrText != "" {
			message += ": " + stderrText
		}
		return pluginExecutionFailed(message, err)
	}
	var result struct {
		Status         string `json:"status"`
		MigrationState string `json:"migrationState"`
		Error          string `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		return pluginExecutionFailed("system plugin data migration output is not valid json", err)
	}
	if result.Status != "success" {
		message := strings.TrimSpace(result.Error)
		if message == "" {
			message = "status " + result.Status
		}
		return pluginExecutionFailed("system plugin data migration failed: "+message, nil)
	}
	switch result.MigrationState {
	case types.DataMigrationStateDataUnchanged, types.DataMigrationStateMigrated:
		return nil
	case types.DataMigrationStateRecovered:
		return pluginExecutionFailed("system plugin data migration did not complete; data was restored to its previous version", nil)
	default:
		return pluginExecutionFailed("system plugin data migration returned an unknown state: "+result.MigrationState, nil)
	}
}
