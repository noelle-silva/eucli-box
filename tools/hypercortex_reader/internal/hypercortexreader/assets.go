package hypercortexreader

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

// runSearchAssets 搜索附件：文本维度、类型、大小、更新时间范围与分页都是同一个接口的参数；
// 未提供关键词时按过滤条件列出附件（按更新时间倒序）。
func runSearchAssets(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_reader session", err, nil)
	}
	query, err := stringArg(input, "query", false)
	if err != nil {
		return s.fail("parse search_assets request", err, nil)
	}
	fields, err := stringListArg(input, "fields")
	if err != nil {
		return s.fail("parse search_assets request", err, nil)
	}
	kind, err := stringArg(input, "kind", false)
	if err != nil {
		return s.fail("parse search_assets request", err, nil)
	}
	sizeFrom, err := numberArg(input, "sizeFrom")
	if err != nil {
		return s.fail("parse search_assets request", err, nil)
	}
	sizeTo, err := numberArg(input, "sizeTo")
	if err != nil {
		return s.fail("parse search_assets request", err, nil)
	}
	updatedFrom, err := numberArg(input, "updatedFromMs")
	if err != nil {
		return s.fail("parse search_assets request", err, nil)
	}
	updatedTo, err := numberArg(input, "updatedToMs")
	if err != nil {
		return s.fail("parse search_assets request", err, nil)
	}
	limit, err := intArg(input, "limit")
	if err != nil {
		return s.fail("parse search_assets request", err, nil)
	}
	offset, err := intArg(input, "offset")
	if err != nil {
		return s.fail("parse search_assets request", err, nil)
	}

	params := map[string]any{}
	setString(params, "query", query)
	setStringList(params, "fields", fields)
	setString(params, "kind", kind)
	setNumber(params, "sizeFrom", sizeFrom)
	setNumber(params, "sizeTo", sizeTo)
	setNumber(params, "updatedFromMs", updatedFrom)
	setNumber(params, "updatedToMs", updatedTo)
	setInt(params, "limit", limit)
	setInt(params, "offset", offset)
	raw, err := s.client.call(ctx, "hypercortex.search.queryAssets", params)
	if err != nil {
		return s.fail("search assets", err, queryMetadata(query))
	}
	items, err := decodeAssetItems(raw)
	if err != nil {
		return s.fail("decode assets result", err, nil)
	}

	facts := []resultFact{intFact("count", len(items))}
	if limit > 0 && len(items) == limit {
		facts = append(facts, intFact("nextOffset", offset+len(items)))
	}
	metadata := map[string]any{"count": len(items)}
	if limit > 0 {
		metadata["limit"] = limit
	}
	if offset > 0 {
		metadata["offset"] = offset
	}
	return s.succeed(actionSearchAssets, renderAssetItems(items, query), facts, metadata)
}

// runListAssets 查看附件清单 / 信息：完整列出附件池的全部附件。
func runListAssets(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	s, err := openSession(input)
	if err != nil {
		return failure("open hypercortex_reader session", err, nil)
	}
	raw, err := s.client.call(ctx, "hypercortex.assets.list", map[string]any{})
	if err != nil {
		return s.fail("list assets", err, nil)
	}
	items, err := decodeAssetItems(raw)
	if err != nil {
		return s.fail("decode assets result", err, nil)
	}
	facts := []resultFact{intFact("count", len(items))}
	return s.succeed(actionListAssets, renderAssetItems(items, ""), facts, map[string]any{"count": len(items)})
}

func decodeAssetItems(raw json.RawMessage) ([]assetItem, error) {
	items := []assetItem{}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func renderAssetItems(items []assetItem, query string) string {
	var builder strings.Builder
	builder.WriteString("## 附件\n\n")
	if strings.TrimSpace(query) == "" {
		builder.WriteString("按修改时间倒序列出附件。\n\n")
	} else {
		builder.WriteString("关键词：" + oneLine(query) + "\n\n")
	}
	if len(items) == 0 {
		builder.WriteString("未找到附件。\n")
		return builder.String()
	}
	for index, item := range items {
		fmt.Fprintf(&builder, "%d. %s（%s）\n", index+1, assetDisplayName(item), firstNonEmpty(item.Kind, "未知类型"))
		fmt.Fprintf(&builder, "   assetId：%s ｜ 大小：%s\n", item.Name, displaySize(item.Size))
		if strings.TrimSpace(item.Mime) != "" {
			fmt.Fprintf(&builder, "   类型：%s\n", item.Mime)
		}
		if strings.TrimSpace(item.RelPath) != "" {
			fmt.Fprintf(&builder, "   路径：%s\n", item.RelPath)
		}
		if len(item.Tags) > 0 {
			fmt.Fprintf(&builder, "   标签：%s\n", strings.Join(item.Tags, "、"))
		}
		if remark := strings.TrimSpace(item.Remark); remark != "" {
			fmt.Fprintf(&builder, "   备注：%s\n", oneLine(remark))
		}
		if stamp := displayTime(item.UpdatedAtMs); stamp != "" {
			fmt.Fprintf(&builder, "   更新时间：%s（updatedAtMs=%d）\n", stamp, int64(item.UpdatedAtMs))
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

// assetDisplayName 给出附件的最优先展示名：显示名 → 来源名 → 系统编号文件名。
func assetDisplayName(item assetItem) string {
	return firstNonEmpty(item.DisplayName, item.SourceName, item.Name, item.AssetID)
}
