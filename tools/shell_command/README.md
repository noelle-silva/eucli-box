# shell_command

`shell_command` 是 e-b 工具调用系统使用的本地一次性命令执行工具。

它接收一条非交互命令，选择一个已启用的随包 Shell Provider，启动进程，等待命令结束，并返回结构化执行结果。

## 工具定位

`shell_command` 只负责一次性命令执行。

它适合用于查看工作区、运行测试、执行构建、检查命令输出、完成短生命周期的本地命令动作。

它不提供交互式终端、不提供 PTY、不支持后续 stdin 输入、不管理后台 session，也不承担长期任务控制职责。

## 根系边界

这个工具只培育几个基础根动作：

- 读取工具输入。
- 读取运行时配置。
- 校验并选择 Provider。
- 解析工作目录。
- 执行一次命令。
- 捕获 stdout、stderr 和合并输出。
- 归一化 exitCode、timedOut、durationMs、truncated 等结果。
- 输出 e-b 工具协议需要的 JSON 结果。

复杂行为不在这里预设成新的“管理器”。复杂能力由这些基础动作和 e-b 的工具调用链协同涌现。

## 源码与运行包

源码目录：

```text
tools/shell_command
```

运行包目录：

```text
data/tools/shell_command
```

构建后的运行包包含：

```text
data/tools/shell_command/
  data.json
  config.json
  binary/windows-amd64/shell_command.exe
  providers/
```

`tools/shell_command` 是开发态源码和声明。

`data/tools/shell_command` 是 e-b 运行时读取的工具包产物。

## 构建方式

使用仓库级工具包构建器构建：

```cmd
scripts\build-tools.cmd -tool shell_command ^
  -asset-root git-bash-root=D:\git\Git ^
  -asset-root powershell-root=E:\TOOOOOLSbox\powershell\7 ^
  -asset-root nushell-root=E:\TOOOOOLSbox\nushell\0.113.1
```

资产入口名称来自 `toolpack.json`。

构建器只会启用真实打包进运行包的 Provider。

如果某个 Provider 的随包可执行文件不存在，构建器会在生成运行时配置时把它标记为不可用，或者在默认 Provider 缺失时直接失败。

## Provider

当前支持的 Provider id：

- `git-bash`
- `powershell`
- `nushell`

默认 Provider：

- `git-bash`

Provider 的可执行文件路径写在 `config.json` 中，并且必须是相对于运行包目录的相对路径。

工具不会在 bundled Provider 缺失时偷偷回退到宿主机系统里的 shell。

## 输入参数

必填参数：

- `command`：要执行的命令。

可选参数：

- `provider`：已启用 Provider 的 id。不传时使用 `config.json` 里的 `defaultProvider`。
- `workdir`：工作目录。相对路径从 e-b 宿主工作目录解析。`.` 表示当前 e-b 宿主工作目录。
- `timeoutMs`：命令超时时间，单位毫秒。超过配置上限时会被钳制到最大值。
- `maxOutputChars`：每个输出字段最多捕获的字符数。超过配置上限时会被钳制到最大值。
- `description`：简短说明这次运行命令的原因。

## 输出结果

工具返回结构化 metadata：

- `stdout`
- `stderr`
- `combinedOutput`
- `exitCode`
- `timedOut`
- `durationMs`
- `provider`
- `workdir`
- `truncated`
- `maxOutputChars`
- `error`

判断命令是否成功时，优先看工具状态和 `exitCode`。

`status == success` 且 `exitCode == 0` 表示命令成功结束。

## AI 调用提示词

当你需要运行一个本地、非交互、一次性的命令并检查结果时，使用 `shell_command`。

适合使用场景：

- 查看工作区状态。
- 搜索或检查文件。
- 运行测试命令。
- 执行构建命令。
- 检查命令输出。
- 完成短生命周期的本地命令动作。

不要用于以下场景：

- 交互式终端会话。
- 需要后续输入的命令。
- PTY 工作流。
- 后台常驻任务。
- 长时间运行的 session。
- 需要 `write_stdin` 的命令。

通过 e-b 文本工具协议请求工具时，每条命令使用一个独立工具块：

```text
<<<TOOL_REQUEST>>>
[tool]: shell_command
[provider]: git-bash
[command]: go test ./...
[workdir]: .
[timeoutMs]: 120000
[maxOutputChars]: 20000
[description]: 运行全部 Go 测试
<<<END_TOOL_REQUEST>>>
```

Provider 选择建议：

- 使用 `git-bash` 执行 POSIX/bash 风格命令，也作为 Windows 开发默认选择。
- 使用 `powershell` 执行 PowerShell 语法、Windows 原生命令、对象管道命令。
- 只在命令本身是 Nushell 语法时使用 `nushell`。

结果处理建议：

- `status == success` 且 `exitCode == 0` 时，才把命令视为成功。
- 继续下一步前，先检查 `stdout`、`stderr` 和 `combinedOutput`。
- 如果 `timedOut == true`，说明命令超时，不要假设命令已经完整完成。
- 如果 `truncated == true`，说明返回内容被截断，只能看到部分输出。

## 直接调试入口

底层调试时，可以直接运行构建后的 `shell_command.exe`，并通过 stdin 传入符合 `ToolExecutionInput` 的 JSON。

产品链路不应该直接启动工具二进制，而应该通过 e-b 的 `tool-calling-system` 调用。

正式调用路径是：

```text
ToolIntent -> NormalizeIntent -> Prepare -> Execute -> shell_command.exe -> ToolResult
```
