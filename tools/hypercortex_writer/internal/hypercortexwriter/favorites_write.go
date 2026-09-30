package hypercortexwriter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

// runCreateFavoriteFolder 建夹：在指定收藏夹（缺省为根）下新建一个子收藏夹。
func runCreateFavoriteFolder(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_writer session", err, "", "", nil)
	}
	title, err := stringArg(input, "title", true)
	if err != nil {
		return s.fail("parse create_favorite_folder request", err, nil)
	}
	folderDescription, err := stringArg(input, "folderDescription", false)
	if err != nil {
		return s.fail("parse create_favorite_folder request", err, nil)
	}
	parentID, err := stringArg(input, "folderId", false)
	if err != nil {
		return s.fail("parse create_favorite_folder request", err, nil)
	}
	expectedVersion, err := numberArg(input, "expectedVersion")
	if err != nil {
		return s.fail("parse create_favorite_folder request", err, nil)
	}

	params := map[string]any{"title": title}
	setString(params, "description", folderDescription)
	setString(params, "parentId", parentID)
	setNumber(params, "expectedVersion", expectedVersion)
	raw, err := s.client.call(ctx, "hypercortex.favorites.createFolder", params)
	if err != nil {
		return s.fail("create favorite folder", err, nil)
	}
	result, err := decodeFavoriteWriteResult(raw)
	if err != nil {
		return s.fail("decode create favorite folder result", err, nil)
	}
	facts := []resultFact{
		textFact("folderId", result.FolderID),
		textFact("parentId", result.ParentID),
		numberFact("version", result.Version),
	}
	metadata := map[string]any{"folderId": result.FolderID, "parentId": result.ParentID, "version": int64(result.Version)}
	return s.succeed(actionCreateFavoriteFolder, renderFavoriteWrite("新建收藏夹", result), facts, metadata)
}

// runUpdateFavoriteFolder 改夹：更新收藏夹的标题与说明（只提交要改的字段）。
func runUpdateFavoriteFolder(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_writer session", err, "", "", nil)
	}
	folderID, err := stringArg(input, "folderId", true)
	if err != nil {
		return s.fail("parse update_favorite_folder request", err, nil)
	}
	patch := map[string]any{}
	if _, ok := argumentValue(input, "title"); ok {
		title, err := stringArg(input, "title", false)
		if err != nil {
			return s.fail("parse update_favorite_folder request", err, nil)
		}
		patch["title"] = title
	}
	if _, ok := argumentValue(input, "folderDescription"); ok {
		folderDescription, err := stringArg(input, "folderDescription", false)
		if err != nil {
			return s.fail("parse update_favorite_folder request", err, nil)
		}
		patch["description"] = folderDescription
	}
	if len(patch) == 0 {
		return s.fail("parse update_favorite_folder request", fmt.Errorf("at least one of title, folderDescription is required"), nil)
	}
	expectedVersion, err := numberArg(input, "expectedVersion")
	if err != nil {
		return s.fail("parse update_favorite_folder request", err, nil)
	}

	params := map[string]any{"folderId": folderID, "patch": patch}
	setNumber(params, "expectedVersion", expectedVersion)
	raw, err := s.client.call(ctx, "hypercortex.favorites.updateFolder", params)
	if err != nil {
		return s.fail("update favorite folder", err, map[string]any{"folderId": folderID})
	}
	result, err := decodeFavoriteWriteResult(raw)
	if err != nil {
		return s.fail("decode update favorite folder result", err, nil)
	}
	changed := result.Changed == nil || *result.Changed
	facts := []resultFact{
		textFact("folderId", result.FolderID),
		numberFact("version", result.Version),
		textFact("changed", boolText(changed)),
	}
	metadata := map[string]any{"folderId": result.FolderID, "version": int64(result.Version), "changed": changed}
	return s.succeed(actionUpdateFavoriteFolder, renderFavoriteWrite("修改收藏夹", result), facts, metadata)
}

