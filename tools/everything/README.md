# everything

`everything` 是 e-b 工具调用系统使用的本机文件搜索工具。

它接收一次 Everything 搜索查询：默认全盘搜索使用工具包自带 Everything 运行环境和项目专属后台支撑；限定目录搜索使用工具包自带 Everything 运行环境准备专属范围文件地图。搜索结果会被整理为稳定的 Markdown 和 metadata。

索引维护交给 Everything 自身运行机制处理。这个工具只有搜索语义，不提供索引管理入口。

如果本机运行缓存文件缺失，工具会在搜索准备完成后要求 Everything 保存当前数据库到工具自己的运行目录。这只是运行缓存持久化，不是重建索引，也不是对外提供索引管理能力。

## 工具定位

`everything` 只负责一次本机文件搜索动作。

它适合用于快速查找本机文件、默认全盘搜索、限定目录搜索、按 Everything 查询语法过滤文件。

它不负责打开文件、不读取文件内容、不管理文件、不做多轮搜索编排、不管理索引；默认全盘搜索只会准备自身必需的项目专属后台支撑。

## 根系边界

这个工具只培育几个基础根动作：

- 读取工具输入。
- 读取运行时配置。
- 定位工具包自带的 Everything 运行环境。
- 准备工具专属的运行目录。
- 准备默认全盘搜索必需的项目专属后台支撑。
- 执行一次本机文件搜索。
- 将搜索结果归一化为可读结果。
- 输出 e-b 工具协议需要的 JSON 结果。

复杂的本机资料查找流程由搜索、命令执行、文件读取等基础工具组合涌现，不在这个工具里预设新的文件管理中心。

## 自带运行环境

工具运行包默认自带 voidtools Everything 运行程序和命令行搜索程序。

默认行为：

- 使用工具包内的 `Everything.exe` 和 `es.exe`。
- 默认全盘搜索时，使用工具专属全盘入口和项目专属 Windows 服务。
- 限定目录搜索时，使用工具专属目录入口。
- 本机运行缓存保存在工具自己的运行区，重新生成工具目录时不会误删已有缓存。
- 用户未指定搜索范围时，默认搜索本机可用盘符。
- 用户指定搜索范围时，只使用该范围建立工具专属文件地图。
- 默认全盘首次使用时，可能需要一次 Windows 服务授权；服务名随工具专属实例名确定。

运行缓存不是打包资源，不会进入仓库，也不是要把索引数据一起发布。保留运行缓存只是为了避免本机重新生成工具目录后从零开始建立文件地图。

当运行缓存文件被手动删除时，下一次搜索会先让 Everything 准备可用索引；如果缓存文件仍不存在，工具会保存当前数据库，让缓存重新落到 `data\tool-data\everything\runtime` 下。

只有当你明确要覆盖自带程序时，才配置工具用户设置 `esPath` 或环境变量 `EVERYTHING_ES_PATH`。

`esPath` 和 `EVERYTHING_ES_PATH` 支持绝对路径；如果只填命令名，则按系统路径查找。

## 输入参数

必填参数：

- `query`：Everything 搜索查询。

可选参数：

- `scopePath`：限定搜索的已存在文件夹。相对路径基于 e-b 主机工作目录；省略时默认搜索本机可用盘符。
- `instanceName`：Everything 实例名；留空表示工具专属实例。
- `maxResults`：返回结果数，上限 500。
- `timeoutMs`：搜索超时时间，单位毫秒；默认给整机搜索和限定目录首次准备预留更长时间。
- `maxOutputChars`：Markdown 输出最大字符数。
- `description`：简短说明这次搜索的原因。

## 输出结果

工具返回 Markdown `content` 和结构化 metadata：

- `query`
- `scopePath`
- `scopePaths`
- `scopeMode`
- `instanceName`
- `resultsCount`
- `durationMs`
- `maxResults`
- `timeoutMs`
- `maxOutputChars`
- `truncated`
- `provider`
- `executableSource`
- `runtimeSource`
- `serviceName`：默认全盘搜索时使用的项目专属服务名。

判断搜索是否成功时，优先看工具状态。

`status == success` 表示自带 Everything 搜索成功且结果已完成归一化。

metadata 只描述本次搜索请求和搜索结果；这个工具只有搜索语义。

## AI 调用提示词

当你需要查找 Windows 本机文件时，使用 `everything`。

通过 e-b 文本工具协议请求工具时，每次搜索使用一个独立工具块：

```text
<<<TOOL_REQUEST>>>
[tool]: everything
[query]: todo.md
[scopePath]: E:\eucli-project
[maxResults]: 20
[description]: 查找项目里的待办文件
<<<END_TOOL_REQUEST>>>
```

结果处理建议：

- `status == success` 时读取 Markdown 搜索结果。
- 如果 `truncated == true`，说明输出被截断，只能看到部分结果。
- 默认全盘首次搜索可能需要准备项目专属后台支撑，耗时会比后续搜索更长；限定目录首次搜索时，工具需要为该目录准备专属文件地图，也可能比后续搜索更长。
