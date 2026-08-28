# general-verification-tools

长期验证工具区：不随任务的验证工具统一住在这里，长期维护。

每个工具一个子文件夹，遵守《开发仓库资产定位规范》与《验证工具体系规范》：

- 源码：本目录下 `<工具>/`
- 本体：`.dev-workspace/.dev-tools-runtime/`（三段式版本号，按版本分子目录）
- 开工：工具进程与子命令工作目录指向各自运行现场，访问主仓库使用显式路径
- 产物：`.dev-workspace/.dev-tools-runtime/`
- 共享件：`.dev-tools/common/`

当前工具：

- `verify-session-facts-regression`：客户端会话事实、设置动作与业务端路由回归。
