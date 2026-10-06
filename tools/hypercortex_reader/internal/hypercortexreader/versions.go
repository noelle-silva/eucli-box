package hypercortexreader

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"eucli-box/tools/hypercortex_reader/internal/types"
)

// runListVersions 列出某篇笔记的版本快照（按发布时间倒序）：版本 id、提交名与发布时间。
func runListVersions(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input, actionListVersions)
	if err != nil {
		return failure("open hypercortex_reader session", err, actionListVersions, "", nil)
	}
	dir, err := stringArg(input, "dir", true)
	if err != nil {
		return s.fail("parse list_versions request", err, nil)
	}
	raw, err := s.client.call(ctx, "hypercortex.notes.versions.list", map[string]any{"packageDir": dir})
	if err != nil {
		return s.fail("list versions", err, map[string]any{"dir": dir})
	}
	versions := []noteVersionSummary{}
	if err := json.Unmarshal(raw, &versions); err != nil {
		return s.fail("decode versions result", err, map[string]any{"dir": dir})
	}
	facts := []resultFact{intFact("count", len(versions))}
	if len(versions) > 0 {
		facts = append(facts, textFact("latest", versions[0].VersionID))
	}
	metadata := map[string]any{"dir": dir, "count": len(versions)}
	if len(versions) > 0 {
		metadata["latestVersionId"] = versions[0].VersionID
	}
	return s.succeed(actionListVersions, renderVersionList(dir, versions), facts, metadata)
}

// runReadVersion 读取某个版本快照的正文：与 read_note 同一套行流与续读机制，
// 版本快照的面顺序、面设置与各面内容都按快照时刻原样呈现。
func runReadVersion(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input, actionReadVersion)
	if err != nil {
		return failure("open hypercortex_reader session", err, actionReadVersion, "", nil)
	}
	dir, err := stringArg(input, "dir", true)
	if err != nil {
		return s.fail("parse read_version request", err, nil)
	}
	versionID, err := stringArg(input, "versionId", true)
	if err != nil {
		return s.fail("parse read_version request", err, nil)
	}
	offset, err := intArg(input, "offset")
	if err != nil {
		return s.fail("parse read_version request", err, nil)
	}
	if _, provided := argumentValue(input, "offset"); provided && offset < 1 {
		return s.fail("parse read_version request", fmt.Errorf("argument \"offset\" must be greater than zero"), nil)
	}
	if offset < 1 {
		offset = 1
	}
	limit, err := intArg(input, "limit")
	if err != nil {
		return s.fail("parse read_version request", err, nil)
	}
	if _, provided := argumentValue(input, "limit"); provided && limit < 1 {
		return s.fail("parse read_version request", fmt.Errorf("argument \"limit\" must be greater than zero"), nil)
	}
	raw, err := s.client.call(ctx, "hypercortex.notes.versions.load", map[string]any{"packageDir": dir, "versionId": versionID})
	if err != nil {
		return s.fail("read version", err, map[string]any{"dir": dir, "versionId": versionID})
	}
	var snapshot noteVersionSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return s.fail("decode version result", err, map[string]any{"dir": dir, "versionId": versionID})
	}
	docs := snapshotFaceDocs(snapshot)
	sections, total := buildNoteSections(docs)
	body, returned, nextOffset := renderVersionWindow(snapshot, sections, total, dir, versionID, offset, limit, s.maxOutput)
	facts := []resultFact{intFact("faces", len(docs)), intFact("lines", returned)}
	if nextOffset > 0 {
		facts = append(facts, intFact("nextOffset", nextOffset), textFact("truncated", "true"))
	}
	metadata := map[string]any{"dir": dir, "versionId": versionID, "faceCount": len(docs), "lines": returned}
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
	if snapshot.Manifest.ID != "" {
		metadata["noteId"] = snapshot.Manifest.ID
	}
	if snapshot.CreatedAtMs > 0 {
		metadata["createdAtMs"] = int64(snapshot.CreatedAtMs)
	}
	return s.succeed(actionReadVersion, body, facts, metadata)
}

