# everything

`everything` 是 e-b 工具调用系统使用的本机文件搜索工具。

它只做一件事：接收一次搜索请求（`query`，可选 `scopePath`），通过 Everything 应用的开放接口执行查询，并把结果整理为稳定的 Markdown 与 metadata。

## 与 Everything 应用的关系

本工具**不自带**任何 Everything 程序，不安装系统服务，不维护索引，也不管理实例。索引、全盘搜索与运行实例都由 Everything 应用负责。

工具通过「用户配置的访问地址 + 访问钥匙」向应用发起请求：

- 访问地址（`endpoint`）：Everything 应用开放接口的地址，例如 `http://127.0.0.1:43210`。
- 访问钥匙（`key`）：该地址对应的一把访问钥匙，随请求以 Bearer 方式携带。

连接信息在工具设置页的用户配置里填写；未配置、地址非法或钥匙缺失时，工具明确失败并指明修复位置。

## 通信协议

工具与应用之间使用本机 HTTP JSON-RPC：

- `POST <endpoint>/rpc`，请求头 `Authorization: Bearer <key>`，`Content-Type: application/json`。
- 请求体：`{"method": "everything.search", "params": {"query": "...", "limit": 80, "scopePath": "..."}}`；`limit` 与 `scopePath` 省略时不携带。
- 响应体：成功为 `{"ok": true, "result": {"query": "...", "limit": 80, "scopePath": "...", "results": [...]}}`；失败为 `{"ok": false, "error": {"message": "...", "code": "..."}}`，`code` 可机器识别。
- 每个结果含 `name`、`path`、`fullPath`、`kind`、`size`、`modifiedAt`。

无法连接时，工具返回明确指引：确认 Everything 应用正在运行，且访问地址与开放端口一致。

## 输入参数

- `query`（必填）：Everything 搜索查询，支持 Everything 查询语法。
- `scopePath`：可选，把搜索限定在一个已存在的文件夹内；相对路径基于主机工作目录解析为绝对路径。省略时由应用按自己的默认范围搜索。
- `maxResults`：可选，返回结果数量上限（正整数）。
- `timeoutMs`：可选，请求时限，单位毫秒。
- `maxOutputChars`：可选，本次调用最多返回的正文字符数，可在工具配置上限内下调。
- `description`：可选，简短说明这次搜索的原因（框架惯例，记入结果元数据）。

## 输出结果

返回 Markdown `content` 与结构化 metadata：

- `query`
- `scopePath`
- `maxResults`
- `timeoutMs`
- `maxOutputChars`
- `resultsCount`
- `durationMs`
- `truncated`
- `description`（提供时）
- `code`（失败且后端带错误码时）

判断是否成功时优先看工具状态；`status == success` 表示搜索完成。

## 边界

本工具只做一次本机文件搜索。它不打开文件、不读取正文、不修改文件、不管理索引、不维护通用系统服务，也不在本地产生运行缓存。
