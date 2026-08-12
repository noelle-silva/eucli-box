# general-verification-tools

长期验证工具区：不随任务的验证工具统一住在这里，长期维护。

每个工具一个子文件夹，遵守《开发仓库资产定位规范》与《验证工具体系规范》：

- 源码：本目录下 `<工具>/`
- 本体：`.dev-workspace/.dev-tools-runtime/`（三段式版本号，按版本分子目录）
- 开工：工作目录指向主仓库（运行时指向）
- 产物：`.dev-workspace/.dev-tools-runtime/`
- 共享件：`.dev-tools/common/`

当前为空，等待长期验证工具进入。
