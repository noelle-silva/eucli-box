# web_search

`web_search` 是 e-b 工具调用系统使用的统一网络搜索工具。

它接收一个搜索查询，选择一个已启用的搜索 Provider，发起一次 HTTP 搜索请求，并把不同 Provider 的响应归一化为稳定的 Markdown 和 metadata。

## 工具定位

`web_search` 只负责一次网络搜索动作。

它适合用于查询即时信息、事实核验、公开资料检索、按 Provider 能力做垂直搜索。

它不负责网页长文本抽取、不做搜索结果缓存、不做多轮研究编排、不做 LLM 总结，也不保存 API Key。

## 根系边界

这个工具只培育几个基础根动作：

- 读取工具输入。
- 读取运行时配置。
- 校验并选择 Provider。
- 复用 `network-request-system` 发起 HTTP 请求。
- 将 Provider 响应归一化为搜索结果。
- 输出 e-b 工具协议需要的 JSON 结果。

复杂搜索工作流由工具调用链组合涌现，不在工具内预设新的“搜索管理器”。

## Provider

当前支持的 Provider id：

- `tavily`
- `anysearch`

默认 Provider：

- `anysearch`

Tavily 需要 API Key，可通过工具用户配置 `tavilyApiKey` 或环境变量 `TAVILY_API_KEY` 提供。

AnySearch 支持匿名访问，但匿名访问限额较低；需要高限额时通过工具用户配置 `anysearchApiKey` 或环境变量 `ANYSEARCH_API_KEY` 提供。

## 输入参数

必填参数：

- `query`：搜索查询。

通用可选参数：

- `provider`：`tavily` 或 `anysearch`。
- `maxResults`：返回结果数。Tavily 上限 20，AnySearch 上限 100。
- `timeoutMs`：请求超时时间，单位毫秒。
- `maxOutputChars`：Markdown 输出最大字符数。
- `includeContent`：是否在 Markdown 中包含 Provider 返回的较长正文。
- `description`：简短说明这次搜索的原因。

Tavily 可选参数：

- `searchDepth`：`ultra-fast`、`fast`、`basic`、`advanced`。
- `topic`：`general`、`news`、`finance`。
- `includeAnswer`
- `includeImages`
- `includeImageDescriptions`
- `includeRawContent`：`text` 或 `markdown`。
- `country`
- `timeRange`
- `startDate`
- `endDate`

AnySearch 可选参数：

- `domain`
- `tag`
- `contentTypes`
- `zone`：`cn` 或 `intl`。
- `language`
- `params`

## 输出结果

工具返回 Markdown `content` 和结构化 metadata：

- `provider`
- `query`
- `resultsCount`
- `statusCode`
- `durationMs`
- `maxResults`
- `maxOutputChars`
- `truncated`
- `providerMetadata`：Provider 返回的 metadata。

判断搜索是否成功时，优先看工具状态。

`status == success` 表示 Provider 请求成功且响应已完成归一化。

## AI 调用提示词

当你需要联网搜索公开信息时，使用 `web_search`。

通过 e-b 文本工具协议请求工具时，每次搜索使用一个独立工具块：

```text
<<<TOOL_REQUEST>>>
[tool]: web_search
[provider]: anysearch
[query]: Go 1.22 release notes
[maxResults]: 5
[domain]: code
[tag]: code.doc
[description]: 查询 Go 版本说明
<<<END_TOOL_REQUEST>>>
```

Provider 选择建议：

- 需要 Tavily 的高级网页搜索能力时使用 `tavily`。
- 需要 AnySearch 的统一搜索、垂直域和匿名访问能力时使用 `anysearch`。

结果处理建议：

- `status == success` 时读取 Markdown 搜索结果。
- 如果 `truncated == true`，说明输出被截断，只能看到部分结果。
- API Key 不要放进 prompt，请通过工具用户配置或环境变量配置。
