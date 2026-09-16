package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const manifestFileName = "fw-app.package.json"
const manifestSchemaVersion = 1

var (
	safeIDPattern        = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	iconExtensions       = map[string]bool{".png": true, ".svg": true}
	displayModes         = map[string]bool{"default": true, "window": true, "top": true}
	formalVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
)

type manifest struct {
	SchemaVersion int            `json:"schemaVersion"`
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	VersionSource string         `json:"versionSource"`
	Package       packageSection `json:"package"`
	Service       string         `json:"service"`
	DisplayMode   string         `json:"displayMode"`
	Commands      []command      `json:"commands"`
}

type packageSection struct {
	WindowsExecutable string `json:"windowsExecutable"`
	Icon              string `json:"icon"`
}

type command struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type report struct {
	ID         string
	Version    string
	Executable string
	Icon       string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "清单校验失败："+err.Error())
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	manifestPath, err := parseOptions(args)
	if err != nil {
		return err
	}
	result, err := checkManifest(manifestPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "%s %s：清单校验通过（主程序 %s，图标 %s）。\n", result.ID, result.Version, result.Executable, result.Icon)
	return nil
}

func parseOptions(args []string) (string, error) {
	flags := flag.NewFlagSet("package-check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	manifestPath := flags.String("manifest", manifestFileName, "清单文件路径")
	if err := flags.Parse(args); err != nil {
		return "", fmt.Errorf("参数无效：%w", err)
	}
	if flags.NArg() != 0 {
		return "", errors.New("不接受位置参数")
	}
	value := strings.TrimSpace(*manifestPath)
	if value == "" {
		return "", errors.New("-manifest 不能为空")
	}
	return value, nil
}

func checkManifest(manifestPath string) (report, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(manifestPath))
	if err != nil {
		return report{}, fmt.Errorf("确定清单路径失败：%w", err)
	}
	parsed, err := readManifest(absolute)
	if err != nil {
		return report{}, err
	}
	if parsed.SchemaVersion != manifestSchemaVersion {
		return report{}, fmt.Errorf("schemaVersion 必须为 %d，当前为 %d", manifestSchemaVersion, parsed.SchemaVersion)
	}
	id := strings.TrimSpace(parsed.ID)
	if !safeIDPattern.MatchString(id) {
		return report{}, fmt.Errorf("id 不合法：%s", id)
	}
	if strings.TrimSpace(parsed.Name) == "" {
		return report{}, errors.New("name 不能为空")
	}
	if strings.TrimSpace(parsed.Description) == "" {
		return report{}, errors.New("description 不能为空")
	}
	if !displayModes[strings.TrimSpace(parsed.DisplayMode)] {
		return report{}, errors.New("displayMode 必须为 default、window 或 top")
	}
	executable, err := normalizeRelativePath(parsed.Package.WindowsExecutable, "package.windowsExecutable")
	if err != nil {
		return report{}, err
	}
	if !strings.HasSuffix(strings.ToLower(executable), ".exe") {
		return report{}, errors.New("package.windowsExecutable 必须指向 .exe")
	}
	icon, err := normalizeRelativePath(parsed.Package.Icon, "package.icon")
	if err != nil {
		return report{}, err
	}
	if !iconExtensions[strings.ToLower(filepath.Ext(icon))] {
		return report{}, errors.New("package.icon 只支持 .png 或 .svg 图标")
	}
	if err := checkCommands(parsed.Commands); err != nil {
		return report{}, err
	}
	versionSource, err := normalizeRelativePath(parsed.VersionSource, "versionSource")
	if err != nil {
		return report{}, err
	}
	root := filepath.Dir(filepath.Dir(absolute))
	version, err := readVersionSource(filepath.Join(root, filepath.FromSlash(versionSource)))
	if err != nil {
		return report{}, err
	}
	if err := requireFile(filepath.Join(root, filepath.FromSlash(icon)), "图标"); err != nil {
		return report{}, err
	}
	if service := strings.TrimSpace(parsed.Service); service != "" {
		servicePath, err := normalizeRelativePath(service, "service")
		if err != nil {
			return report{}, err
		}
		if err := requireFile(filepath.Join(root, filepath.FromSlash(servicePath)), "服务声明"); err != nil {
			return report{}, err
		}
	}
	return report{ID: id, Version: version, Executable: executable, Icon: icon}, nil
}

func readManifest(path string) (manifest, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return manifest{}, fmt.Errorf("读取清单文件失败：%w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	var parsed manifest
	if err := decoder.Decode(&parsed); err != nil {
		return manifest{}, fmt.Errorf("解析清单文件失败：%w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return manifest{}, errors.New("解析清单文件失败：JSON 后存在多余内容")
		}
		return manifest{}, fmt.Errorf("解析清单文件失败：%w", err)
	}
	return parsed, nil
}

func readVersionSource(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取 versionSource 失败：%w", err)
	}
	var info struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(payload, &info); err != nil {
		return "", fmt.Errorf("versionSource 必须是 JSON 对象：%w", err)
	}
	version := strings.TrimSpace(info.Version)
	if !formalVersionPattern.MatchString(version) {
		return "", errors.New("versionSource 版本无效：必须是三段正式版本，例如 0.1.0")
	}
	return version, nil
}

func checkCommands(commands []command) error {
	seen := map[string]bool{}
	for index, item := range commands {
		id := strings.TrimSpace(item.ID)
		title := strings.TrimSpace(item.Title)
		if title == "" {
			return fmt.Errorf("commands[%d].title 不能为空", index)
		}
		if len([]rune(title)) > 80 {
			return fmt.Errorf("commands[%d].title 不能超过 80 字", index)
		}
		if !safeIDPattern.MatchString(id) {
			return fmt.Errorf("commands[%d].id 不合法：%s", index, id)
		}
		if seen[id] {
			return fmt.Errorf("commands[%d].id 重复：%s", index, id)
		}
		seen[id] = true
	}
	return nil
}

func normalizeRelativePath(value string, field string) (string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if normalized == "" {
		return "", fmt.Errorf("%s 不能为空", field)
	}
	if filepath.IsAbs(normalized) || strings.HasPrefix(normalized, "/") || (len(normalized) >= 2 && normalized[1] == ':') {
		return "", fmt.Errorf("%s 不允许是绝对路径：%s", field, normalized)
	}
	for _, segment := range strings.Split(normalized, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("%s 不安全：%s", field, normalized)
		}
	}
	return normalized, nil
}

func requireFile(path string, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s 文件不存在：%s", label, path)
	}
	if info.IsDir() {
		return fmt.Errorf("%s 必须是文件：%s", label, path)
	}
	if info.Size() == 0 {
		return fmt.Errorf("%s 文件为空：%s", label, path)
	}
	return nil
}