// runAddFavoriteItem 放入：把笔记、附件或子收藏夹收进指定收藏夹。
func runAddFavoriteItem(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_writer session", err, "", "", nil)
	}
	folderID, err := stringArg(input, "folderId", true)
	if err != nil {
		return s.fail("parse add_favorite_item request", err, nil)
	}
	kind, err := favoriteItemKindArg(input)
	if err != nil {
		return s.fail("parse add_favorite_item request", err, nil)
	}
	targetID, err := stringArg(input, "targetId", true)
	if err != nil {
		return s.fail("parse add_favorite_item request", err, nil)
	}
	expectedVersion, err := numberArg(input, "expectedVersion")
	if err != nil {
		return s.fail("parse add_favorite_item request", err, nil)
	}

	params := map[string]any{"folderId": folderID, "kind": kind, "targetId": targetID}
	setNumber(params, "expectedVersion", expectedVersion)
	raw, err := s.client.call(ctx, "hypercortex.favorites.addItem", params)
	if err != nil {
		return s.fail("add favorite item", err, map[string]any{"folderId": folderID, "kind": kind, "targetId": targetID})
	}
	result, err := decodeFavoriteWriteResult(raw)
	if err != nil {
		return s.fail("decode add favorite item result", err, nil)
	}
	facts := []resultFact{
		textFact("folderId", result.FolderID),
		textFact("kind", kind),
		textFact("targetId", targetID),
		numberFact("version", result.Version),
	}
	metadata := map[string]any{"folderId": result.FolderID, "kind": kind, "targetId": targetID, "version": int64(result.Version)}
	return s.succeed(actionAddFavoriteItem, renderFavoriteItemWrite("放入收藏夹", result, kind, targetID), facts, metadata)
}

// runRemoveFavoriteItem 移出：把条目从收藏夹中摘除（不动被收藏的对象本身）。
func runRemoveFavoriteItem(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_writer session", err, "", "", nil)
	}
	folderID, err := stringArg(input, "folderId", true)
	if err != nil {
		return s.fail("parse remove_favorite_item request", err, nil)
	}
	kind, err := favoriteItemKindArg(input)
	if err != nil {
		return s.fail("parse remove_favorite_item request", err, nil)
	}
	targetID, err := stringArg(input, "targetId", true)
	if err != nil {
		return s.fail("parse remove_favorite_item request", err, nil)
	}
	expectedVersion, err := numberArg(input, "expectedVersion")
	if err != nil {
		return s.fail("parse remove_favorite_item request", err, nil)
	}

	params := map[string]any{"folderId": folderID, "kind": kind, "targetId": targetID}
	setNumber(params, "expectedVersion", expectedVersion)
	raw, err := s.client.call(ctx, "hypercortex.favorites.removeItem", params)
	if err != nil {
		return s.fail("remove favorite item", err, map[string]any{"folderId": folderID, "kind": kind, "targetId": targetID})
	}
	result, err := decodeFavoriteWriteResult(raw)
	if err != nil {
		return s.fail("decode remove favorite item result", err, nil)
	}
	facts := []resultFact{
		textFact("folderId", result.FolderID),
		textFact("kind", kind),
		textFact("targetId", targetID),
		numberFact("version", result.Version),
	}
	metadata := map[string]any{"folderId": result.FolderID, "kind": kind, "targetId": targetID, "version": int64(result.Version)}
	return s.succeed(actionRemoveFavoriteItem, renderFavoriteItemWrite("移出收藏夹", result, kind, targetID), facts, metadata)
}

