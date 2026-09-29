// Package hypercortexwriter 是 hypercortex_writer 工具的业务动作：
// 通过 HyperCortex 的开放访问入口，创建与编辑知识库内容（笔记、附件、收藏夹）。
package hypercortexwriter

import (
	"context"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

// Execute 执行一次工具动作。
func Execute(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	if err := ctx.Err(); err != nil {
		return failure("tool execution cancelled", err, nil)
	}
	action, err := stringArg(input, "action", true)
	if err != nil {
		return failure("parse hypercortex_writer request", err, nil)
	}
	action = strings.ToLower(action)
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
		return failure("parse hypercortex_writer request", fmt.Errorf("unsupported action %q", action), map[string]any{"action": action})
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

// session 是一次动作执行的公共装配：解析后的仓库条目、对外客户端与输出上限。
type session struct {
	entry     repoEntry
	client    *rpcClient
	maxOutput int
}

// openSession 装配一次动作执行：装载配置、解析仓库并建立客户端；
// 任何一步失败都快速失败并说明修复位置。
func openSession(input types.ToolExecutionInput) (session, error) {
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
	return session{entry: entry, client: client, maxOutput: maxOutput}, nil
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

// fail 返回带仓库身份的失败输出。
func (s session) fail(scope string, err error, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["repo"] = s.entry.ID
	return failure(scope, err, metadata)
}

// failure 是统一的失败输出：如实说明失败原因，不返回假成功。
func failure(scope string, err error, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	errorMessage := scope
	if err != nil {
		errorMessage = scope + ": " + err.Error()
	}
	metadata["error"] = errorMessage
	return types.ToolExecutionOutput{Status: types.ToolStatusFailed, Content: errorMessage, Error: errorMessage, Metadata: metadata}
}
