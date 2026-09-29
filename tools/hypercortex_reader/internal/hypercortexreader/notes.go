package hypercortexreader

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

// runSearchNotes 搜索笔记：关键词、匹配维度、面类型、收藏夹范围、更新时间范围与分页
// 都是同一个接口的参数；未提供关键词时按更新时间倒序列出笔记。
func runSearchNotes(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_reader session", err, nil)
	}
	query, err := stringArg(input, "query", false)
	if err != nil {
		return s.fail("parse search_notes request", err, nil)
	}
	fields, err := stringListArg(input, "fields")
	if err != nil {
		return s.fail("parse search_notes request", err, nil)
	}
	faceKinds, err := stringListArg(input, "faceKinds")
	if err != nil {
		return s.fail("parse search_notes request", err, nil)
	}
	folderID, err := stringArg(input, "folderId", false)
	if err != nil {
		return s.fail("parse search_notes request", err, nil)
	}
	updatedFrom, err := numberArg(input, "updatedFromMs")
	if err != nil {
		return s.fail("parse search_notes request", err, nil)
	}
	updatedTo, err := numberArg(input, "updatedToMs")
	if err != nil {
		return s.fail("parse search_notes request", err, nil)
	}
	limit, err := intArg(input, "limit")
	if err != nil {
		return s.fail("parse search_notes request", err, nil)
	}
	offset, err := intArg(input, "offset")
	if err != nil {
		return s.fail("parse search_notes request", err, nil)
	}

	params := map[string]any{}
	setString(params, "query", query)
	setStringList(params, "fields", fields)
	setStringList(params, "faceKinds", faceKinds)
	setString(params, "folderId", folderID)
	setNumber(params, "updatedFromMs", updatedFrom)
	setNumber(params, "updatedToMs", updatedTo)
	setInt(params, "limit", limit)
	setInt(params, "offset", offset)
	raw, err := s.client.call(ctx, "hypercortex.search.query", params)
	if err != nil {
		return s.fail("search notes", err, queryMetadata(query))
	}
	var result noteSearchResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return s.fail("decode search result", err, nil)
	}

	facts := []resultFact{intFact("count", len(result.Items))}
	if kinds := faceKindNames(result.Kinds); len(kinds) > 0 {
		facts = append(facts, textFact("faceKinds", strings.Join(kinds, ",")))
	}
	if limit > 0 && len(result.Items) == limit {
		facts = append(facts, intFact("nextOffset", offset+len(result.Items)))
	}
	metadata := map[string]any{"count": len(result.Items)}
	if limit > 0 {
		metadata["limit"] = limit
	}
	if offset > 0 {
		metadata["offset"] = offset
	}
	if folderID != "" {
		metadata["folderId"] = folderID
	}
	return s.succeed(actionSearchNotes, renderNoteSearch(result, query), facts, metadata)
}

// runReadNote 读取某篇笔记：笔记自身信息与全部面的正文。
func runReadNote(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_reader session", err, nil)
	}
	dir, err := stringArg(input, "dir", true)
	if err != nil {
		return s.fail("parse read_note request", err, nil)
	}
	manifestRaw, err := s.client.call(ctx, "hypercortex.notes.loadManifest", map[string]any{"packageDir": dir})
	if err != nil {
		return s.fail("read note manifest", err, map[string]any{"dir": dir})
	}
	var manifest noteManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return s.fail("decode note manifest", err, map[string]any{"dir": dir})
	}
	faceIDs := orderedFaceIDs(manifest)
	docs := make([]noteFaceDoc, 0, len(faceIDs))
	for _, faceID := range faceIDs {
		faceRaw, err := s.client.call(ctx, "hypercortex.notes.loadFace", map[string]any{"packageDir": dir, "faceId": faceID})
		if err != nil {
			return s.fail("read note face", err, map[string]any{"dir": dir, "faceId": faceID})
		}
		var doc noteFaceDoc
		if err := json.Unmarshal(faceRaw, &doc); err != nil {
			return s.fail("decode note face", err, map[string]any{"dir": dir, "faceId": faceID})
		}
		docs = append(docs, doc)
	}
	facts := []resultFact{intFact("faces", len(docs))}
	metadata := map[string]any{"noteId": manifest.ID, "dir": dir, "faceCount": len(docs)}
	if manifest.UpdatedAtMs > 0 {
		metadata["updatedAtMs"] = int64(manifest.UpdatedAtMs)
	}
	return s.succeed(actionReadNote, renderNote(manifest, docs, dir), facts, metadata)
}

// runNoteRelations 查看引用 / 被引用关系：关注笔记、半径与方向都是同一个接口的参数。
func runNoteRelations(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_reader session", err, nil)
	}
	noteID, err := stringArg(input, "noteId", true)
	if err != nil {
		return s.fail("parse note_relations request", err, nil)
	}
	radius, err := intArg(input, "radius")
	if err != nil {
		return s.fail("parse note_relations request", err, nil)
	}
	direction, err := stringArg(input, "direction", false)
	if err != nil {
		return s.fail("parse note_relations request", err, nil)
	}
	params := map[string]any{"noteId": noteID}
	setInt(params, "radius", radius)
	setString(params, "direction", direction)
	raw, err := s.client.call(ctx, "hypercortex.refs.queryRelations", params)
	if err != nil {
		return s.fail("query note relations", err, map[string]any{"noteId": noteID})
	}
	var result refRelationResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return s.fail("decode relations result", err, map[string]any{"noteId": noteID})
	}
	facts := []resultFact{intFact("nodes", len(result.Nodes)), intFact("edges", len(result.Edges))}
	return s.succeed(actionNoteRelations, renderRelations(result, noteID, radius, direction), facts, map[string]any{"noteId": noteID})
}

