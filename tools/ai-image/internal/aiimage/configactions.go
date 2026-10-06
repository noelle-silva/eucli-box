package aiimage

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"eucli-box/tools/ai-image/internal/types"
)

// runConfigList 列出配置区全部文件。
func runConfigList(input types.ToolExecutionInput) types.ToolExecutionOutput {
	root, err := configRoot(input)
	if err != nil {
		return failure("list tool config files", err, nil)
	}
	entries, err := listConfigFiles(root)
	if err != nil {
		return failure("list tool config files", err, nil)
	}
	files := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		files = append(files, map[string]any{"path": entry.Path, "bytes": entry.Bytes})
	}
	content, err := marshalCanonical(map[string]any{"files": files})
	if err != nil {
		return failure("list tool config files", err, nil)
	}
	return success(string(content), map[string]any{"action": actionConfigList, "count": len(files)})
}

// requireConfigFile 校验配置管理动作的文件参数；缺失时给出可操作的指引。
func requireConfigFile(args arguments) error {
	if args.File == "" {
		return fmt.Errorf("缺少 file 参数（配置区相对路径，如 providers.json 或 adapters/<id>.json）")
	}
	return nil
}

// runConfigRead 读取配置区单个文件；providers.json 的 apiKey 输出打码。
func runConfigRead(input types.ToolExecutionInput, args arguments) types.ToolExecutionOutput {
	if err := requireConfigFile(args); err != nil {
		return failure("read tool config file", err, nil)
	}
	root, err := configRoot(input)
	if err != nil {
		return failure("read tool config file", err, nil)
	}
	content, cleaned, err := readConfigFile(root, args.File)
	if err != nil {
		return failure("read tool config file", err, map[string]any{"file": args.File})
	}
	masked := false
	if cleaned == providersFileName {
		content, masked, err = maskProviderContent(content)
		if err != nil {
			return failure("read tool config file", err, map[string]any{"file": cleaned})
		}
	}
	return success(content, map[string]any{"action": actionConfigRead, "file": cleaned, "masked": masked})
}

// runConfigWrite 整份写入配置区文件；providers.json 与适配文件按结构校验，
// providers.json 中的打码 apiKey 统一还原为原真实 Key。
func runConfigWrite(input types.ToolExecutionInput, args arguments) types.ToolExecutionOutput {
	if err := requireConfigFile(args); err != nil {
		return failure("write tool config file", err, nil)
	}
	if args.Content == nil {
		return failure("write tool config file", fmt.Errorf("缺少 content 参数"), map[string]any{"file": args.File})
	}
	root, err := configRoot(input)
	if err != nil {
		return failure("write tool config file", err, nil)
	}
	cleaned, err := validateConfigContent(root, args.File, *args.Content)
	if err != nil {
		return failure("write tool config file", err, map[string]any{"file": args.File})
	}
	lock, err := acquireConfigWriteLock(input)
	if err != nil {
		return failure("write tool config file", err, map[string]any{"file": cleaned})
	}
	defer lock.release()
	nextContent := *args.Content
	if cleaned == providersFileName {
		nextContent, err = restoreProviderContent(root, nextContent)
		if err != nil {
			return failure("write tool config file", err, map[string]any{"file": cleaned})
		}
	}
	if _, err := writeConfigFile(root, cleaned, nextContent); err != nil {
		return failure("write tool config file", err, map[string]any{"file": cleaned})
	}
	return success("配置文件已写入："+cleaned, map[string]any{"action": actionConfigWrite, "file": cleaned})
}

// runConfigDelete 删除配置区文件。
func runConfigDelete(input types.ToolExecutionInput, args arguments) types.ToolExecutionOutput {
	if err := requireConfigFile(args); err != nil {
		return failure("delete tool config file", err, nil)
	}
	root, err := configRoot(input)
	if err != nil {
		return failure("delete tool config file", err, nil)
	}
	lock, err := acquireConfigWriteLock(input)
	if err != nil {
		return failure("delete tool config file", err, map[string]any{"file": args.File})
	}
	defer lock.release()
	cleaned, err := deleteConfigFile(root, args.File)
	if err != nil {
		return failure("delete tool config file", err, map[string]any{"file": args.File})
	}
	return success("配置文件已删除："+cleaned, map[string]any{"action": actionConfigDelete, "file": cleaned})
}

