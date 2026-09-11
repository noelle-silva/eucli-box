# everything

`everything` 是 e-b 工具调用系统使用的本机文件搜索工具。

它接收一次动作请求：`search` 查询本机文件，`authorize` 一次性授权安装权限管家，`index` 显式准备并持久化全盘索引。默认全盘搜索使用工具包自带 Everything 运行环境搜索可用盘符；限定目录搜索使用该运行环境准备专属范围文件地图。搜索结果会被整理为稳定的 Markdown 和 metadata。

## 动作说明

- `search`（默认）：执行一次本机文件搜索，行为与既有版本一致。
- `authorize`：幂等的一次性授权动作。未授权时通过系统提权安装或修复权限管家；已授权时直接返回已授权。不接受搜索专属参数。
- `index`：显式准备全盘索引并持久化索引数据，不执行搜索。不接受搜索专属参数。

推荐流程：先 `authorize`，再 `index`，之后 `search`。全盘动作（未指定 `scopePath` 的 `search` 与 `index`）在权限管家不健康时会立即失败，并明确提示先调用 `authorize`、由用户在电脑前确认系统弹窗；拦截不会启动实例，也不会进入长时间等待。指定 `scopePath` 的搜索不需要授权，不受拦截影响。

`authorize` 与 `index` 仅支持 Windows；其他平台上这两个动作明确失败，`search` 保持既有行为。

## 权限管家机制

普通账户下的 Everything 实例无法直接读取磁盘底层数据，全盘索引会空转。权限管家是安装到系统保护区、指向保护区内引擎副本的系统服务，服务名为 `Everything (<默认实例名>)`。全盘实例保持普通账户运行，通过同名服务读取卷数据；指定目录的实例不使用管家。

授权动作先加独占授权锁（授权进行中会明确返回），再检测管家状态：健康则直接返回已授权；否则通过系统提权（runas）以管理员身份启动工具本体的隐藏模式 `--steward-install <配置文件路径>`。隐藏模式依次完成：停止并删除旧服务登记（无论其指向何处）、建立保护区目录、复制工具包内引擎到保护区（覆盖）、以保护区引擎安装服务（实例名一致）、启动服务并等待进入运行状态，最后把成功或失败原因写入结构化结果文件。授权动作读取结果文件；等待期间尊重调用方时限与停止指令。用户拒绝提权时明确返回授权被拒绝；结果缺失时按失败返回并说明未完成。授权完成后复核状态，健康才算成功。授权动作不触发全盘索引。

健康（已授权）必须同时满足：服务存在；服务登记的二进制路径等于运行时推导的保护区引擎路径；保护区引擎与当前工具包内引擎副本内容一致（哈希一致）；服务处于运行状态。状态检测不需要管理员权限，能区分未安装、路径不符、版本不符、已停止与健康。

## 保护区位置

保护区位于系统受保护的程序安装区域（Windows 上由系统提供，通常为 `Program Files`）下、由产品名称与工具标识组成的专属子目录 `eucli-box-everything`，其中存放引擎副本 `Everything.exe`。该位置在运行时通过系统标准方式取得，不写死盘符或绝对路径，也不设置回退目录；系统未能提供该目录时授权动作明确失败。

## 生命周期说明

- 权限管家由一次管理员授权安装，长期有效；引擎换代导致工具包与保护区引擎哈希不一致时，全盘动作会被拦截并引导重新授权，以完成管家更新。
- 删除或替换工具本体不会自动清理权限管家；显式退役路径属于后续任务，当前版本不实现。
- 全盘搜索与索引动作结束后，按保活配置收尾实例：默认立即关闭实例；开启保活后实例保持运行 `keepAliveSeconds` 秒，由工具内置的保活守卫进程到期关闭。
- 授权动作本身不启动全盘实例，也不产生索引数据。

## 自带运行环境

工具运行包默认自带 voidtools Everything 运行程序和命令行搜索程序。

默认行为：

- 使用工具包内的 `Everything.exe` 和 `es.exe`。
- 默认全盘搜索使用工具专属全盘入口，限定目录搜索使用工具专属目录入口。
- 本机运行缓存保存在工具自己的运行区，重新生成工具目录时不会误删已有缓存。
- 用户未指定搜索范围时，默认搜索本机可用盘符。
- 用户指定搜索范围时，只使用该范围建立工具专属文件地图。

