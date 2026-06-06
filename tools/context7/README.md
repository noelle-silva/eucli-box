# context7

`context7` 是 e-b 工具调用系统使用的 Context7 文档查询工具。

它连接 Context7 的公开文档接口，先查找资料库 id，再按资料库 id 拉取当前文档片段，并把结果归一化为稳定的 Markdown 和 metadata。

## 工具定位

`context7` 只负责一次 Context7 文档查询动作。

它适合用于查询库、框架、API 的当前用法、安装方式、配置示例、版本相关说明。

它不负责通用网页搜索、不做网页抓取、不做多轮研究编排、不保存 API Key，也不替代 `web_search` 的公开信息搜索能力。

## 根系边界

这个工具只培育两个基础根动作：

- `search`：根据库名和问题查找 Context7 资料库 id。
- `docs`：根据 Context7 资料库 id 和问题读取文档片段。

复杂研究流程由工具调用链组合完成，不在工具内预设新的“文档研究管理器”。

## API Key

Context7 支持匿名访问，但限额较低。

需要更高限额时，通过工具用户配置 `context7ApiKey` 或环境变量 `CONTEXT7_API_KEY` 提供。

API Key 不要放进 prompt。

## 输入参数

必填参数：

- `action`：`search` 或 `docs`。
- `query`：具体文档问题或任务。

`search` 必填参数：

- `libraryName`：库名，例如 `react`、`nextjs`、`prisma`。

`docs` 必填参数：

- `libraryId`：Context7 资料库 id，例如 `/facebook/react`、`/vercel/next.js`。

通用可选参数：

- `fast`：是否跳过 Context7 重排序以降低延迟。
- `timeoutMs`：请求超时时间，单位毫秒。
- `maxOutputChars`：Markdown 输出最大字符数。
- `description`：简短说明这次查询的原因。

## 输出结果

工具返回 Markdown `content` 和结构化 metadata：

- `action`
- `query`
- `libraryName` 或 `libraryId`
- `statusCode`
- `durationMs`
- `truncated`
- `resultsCount`、`codeSnippets` 或 `infoSnippets`

判断查询是否成功时，优先看工具状态。

`status == success` 表示 Context7 请求成功且响应已完成归一化。

## AI 调用提示词

当你需要查询库、框架、API 的当前官方文档时，使用 `context7`。

先用 `search` 查找资料库 id：

```text
<<<TOOL_REQUEST>>>
[tool]: context7
[action]: search
[libraryName]: nextjs
[query]: How to set up middleware authentication in Next.js 15
[description]: 查询 Next.js 文档库 id
<<<END_TOOL_REQUEST>>>
```

再用 `docs` 读取具体文档：

```text
<<<TOOL_REQUEST>>>
[tool]: context7
[action]: docs
[libraryId]: /vercel/next.js
[query]: How to set up middleware authentication in Next.js 15
[description]: 查询 Next.js 中间件认证文档
<<<END_TOOL_REQUEST>>>
```

使用建议：

- 已知资料库 id 时直接使用 `docs`。
- 不知道资料库 id 时先使用 `search`。
- 查询要描述具体目标，不要只写单个关键词。
- API Key 不要放进 prompt，请通过工具用户配置或环境变量配置。
