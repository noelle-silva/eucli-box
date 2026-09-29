package main

// 数据迁移步骤登记处。
//
// 数据版本升高时，在 init 中按级登记迁移步骤；每级步骤必须声明：
// 唯一标识、起始版本、目标版本、改变范围（相对数据目录的路径前缀）、
// 开始条件、执行动作与结果核对。历史步骤保留，不删不改。
//
// 模板当前没有数据变化，因此没有登记任何步骤。
// 需要登记时，参照下列结构，并同步升高 migration.go 中的目标数据版本：
//
//	func init() {
//		err := datamigration.Register(datamigration.Step{
//			ID:          "1.0.0-to-1.1.0",
//			FromVersion: "1.0.0",
//			ToVersion:   "1.1.0",
//			Scope:       []string{"state"},
//			Precheck:    precheck,
//			Apply:       apply,
//			Verify:      verify,
//		})
//		if err != nil {
//			panic(fmt.Sprintf("登记数据迁移步骤失败：%v", err))
//		}
//	}