运行缓存不是打包资源，不会进入仓库，也不是要把索引数据一起发布。保留运行缓存只是为了避免本机重新生成工具目录后从零开始建立文件地图。

当运行缓存文件被手动删除时，下一次全盘动作会先让 Everything 准备可用索引；如果缓存文件仍不存在，工具会保存当前数据库，让缓存重新落到 `data\tool-data\everything\runtime` 下。

只有当你明确要覆盖自带程序时，才配置工具用户设置 `esPath` 或环境变量 `EVERYTHING_ES_PATH`。

`esPath` 和 `EVERYTHING_ES_PATH` 支持绝对路径；如果只填命令名，则按系统路径查找。

### 运行实例生命周期

- 默认情况下（`keepAliveEnabled` 关闭，默认值）：一次搜索或索引返回结果后，工具立即关闭自己启动的 Everything 实例；下次动作会重新启动实例。实例的存活时间不超过一次工具调用。
- 开启 `keepAliveEnabled` 后：动作返回结果后实例继续运行；如果在 `keepAliveSeconds` 秒内没有新的动作使用，超时后由工具内置的保活守卫进程自动关闭实例。
- `keepAliveSeconds` 默认 300 秒；`keepAliveEnabled` 关闭时该值保留不生效，切换开关时数值不会丢失。
- 实例存活时间始终受控：要么随本次调用结束，要么在保活窗口到期后结束，不会无限期留在后台。

### 工具用户配置

- `keepAliveEnabled`：是否在搜索或索引后保持 Everything 实例运行一段时间。默认关闭。
- `keepAliveSeconds`：保活窗口秒数，仅 `keepAliveEnabled` 开启时生效。默认 300。

## 输入参数

通用参数：

- `action`：动作，取值 `search`、`authorize`、`index`，缺省为 `search`。
- `timeoutMs`：动作超时时间，单位毫秒；任何动作都受调用方时限约束。
- `description`：简短说明这次动作的原因。

`search` 参数：

- `query`（必填）：Everything 搜索查询。
- `scopePath`：限定搜索的已存在文件夹。相对路径基于 e-b 主机工作目录；省略时默认搜索本机可用盘符。
- `instanceName`：Everything 实例名；留空表示工具专属默认实例。全盘搜索只支持默认实例，显式传入其他实例名会明确失败。
- `maxResults`：返回结果数，上限 500。
- `maxOutputChars`：Markdown 输出最大字符数。

`authorize` 与 `index` 不接受 `query`、`scopePath`、`instanceName`、`maxResults`、`maxOutputChars` 这些搜索专属参数。

## 输出结果

`search` 返回 Markdown `content` 和结构化 metadata：

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

`index` 成功时返回索引就绪说明，metadata 包含：

- `action`
- `scopeMode`（全盘）
- `scopePaths`（各盘符）
- `instanceName`
- `durationMs`
- `driveEntryCounts`（各盘可见条目数量）
- `entryCount`（可见条目总数）
- `timeoutMs`
- `description`（提供时）

`authorize` 成功时返回已授权或授权完成说明，metadata 包含 `action`、`stewardState`、`serviceName`、`protectedEnginePath`、`alreadyAuthorized`。

判断动作是否成功时，优先看工具状态；`status == success` 表示动作完成。

## AI 调用提示词

当你需要查找 Windows 本机文件时，使用 `everything`。

全盘搜索前需要一次授权安装权限管家，请先通过模型自带工具调用通道调用：

```json
{
  "action": "authorize",
  "description": "授权安装全盘索引权限管家"
}
```

授权成功后，可以显式准备全盘索引：

```json
{
  "action": "index",
  "description": "准备全盘索引"
}
```

之后每次搜索传入参数，例如：

```json
{
  "query": "todo.md",
  "scopePath": "E:\\eucli-project",
  "maxResults": 20,
  "description": "查找项目里的待办文件"
}
```

结果处理建议：

- `status == success` 时读取 Markdown 结果。
- 如果 `truncated == true`，说明输出被截断，只能看到部分结果。
- 全盘动作在权限管家不健康时会快速失败并提示先授权；授权需要用户在电脑前确认系统弹窗。
- 默认全盘首次搜索需要准备本机索引，耗时会比后续搜索更长；限定目录首次搜索时，工具需要为该目录准备专属文件地图，也可能比后续搜索更长。
