package hypercortexwriter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

// runCreateNote 新建笔记：按提交的标题、简介、标签与面类型清单建立空笔记。
func runCreateNote(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_writer session", err, "", "", nil)
	}
	title, err := stringArg(input, "title", false)
	if err != nil {
		return s.fail("parse create_note request", err, nil)
	}
	noteDescription, err := stringArg(input, "noteDescription", false)
	if err != nil {
		return s.fail("parse create_note request", err, nil)
	}
	tags, err := stringListArg(input, "tags")
	if err != nil {
		return s.fail("parse create_note request", err, nil)
	}
	faceKinds, err := stringListArg(input, "faceKinds")
	if err != nil {
		return s.fail("parse create_note request", err, nil)
	}

	noteInput := map[string]any{}
	setString(noteInput, "title", title)
	setString(noteInput, "description", noteDescription)
	setStringList(noteInput, "tags", tags)
	setStringList(noteInput, "faceKinds", faceKinds)
	raw, err := s.client.call(ctx, "hypercortex.notes.create", map[string]any{"input": noteInput})
	if err != nil {
		return s.fail("create note", err, nil)
	}
	var result noteSaveResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return s.fail("decode create note result", err, nil)
	}
	version := noteResultVersion(result)
	facts := []resultFact{
		textFact("noteId", result.Meta.ID),
		textFact("dir", result.Meta.Dir),
		numberFact("version", version),
		intFact("faces", len(result.Manifest.Faces)),
	}
	metadata := map[string]any{"noteId": result.Meta.ID, "dir": result.Meta.Dir, "version": int64(version), "faceCount": len(result.Manifest.Faces)}
	return s.succeed(actionCreateNote, renderNoteWrite("新建笔记", result), facts, metadata)
}

// runWriteNote 整篇写入：一次写入提交的面内容与笔记元数据（未提交的面保持不变）。
func runWriteNote(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_writer session", err, "", "", nil)
	}
	dir, err := stringArg(input, "dir", true)
	if err != nil {
		return s.fail("parse write_note request", err, nil)
	}
	noteID, err := stringArg(input, "noteId", true)
	if err != nil {
		return s.fail("parse write_note request", err, nil)
	}
	title, err := stringArg(input, "title", true)
	if err != nil {
		return s.fail("parse write_note request", err, nil)
	}
	noteDescription, err := stringArg(input, "noteDescription", false)
	if err != nil {
		return s.fail("parse write_note request", err, nil)
	}
	tags, err := stringListArg(input, "tags")
	if err != nil {
		return s.fail("parse write_note request", err, nil)
	}
	faceKinds, err := stringListArg(input, "faceKinds")
	if err != nil {
		return s.fail("parse write_note request", err, nil)
	}
	faces, err := noteFaceContentsArg(input)
	if err != nil {
		return s.fail("parse write_note request", err, nil)
	}
	expectedVersion, err := numberArg(input, "expectedVersion")
	if err != nil {
		return s.fail("parse write_note request", err, nil)
	}

	noteInput := map[string]any{"id": noteID, "packageDir": dir, "title": title}
	if _, ok := argumentValue(input, "noteDescription"); ok {
		noteInput["description"] = noteDescription
	}
	if _, ok := argumentValue(input, "tags"); ok {
		noteInput["tags"] = tags
	}
	setStringList(noteInput, "faceKinds", faceKinds)
	if len(faces) > 0 {
		noteInput["faces"] = faces
	}
	// 防误建守卫：本动作只更新已有笔记；目录不存在时快速失败，绝不静默创建新笔记。
	existing, err := s.readExistingManifest(ctx, dir)
	if err != nil {
		return s.fail("write note", err, map[string]any{"dir": dir})
	}
	if existing == nil {
		return s.fail("write note", fmt.Errorf("笔记不存在：%s；本动作只更新已有笔记，如需新建请使用 create_note", dir), map[string]any{"dir": dir})
	}
	if existing.ID != "" && existing.ID != noteID {
		return s.fail("write note", fmt.Errorf("笔记目录归属不匹配：%s 属于笔记 %s，提交的 noteId 是 %s", dir, existing.ID, noteID), map[string]any{"dir": dir, "noteId": noteID})
	}
	params := map[string]any{"input": noteInput}
	setNumber(params, "expectedVersion", expectedVersion)
	raw, err := s.client.call(ctx, "hypercortex.notes.saveFaces", params)
	if err != nil {
		return s.fail("write note", err, map[string]any{"dir": dir, "noteId": noteID})
	}
	var result noteSaveResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return s.fail("decode write note result", err, nil)
	}
	version := noteResultVersion(result)
	facts := []resultFact{
		textFact("noteId", result.Meta.ID),
		textFact("dir", result.Meta.Dir),
		numberFact("version", version),
		intFact("facesWritten", len(faces)),
	}
	metadata := map[string]any{"noteId": result.Meta.ID, "dir": result.Meta.Dir, "version": int64(version), "facesWritten": len(faces)}
	return s.succeed(actionWriteNote, renderNoteWrite("写入笔记", result), facts, metadata)
}

