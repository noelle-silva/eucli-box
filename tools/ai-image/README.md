# ai-image

多运营商 AI 画图工具：按配置的运营商与模型生成图片，支持内置通用协议与声明式适配文件，参考图取自会话图片，生成结果挂回会话。

## 定位

ai-image 是工具体系中的独立基础动作：一次调用生成一张图片并挂回当前会话。它自己管理自己的运营商配置与适配文件，不需要宿主或其他工具介入；配置的语义校验全部由工具负责。

## 动作

- `generate`：按提示词生成图片。可选运营商、模型、参考图与请求时限；生成结果经会话附件写入能力由宿主代写并挂回本次回复。
- `session_images`：列出当前会话的图片附件（序号、id、名称）。
- `config_list` / `config_read` / `config_write` / `config_delete` / `config_edit`：读写本工具配置区（工具数据目录下 `config/`）的文件。

## 配置区结构

- `config/providers.json`：多运营商与多模型配置。
- `config/adapters/<id>.json`：声明式适配文件，一适配一文件，文件名即适配 id。

## 内置协议

- `images`：POST `{baseUrl}/images/generations`，JSON 体 `{model, prompt, n:1}`。
- `images-edits`：POST `{baseUrl}/images/edits`，multipart（`model`、`prompt`、参考图 `image[]`）；无参考图明确失败。
- `chat`：POST `{baseUrl}/chat/completions`，体 `{model, messages, temperature:0.2}`；有参考图时 user 内容为 text + image_url 数组。

## 适配文件

纯声明式 JSON：`id`（必须等于文件名）、`name`、`description`、`request`（`method`、`path`、`headers`、`json` 或 `form` 二选一）、`response.imagePath`（点分取图路径）。

模板占位符：`{{apiKey}}`、`{{model}}`、`{{prompt}}` 可用于 path、headers、json 与 form 字符串；`json` 模板中整值 `{{images}}` 与 `{{imagesBase64}}` 分别展开为参考图 data URL 数组与 base64 数组；`form.images.filename` 可用 `{{index}}` 与 `{{ext}}`。未知占位符在写入时明确失败。

## 边界

- 不做交互式编辑、不管理图片库、不接管通用搜索或文件管理。
- 配置管理只读写本工具自己的配置区，不访问其他工具或宿主的配置。
- 会话交互只使用会话附件读取与写入两项能力；未授权时透传宿主标准拒绝文案。
- 取图支持 URL / base64 / dataURL / JSON 字段；远程图片与请求体设有命名上限。
