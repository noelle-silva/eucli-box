# .dev-tools

开发工具区：所有长期开发工具的统一位置，与产品源码分离。

- 由主仓库 Git 统一管理；运行产物（本体、临时、缓存）由 `.gitignore` 筛选，不进入 Git 记录。
- 一个工具一个文件夹（入口、源码、测试、文档聚合一家），各语言结构在工具文件夹内自由展开。
- 工具只分两类：随任务的（随任务归档）与不随任务的（长期，统一住这里）。
- `.dev-tools/general-verification-tools/` 是长期验证工具专区。
- `.dev-tools/common/` 是工具族共享区：私有辅助代码与共用规则模块住这里。
- 与产品共享的公共约定为单一事实源，只存在主仓库一份，不复制。
- 结构遵守《开发哲学：四问定位法》与《开发仓库资产定位规范》。

## 已入住工具

- `build/`：AI 工具的本地开发构建入口（见下节）。
- `eucli-toolpack/`：把工具打包为成品（`-tool <id|all>`）。
- `eucli-store-sync/`：一键铺货——构建成品并以货架形态直接上架本地开发货架。
- `eucli-tool-scaffold/`：从基础模板生成新 AI 工具（入口 `create-tool.cmd`）。
- `eucli-version/`：发布物三段正式版本的检查与调整。
- `eucli-release/`：正式发布动作（`build` / `publish` / `remote` / `list` / `delist`，入口 `release.cmd`）。
- `eucli-release-assets/`：发布所需随包资产的准备。
- `dev-box/`：开发盒子（当前源码业务端编译、普通模式启动与连接信息输出；入口 `run-dev-box.cmd`）。
- `worktree-overlay/`：worktree 覆盖工具。
- `common/`：工具族共享区（`toolkit`、`toolruntime`、`release*` 等公共模块）。
- `general-verification-tools/`：长期验证工具专区，当前入住 7 个：
  - `verify-background-access/`：后台访问验证
  - `verify-command-execution-limit-protection/`：命令执行时限保护验证
  - `verify-data-migration/`：数据迁移验证
  - `verify-dev-box/`：开发盒子验证
  - `verify-release-build/`：发布成品制作验证
  - `verify-release-publish/`：GitHub 发布链验证
  - `verify-tool-plugin-update/`：工具与插件安装更新验证

## 工具开发构建入口

在任意 AI 工具目录里运行：

```
go run devtools/build
```

即把当前目录对应的工具构建为成品，并按四段开发版本铺入本地开发货架。工具身份来自它自己的 `tool.json`，入口不向工具目录写入任何文件。额外参数会原样转交给底层铺货命令，例如：

```
go run devtools/build -version 0.2.0.9
```

说明：`devtools` 是 `.dev-tools/` 的 Go 模块名，由仓库根 `go.work` 纳入工作区，因此在任意工具目录内都能解析到本入口，无需在仓库根存在同名实体目录。底层铺货命令是 `eucli-store-sync`；需要更细的落点控制（工作区、输出根等）时可直接调用它。
