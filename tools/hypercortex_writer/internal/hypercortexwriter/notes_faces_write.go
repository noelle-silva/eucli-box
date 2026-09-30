package hypercortexwriter

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"eucli-box/pkg/types"
)

// runListFaceKinds 选面：列出可用于新建笔记的面类型清单。
func runListFaceKinds(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	config, err := loadConfig(input.ToolBodyDirectory)
	if err != nil {
		return failure("load hypercortex_writer config", err, actionListFaceKinds, "", nil)
	}
	repos, err := loadRepoConfig(input)
	if err != nil {
		return failure("load repository configuration", err, actionListFaceKinds, "", nil)
	}
	maxOutput, err := effectiveMaxOutputChars(input, config)
	if err != nil {
		return failure("load hypercortex_writer config", err, actionListFaceKinds, "", nil)
	}
	selector, err := stringArg(input, "repo", false)
	if err != nil {
		return failure("parse list_face_kinds request", err, actionListFaceKinds, "", nil)
	}
	entry, err := repos.resolve(selector)
	if err != nil {
		return failure("parse list_face_kinds request", err, actionListFaceKinds, "", nil)
	}
	client, err := newRPCClient(repos.Endpoint, entry.Key)
	if err != nil {
		return failure("open hypercortex_writer session", err, "", "", nil)
	}
	raw, err := client.call(ctx, "hypercortex.notes.listFacePlugins", map[string]any{})
	if err != nil {
		return failure("list face kinds", err, actionListFaceKinds, entry.ID, nil)
	}
	kinds := []faceKindInfo{}
	if err := json.Unmarshal(raw, &kinds); err != nil {
		return failure("decode face kinds result", err, actionListFaceKinds, entry.ID, nil)
	}
	facts := []resultFact{intFact("count", len(kinds))}
	content, _ := composeContent(renderFaceKinds(kinds), actionListFaceKinds, entry.ID, facts, maxOutput)
	return types.ToolExecutionOutput{
		Status:  types.ToolStatusSuccess,
		Content: content,
		Metadata: map[string]any{
			"action": actionListFaceKinds,
			"repo":   entry.ID,
			"count":  len(kinds),
		},
	}
}

// runSaveFaceOrder 调整面排序：提交新的面顺序（未列出的面自动补齐到末尾）。
func runSaveFaceOrder(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_writer session", err, "", "", nil)
	}
	dir, err := stringArg(input, "dir", true)
	if err != nil {
		return s.fail("parse save_face_order request", err, nil)
	}
	faceOrder, err := stringListArg(input, "faceOrder")
	if err != nil {
		return s.fail("parse save_face_order request", err, nil)
	}
	if len(faceOrder) == 0 {
		return s.fail("parse save_face_order request", fmt.Errorf("argument \"faceOrder\" must list at least one face id"), nil)
	}
	expectedVersion, err := numberArg(input, "expectedVersion")
	if err != nil {
		return s.fail("parse save_face_order request", err, nil)
	}

	params := map[string]any{"packageDir": dir, "faceOrder": faceOrder}
	setNumber(params, "expectedVersion", expectedVersion)
	raw, err := s.client.call(ctx, "hypercortex.notes.saveFaceOrder", params)
	if err != nil {
		return s.fail("save face order", err, map[string]any{"dir": dir})
	}
	var result noteSaveResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return s.fail("decode save face order result", err, nil)
	}
	version := noteResultVersion(result)
	facts := []resultFact{
		textFact("noteId", result.Meta.ID),
		textFact("faceOrder", strings.Join(result.Manifest.FaceOrder, ",")),
		numberFact("version", version),
	}
	metadata := map[string]any{"noteId": result.Meta.ID, "dir": dir, "version": int64(version), "faceOrder": result.Manifest.FaceOrder}
	return s.succeed(actionSaveFaceOrder, renderFaceOrder(result, dir), facts, metadata)
}

