package hypercortexwriter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"eucli-box/tools/hypercortex_writer/internal/types"
)

// runUploadAssets 上传附件：提交本机文件路径清单，等待全部传完，
// 返回每个附件的编号与可直接写入笔记正文的引用标记。
func runUploadAssets(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_writer session", err, "", "", nil)
	}
	files, err := assetUploadFilesArg(input)
	if err != nil {
		return s.fail("parse upload_assets request", err, nil)
	}
	if len(files) == 0 {
		return s.fail("parse upload_assets request", fmt.Errorf("argument \"files\" must list at least one file"), nil)
	}
	raw, err := s.client.call(ctx, "hypercortex.assets.upload.sync", map[string]any{"files": files})
	if err != nil {
		return s.fail("upload assets", err, nil)
	}
	resources := []resourceRef{}
	if err := json.Unmarshal(raw, &resources); err != nil {
		return s.fail("decode upload assets result", err, nil)
	}
	facts := []resultFact{intFact("count", len(resources))}
	metadata := map[string]any{"count": len(resources)}
	assetIDs := make([]string, 0, len(resources))
	for _, resource := range resources {
		if resource.AssetID != "" {
			assetIDs = append(assetIDs, resource.AssetID)
		}
	}
	if len(assetIDs) > 0 {
		metadata["assetIds"] = assetIDs
	}
	return s.succeed(actionUploadAssets, renderUploadAssets(resources), facts, metadata)
}

// runUpdateAssetMetadata 改附件信息：全量提交附件的显示名、备注与标签三个可编辑字段
// （与后端接口一致：未提交的字段按空处理，只想改一项时先读取再整组提交）。
func runUpdateAssetMetadata(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_writer session", err, "", "", nil)
	}
	assetID, err := stringArg(input, "assetId", true)
	if err != nil {
		return s.fail("parse update_asset_metadata request", err, nil)
	}
	ext, err := stringArg(input, "ext", false)
	if err != nil {
		return s.fail("parse update_asset_metadata request", err, nil)
	}
	displayName, err := stringArg(input, "displayName", false)
	if err != nil {
		return s.fail("parse update_asset_metadata request", err, nil)
	}
	remark, err := stringArg(input, "remark", false)
	if err != nil {
		return s.fail("parse update_asset_metadata request", err, nil)
	}
	tags, err := stringListArg(input, "tags")
	if err != nil {
		return s.fail("parse update_asset_metadata request", err, nil)
	}

	params := map[string]any{
		"assetId": assetID,
		"metadata": map[string]any{
			"displayName": displayName,
			"remark":      remark,
			"tags":        tags,
		},
	}
	setString(params, "ext", ext)
	raw, err := s.client.call(ctx, "hypercortex.assets.updateMetadata", params)
	if err != nil {
		return s.fail("update asset metadata", err, map[string]any{"assetId": assetID})
	}
	var item assetItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return s.fail("decode update asset metadata result", err, nil)
	}
	facts := []resultFact{textFact("assetId", item.AssetID), textFact("name", item.Name)}
	metadata := map[string]any{"assetId": item.AssetID, "name": item.Name}
	return s.succeed(actionUpdateAssetMetadata, renderAssetItem("修改附件信息", item), facts, metadata)
}

// assetUploadFilesArg 解析上传提交的文件清单：条目字段只允许 path / displayName，
// path 必须显式提供且非空。
func assetUploadFilesArg(input types.ToolExecutionInput) ([]map[string]any, error) {
	items, err := objectListArg(input, "files")
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for index, item := range items {
		entry := map[string]any{}
		for key := range item {
			switch key {
			case "path", "displayName":
			default:
				return nil, fmt.Errorf("argument \"files\" item %d has unknown field %q", index+1, key)
			}
		}
		path, ok := item["path"].(string)
		if !ok || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("argument \"files\" item %d requires a non-empty \"path\"", index+1)
		}
		entry["path"] = strings.TrimSpace(path)
		if displayName, ok := item["displayName"]; ok {
			text, ok := displayName.(string)
			if !ok {
				return nil, fmt.Errorf("argument \"files\" item %d \"displayName\" must be a string", index+1)
			}
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				entry["displayName"] = trimmed
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

// renderUploadAssets 渲染上传结果：附件编号与引用标记逐条列出。
func renderUploadAssets(resources []resourceRef) string {
	var builder strings.Builder
	builder.WriteString("## 上传附件\n\n")
	if len(resources) == 0 {
		builder.WriteString("没有上传任何附件。\n")
		return builder.String()
	}
	for index, resource := range resources {
		fmt.Fprintf(&builder, "%d. %s\n", index+1, firstNonEmpty(resource.Name, resource.AssetID))
		fmt.Fprintf(&builder, "   assetId：%s\n", resource.AssetID)
		if kindText := assetTypeText(resource.Kind, resource.Mime); kindText != "" {
			fmt.Fprintf(&builder, "   类型：%s\n", kindText)
		}
		if resource.Marker != "" {
			fmt.Fprintf(&builder, "   引用标记：%s\n", resource.Marker)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("将引用标记写入笔记正文即可引用对应附件。\n")
	return builder.String()
}

// renderAssetItem 渲染单个附件信息。
func renderAssetItem(heading string, item assetItem) string {
	var builder strings.Builder
	builder.WriteString("## " + heading + "\n\n")
	fmt.Fprintf(&builder, "- assetId：%s\n", firstNonEmpty(item.AssetID, item.Name))
	if name := firstNonEmpty(item.DisplayName, item.SourceName, item.Name); name != "" {
		fmt.Fprintf(&builder, "- 显示名：%s\n", name)
	}
	if kindText := assetTypeText(item.Kind, item.Mime); kindText != "" {
		fmt.Fprintf(&builder, "- 类型：%s\n", kindText)
	}
	if item.Size > 0 {
		fmt.Fprintf(&builder, "- 大小：%s\n", displaySize(item.Size))
	}
	if len(item.Tags) > 0 {
		fmt.Fprintf(&builder, "- 标签：%s\n", strings.Join(item.Tags, "、"))
	}
	if remark := strings.TrimSpace(item.Remark); remark != "" {
		fmt.Fprintf(&builder, "- 备注：%s\n", oneLine(remark))
	}
	if stamp := displayTime(item.UpdatedAtMs); stamp != "" {
		fmt.Fprintf(&builder, "- 更新时间：%s\n", stamp)
	}
	return builder.String()
}

// assetTypeText 渲染附件类型的统一文本（与读工具同款）：kind（mime）；缺失部分自动省略。
func assetTypeText(kind string, mime string) string {
	kindText := strings.TrimSpace(kind)
	mimeText := strings.TrimSpace(mime)
	switch {
	case kindText != "" && mimeText != "":
		return kindText + "（" + mimeText + "）"
	case kindText != "":
		return kindText
	default:
		return mimeText
	}
}
