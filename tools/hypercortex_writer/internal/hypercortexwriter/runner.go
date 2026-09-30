// Package hypercortexwriter 是 hypercortex_writer 工具的业务动作：
// 通过 HyperCortex 的开放访问入口，创建与编辑知识库内容（笔记、附件、收藏夹）。
package hypercortexwriter

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

// Execute 执行一次工具动作。
func Execute(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	if err := ctx.Err(); err != nil {
		return failure("tool execution cancelled", err, "", "", nil)
	}
	action, err := stringArg(input, "action", true)
	if err != nil {
		return failure("parse hypercortex_writer request", err, "", "", nil)
	}
	action = strings.ToLower(action)
	if _, ok := actionArgumentNames[action]; !ok {
		return failure("parse hypercortex_writer request", fmt.Errorf("unsupported action %q", action), action, "", map[string]any{"action": action})
	}
	if err := validateArguments(input, action); err != nil {
		return failure("parse hypercortex_writer request", err, action, "", map[string]any{"action": action})
	}
	switch action {
	case actionListFaceKinds:
		return runListFaceKinds(ctx, input)
	case actionCreateNote:
		return runCreateNote(ctx, input)
	case actionWriteNote:
		return runWriteNote(ctx, input)
	case actionPatchFace:
		return runPatchFace(ctx, input)
	case actionSaveFaceOrder:
		return runSaveFaceOrder(ctx, input)
	case actionSaveFaceSettings:
		return runSaveFaceSettings(ctx, input)
	case actionDeleteFace:
		return runDeleteFace(ctx, input)
	case actionPublishVersion:
		return runPublishVersion(ctx, input)
	case actionUpdateNoteMetadata:
		return runUpdateNoteMetadata(ctx, input)
	case actionUploadAssets:
		return runUploadAssets(ctx, input)
	case actionUpdateAssetMetadata:
		return runUpdateAssetMetadata(ctx, input)
	case actionCreateFavoriteFolder:
		return runCreateFavoriteFolder(ctx, input)
	case actionUpdateFavoriteFolder:
		return runUpdateFavoriteFolder(ctx, input)
	case actionAddFavoriteItem:
		return runAddFavoriteItem(ctx, input)
	case actionRemoveFavoriteItem:
		return runRemoveFavoriteItem(ctx, input)
	case actionMoveFavoriteItem:
		return runMoveFavoriteItem(ctx, input)
	default:
		return failure("parse hypercortex_writer request", fmt.Errorf("unsupported action %q", action), action, "", map[string]any{"action": action})
	}
}

const (
	actionListFaceKinds        = "list_face_kinds"
	actionCreateNote           = "create_note"
	actionWriteNote            = "write_note"
	actionPatchFace            = "patch_face"
	actionSaveFaceOrder        = "save_face_order"
	actionSaveFaceSettings     = "save_face_settings"
	actionDeleteFace           = "delete_face"
	actionPublishVersion       = "publish_version"
	actionUpdateNoteMetadata   = "update_note_metadata"
	actionUploadAssets         = "upload_assets"
	actionUpdateAssetMetadata  = "update_asset_metadata"
	actionCreateFavoriteFolder = "create_favorite_folder"
	actionUpdateFavoriteFolder = "update_favorite_folder"
	actionAddFavoriteItem      = "add_favorite_item"
	actionRemoveFavoriteItem   = "remove_favorite_item"
	actionMoveFavoriteItem     = "move_favorite_item"
)

// actionArgumentNames 是每个动作允许的参数名清单（公共参数除外）：分发前统一校验，
// 未知参数快速失败，杜绝「传了但没生效」的静默忽略（含已废弃的旧参数名）。
var actionArgumentNames = map[string][]string{
	actionListFaceKinds:        {},
	actionCreateNote:           {"title", "noteDescription", "tags", "faceKinds"},
	actionWriteNote:            {"dir", "noteId", "title", "noteDescription", "tags", "faceKinds", "faces", "expectedVersion"},
	actionPatchFace:            {"dir", "faceId", "oldString", "newString", "replaceAll", "expectedVersion"},
	actionSaveFaceOrder:        {"dir", "faceOrder", "expectedVersion"},
	actionSaveFaceSettings:     {"dir", "faceId", "settings", "expectedVersion"},
	actionDeleteFace:           {"dir", "faceId", "mode"},
	actionPublishVersion:       {"dir", "commitName"},
	actionUpdateNoteMetadata:   {"dir", "title", "noteDescription", "tags", "expectedVersion"},
	actionUploadAssets:         {"files"},
	actionUpdateAssetMetadata:  {"assetId", "ext", "displayName", "remark", "tags"},
	actionCreateFavoriteFolder: {"title", "folderDescription", "folderId", "expectedVersion"},
	actionUpdateFavoriteFolder: {"folderId", "title", "folderDescription", "expectedVersion"},
	actionAddFavoriteItem:      {"folderId", "kind", "targetId", "expectedVersion"},
	actionRemoveFavoriteItem:   {"folderId", "kind", "targetId", "expectedVersion"},
	actionMoveFavoriteItem:     {"fromFolderId", "toFolderId", "kind", "targetId", "expectedVersion"},
}