// runSaveFaceSettings 改某个面的设置：补丁语义，值为 null 表示删除该设置项。
func runSaveFaceSettings(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_writer session", err, "", "", nil)
	}
	dir, err := stringArg(input, "dir", true)
	if err != nil {
		return s.fail("parse save_face_settings request", err, nil)
	}
	faceID, err := stringArg(input, "faceId", true)
	if err != nil {
		return s.fail("parse save_face_settings request", err, nil)
	}
	settings, err := mapArg(input, "settings")
	if err != nil {
		return s.fail("parse save_face_settings request", err, nil)
	}
	if _, ok := argumentValue(input, "settings"); !ok {
		return s.fail("parse save_face_settings request", fmt.Errorf("argument \"settings\" is required"), nil)
	}
	expectedVersion, err := numberArg(input, "expectedVersion")
	if err != nil {
		return s.fail("parse save_face_settings request", err, nil)
	}

	params := map[string]any{"packageDir": dir, "faceId": faceID, "settings": settings}
	setNumber(params, "expectedVersion", expectedVersion)
	raw, err := s.client.call(ctx, "hypercortex.notes.saveFaceSettings", params)
	if err != nil {
		return s.fail("save face settings", err, map[string]any{"dir": dir, "faceId": faceID})
	}
	var result noteSaveResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return s.fail("decode save face settings result", err, nil)
	}
	version := noteResultVersion(result)
	// 回显保存后该面的实际设置：写入侧与读取侧使用同一渲染，设置是否生效当场可验证。
	savedSettings := result.Manifest.Faces[faceID].Settings
	facts := []resultFact{
		textFact("noteId", result.Meta.ID),
		textFact("faceId", faceID),
		numberFact("version", version),
	}
	if settings := renderFaceSettingsText(savedSettings); settings != "" {
		facts = append(facts, textFact("settings", settings))
	}
	metadata := map[string]any{"noteId": result.Meta.ID, "faceId": faceID, "version": int64(version), "settings": savedSettings}
	return s.succeed(actionSaveFaceSettings, renderFaceSettings(result, dir, faceID, savedSettings), facts, metadata)
}

// runDeleteFace 删面：默认移入回收站，mode=permanent 时永久删除。
// 回收站中的面需在 HyperCortex 界面侧的回收站里恢复，工具集不提供恢复动作。
func runDeleteFace(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_writer session", err, "", "", nil)
	}
	dir, err := stringArg(input, "dir", true)
	if err != nil {
		return s.fail("parse delete_face request", err, nil)
	}
	faceID, err := stringArg(input, "faceId", true)
	if err != nil {
		return s.fail("parse delete_face request", err, nil)
	}
	mode, err := stringArg(input, "mode", false)
	if err != nil {
		return s.fail("parse delete_face request", err, nil)
	}
	mode = strings.ToLower(mode)
	if mode != "" && mode != "trash" && mode != "permanent" {
		return s.fail("parse delete_face request", fmt.Errorf("argument \"mode\" must be one of trash, permanent"), nil)
	}

	params := map[string]any{"packageDir": dir, "faceId": faceID}
	setString(params, "mode", mode)
	raw, err := s.client.call(ctx, "hypercortex.notes.deleteFace", params)
	if err != nil {
		return s.fail("delete face", err, map[string]any{"dir": dir, "faceId": faceID})
	}
	var result noteSaveResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return s.fail("decode delete face result", err, nil)
	}
	version := noteResultVersion(result)
	facts := []resultFact{
		textFact("noteId", result.Meta.ID),
		textFact("faceId", faceID),
		numberFact("version", version),
	}
	metadata := map[string]any{"noteId": result.Meta.ID, "faceId": faceID, "version": int64(version)}
	return s.succeed(actionDeleteFace, renderDeleteFace(result, dir, faceID, mode), facts, metadata)
}

// runPublishVersion 存版本快照：为笔记当前内容发布一个具名提交。
func runPublishVersion(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_writer session", err, "", "", nil)
	}
	dir, err := stringArg(input, "dir", true)
	if err != nil {
		return s.fail("parse publish_version request", err, nil)
	}
	commitName, err := stringArg(input, "commitName", true)
	if err != nil {
		return s.fail("parse publish_version request", err, nil)
	}
	raw, err := s.client.call(ctx, "hypercortex.notes.versions.publish", map[string]any{"packageDir": dir, "commitName": commitName})
	if err != nil {
		return s.fail("publish version", err, map[string]any{"dir": dir})
	}
	var summary noteVersionSummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		return s.fail("decode publish version result", err, nil)
	}
	facts := []resultFact{
		textFact("versionId", summary.VersionID),
		textFact("commitName", summary.CommitName),
	}
	if summary.CreatedAtMs > 0 {
		facts = append(facts, numberFact("createdAtMs", summary.CreatedAtMs))
	}
	metadata := map[string]any{"dir": dir, "versionId": summary.VersionID, "commitName": summary.CommitName}
	return s.succeed(actionPublishVersion, renderPublishVersion(dir, summary), facts, metadata)
}