// runPatchFace 精确改字：在某个面里替换一段文本，可选替换全部匹配。
func runPatchFace(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_writer session", err, "", "", nil)
	}
	dir, err := stringArg(input, "dir", true)
	if err != nil {
		return s.fail("parse patch_face request", err, nil)
	}
	faceID, err := stringArg(input, "faceId", true)
	if err != nil {
		return s.fail("parse patch_face request", err, nil)
	}
	oldString, err := rawStringArg(input, "oldString", true)
	if err != nil {
		return s.fail("parse patch_face request", err, nil)
	}
	if _, ok := argumentValue(input, "newString"); !ok {
		return s.fail("parse patch_face request", fmt.Errorf("argument \"newString\" is required (use an empty string to delete the old text)"), nil)
	}
	newString, err := rawStringArg(input, "newString", false)
	if err != nil {
		return s.fail("parse patch_face request", err, nil)
	}
	replaceAll, err := boolArg(input, "replaceAll")
	if err != nil {
		return s.fail("parse patch_face request", err, nil)
	}
	expectedVersion, err := numberArg(input, "expectedVersion")
	if err != nil {
		return s.fail("parse patch_face request", err, nil)
	}

	params := map[string]any{"packageDir": dir, "faceId": faceID, "oldString": oldString, "newString": newString}
	if replaceAll {
		params["replaceAll"] = true
	}
	setNumber(params, "expectedVersion", expectedVersion)
	raw, err := s.client.call(ctx, "hypercortex.notes.patchFace", params)
	if err != nil {
		return s.fail("patch face", err, map[string]any{"dir": dir, "faceId": faceID})
	}
	var result noteSaveResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return s.fail("decode patch face result", err, nil)
	}
	version := noteResultVersion(result)
	facts := []resultFact{
		textFact("noteId", result.Meta.ID),
		textFact("faceId", faceID),
		numberFact("version", version),
	}
	metadata := map[string]any{"noteId": result.Meta.ID, "faceId": faceID, "version": int64(version)}
	return s.succeed(actionPatchFace, renderPatchFace(result, faceID, replaceAll), facts, metadata)
}

// runUpdateNoteMetadata 改笔记元数据：只改提交的字段（标题 / 简介 / 标签），缺失沿用旧值。
func runUpdateNoteMetadata(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_writer session", err, "", "", nil)
	}
	dir, err := stringArg(input, "dir", true)
	if err != nil {
		return s.fail("parse update_note_metadata request", err, nil)
	}
	metadata := map[string]any{}
	title, err := stringArg(input, "title", false)
	if err != nil {
		return s.fail("parse update_note_metadata request", err, nil)
	}
	if _, ok := argumentValue(input, "title"); ok {
		metadata["title"] = title
	}
	if _, ok := argumentValue(input, "noteDescription"); ok {
		noteDescription, err := stringArg(input, "noteDescription", false)
		if err != nil {
			return s.fail("parse update_note_metadata request", err, nil)
		}
		metadata["description"] = noteDescription
	}
	if _, ok := argumentValue(input, "tags"); ok {
		tags, err := stringListArg(input, "tags")
		if err != nil {
			return s.fail("parse update_note_metadata request", err, nil)
		}
		metadata["tags"] = tags
	}
	if len(metadata) == 0 {
		return s.fail("parse update_note_metadata request", fmt.Errorf("at least one of title, noteDescription, tags is required"), nil)
	}
	expectedVersion, err := numberArg(input, "expectedVersion")
	if err != nil {
		return s.fail("parse update_note_metadata request", err, nil)
	}

	params := map[string]any{"packageDir": dir, "metadata": metadata}
	setNumber(params, "expectedVersion", expectedVersion)
	raw, err := s.client.call(ctx, "hypercortex.notes.updateMetadata", params)
	if err != nil {
		return s.fail("update note metadata", err, map[string]any{"dir": dir})
	}
	var result noteMetadataResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return s.fail("decode update note metadata result", err, nil)
	}
	facts := []resultFact{
		textFact("dir", dir),
		numberFact("version", result.Version),
		textFact("changed", boolText(result.Changed)),
	}
	metadataOut := map[string]any{"dir": dir, "version": int64(result.Version), "changed": result.Changed}
	if result.Meta.ID != "" {
		facts = append(facts, textFact("noteId", result.Meta.ID))
		metadataOut["noteId"] = result.Meta.ID
	}
	return s.succeed(actionUpdateNoteMetadata, renderUpdateNoteMetadata(dir, result), facts, metadataOut)
}

