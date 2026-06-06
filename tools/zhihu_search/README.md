# zhihu_search

`zhihu_search` 是 e-b 工具调用系统使用的知乎开放平台搜索工具。

它通过知乎开放平台查询知乎内容，把返回结果整理成稳定的 Markdown 和 metadata。

## 工具定位

`zhihu_search` 只负责一次知乎查询动作。

它适合用于查询知乎站内问答、文章、作者摘要，以及更广范围的知乎关联内容。

它不负责网页正文抓取、不做多轮研究编排、不缓存结果、不保存真实密钥，也不替模型总结搜索结果。

## 根系边界

这个工具只培育两个基础根动作：

- `zhihu_search`：知乎站内内容搜索，最多返回 10 条结果。
- `global_search`：知乎全局搜索，最多返回 20 条结果。

复杂资料研究流程由工具调用链组合完成，不在工具内新增研究中心。

## API Secret

知乎开放平台 Access Secret 通过工具用户配置 `zhihuAccessSecret` 或环境变量 `ZHIHU_ACCESS_SECRET` 提供。

API Secret 不要放进 prompt，也不要写入仓库。

## 输入参数

必填参数：

- `query`：搜索查询。

通用可选参数：

- `searchType`：`zhihu_search` 或 `global_search`。也接受 `zhihu`、`site`、`global`、`all` 等别名。
- `count`：返回结果数。`zhihu_search` 上限 10，`global_search` 上限 20。
- `timeoutMs`：请求超时时间，单位毫秒。
- `maxOutputChars`：Markdown 输出最大字符数。
- `description`：简短说明这次查询的原因。

## 输出结果

工具返回 Markdown `content` 和结构化 metadata：

- `searchType`
- `query`
- `resultsCount`
- `statusCode`
- `durationMs`
- `code`
- `apiMessage`
- `sources`
- `items`
- `truncated`

`items` 中包含标题、链接、作者、摘要和编辑时间。

站内搜索结果额外包含点赞数和评论数。

判断查询是否成功时，优先看工具状态。

`status == success` 表示知乎接口请求成功且响应已完成归一化。

## AI 调用提示词

当你需要查询知乎内容时，使用 `zhihu_search`。

站内搜索：

```text
<<<TOOL_REQUEST>>>
[tool]: zhihu_search
[query]: AI Agent 应用实践
[searchType]: zhihu_search
[count]: 5
[description]: 查询知乎站内讨论
<<<END_TOOL_REQUEST>>>
```

全局搜索：

```text
<<<TOOL_REQUEST>>>
[tool]: zhihu_search
[query]: 如何理解 rave 文化
[searchType]: global_search
[count]: 8
[description]: 查询知乎全局内容
<<<END_TOOL_REQUEST>>>
```

使用建议：

- 查知乎问答、文章等站内内容时使用 `zhihu_search`。
- 查更广的知乎关联内容时使用 `global_search`。
- API Secret 不要放进 prompt，请通过工具用户配置或环境变量配置。
