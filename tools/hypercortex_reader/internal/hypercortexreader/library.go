package hypercortexreader

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

// runListFavorites 查看收藏夹结构与其内容。
func runListFavorites(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_reader session", err, nil)
	}
	raw, err := s.client.call(ctx, "hypercortex.favorites.tryLoad", map[string]any{})
	if err != nil {
		return s.fail("list favorites", err, nil)
	}
	var doc favoritesDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return s.fail("decode favorites result", err, nil)
	}
	payload, folderCount, itemCount := renderFavorites(doc)
	facts := []resultFact{intFact("folders", folderCount), intFact("items", itemCount)}
	metadata := map[string]any{"folderCount": folderCount, "itemCount": itemCount}
	if doc.UpdatedAtMs > 0 {
		metadata["updatedAtMs"] = int64(doc.UpdatedAtMs)
	}
	return s.succeed(actionListFavorites, payload, facts, metadata)
}

// runListTrash 查看回收站（只读）。
func runListTrash(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_reader session", err, nil)
	}
	raw, err := s.client.call(ctx, "hypercortex.trash.list", map[string]any{})
	if err != nil {
		return s.fail("list trash", err, nil)
	}
	items := []trashItem{}
	if err := json.Unmarshal(raw, &items); err != nil {
		return s.fail("decode trash result", err, nil)
	}
	facts := []resultFact{intFact("count", len(items))}
	return s.succeed(actionListTrash, renderTrash(items), facts, map[string]any{"count": len(items)})
}

// runListRepos 查看仓库清单：只列出工具配置里注册的条目（仓库 id 与描述），不展示钥匙；
// 该动作不访问 HyperCortex，不依赖它是否在运行。
func runListRepos(input types.ToolExecutionInput) types.ToolExecutionOutput {
	config, err := loadConfig(input.ToolBodyDirectory)
	if err != nil {
		return failure("load hypercortex_reader config", err, nil)
	}
	repos, err := loadRepoConfig(input)
	if err != nil {
		return failure("load repository configuration", err, nil)
	}
	maxOutput, err := effectiveMaxOutputChars(input, config)
	if err != nil {
		return failure("load hypercortex_reader config", err, nil)
	}
	facts := []resultFact{intFact("count", len(repos.Repos)), textFact("default", repos.DefaultRepo)}
	content, _ := composeContent(renderRepos(repos), actionListRepos, "local", facts, maxOutput)
	return types.ToolExecutionOutput{
		Status:  types.ToolStatusSuccess,
		Content: content,
		Metadata: map[string]any{
			"action":      actionListRepos,
			"count":       len(repos.Repos),
			"defaultRepo": repos.DefaultRepo,
		},
	}
}

// renderRepos 渲染已注册仓库条目：只有 id 与描述，钥匙永不进入输出。
func renderRepos(repos repoConfig) string {
	var builder strings.Builder
	builder.WriteString("## 已注册仓库\n\n")
	for _, entry := range repos.Repos {
		label := entry.ID
		if entry.ID == repos.DefaultRepo {
			label += "（默认）"
		}
		if entry.Description != "" {
			fmt.Fprintf(&builder, "- %s：%s\n", label, entry.Description)
			continue
		}
		fmt.Fprintf(&builder, "- %s\n", label)
	}
	return builder.String()
}

// renderFavorites 以深度优先方式渲染收藏夹树，并返回收藏夹数与内容条目数。
func renderFavorites(doc favoritesDoc) (string, int, int) {
	var builder strings.Builder
	builder.WriteString("## 收藏夹\n\n")
	if doc.RootFolderID == "" && len(doc.Folders) == 0 {
		builder.WriteString("该仓库尚未初始化收藏夹。\n")
		return builder.String(), 0, 0
	}
	if doc.UpdatedAtMs > 0 {
		fmt.Fprintf(&builder, "版本（updatedAtMs）：%d\n\n", int64(doc.UpdatedAtMs))
	}
	rootID := firstNonEmpty(doc.RootFolderID, "root")
	folderCount := 0
	itemCount := 0
	visited := map[string]bool{}
	var walk func(folderID string, depth int)
	walk = func(folderID string, depth int) {
		indent := strings.Repeat("  ", depth)
		folder, ok := doc.Folders[folderID]
		if !ok {
			fmt.Fprintf(&builder, "%s- 收藏夹：%s（未找到）\n", indent, folderID)
			return
		}
		if visited[folderID] {
			fmt.Fprintf(&builder, "%s- 收藏夹：%s（已在前文列出）\n", indent, folderID)
			return
		}
		visited[folderID] = true
		folderCount++
		title := firstNonEmpty(folder.Title, folderID)
		if depth == 0 {
			fmt.Fprintf(&builder, "### %s（%s）\n\n", title, folderID)
		} else {
			fmt.Fprintf(&builder, "%s- 收藏夹：%s（%s）\n", indent, title, folderID)
		}
		if description := strings.TrimSpace(folder.Description); description != "" {
			fmt.Fprintf(&builder, "%s  说明：%s\n", indent, oneLine(description))
		}
		for _, ref := range doc.RefsByFolderID[folderID] {
			switch strings.TrimSpace(ref.Kind) {
			case "folder":
				walk(ref.TargetID, depth+1)
			case "note":
				itemCount++
				fmt.Fprintf(&builder, "%s- 笔记：%s\n", indent, ref.TargetID)
			case "asset":
				itemCount++
				fmt.Fprintf(&builder, "%s- 附件：%s\n", indent, ref.TargetID)
			default:
				itemCount++
				fmt.Fprintf(&builder, "%s- %s：%s\n", indent, ref.Kind, ref.TargetID)
			}
		}
		if depth == 0 {
			builder.WriteString("\n")
		}
	}
	walk(rootID, 0)
	return builder.String(), folderCount, itemCount
}

// renderTrash 渲染回收站条目（只读，删除类动作不属于本工具）。
func renderTrash(items []trashItem) string {
	var builder strings.Builder
	builder.WriteString("## 回收站\n\n")
	if len(items) == 0 {
		builder.WriteString("回收站为空。\n")
		return builder.String()
	}
	for index, item := range items {
		fmt.Fprintf(&builder, "%d. [%s] %s\n", index+1, trashKindLabel(item.Kind), firstNonEmpty(item.Title, item.ID))
		fmt.Fprintf(&builder, "   id：%s\n", item.ID)
		if stamp := displayTime(item.DeletedAtMs); stamp != "" {
			fmt.Fprintf(&builder, "   删除时间：%s\n", stamp)
		}
		if strings.TrimSpace(item.OriginalDir) != "" {
			fmt.Fprintf(&builder, "   原位置：%s\n", item.OriginalDir)
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func trashKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case "note":
		return "笔记"
	case "asset":
		return "附件"
	case "face":
		return "面"
	default:
		return kind
	}
}