// runConfigEdit 字段级修改：点分路径 set/remove，改后整体校验并落盘；
// providers.json 中的打码 apiKey 经统一入口还原为原真实 Key。
func runConfigEdit(input types.ToolExecutionInput, args arguments) types.ToolExecutionOutput {
	if err := requireConfigFile(args); err != nil {
		return failure("edit tool config file", err, nil)
	}
	if args.Field == "" {
		return failure("edit tool config file", fmt.Errorf("缺少 field 参数"), map[string]any{"file": args.File})
	}
	if !args.HasValue && !args.Remove {
		return failure("edit tool config file", fmt.Errorf("必须提供 value 或 remove=true"), map[string]any{"file": args.File})
	}
	root, err := configRoot(input)
	if err != nil {
		return failure("edit tool config file", err, nil)
	}
	lock, err := acquireConfigWriteLock(input)
	if err != nil {
		return failure("edit tool config file", err, map[string]any{"file": args.File})
	}
	defer lock.release()
	content, cleaned, err := readConfigFile(root, args.File)
	if err != nil {
		return failure("edit tool config file", err, map[string]any{"file": args.File})
	}
	editBase := content
	if cleaned == providersFileName {
		editBase, _, err = maskProviderContent(content)
		if err != nil {
			return failure("edit tool config file", err, map[string]any{"file": cleaned})
		}
	}
	var document any
	if err := json.Unmarshal([]byte(editBase), &document); err != nil {
		return failure("edit tool config file", fmt.Errorf("配置文件不是合法 JSON: %w", err), map[string]any{"file": cleaned})
	}
	segments, err := parseFieldPath(args.Field)
	if err != nil {
		return failure("edit tool config file", err, map[string]any{"file": cleaned, "field": args.Field})
	}
	updated, err := applyFieldEdit(document, segments, args)
	if err != nil {
		return failure("edit tool config file", err, map[string]any{"file": cleaned, "field": args.Field})
	}
	encoded, err := marshalCanonical(updated)
	if err != nil {
		return failure("edit tool config file", err, map[string]any{"file": cleaned})
	}
	nextContent := string(encoded)
	if cleaned == providersFileName {
		nextContent, err = restoreProviderContent(root, nextContent)
		if err != nil {
			return failure("edit tool config file", err, map[string]any{"file": cleaned})
		}
	}
	if _, err := validateConfigContent(root, cleaned, nextContent); err != nil {
		return failure("edit tool config file", err, map[string]any{"file": cleaned, "field": args.Field})
	}
	if _, err := writeConfigFile(root, cleaned, nextContent); err != nil {
		return failure("edit tool config file", err, map[string]any{"file": cleaned})
	}
	return success("配置文件已修改："+cleaned+"（"+args.Field+"）", map[string]any{"action": actionConfigEdit, "file": cleaned, "field": args.Field})
}

// validateConfigContent 对已知配置文件按结构校验，未知文件只要求路径合法。
func validateConfigContent(root string, relPath string, content string) (string, error) {
	cleaned, err := types.CleanToolConfigRelPath(relPath)
	if err != nil {
		return "", err
	}
	switch {
	case cleaned == providersFileName:
		if _, err := parseProviderConfig(content); err != nil {
			return "", err
		}
	case isAdapterPath(cleaned):
		adapterID := adapterIDFromPath(cleaned)
		if _, err := parseAdapterFile(adapterID, content); err != nil {
			return "", err
		}
	}
	return cleaned, nil
}

// isAdapterPath 判定路径是否位于适配文件目录。
func isAdapterPath(relPath string) bool {
	return strings.HasPrefix(relPath, adaptersDirName+"/")
}