// snapshotFaceDocs 把版本快照的面集合整理为按面顺序排列的面文档；
// 面顺序缺项按标识排序补齐，与 read_note 的面顺序口径一致。
func snapshotFaceDocs(snapshot noteVersionSnapshot) []noteFaceDoc {
	ids := make([]string, 0, len(snapshot.Faces))
	seen := map[string]bool{}
	for _, id := range snapshot.Manifest.FaceOrder {
		if _, ok := snapshot.Faces[id]; !ok || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	rest := make([]string, 0, len(snapshot.Faces))
	for id := range snapshot.Faces {
		if !seen[id] {
			rest = append(rest, id)
		}
	}
	sort.Strings(rest)
	ids = append(ids, rest...)

	docs := make([]noteFaceDoc, 0, len(ids))
	for _, id := range ids {
		saved := snapshot.Faces[id]
		face := saved.Manifest
		face.ID = firstNonEmpty(face.ID, id)
		docs = append(docs, noteFaceDoc{
			ID:          id,
			PackageDir:  snapshot.PackageDir,
			NoteID:      snapshot.NoteID,
			NoteTitle:   snapshot.Manifest.Title,
			Face:        face,
			Content:     saved.Content,
			Exists:      strings.TrimSpace(saved.Content) != "",
			CreatedAtMs: snapshot.CreatedAtMs,
			UpdatedAtMs: snapshot.CreatedAtMs,
		})
	}
	return docs
}

// renderVersionWindow 渲染版本快照的续读窗口：头部标明版本身份，正文按行取窗口。
func renderVersionWindow(snapshot noteVersionSnapshot, sections []noteSection, total int, dir string, versionID string, offset int, limit int, budget int) (string, int, int) {
	windowEnd := total
	if limit > 0 && offset+limit-1 < windowEnd {
		windowEnd = offset + limit - 1
	}
	packer := newLinePacker(budget)

	headerLines := versionHeaderLines(snapshot, dir, versionID, total, offset)
	for _, line := range headerLines {
		if !packer.tryAppend(line) {
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
		if !packer.tryAppend(sectionHeader(section, len(sections), interStart > section.startLine)) {
			break
		}
		full := true
		for global := interStart; global <= interEnd; global++ {
			if !packer.tryAppend(section.lines[global-section.startLine]) {
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
		packer.tryAppend("（该版本没有可读的正文内容）")
	} else if returned == 0 && offset > total {
		packer.tryAppend("（没有更多内容）")
	}
	nextOffset := 0
	if lastLine < total {
		nextOffset = lastLine + 1
	}
	return packer.text(), returned, nextOffset
}

// versionHeaderLines 给出快照头部：从头读给完整版本信息；续读只给一行定位头。
func versionHeaderLines(snapshot noteVersionSnapshot, dir string, versionID string, total int, offset int) []string {
	title := firstNonEmpty(snapshot.Manifest.Title, "未命名")
	if offset > 1 {
		return []string{fmt.Sprintf("## 版本快照：%s（续读自第 %d 行）", title, offset)}
	}
	lines := []string{fmt.Sprintf("## 版本快照：%s", title), ""}
	lines = append(lines, "- 版本 id："+versionID, "- dir："+dir)
	if snapshot.NoteID != "" {
		lines = append(lines, "- noteId："+snapshot.NoteID)
	}
	if snapshot.CommitName != "" {
		lines = append(lines, "- 提交名："+snapshot.CommitName)
	}
	if stamp := displayTime(snapshot.CreatedAtMs); stamp != "" {
		lines = append(lines, "- 发布时间："+stamp)
	}
	if description := strings.TrimSpace(snapshot.Manifest.Description); description != "" {
		lines = append(lines, "- 简介："+oneLine(description))
	}
	if len(snapshot.Manifest.Tags) > 0 {
		lines = append(lines, "- 标签："+strings.Join(snapshot.Manifest.Tags, "、"))
	}
	lines = append(lines, fmt.Sprintf("- 正文：共 %d 行", total))
	return lines
}

// renderVersionList 渲染版本清单。
func renderVersionList(dir string, versions []noteVersionSummary) string {
	var builder strings.Builder
	builder.WriteString("## 版本快照清单\n\n")
	fmt.Fprintf(&builder, "- dir：%s\n\n", dir)
	if len(versions) == 0 {
		builder.WriteString("该笔记尚未发布任何版本快照。\n")
		return builder.String()
	}
	for index, version := range versions {
		fmt.Fprintf(&builder, "%d. %s\n", index+1, version.VersionID)
		if version.CommitName != "" {
			fmt.Fprintf(&builder, "   提交名：%s\n", version.CommitName)
		}
		if stamp := displayTime(version.CreatedAtMs); stamp != "" {
			fmt.Fprintf(&builder, "   发布时间：%s（createdAtMs=%d）\n", stamp, int64(version.CreatedAtMs))
		}
		if len(version.FaceIDs) > 0 {
			fmt.Fprintf(&builder, "   面：%s\n", strings.Join(version.FaceIDs, "、"))
		}
		builder.WriteString("\n")
	}
	return builder.String()
}
