# hypercortex_writer

`hypercortex_writer` 是 HyperCortex 知识库的写入工具。它通过 HyperCortex 应用开放的外部访问入口，完成新建与编辑笔记、调整面与元数据、上传附件、管理收藏夹内容。

## 连接配置

在设置页「AI 工具 → 本工具 → 用户配置」里填写（表单里可直接增删仓库条目，不需要手写 JSON）：

- `访问地址`：HyperCortex 设置页「外部访问管理」里显示的访问地址（配置了开放端口后才可用）。
- `仓库条目`：可增删的多条配置，每条含：
  - `仓库 id`：动作里用 `repo` 参数引用的名字，可含中文；
  - `描述`：给人看的说明；
  - `访问钥匙`：该仓库对应的访问密钥。
- `默认仓库`：动作未指定 `repo` 参数时使用的条目 id。
- `最大输出字符数`（可选）：下调单次输出上限，不能超过随包上限。

一把访问钥匙只通一个仓库；仓库身份由密钥自身决定，工具不会也无法越权指定其他仓库。

## 防覆盖保险丝

所有修改动作都可以传 `expectedVersion`（先读取时得到的 `updatedAtMs`）：与当前版本不一致时写入会被拒绝并回报当前版本；修改成功后信息条回传新版本，供下一次修改回传。

## 动作

### 笔记

- `list_face_kinds`：列出可用于新建笔记的面类型清单。
- `create_note`：新建空笔记（标题必填，`noteDescription`、标签、面类型清单可选；非法面类型会直接报错）。
- `write_note`：整篇写入一篇已有笔记（**不会创建新笔记**，笔记不存在会报错；新建请用 `create_note`；定位笔记、标题必填；`faces` 提交要写的面内容，未提交的面保持不变；可补新面）。
- `patch_face`：在某个面里精确替换一段文本（`newString` 为空串表示删除，`replaceAll` 可选）。
- `save_face_order`：调整面顺序（未列出的面自动补齐）。
- `save_face_settings`：以补丁语义改某个面的设置（值为 null 表示删除该设置项）；返回结果与信息条回显保存后该面的实际设置，可当场验证是否生效；不被该面协议支持的设置项会直接报错（不静默丢弃）。
- `delete_face`：删面（缺省移入回收站，`permanent` 永久删除）；回收站中的面需在 HyperCortex 界面侧的回收站恢复，工具集不提供恢复动作。
- `publish_version`：为笔记当前内容发布具名版本快照；快照可用读工具的 `list_versions` / `read_version` 列出与读回。
- `update_note_metadata`：改笔记的标题 / 简介 / 标签（只改提交的字段；简介参数为 `noteDescription`）。

### 附件

- `upload_assets`：上传本机文件为附件（一次调用等待全部传完，返回附件编号与引用标记；标记可直接写入笔记正文；本机源文件不存在时会明确报错）。
- `update_asset_metadata`：改附件的显示名 / 备注 / 标签（三个可编辑字段整组提交，未提供的字段按空处理；只想改一项时先读取该附件再整组回填）。

### 收藏夹

- `create_favorite_folder`：建收藏夹（说明参数为 `folderDescription`）。
- `update_favorite_folder`：改收藏夹（标题 / 说明）。
- `add_favorite_item`：把笔记 / 附件 / 子收藏夹收进收藏夹。
- `remove_favorite_item`：把条目移出收藏夹（只摘收藏引用，不删除对象本身）。
- `move_favorite_item`：把条目挪到别的收藏夹。

## 参数命名说明

本工具的简介 / 说明参数使用专属名称（`noteDescription` / `folderDescription`），避免与框架里作为「调用原因」的 `description` 惯例撞名。

## 输出与边界

- 每次返回（成功与失败）的正文末尾都有一条 `[hypercortex_writer]` 信息条，说明本次调用的关键事实（仓库、动作、编号、版本、条数等）；信息条不计入输出上限、永久完整。
- 失败信息条携带出错对象标识（`dir` / `noteId` / `faceId` / `folderId`）与机器可读的错误码 `code`（如 `VERSION_CONFLICT`、`UNKNOWN_FACE_KIND`、`DUPLICATE_FAVORITE`、`PATH_ESCAPE`）；文案可改，码不变，便于程序化区分错误类型。
- 未知参数会直接报错，不会被静默忽略（包括已废弃的旧参数名，如 `description`）。
- 正文超出上限时按行边界截断（仅当首行本身就超上限时才按字符切），信息条标注 `truncated=true`。
- 正文上限由随包 `config.json` 声明，可在工具用户配置里下调；每次调用还可用可选入参 `maxOutputChars` 再下调（不能突破配置上限）。
- 防覆盖保险丝 `expectedVersion` 是可选参数：不传则不校验；传了才会在版本不一致时拒绝写入。
- 删除笔记、删除附件、删除收藏夹不属于本工具；回收站恢复也需在界面侧操作。