// adapterIDFromPath 从适配文件路径取适配 id（文件名去扩展名）。
func adapterIDFromPath(relPath string) string {
	base := relPath[strings.LastIndex(relPath, "/")+1:]
	return strings.TrimSuffix(base, ".json")
}

// parseFieldPath 把点分字段路径切成段；空段与空白明确失败。
func parseFieldPath(value string) ([]string, error) {
	raw := strings.Split(value, ".")
	segments := make([]string, 0, len(raw))
	for _, segment := range raw {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return nil, fmt.Errorf("字段路径包含空分段: %q", value)
		}
		segments = append(segments, segment)
	}
	return segments, nil
}

// applyFieldEdit 在 JSON 文档上按路径执行 set 或 remove；数组按十进制下标定位。
func applyFieldEdit(document any, segments []string, args arguments) (any, error) {
	if args.Remove {
		updated, removed := removeFieldAtPath(document, segments)
		if !removed {
			return nil, fmt.Errorf("字段不存在，无法删除: %s", args.Field)
		}
		return updated, nil
	}
	return setFieldAtPath(document, segments, args.Value)
}

func setFieldAtPath(document any, segments []string, value any) (any, error) {
	if len(segments) == 0 {
		return value, nil
	}
	segment := segments[0]
	switch typed := document.(type) {
	case map[string]any:
		child, err := setFieldAtPath(typed[segment], segments[1:], value)
		if err != nil {
			return nil, err
		}
		typed[segment] = child
		return typed, nil
	case []any:
		index, err := parseArrayIndex(segment, len(typed))
		if err != nil {
			return nil, err
		}
		child, err := setFieldAtPath(typed[index], segments[1:], value)
		if err != nil {
			return nil, err
		}
		typed[index] = child
		return typed, nil
	case nil:
		if isArraySegment(segments) {
			index, err := parseArrayIndex(segment, 0)
			if err != nil {
				return nil, err
			}
			items := make([]any, index+1)
			child, err := setFieldAtPath(nil, segments[1:], value)
			if err != nil {
				return nil, err
			}
			items[index] = child
			return items, nil
		}
		child, err := setFieldAtPath(nil, segments[1:], value)
		if err != nil {
			return nil, err
		}
		return map[string]any{segment: child}, nil
	default:
		return nil, fmt.Errorf("字段路径经过非对象节点: %s", segment)
	}
}

func removeFieldAtPath(document any, segments []string) (any, bool) {
	if len(segments) == 0 {
		return document, false
	}
	segment := segments[0]
	switch typed := document.(type) {
	case map[string]any:
		if len(segments) == 1 {
			if _, ok := typed[segment]; !ok {
				return document, false
			}
			delete(typed, segment)
			return typed, true
		}
		child, ok := typed[segment]
		if !ok {
			return document, false
		}
		updated, removed := removeFieldAtPath(child, segments[1:])
		if !removed {
			return document, false
		}
		typed[segment] = updated
		return typed, true
	case []any:
		index, err := parseArrayIndex(segment, len(typed))
		if err != nil {
			return document, false
		}
		if len(segments) == 1 {
			return append(typed[:index], typed[index+1:]...), true
		}
		updated, removed := removeFieldAtPath(typed[index], segments[1:])
		if !removed {
			return document, false
		}
		typed[index] = updated
		return typed, true
	default:
		return document, false
	}
}

// parseArrayIndex 解析数组下标：必须是十进制非负整数且不越界。
func parseArrayIndex(segment string, length int) (int, error) {
	index, err := strconv.Atoi(segment)
	if err != nil || index < 0 {
		return 0, fmt.Errorf("数组下标无效: %q", segment)
	}
	if index >= length {
		return 0, fmt.Errorf("数组下标越界: %d（长度 %d）", index, length)
	}
	return index, nil
}

// isArraySegment 判定下一段是否为数组下标，用于决定空节点补数组还是对象。
func isArraySegment(segments []string) bool {
	if len(segments) == 0 {
		return false
	}
	if _, err := strconv.Atoi(segments[0]); err != nil {
		return false
	}
	return true
}
