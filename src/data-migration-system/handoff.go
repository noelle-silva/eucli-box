package datamigration

import (
	"context"
	"os"

	datastorage "eucli-box/src/data-storage-system"
)

// Handoff 是数据迁移职责交给程序更换职责的持久事实。
// 它只报告事实，不做恢复决定；决定权在程序更换职责。
type Handoff struct {
	StatusPresent      bool    // status.json 是否存在
	Outcome            Outcome // 最近一次写下的四态结果（StatusPresent 为 false 时为零值）
	Completed          bool    // status.json 的 completed 字段
	ProcessPending     bool    // process.json 是否存在；存在即有未完成迁移
	CurrentDataVersion string  // meta/version.json 当前值；文件不存在时为空串
}

// ReadHandoff 读取迁移工作区的持久事实，作为程序更换职责判断数据状态的唯一事实入口。
// 工作区位置复用 workspace.go 的既有路径函数，不重复拼路径。
func ReadHandoff(ctx context.Context, dataDir string) (Handoff, error) {
	w, err := newWorkspace(dataDir)
	if err != nil {
		return Handoff{}, migrationStatusUnknown("failed to resolve migration workspace", err)
	}
	var handoff Handoff
	_, err = os.Stat(w.processFile())
	switch {
	case err == nil:
		handoff.ProcessPending = true
	case os.IsNotExist(err):
		handoff.ProcessPending = false
	default:
		return Handoff{}, migrationStatusUnknown("failed to inspect migration process record", err)
	}
	record, present, err := readStatusRecord(w)
	if err != nil {
		return Handoff{}, err
	}
	if present {
		handoff.StatusPresent = true
		handoff.Outcome = Outcome{
			State:  State(record.Outcome),
			From:   record.FromVersion,
			To:     record.TargetVersion,
			Detail: record.Detail,
		}
		handoff.Completed = record.Completed
	}
	version, exists, err := datastorage.ReadStorageVersion(ctx, dataDir)
	if err != nil {
		return Handoff{}, migrationStatusUnknown("failed to read current data version", err)
	}
	if exists {
		handoff.CurrentDataVersion = version.Version
	}
	return handoff, nil
}
