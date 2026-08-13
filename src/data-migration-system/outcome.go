package datamigration

// State 是数据迁移交给程序更换职责的四态结果。
type State string

const (
	StateDataUnchanged  State = "data-unchanged"
	StateMigrated       State = "migrated"
	StateRecovered      State = "recovered"
	StateRecoveryFailed State = "recovery-failed"
)

// Outcome 是一次启动的数据迁移结果。
type Outcome struct {
	State  State
	From   string // 迁移前数据版本；数据未改变时为当前版本
	To     string // 目标数据版本
	Detail string // 人类可读摘要（每级结果或失败原因）
}
