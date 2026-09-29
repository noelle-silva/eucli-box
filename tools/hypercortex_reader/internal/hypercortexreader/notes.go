package hypercortexreader

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

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

// runReadNote 读取某篇笔记：笔记自身信息与全部面的正文；
// 支持按行续读：offset 为 1 基全局行号（跨面连续），limit 为本次最多返回的行数。
func runReadNote(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_reader session", err, nil)
	}
	dir, err := stringArg(input, "dir", true)
	if err != nil {
		return s.fail("parse read_note request", err, nil)
	}
	offset, err := intArg(input, "offset")
	if err != nil {
		return s.fail("parse read_note request", err, nil)
	}
	if _, provided := argumentValue(input, "offset"); provided && offset < 1 {
		return s.fail("parse read_note request", fmt.Errorf("argument \"offset\" must be greater than zero"), nil)
	}
	if offset < 1 {
		offset = 1
	}
	limit, err := intArg(input, "limit")
	if err != nil {
		return s.fail("parse read_note request", err, nil)
	}
	if _, provided := argumentValue(input, "limit"); provided && limit < 1 {
		return s.fail("parse read_note request", fmt.Errorf("argument \"limit\" must be greater than zero"), nil)
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
	sections, total := buildNoteSections(docs)
	body, returned, nextOffset := renderNoteWindow(manifest, sections, total, dir, offset, limit, s.maxOutput)
	facts := []resultFact{intFact("faces", len(docs)), intFact("lines", returned)}
	if nextOffset > 0 {
		facts = append(facts, intFact("nextOffset", nextOffset), textFact("truncated", "true"))
	}
	metadata := map[string]any{"noteId": manifest.ID, "dir": dir, "faceCount": len(docs), "lines": returned}
	if offset > 1 {
		metadata["offset"] = offset
	}
	if limit > 0 {
		metadata["limit"] = limit
	}
	if nextOffset > 0 {
		metadata["nextOffset"] = nextOffset
		metadata["truncated"] = true
	}
	if manifest.UpdatedAtMs > 0 {
		metadata["updatedAtMs"] = int64(manifest.UpdatedAtMs)
	}
	return s.succeed(actionReadNote, body, facts, metadata)
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
	if _, provided := argumentValue(input, "radius"); provided && radius < 1 {
		return s.fail("parse note_relations request", fmt.Errorf("argument \"radius\" must be greater than zero"), nil)
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

// noteSection 是续读视图中的一个面段落：正文按行拆分后全局连续编号。
type noteSection struct {
	index     int
	doc       noteFaceDoc
	lines     []string
	startLine int
}

// buildNoteSections 把各面正文拆成连续行流：面顺序不变，行号跨面连续；返回行流与总行数。
func buildNoteSections(docs []noteFaceDoc) ([]noteSection, int) {
	sections := make([]noteSection, 0, len(docs))
	line := 1
	for index, doc := range docs {
		lines := contentLines(doc)
		sections = append(sections, noteSection{index: index + 1, doc: doc, lines: lines, startLine: line})
		line += len(lines)
	}
	return sections, line - 1
}

// contentLines 把面正文拆成行：统一换行，并去掉文件末尾换行产生的空尾行。
func contentLines(doc noteFaceDoc) []string {
	if !doc.Exists {
		return nil
	}
	content := strings.ReplaceAll(doc.Content, "\r\n", "\n")
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

// renderNoteWindow 渲染续读窗口：正文按行取 [offset, 窗口末]，并在正文预算内装行；
// 信息条不计入预算、永久完整。返回正文、返回行数与下一行号（没有更多内容时为 0）。
func renderNoteWindow(manifest noteManifest, sections []noteSection, total int, dir string, offset int, limit int, budget int) (string, int, int) {
	windowEnd := total
	if limit > 0 && offset+limit-1 < windowEnd {
		windowEnd = offset + limit - 1
	}
	out := []string{}
	used := 0
	tryAppend := func(line string) bool {
		need := utf8.RuneCountInString(line)
		if len(out) > 0 {
			need++
		}
		if used+need > budget {
			return false
		}
		out = append(out, line)
		used += need
		return true
	}

	headerLines := noteHeaderLines(manifest, dir, total, offset)
	for _, line := range headerLines {
		if !tryAppend(line) {
			// 预算连头部都装不下：按行边界如实截断，并指示从本次起点续读。
			header, _ := truncateBody(strings.Join(headerLines, "\n"), budget)
			return header, 0, offset
		}
	}

	returned := 0
	lastLine := offset - 1
	for _, section := range sections {
		if len(section.lines) == 0 {
			continue
		}
		interStart := max(offset, section.startLine)
		interEnd := min(windowEnd, section.startLine+len(section.lines)-1)
		if interStart > interEnd {
			continue
		}
		if !tryAppend(sectionHeader(section, len(sections), interStart > section.startLine)) {
			break
		}
		full := true
		for global := interStart; global <= interEnd; global++ {
			if !tryAppend(section.lines[global-section.startLine]) {
				full = false
				break
			}
			returned++
			lastLine = global
		}
		if !full {
			break
		}
	}

	if total == 0 {
		tryAppend("（没有可读的正文内容）")
	} else if returned == 0 && offset > total {
		tryAppend("（没有更多内容）")
	}
	nextOffset := 0
	if lastLine < total {
		nextOffset = lastLine + 1
	}
	return strings.Join(out, "\n"), returned, nextOffset
}

// noteHeaderLines 给出笔记头部：从头读给完整信息；续读只给一行定位头，省正文预算。
func noteHeaderLines(manifest noteManifest, dir string, total int, offset int) []string {
	title := firstNonEmpty(manifest.Title, "未命名")
	if offset > 1 {
		return []string{fmt.Sprintf("## 笔记：%s（续读自第 %d 行）", title, offset)}
	}
	lines := []string{fmt.Sprintf("## 笔记：%s", title), ""}
	lines = append(lines, "- id："+manifest.ID, "- dir："+dir)
	if stamp := displayTime(manifest.CreatedAtMs); stamp != "" {
		line := "- 创建：" + stamp
		if updated := displayTime(manifest.UpdatedAtMs); updated != "" {
			line += " ｜ 更新：" + updated
		}
		lines = append(lines, line)
	}
	if manifest.UpdatedAtMs > 0 {
		lines = append(lines, fmt.Sprintf("- 版本（updatedAtMs）：%d", int64(manifest.UpdatedAtMs)))
	}
	if len(manifest.Tags) > 0 {
		lines = append(lines, "- 标签："+strings.Join(manifest.Tags, "、"))
	}
	if description := strings.TrimSpace(manifest.Description); description != "" {
		lines = append(lines, "- 简介："+oneLine(description))
	}
	lines = append(lines, fmt.Sprintf("- 正文：共 %d 行", total))
	return lines
}

// sectionHeader 给出面段落头；续读窗口从面中间开始时标注「（续）」。
func sectionHeader(section noteSection, totalFaces int, continued bool) string {
	face := section.doc.Face
	suffix := ""
	if continued {
		suffix = "（续）"
	}
	return fmt.Sprintf("### 面 %d/%d：%s（kind=%s，faceId=%s）%s", section.index, totalFaces, firstNonEmpty(face.Title, face.ID), face.Kind, face.ID, suffix)
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