// commonArgumentNames 是全部动作共用的参数名（含框架级超时预算）。
var commonArgumentNames = []string{"action", "repo", "maxOutputChars", "timeoutMs"}

// validateArguments 校验本次调用参数都在该动作允许的清单内；未知参数快速失败并逐个点名。
func validateArguments(input types.ToolExecutionInput, action string) error {
	allowed := map[string]bool{}
	for _, key := range commonArgumentNames {
		allowed[key] = true
	}
	for _, key := range actionArgumentNames[action] {
		allowed[key] = true
	}
	unknown := make([]string, 0)
	for key := range input.Arguments {
		if !allowed[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("unknown argument(s) for action %s: %s", action, strings.Join(unknown, ", "))
	}
	return nil
}

// actionFromInput 从调用参数读取规范化后的动作名；缺失时返回空串。
func actionFromInput(input types.ToolExecutionInput) string {
	action, _ := stringArg(input, "action", false)
	return strings.ToLower(action)
}

// session 是一次动作执行的公共装配：解析后的仓库条目、对外客户端、输出上限与动作名。
type session struct {
	entry     repoEntry
	client    *rpcClient
	maxOutput int
	action    string
}

// openSession 装配一次动作执行：装载配置、解析仓库并建立客户端；
// 任何一步失败都快速失败并说明修复位置。动作名从调用参数自读，调用点无需重复传递。
func openSession(input types.ToolExecutionInput) (session, error) {
	action := actionFromInput(input)
	config, err := loadConfig(input.ToolBodyDirectory)
	if err != nil {
		return session{}, err
	}
	repos, err := loadRepoConfig(input)
	if err != nil {
		return session{}, err
	}
	selector, err := stringArg(input, "repo", false)
	if err != nil {
		return session{}, err
	}
	entry, err := repos.resolve(selector)
	if err != nil {
		return session{}, err
	}
	client, err := newRPCClient(repos.Endpoint, entry.Key)
	if err != nil {
		return session{}, err
	}
	maxOutput, err := effectiveMaxOutputChars(input, config)
	if err != nil {
		return session{}, err
	}
	return session{entry: entry, client: client, maxOutput: maxOutput, action: strings.ToLower(action)}, nil
}

// succeed 组合正文与信息条，并返回成功输出。
func (s session) succeed(action string, payload string, facts []resultFact, metadata map[string]any) types.ToolExecutionOutput {
	content, truncated := composeContent(payload, action, s.entry.ID, facts, s.maxOutput)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["action"] = action
	metadata["repo"] = s.entry.ID
	if s.entry.Description != "" {
		metadata["repoDescription"] = s.entry.Description
	}
	if truncated {
		metadata["truncated"] = true
	}
	return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: content, Metadata: metadata}
}

// fail 返回带仓库身份的失败输出：失败同样携带信息条（动作、仓库、标识符与错误码）。
func (s session) fail(scope string, err error, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["repo"] = s.entry.ID
	return failure(scope, err, s.action, s.entry.ID, metadata)
}

// failure 是统一的失败输出：如实说明失败原因，不返回假成功；
// 失败同样携带信息条（动作、仓库、标识符与错误码）。
func failure(scope string, err error, action string, repoID string, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	errorMessage := scope
	if err != nil {
		errorMessage = scope + ": " + err.Error()
	}
	if code := errorCode(err); code != "" {
		metadata["code"] = code
	}
	metadata["error"] = errorMessage
	if action != "" {
		metadata["action"] = action
	}
	if repoID != "" {
		metadata["repo"] = repoID
	}
	content := composeFailure(errorMessage, action, repoID, metadata)
	return types.ToolExecutionOutput{Status: types.ToolStatusFailed, Content: content, Error: errorMessage, Metadata: metadata}
}