// readExistingManifest 试读笔记清单：笔记不存在返回 nil；其余错误如实返回。
func (s session) readExistingManifest(ctx context.Context, dir string) (*noteManifest, error) {
	raw, err := s.client.call(ctx, "hypercortex.notes.tryReadManifest", map[string]any{"packageDir": dir})
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var manifest noteManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// noteFaceContentsArg 解析 write_note 提交的面内容清单：条目字段只允许 faceId / kind / content，
// kind 与 content 必须显式提供（content 可为空串表示清空该面），未知字段快速失败。
func noteFaceContentsArg(input types.ToolExecutionInput) ([]map[string]any, error) {
	items, err := objectListArg(input, "faces")
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for index, item := range items {
		entry := map[string]any{}
		for key := range item {
			switch key {
			case "faceId", "kind", "content":
			default:
				return nil, fmt.Errorf("argument \"faces\" item %d has unknown field %q", index+1, key)
			}
		}
		kind, ok := item["kind"].(string)
		if !ok || strings.TrimSpace(kind) == "" {
			return nil, fmt.Errorf("argument \"faces\" item %d requires a non-empty \"kind\"", index+1)
		}
		content, ok := item["content"].(string)
		if !ok {
			return nil, fmt.Errorf("argument \"faces\" item %d requires a string \"content\" (empty string clears the face)", index+1)
		}
		entry["kind"] = strings.TrimSpace(kind)
		entry["content"] = content
		if faceID, ok := item["faceId"]; ok {
			text, ok := faceID.(string)
			if !ok {
				return nil, fmt.Errorf("argument \"faces\" item %d \"faceId\" must be a string", index+1)
			}
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				entry["faceId"] = trimmed
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

// noteResultVersion 取笔记写入结果的新版本标记：优先显式版本，缺省回落到清单更新时间。
func noteResultVersion(result noteSaveResult) float64 {
	if result.Version > 0 {
		return result.Version
	}
	return result.Manifest.UpdatedAtMs
}

// renderNoteWrite 渲染笔记写入结果的公共头部。
func renderNoteWrite(heading string, result noteSaveResult) string {
	var builder strings.Builder
	builder.WriteString("## " + heading + "\n\n")
	fmt.Fprintf(&builder, "- noteId：%s\n", firstNonEmpty(result.Meta.ID, result.Manifest.ID))
	fmt.Fprintf(&builder, "- dir：%s\n", result.Meta.Dir)
	if title := strings.TrimSpace(result.Manifest.Title); title != "" {
		fmt.Fprintf(&builder, "- 标题：%s\n", title)
	}
	if stamp := displayTime(result.Manifest.UpdatedAtMs); stamp != "" {
		fmt.Fprintf(&builder, "- 更新时间：%s（updatedAtMs=%d）\n", stamp, int64(result.Manifest.UpdatedAtMs))
	}
	builder.WriteString(fmt.Sprintf("- 版本（updatedAtMs）：%d\n", int64(noteResultVersion(result))))
	if len(result.Manifest.FaceOrder) > 0 {
		fmt.Fprintf(&builder, "- 面顺序：%s\n", strings.Join(result.Manifest.FaceOrder, "、"))
	}
	return builder.String()
}

// renderPatchFace 渲染改字结果。
func renderPatchFace(result noteSaveResult, faceID string, replaceAll bool) string {
	var builder strings.Builder
	builder.WriteString("## 修改面内容\n\n")
	fmt.Fprintf(&builder, "- noteId：%s\n", result.Meta.ID)
	fmt.Fprintf(&builder, "- faceId：%s\n", faceID)
	if replaceAll {
		builder.WriteString("- 替换方式：全部匹配\n")
	} else {
		builder.WriteString("- 替换方式：唯一匹配\n")
	}
	builder.WriteString(fmt.Sprintf("- 版本（updatedAtMs）：%d\n", int64(noteResultVersion(result))))
	return builder.String()
}

// renderUpdateNoteMetadata 渲染笔记元数据修改结果。
func renderUpdateNoteMetadata(dir string, result noteMetadataResult) string {
	var builder strings.Builder
	builder.WriteString("## 修改笔记元数据\n\n")
	if result.Meta.ID != "" {
		fmt.Fprintf(&builder, "- noteId：%s\n", result.Meta.ID)
	}
	fmt.Fprintf(&builder, "- dir：%s\n", dir)
	if !result.Changed {
		builder.WriteString("- 结果：字段与提交值一致，未发生变化\n")
	} else {
		builder.WriteString("- 结果：已更新\n")
	}
	builder.WriteString(fmt.Sprintf("- 版本（updatedAtMs）：%d\n", int64(result.Version)))
	return builder.String()
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
