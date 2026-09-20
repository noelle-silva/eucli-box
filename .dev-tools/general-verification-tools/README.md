# general-verification-tools

长期验证工具区：不随任务的验证工具统一住在这里，长期维护。

每个工具一个子文件夹，遵守《开发仓库资产定位规范》与《验证工具体系规范》：

- 源码：本目录下 `<工具>/`
- 本体：`.dev-workspace/.dev-tools-runtime/`（三段式版本号，按版本分子目录）
- 开工：工具进程与子命令工作目录指向各自运行现场，访问主仓库使用显式路径
- 产物：`.dev-workspace/.dev-tools-runtime/`
- 共享件：`.dev-tools/common/`

当前工具：

- `verify-background-access`：后台运行与业务端访问设置验证。
- `verify-command-execution-limit-protection`：命令执行时限保护验证。
- `verify-data-migration`：数据迁移验证。
- `verify-dev-box`：开发态业务端链路验证。
- `verify-release-build`：正式成品构建验证。
- `verify-release-publish`：发布预检与远端成品复核。
- `verify-tool-plugin-update`：工具与插件首次安装和手动更新验证。
