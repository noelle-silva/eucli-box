# ai-image

多运营商 AI 画图工具：按配置的运营商与模型生成图片，支持内置通用协议与声明式适配文件，参考图取自会话图片，生成结果挂回会话。

## 定位

ai-image 是工具体系中的独立基础动作：一次调用生成一张图片并挂回当前会话。它自己管理自己的运营商配置与适配文件，不需要宿主或其他工具介入；配置的语义校验全部由工具负责。

## 动作

- `generate`：按提示词生成图片。可选运营商、模型、参考图与生图请求时限；生成结果经会话附件写入能力由宿主代写并挂回本次回复。
- `session_images`：列出当前会话的图片附件（序号、id、名称）。
- `config_list` / `config_read` / `config_write` / `config_delete` / `config_edit`：读写本工具配置区（工具数据目录下 `config/`）的文件；除 `config_list` 外都必须提供 `file`（配置区相对路径）。

## 配置区结构

- `config/providers.json`：多运营商与多模型配置。
- `config/adapters/<id>.json`：声明式适配文件，一适配一文件，文件名即适配 id。

## 凭据保护

`config_read` 输出 `providers.json` 时 apiKey 打码为 `********`；`config_write` 与 `config_edit` 落盘前统一还原打码占位为磁盘上的原真实 Key。原配置中没有可保留的真实 Key 时（新运营商、原文件缺失或原值本身就是打码占位），写入明确拒绝并提示提供明文 apiKey，绝不把打码占位落盘为密钥。

## 内置协议

- `images`：POST `{baseUrl}/images/generations`，JSON 体 `{model, prompt, n:1}`。
- `images-edits`：POST `{baseUrl}/images/edits`，multipart（`model`、`prompt`、参考图 `image[]`）；无参考图明确失败。
- `chat`：POST `{baseUrl}/chat/completions`，体 `{model, messages, temperature:0.2}`；有参考图时 user 内容为 text + image_url 数组。

`timeoutMs` 只作用于生图 HTTP 请求（缺省 120 秒，显式值范围 5000-3600000 毫秒，越界返回参数错误）；本地配置动作与控制通道不受该时限影响。运营商声明了 `models` 列表时，默认模型与调用覆盖模型都必须在列表内。参考图须为可识别格式且最短边至少 64 像素，不合规在本地直接拒绝、不发上游。

生图请求遇连接中断与 502/503/504 会退避后自动重试一次；失败结果统一带有可重试标记与建议动作。

## 并发与错误

配置写入（config_write / config_edit / config_delete）以跨进程锁文件串行化，文件替换带退避重试；并发编辑不会静默丢失，锁冲突返回可重试提示。配置读写失败只呈现配置区逻辑路径（如 `providers.json`），不泄露内部绝对路径与临时文件命名。

## 适配文件

纯声明式 JSON：`id`（必须等于文件名）、`name`、`description`、`request`（`method`、`path`、`headers`、`json` 或 `form` 二选一）、`response.imagePath`（点分取图路径）。

模板占位符：`{{apiKey}}`、`{{model}}`、`{{prompt}}` 可用于 path、headers、json 与 form 字符串；`json` 模板中整值 `{{images}}` 与 `{{imagesBase64}}` 分别展开为参考图 data URL 数组与 base64 数组；`form.images.filename` 可用 `{{index}}` 与 `{{ext}}`。未知占位符在写入时明确失败。

## 边界

- 不做交互式编辑、不管理图片库、不接管通用搜索或文件管理。
- 配置管理只读写本工具自己的配置区，不访问其他工具或宿主的配置。
- 会话交互只使用会话附件读取与写入两项能力；未授权时透传宿主标准拒绝文案。
- 取图支持 URL / base64 / dataURL / JSON 字段；远程图片与请求体设有命名上限。
