# 更新记录

## 0.1.2 - 2026-09-30

- `search_notes` 信息条修正：`faceKinds` 回显本次实际生效的筛选值（此前误回显默认可搜类型）；未筛选时改给 `searchableKinds` 可选项清单。
- 新增 `list_versions` / `read_version`：版本快照可列出、可读回（与 `read_note` 同一套行流与续读机制），版本闭环完整。
- `list_assets` 支持 `limit` / `offset` 分页续读：信息条回 `count` / `total` / `nextOffset`，不再只能靠截断。
- `search_assets` 信息条新增 `total`（命中总数，与窗口无关）。
- 失败结果同样带信息条：含动作、仓库、出错对象标识（`dir` / `noteId` / `faceId`）与机器可读错误码 `code`。
- 未知参数快速失败（不静默忽略）；后端错误码经响应信封透传。
- `list_trash` 文案注明：恢复需在 HyperCortex 界面侧的回收站操作。
- 文档补充：HTML 面正文不参与搜索；空面不输出段落头（面计数以信息条为准）。

## 0.1.1 - 2026-09-30

- read_note 的面段落头附带面设置（如有）：写入工具设置的面设置可在此读回验证。
- 附件类型展示与写工具统一为「kind（mime）」单行格式。

## 0.1.0 - 2026-09-29

- 建立 HyperCortex 只读访问工具：8 个读取动作（搜索笔记、读取笔记、引用关系、收藏夹、仓库清单、搜索附件、附件清单、回收站）与仓库配置、输出信息条。
- 连接配置改为工具用户配置字段（访问地址 / 仓库条目列表 / 默认仓库 / 最大输出字符数），可在设置页表单里直接增删仓库条目。
- 新增可选入参 maxOutputChars：AI 可按次下调本次返回的字符上限（不能突破工具配置的上限）。
- note_relations 的显式半径改为必须大于零：0、负数与非整数直接报错；缺省不传仍为 1。
- 正文预算重定：maxOutputChars 只约束正文；信息条不计入上限、永久完整；正文截断优先落在行边界。
- read_note 支持按行续读：offset / limit（1 基全局行号，跨面连续），信息条回 nextOffset。
- note_relations 支持分段读取：section（nodes / edges）+ limit / offset，信息条回该部分计数与 nextOffset。
- note_relations 不带 section 时拒绝 limit / offset（不再静默忽略）；整体返回与分段模式的头部格式统一。
- 截断规则描述与实现对齐（整行延后、仅首行超限才字符切），说明中补充「分段优先用 limit」。