// renderFaceKinds 渲染面类型清单。
func renderFaceKinds(kinds []faceKindInfo) string {
	var builder strings.Builder
	builder.WriteString("## 可用面类型\n\n")
	if len(kinds) == 0 {
		builder.WriteString("未声明任何面类型。\n")
		return builder.String()
	}
	for _, kind := range kinds {
		if strings.TrimSpace(kind.Kind) == "" {
			continue
		}
		label := firstNonEmpty(kind.Label, kind.Kind)
		fmt.Fprintf(&builder, "- %s（%s）", label, kind.Kind)
		if kind.DefaultFaceID != "" {
			fmt.Fprintf(&builder, " ｜ 默认面 id：%s", kind.DefaultFaceID)
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

// renderFaceOrder 渲染面排序结果。
func renderFaceOrder(result noteSaveResult, dir string) string {
	var builder strings.Builder
	builder.WriteString("## 调整面顺序\n\n")
	fmt.Fprintf(&builder, "- noteId：%s\n", result.Meta.ID)
	fmt.Fprintf(&builder, "- dir：%s\n", dir)
	if len(result.Manifest.FaceOrder) > 0 {
		fmt.Fprintf(&builder, "- 新面顺序：%s\n", strings.Join(result.Manifest.FaceOrder, "、"))
	}
	builder.WriteString(fmt.Sprintf("- 版本（updatedAtMs）：%d\n", int64(noteResultVersion(result))))
	return builder.String()
}

// renderFaceSettings 渲染面设置修改结果：回显保存后该面的实际设置。
func renderFaceSettings(result noteSaveResult, dir string, faceID string, settings map[string]any) string {
	var builder strings.Builder
	builder.WriteString("## 修改面设置\n\n")
	fmt.Fprintf(&builder, "- noteId：%s\n", result.Meta.ID)
	fmt.Fprintf(&builder, "- dir：%s\n", dir)
	fmt.Fprintf(&builder, "- faceId：%s\n", faceID)
	if text := renderFaceSettingsText(settings); text != "" {
		fmt.Fprintf(&builder, "- 当前设置：%s\n", text)
	} else {
		builder.WriteString("- 当前设置：（空）\n")
	}
	builder.WriteString(fmt.Sprintf("- 版本（updatedAtMs）：%d\n", int64(noteResultVersion(result))))
	return builder.String()
}

// renderFaceSettingsText 把面设置渲染为稳定顺序的单行文本（与读工具同款）；空设置返回空串。
func renderFaceSettingsText(settings map[string]any) string {
	if len(settings) == 0 {
		return ""
	}
	keys := make([]string, 0, len(settings))
	for key := range settings {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+settingValueText(settings[key]))
	}
	return strings.Join(parts, "，")
}

// settingValueText 把设置值渲染为紧凑文本；对象与数组以 JSON 原文呈现。
func settingValueText(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case string:
		return oneLine(typed)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(raw)
	}
}

// renderDeleteFace 渲染删面结果。
func renderDeleteFace(result noteSaveResult, dir string, faceID string, mode string) string {
	var builder strings.Builder
	builder.WriteString("## 删除面\n\n")
	fmt.Fprintf(&builder, "- noteId：%s\n", result.Meta.ID)
	fmt.Fprintf(&builder, "- dir：%s\n", dir)
	fmt.Fprintf(&builder, "- faceId：%s\n", faceID)
	if mode == "permanent" {
		builder.WriteString("- 删除方式：永久删除\n")
	} else {
		builder.WriteString("- 删除方式：移入回收站（恢复需在 HyperCortex 界面侧的回收站操作）\n")
	}
	builder.WriteString(fmt.Sprintf("- 版本（updatedAtMs）：%d\n", int64(noteResultVersion(result))))
	return builder.String()
}

// renderPublishVersion 渲染版本快照结果。
func renderPublishVersion(dir string, summary noteVersionSummary) string {
	var builder strings.Builder
	builder.WriteString("## 版本快照\n\n")
	fmt.Fprintf(&builder, "- dir：%s\n", dir)
	fmt.Fprintf(&builder, "- 版本 id：%s\n", summary.VersionID)
	fmt.Fprintf(&builder, "- 提交名：%s\n", summary.CommitName)
	if stamp := displayTime(summary.CreatedAtMs); stamp != "" {
		fmt.Fprintf(&builder, "- 发布时间：%s\n", stamp)
	}
	return builder.String()
}
