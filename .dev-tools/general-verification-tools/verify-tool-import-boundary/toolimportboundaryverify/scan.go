package verify

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ImportRef 是一处源码引用的位置事实。
type ImportRef struct {
	File   string
	Line   int
	Import string
}

// Tool 是一个 AI 工具的源码边界：名字与所在文件夹。
type Tool struct {
	Name string
	Dir  string
}

// discoverTools 列出 tools 目录下的一级子目录，即全部 AI 工具。
func discoverTools(toolsDir string) ([]Tool, error) {
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return nil, fmt.Errorf("读取工具目录失败：%w", err)
	}
	tools := make([]Tool, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		tools = append(tools, Tool{Name: entry.Name(), Dir: filepath.Join(toolsDir, entry.Name())})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, nil
}

// toolImportPrefix 返回工具源码应处的包路径前缀。
// 当前工具都是根模块的一部分；工具目录内出现独立 go.mod 说明架构变化，超出本校验约定，直接失败。
func toolImportPrefix(rootModule string, tool Tool) (string, error) {
	goModPath := filepath.Join(tool.Dir, "go.mod")
	if _, err := os.Stat(goModPath); err == nil {
		return "", fmt.Errorf("工具 %s 目录内出现独立 go.mod，超出当前校验约定", tool.Name)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return rootModule + "/tools/" + tool.Name, nil
}

// scanToolImports 扫描一个工具文件夹内的全部 Go 源码引用，返回越界引用。
func scanToolImports(repoRoot string, tool Tool, repoModules []string, allowedPrefix string) ([]ImportRef, error) {
	violations := make([]ImportRef, 0)
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(tool.Dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return fmt.Errorf("解析源码失败 %s：%w", path, parseErr)
		}
		relative, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return relErr
		}
		for _, spec := range file.Imports {
			importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				continue
			}
			if foreignImport(repoModules, allowedPrefix, importPath) {
				violations = append(violations, ImportRef{
					File:   filepath.ToSlash(relative),
					Line:   fset.Position(spec.Pos()).Line,
					Import: importPath,
				})
			}
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].File != violations[j].File {
			return violations[i].File < violations[j].File
		}
		return violations[i].Line < violations[j].Line
	})
	return violations, nil
}

// foreignImport 判断 importPath 是否引用了「本仓库内、但不在 allowedPrefix 内」的代码。
// 不匹配任何仓库内模块的引用（标准库与第三方依赖）不算违规。
func foreignImport(repoModules []string, allowedPrefix string, importPath string) bool {
	if matchingModule(repoModules, importPath) == "" {
		return false
	}
	return importPath != allowedPrefix && !strings.HasPrefix(importPath, allowedPrefix+"/")
}

// matchingModule 返回 importPath 所属的仓库内模块路径，找不到返回空串。
func matchingModule(repoModules []string, importPath string) string {
	for _, modulePath := range repoModules {
		if importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/") {
			return modulePath
		}
	}
	return ""
}

// discoverRepositoryModules 识别本仓库内的全部 Go 模块路径：工作区各 use 目录与仓库根。
// 返回按长度降序排列，保证前缀匹配取最具体的模块。
func discoverRepositoryModules(repoRoot string) ([]string, error) {
	modulePaths := make([]string, 0)
	seen := map[string]bool{}
	add := func(modulePath string) {
		modulePath = strings.TrimSpace(modulePath)
		if modulePath == "" || seen[modulePath] {
			return
		}
		seen[modulePath] = true
		modulePaths = append(modulePaths, modulePath)
	}

	goWorkPath := filepath.Join(repoRoot, "go.work")
	if _, err := os.Stat(goWorkPath); err == nil {
		dirs, parseErr := parseGoWorkUsePaths(goWorkPath)
		if parseErr != nil {
			return nil, parseErr
		}
		for _, dir := range dirs {
			modulePath, readErr := readModulePath(filepath.Join(repoRoot, filepath.FromSlash(dir), "go.mod"))
			if readErr != nil {
				return nil, readErr
			}
			add(modulePath)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	rootModule, err := readModulePath(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		return nil, err
	}
	add(rootModule)

	sort.Slice(modulePaths, func(i, j int) bool { return len(modulePaths[i]) > len(modulePaths[j]) })
	return modulePaths, nil
}

// parseGoWorkUsePaths 解析 go.work 中 use 指令列出的目录（相对工作区根）。
func parseGoWorkUsePaths(goWorkPath string) ([]string, error) {
	content, err := os.ReadFile(goWorkPath)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败：%w", goWorkPath, err)
	}
	dirs := make([]string, 0)
	inBlock := false
	for _, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(stripComment(rawLine))
		if line == "" {
			continue
		}
		if inBlock {
			if line == ")" {
				inBlock = false
				continue
			}
			dirs = append(dirs, unquoteWorkValue(line))
			continue
		}
		if line == "use (" || strings.HasPrefix(line, "use (") {
			inBlock = true
			continue
		}
		if strings.HasPrefix(line, "use ") {
			dirs = append(dirs, unquoteWorkValue(strings.TrimSpace(strings.TrimPrefix(line, "use "))))
		}
	}
	return dirs, nil
}

// readModulePath 读取 go.mod 的 module 路径。
func readModulePath(goModPath string) (string, error) {
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return "", fmt.Errorf("读取 %s 失败：%w", goModPath, err)
	}
	for _, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(stripComment(rawLine))
		if strings.HasPrefix(line, "module ") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "module ")), `"`), nil
		}
	}
	return "", fmt.Errorf("%s 缺少 module 声明", goModPath)
}

// unquoteWorkValue 取一行 go.work 值的首个字段并去掉引号。
func unquoteWorkValue(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[0], `"`)
}

// stripComment 去除行内注释。
func stripComment(line string) string {
	if index := strings.Index(line, "//"); index >= 0 {
		return line[:index]
	}
	return line
}
