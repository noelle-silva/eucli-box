# cf-adapter（eucli-code-factory 契约 3 适配器）

为 `eucli-box` 实现 `eucli-code-factory` 契约 3（`.dev-tools/adapter.toml`）要求的 7 条命令：

- `deps`：准备共享区 Go 模块缓存与分支依赖目录，并在主模块与 `.dev-tools` 执行 `go mod download`。
- `build`：正式构建 `eucli-box` 可执行文件至 `EUCLI_CF_BUILD_DIR`（`programs/` 自动跟随构建目录隔离，不污染源码树）。
- `start`：校验实例身份后缀与分配端口，在 `EUCLI_CF_DATA_ROOT` 准备隔离的 `service-profile.json`（端口固定为 `EUCLI_CF_PORT_BASE`），并通过 `EUCLI_BOX_DATA_DIR` 在后台拉起正式产物。
- `health`：使用当前实例 `service-profile.json` 中的访问钥匙探测 `GET /api/box/info`。
- `stop`：幂等终止当前实例进程树并等待端口释放。
- `seed`：本服务启动时自动完成初始化与数据迁移，无需外部预置基线数据，按契约返回 `not-applicable`。
- `test`：先对运行中正式实例执行关键网关接口联调校验，再执行业务端（`./pkg/... ./src/... ./cmd/...`）与 `.dev-tools`（`./...`）双模块自动化测试。
