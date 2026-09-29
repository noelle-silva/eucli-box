package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// templateDirectoryName 是 AI 工具基础模板在工具目录中的固定位置。
const templateDirectoryName = "_tool_template"

// templatePackageName 是模板业务动作包的固定名称。
const templatePackageName = "tooltemplate"

// scaffoldInput 是一次生成动作的全部参数。
type scaffoldInput struct {
	ID          string
	Name        string
	Description string
	Version     string
	PackageName string
}

// scaffoldResult 是一次生成动作的结果。
type scaffoldResult struct {
	Directory string
}

// scaffold 把基础模板整份复制为新工具目录，并改写全部身份信息；
// 任一环节失败即清除生成物，绝不留下半成品。
func scaffold(root string, input scaffoldInput) (scaffoldResult, error) {
	templateDirectory := filepath.Join(root, "tools", templateDirectoryName)
	if err := requireDirectory(templateDirectory, "AI 工具基础模板"); err != nil {
		return scaffoldResult{}, err
	}
	targetDirectory := filepath.Join(root, "tools", input.ID)
	if _, err := os.Stat(targetDirectory); err == nil {
		return scaffoldResult{}, fmt.Errorf("目标工具目录已存在，拒绝覆盖：%s", targetDirectory)
	} else if !os.IsNotExist(err) {
		return scaffoldResult{}, fmt.Errorf("检查目标工具目录失败：%w", err)
	}
	if err := generateTool(templateDirectory, targetDirectory, input); err != nil {
		_ = os.RemoveAll(targetDirectory)
		return scaffoldResult{}, err
	}
	return scaffoldResult{Directory: targetDirectory}, nil
}

// generateTool 完成生成动作的全部步骤：复制模板（含目录改名）与改写身份信息。
func generateTool(templateDirectory string, targetDirectory string, input scaffoldInput) error {
	if err := copyTemplate(templateDirectory, targetDirectory, input); err != nil {
		return err
	}
	return rewriteIdentity(targetDirectory, input)
}

// requireDirectory 校验路径存在且是目录。
func requireDirectory(path string, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s不可用：%w", label, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s不是目录：%s", label, path)
	}
	return nil
}

// copyTemplate 把模板目录整份复制到目标目录，并按新工具身份重命名两级目录：
// 入口目录 cmd/_tool_template 改为 cmd/<工具 ID>；
// 业务动作包 internal/tooltemplate 改为 internal/<Go 包名>。
func copyTemplate(templateDirectory string, targetDirectory string, input scaffoldInput) error {
	return filepath.WalkDir(templateDirectory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(templateDirectory, path)
		if err != nil {
			return err
		}
		target := filepath.Join(targetDirectory, renameTemplatePath(relative, input))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

// renameTemplatePath 把模板内的相对路径改写为新工具身份下的相对路径。
func renameTemplatePath(relative string, input scaffoldInput) string {
	segments := strings.Split(filepath.ToSlash(relative), "/")
	if len(segments) >= 2 {
		switch segments[0] + "/" + segments[1] {
		case "cmd/" + templateDirectoryName:
			segments[1] = input.ID
		case "internal/" + templatePackageName:
			segments[1] = input.PackageName
		}
	}
	return filepath.Join(segments...)
}

// copyFile 复制单个文件；模板内全部是文本资料，直接整份读写。
func copyFile(source string, target string) error {
	payload, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("读取模板文件失败：%w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, payload, 0o644)
}

// rewriteIdentity 改写生成目录内的全部身份信息：
// 工具定义改写身份字段，Go 源码改写导入路径与包名，说明书与更新记录按新工具重写。
func rewriteIdentity(targetDirectory string, input scaffoldInput) error {
	return filepath.WalkDir(targetDirectory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		switch {
		case filepath.Base(path) == "tool.json":
			return rewriteToolDefinition(path, input)
		case strings.HasSuffix(path, ".go"):
			return rewriteGoSource(path, input)
		case filepath.Base(path) == "README.md":
			return writeReadme(path, input)
		case filepath.Base(path) == "CHANGELOG.md":
			return writeChangelog(path, input)
		}
		return nil
	})
}

// rewriteToolDefinition 改写工具定义文件的身份字段；模板结构与预期不符时失败。
// 只替换顶层字段的值，保留模板的字段顺序与排版。
func rewriteToolDefinition(path string, input scaffoldInput) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取工具定义失败：%w", err)
	}
	updated, err := replaceTopLevelStringFields(payload, map[string]string{
		"id":                input.ID,
		"name":              input.Name,
		"description":       input.Description,
		"version":           input.Version,
		"promptDescription": input.Description,
	})
	if err != nil {
		return fmt.Errorf("改写工具定义失败：%w", err)
	}
	return os.WriteFile(path, updated, 0o644)
}