// orderedFaceIDs 按面顺序给出全部面标识；顺序表缺项的面按标识排序补齐。
func orderedFaceIDs(manifest noteManifest) []string {
	ids := make([]string, 0, len(manifest.Faces))
	seen := map[string]bool{}
	for _, faceID := range manifest.FaceOrder {
		if _, ok := manifest.Faces[faceID]; !ok || seen[faceID] {
			continue
		}
		seen[faceID] = true
		ids = append(ids, faceID)
	}
	rest := make([]string, 0, len(manifest.Faces))
	for faceID := range manifest.Faces {
		if !seen[faceID] {
			rest = append(rest, faceID)
		}
	}
	sort.Strings(rest)
	return append(ids, rest...)
}

func faceKindNames(kinds []faceKindInfo) []string {
	names := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		if trimmed := strings.TrimSpace(kind.Kind); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names
}

func renderNoteSearch(result noteSearchResult, query string) string {
	var builder strings.Builder
	builder.WriteString("## 笔记搜索\n\n")
	if strings.TrimSpace(query) == "" {
		builder.WriteString("未指定关键词：按更新时间倒序列出笔记。\n\n")
	} else {
		builder.WriteString("关键词：" + oneLine(query) + "\n\n")
	}
	if len(result.Items) == 0 {
		builder.WriteString("未找到匹配的笔记。\n")
		return builder.String()
	}
	for index, hit := range result.Items {
		fmt.Fprintf(&builder, "%d. %s\n", index+1, firstNonEmpty(hit.Title, "未命名"))
		fmt.Fprintf(&builder, "   noteId: %s\n", hit.NoteID)
		fmt.Fprintf(&builder, "   dir: %s\n", hit.Dir)
		if stamp := displayTime(hit.UpdatedAtMs); stamp != "" {
			fmt.Fprintf(&builder, "   更新时间：%s（updatedAtMs=%d）\n", stamp, int64(hit.UpdatedAtMs))
		}
		if description := strings.TrimSpace(hit.Description); description != "" {
			fmt.Fprintf(&builder, "   简介：%s\n", oneLine(description))
		}
		if len(hit.NoteFields) > 0 {
			fmt.Fprintf(&builder, "   命中维度：%s\n", joinNonEmpty(hit.NoteFields))
		}
		for _, faceHit := range hit.FaceHits {
			label := firstNonEmpty(faceHit.Title, faceHit.Kind)
			fmt.Fprintf(&builder, "   面命中：%s《%s》（faceId=%s）%s\n", faceHit.Kind, label, faceHit.FaceID, oneLine(faceHit.Snippet))
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func renderNote(manifest noteManifest, docs []noteFaceDoc, dir string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "## 笔记：%s\n\n", firstNonEmpty(manifest.Title, "未命名"))
	fmt.Fprintf(&builder, "- id：%s\n", manifest.ID)
	fmt.Fprintf(&builder, "- dir：%s\n", dir)
	if stamp := displayTime(manifest.CreatedAtMs); stamp != "" {
		fmt.Fprintf(&builder, "- 创建：%s", stamp)
		if updated := displayTime(manifest.UpdatedAtMs); updated != "" {
			fmt.Fprintf(&builder, " ｜ 更新：%s", updated)
		}
		builder.WriteString("\n")
	}
	if manifest.UpdatedAtMs > 0 {
		fmt.Fprintf(&builder, "- 版本（updatedAtMs）：%d\n", int64(manifest.UpdatedAtMs))
	}
	if len(manifest.Tags) > 0 {
		fmt.Fprintf(&builder, "- 标签：%s\n", strings.Join(manifest.Tags, "、"))
	}
	if description := strings.TrimSpace(manifest.Description); description != "" {
		fmt.Fprintf(&builder, "- 简介：%s\n", oneLine(description))
	}
	builder.WriteString("\n")
	for index, doc := range docs {
		face := doc.Face
		fmt.Fprintf(&builder, "### 面 %d/%d：%s（kind=%s，faceId=%s）\n\n", index+1, len(docs), firstNonEmpty(face.Title, face.ID), face.Kind, face.ID)
		if !doc.Exists {
			builder.WriteString("（该面文件不存在）\n\n")
			continue
		}
		builder.WriteString(doc.Content)
		builder.WriteString("\n\n")
	}
	return builder.String()
}

func renderRelations(result refRelationResult, noteID string, radius int, direction string) string {
	var builder strings.Builder
	builder.WriteString("## 引用关系\n\n")
	fmt.Fprintf(&builder, "- 关注点：%s\n", noteID)
	if radius > 0 {
		fmt.Fprintf(&builder, "- 半径：%d\n", radius)
	} else {
		builder.WriteString("- 半径：1（缺省）\n")
	}
	fmt.Fprintf(&builder, "- 方向：%s\n\n", firstNonEmpty(direction, "both（缺省）"))
	fmt.Fprintf(&builder, "节点（%d）：\n", len(result.Nodes))
	for _, node := range result.Nodes {
		fmt.Fprintf(&builder, "- %s（距离 %d）\n", node.NoteID, node.Distance)
	}
	builder.WriteString("\n")
	fmt.Fprintf(&builder, "边（%d）：\n", len(result.Edges))
	for _, edge := range result.Edges {
		from := edge.FromNoteID
		if edge.FromFaceID != "" {
			from += "（faceId=" + edge.FromFaceID + "）"
		}
		to := edge.ToNoteID
		if edge.ToFaceID != "" {
			to += "（faceId=" + edge.ToFaceID + "）"
		}
		fmt.Fprintf(&builder, "- %s → %s\n", from, to)
	}
	return builder.String()
}
