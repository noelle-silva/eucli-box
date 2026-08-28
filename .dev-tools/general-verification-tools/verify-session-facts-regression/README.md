# verify-session-facts-regression

长期验证客户端会话事实转换、设置动作请求和相关业务端路由。该工具属于长期回归工具，持续覆盖任务 040 形成的会话事实边界，不随任务 040 归档。

入口：`verify-session-facts-regression.cmd`

本工具通过统一入口调用 `.dev-tools/common/toolkit`，执行客户端永久测试、客户端类型检查、网关/存储测试和客户端后台投影测试。工具进程和子命令以运行现场为工作目录，测试目标通过显式仓库路径传入；运行目录参数必须是 `.dev-workspace/.dev-tools-runtime/verify-session-facts-regression/run-*` 下的普通目录。

每次运行的命令证据与 `report.json` 写入 `.dev-workspace/.dev-tools-runtime/verify-session-facts-regression/run-*/evidence/`。任一检查失败都会写入失败项并以非 0 退出；通过时由统一入口收尾并清理临时运行资料。