type fieldSpan struct {
	start int
	end   int
}

// replaceTopLevelStringFields 替换 JSON 对象顶层字符串字段的值：
// 每个待替换字段必须存在且为字符串，字段缺失或重复即失败；排版与其余字段原样保留。
func replaceTopLevelStringFields(payload []byte, replacements map[string]string) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, fmt.Errorf("必须是 JSON 对象")
	}
	spans := map[string]fieldSpan{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("字段名必须是字符串")
		}
		valueStart := int(decoder.InputOffset())
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}
		valueEnd := int(decoder.InputOffset())
		if _, wanted := replacements[key]; !wanted {
			continue
		}
		if _, exists := spans[key]; exists {
			return nil, fmt.Errorf("字段 %q 重复出现", key)
		}
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, fmt.Errorf("字段 %q 必须是字符串", key)
		}
		segment := payload[valueStart:valueEnd]
		openQuote := bytes.IndexByte(segment, '"')
		closeQuote := bytes.LastIndexByte(segment, '"')
		if openQuote < 0 || closeQuote < openQuote {
			return nil, fmt.Errorf("无法定位字段 %q 的值", key)
		}
		spans[key] = fieldSpan{start: valueStart + openQuote, end: valueStart + closeQuote + 1}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(replacements))
	for key := range replacements {
		if _, ok := spans[key]; !ok {
			return nil, fmt.Errorf("缺少字段 %q", key)
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i int, j int) bool { return spans[keys[i]].start > spans[keys[j]].start })
	result := payload
	for _, key := range keys {
		span := spans[key]
		encoded, err := json.Marshal(replacements[key])
		if err != nil {
			return nil, err
		}
		next := make([]byte, 0, len(result)-(span.end-span.start)+len(encoded))
		next = append(next, result[:span.start]...)
		next = append(next, encoded...)
		next = append(next, result[span.end:]...)
		result = next
	}
	return result, nil
}

// rewriteGoSource 改写 Go 源码中的模板身份引用：导入路径与新包名。
// 替换按词边界进行：新工具 ID 或包名本身包含模板词时，不会发生二次改写。
func rewriteGoSource(path string, input scaffoldInput) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated := string(payload)
	updated = strings.ReplaceAll(updated, "eucli-box/tools/"+templateDirectoryName+"/", "eucli-box/tools/"+input.ID+"/")
	updated = replaceWord(updated, templatePackageName, input.PackageName)
	return os.WriteFile(path, []byte(updated), 0o644)
}

// replaceWord 按标识符词边界替换文本：模板词作为独立标识符出现时才替换。
func replaceWord(source string, previous string, next string) string {
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(previous) + `\b`)
	return pattern.ReplaceAllString(source, next)
}

// writeReadme 为新工具写入说明书首版：身份与描述就位，动作说明待实现时补充。
func writeReadme(path string, input scaffoldInput) error {
	content := fmt.Sprintf("# %s\n\n%s\n\n本工具由 AI 工具模板生成；实现业务动作后，请在此补充工具定位、动作说明与边界。\n", input.ID, input.Description)
	return os.WriteFile(path, []byte(content), 0o644)
}

// writeChangelog 为新工具写入更新记录首版：登记建立事实与初始版本。
func writeChangelog(path string, input scaffoldInput) error {
	content := fmt.Sprintf("# 更新记录\n\n## %s - %s\n\n- 从 AI 工具模板建立 %s。\n", input.Version, time.Now().Format("2006-01-02"), input.ID)
	return os.WriteFile(path, []byte(content), 0o644)
}
