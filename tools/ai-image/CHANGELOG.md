# 更新记录

## 0.1.0 - 2026-09-29

- 从 AI 工具模板建立 ai-image。
- 实现多运营商生图：内置 images、images-edits、chat 三家协议与声明式适配文件；取图支持 URL / base64 / dataURL / JSON 字段。
- 实现配置管理动作（config_list / config_read / config_write / config_delete / config_edit）：直接读写工具自身配置区，校验、大小上限与 apiKey 打码全在工具内部。
- 参考图取自会话图片、生成结果经会话附件写入能力挂回会话；工具只声明会话附件读、写两项能力。