// runMoveFavoriteItem 挪夹：把条目从一个收藏夹移到另一个收藏夹。
func runMoveFavoriteItem(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_writer session", err, "", "", nil)
	}
	fromFolderID, err := stringArg(input, "fromFolderId", true)
	if err != nil {
		return s.fail("parse move_favorite_item request", err, nil)
	}
	toFolderID, err := stringArg(input, "toFolderId", true)
	if err != nil {
		return s.fail("parse move_favorite_item request", err, nil)
	}
	kind, err := favoriteItemKindArg(input)
	if err != nil {
		return s.fail("parse move_favorite_item request", err, nil)
	}
	targetID, err := stringArg(input, "targetId", true)
	if err != nil {
		return s.fail("parse move_favorite_item request", err, nil)
	}
	expectedVersion, err := numberArg(input, "expectedVersion")
	if err != nil {
		return s.fail("parse move_favorite_item request", err, nil)
	}

	params := map[string]any{"fromFolderId": fromFolderID, "toFolderId": toFolderID, "kind": kind, "targetId": targetID}
	setNumber(params, "expectedVersion", expectedVersion)
	raw, err := s.client.call(ctx, "hypercortex.favorites.moveItem", params)
	if err != nil {
		return s.fail("move favorite item", err, map[string]any{"fromFolderId": fromFolderID, "toFolderId": toFolderID})
	}
	result, err := decodeFavoriteWriteResult(raw)
	if err != nil {
		return s.fail("decode move favorite item result", err, nil)
	}
	facts := []resultFact{
		textFact("fromFolderId", result.FromFolderID),
		textFact("toFolderId", result.ToFolderID),
		textFact("kind", kind),
		textFact("targetId", targetID),
		numberFact("version", result.Version),
	}
	metadata := map[string]any{"fromFolderId": result.FromFolderID, "toFolderId": result.ToFolderID, "kind": kind, "targetId": targetID, "version": int64(result.Version)}
	return s.succeed(actionMoveFavoriteItem, renderFavoriteMove(result, kind, targetID), facts, metadata)
}

// favoriteItemKindArg 读取收藏条目类型：只接受 note / asset / folder。
func favoriteItemKindArg(input types.ToolExecutionInput) (string, error) {
	kind, err := stringArg(input, "kind", true)
	if err != nil {
		return "", err
	}
	switch kind {
	case "note", "asset", "folder":
		return kind, nil
	default:
		return "", fmt.Errorf("argument \"kind\" must be one of note, asset, folder")
	}
}

func decodeFavoriteWriteResult(raw json.RawMessage) (favoriteWriteResult, error) {
	result := favoriteWriteResult{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return favoriteWriteResult{}, err
	}
	return result, nil
}

// renderFavoriteWrite 渲染收藏夹写入结果的公共头部。
func renderFavoriteWrite(heading string, result favoriteWriteResult) string {
	var builder strings.Builder
	builder.WriteString("## " + heading + "\n\n")
	if result.FolderID != "" {
		fmt.Fprintf(&builder, "- 收藏夹：%s\n", result.FolderID)
	}
	if result.ParentID != "" {
		fmt.Fprintf(&builder, "- 上级收藏夹：%s\n", result.ParentID)
	}
	if result.Changed != nil && !*result.Changed {
		builder.WriteString("- 结果：与提交值一致，未发生变化\n")
	}
	builder.WriteString(fmt.Sprintf("- 版本（updatedAtMs）：%d\n", int64(result.Version)))
	return builder.String()
}

// renderFavoriteItemWrite 渲染条目放入 / 移出结果。
func renderFavoriteItemWrite(heading string, result favoriteWriteResult, kind string, targetID string) string {
	var builder strings.Builder
	builder.WriteString("## " + heading + "\n\n")
	fmt.Fprintf(&builder, "- 收藏夹：%s\n", result.FolderID)
	fmt.Fprintf(&builder, "- 条目：%s %s\n", kind, targetID)
	if result.RefID != "" {
		fmt.Fprintf(&builder, "- 条目 id：%s\n", result.RefID)
	}
	builder.WriteString(fmt.Sprintf("- 版本（updatedAtMs）：%d\n", int64(result.Version)))
	return builder.String()
}

// renderFavoriteMove 渲染条目挪夹结果。
func renderFavoriteMove(result favoriteWriteResult, kind string, targetID string) string {
	var builder strings.Builder
	builder.WriteString("## 移动收藏条目\n\n")
	fmt.Fprintf(&builder, "- 来源收藏夹：%s\n", result.FromFolderID)
	fmt.Fprintf(&builder, "- 目标收藏夹：%s\n", result.ToFolderID)
	fmt.Fprintf(&builder, "- 条目：%s %s\n", kind, targetID)
	builder.WriteString(fmt.Sprintf("- 版本（updatedAtMs）：%d\n", int64(result.Version)))
	return builder.String()
}
