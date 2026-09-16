package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const protocolFileName = "fast-window-dev-protocol.json"

const (
	contractVersion = 1
	receiptPrefix   = "FAST-WINDOW-DEV-RECEIPT: "

	statusSucceeded = "succeeded"
	statusFailed    = "failed"

	storeManifestFileName        = "fw-app.package.json"
	storeRuntimeManifestFileName = "fw-app.json"
	storePackageDirName          = "dist"
	storeIconDirName             = "assets"
)

type actionDefinition struct {
	Command      string            `json:"command"`
	Artifact     map[string]string `json:"artifact"`
	StorePackage bool              `json:"storePackage"`
}

// UnmarshalJSON 同时接受两种动作形态：纯命令字符串，或带产物与商店化声明的对象。
func (definition *actionDefinition) UnmarshalJSON(payload []byte) error {
	var text string
	if err := json.Unmarshal(payload, &text); err == nil {
		definition.Command = text
		return nil
	}
	var parsed struct {
		Command      string            `json:"command"`
		Artifact     map[string]string `json:"artifact"`
		StorePackage bool              `json:"storePackage"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return err
	}
	definition.Command = parsed.Command
	definition.Artifact = parsed.Artifact
	definition.StorePackage = parsed.StorePackage
	return nil
}

type protocol struct {
	Actions map[string]actionDefinition `json:"actions"`
}

type receiptData struct {
	Result   any               `json:"result"`
	Artifact map[string]string `json:"artifact"`
}

type receipt struct {
	ContractVersion int         `json:"contractVersion"`
	Action          string      `json:"action"`
	Status          string      `json:"status"`
	ExitCode        *int        `json:"exitCode"`
	Error           string      `json:"error"`
	Data            receiptData `json:"data"`
}

type storeManifest struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	VersionSource string                 `json:"versionSource"`
	Package       storeManifestPackage   `json:"package"`
	Service       string                 `json:"service"`
	DisplayMode   string                 `json:"displayMode"`
	Commands      []storeManifestCommand `json:"commands"`
}

type storeManifestPackage struct {
	WindowsExecutable string `json:"windowsExecutable"`
	Icon              string `json:"icon"`
}

type storeManifestCommand struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type storeRuntimeManifest struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Version           string                 `json:"version"`
	WindowsExecutable string                 `json:"windowsExecutable"`
	Icon              string                 `json:"icon,omitempty"`
	Service           string                 `json:"service,omitempty"`
	DisplayMode       string                 `json:"displayMode,omitempty"`
	Commands          []storeManifestCommand `json:"commands,omitempty"`
}

func main() {
	os.Exit(runMain(os.Args[1:], protocolFileName, os.Stdout))
}

func runMain(args []string, protocolFile string, output io.Writer) int {
	result := executeAction(args, protocolFile)
	if err := writeReceipt(output, result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if result.Status == statusSucceeded {
		return 0
	}
	return 1
}

func executeAction(args []string, protocolFile string) receipt {
	if len(args) != 1 {
		return failureReceipt("", errors.New("用法：fast-window-dev-tool <动作代号>"))
	}
	action := strings.TrimSpace(args[0])
	parsed, err := readProtocol(protocolFile)
	if err != nil {
		return failureReceipt(action, err)
	}
	definition, ok := parsed.Actions[action]
	if !ok {
		return failureReceipt(action, fmt.Errorf("协议未定义动作 %q（可用动作：%s）", action, strings.Join(actionNames(parsed.Actions), ", ")))
	}
	if strings.TrimSpace(definition.Command) == "" {
		return failureReceipt(action, fmt.Errorf("协议动作 %q 的命令为空", action))
	}
	exitCode, result, artifact, err := runCommand(definition)
	if err != nil {
		return failureReceipt(action, err)
	}
	if definition.StorePackage && exitCode == 0 {
		packaged, err := buildStorePackage(filepath.Dir(protocolFile), artifact)
		if err != nil {
			return failureReceipt(action, err)
		}
		artifact = packaged
	}
	status := statusSucceeded
	if exitCode != 0 {
		status = statusFailed
	}
	return receipt{
		ContractVersion: contractVersion,
		Action:          action,
		Status:          status,
		ExitCode:        &exitCode,
		Data: receiptData{
			Result:   result,
			Artifact: artifact,
		},
	}
}

func failureReceipt(action string, cause error) receipt {
	return receipt{
		ContractVersion: contractVersion,
		Action:          action,
		Status:          statusFailed,
		Error:           cause.Error(),
		Data:            receiptData{Artifact: map[string]string{}},
	}
}

func readProtocol(file string) (protocol, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return protocol{}, fmt.Errorf("读取协议文件失败：%w", err)
	}
	var parsed protocol
	if err := json.Unmarshal(data, &parsed); err != nil {
		return protocol{}, fmt.Errorf("解析协议文件失败：%w", err)
	}
	return parsed, nil
}

func actionNames(actions map[string]actionDefinition) []string {
	names := make([]string, 0, len(actions))
	for name := range actions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func runCommand(definition actionDefinition) (int, any, map[string]string, error) {
	var captured bytes.Buffer
	exitCode, err := runShellCommand(definition.Command, &captured)
	if err != nil {
		return 0, nil, nil, err
	}
	result := parseJSONOutput(captured.Bytes())
	return exitCode, result, extractArtifact(definition.Artifact, result), nil
}

// runShellCommand 执行 shell 命令并捕获标准输出；Windows 上命令原文写入临时脚本再执行，
// 避免命令行参数转义破坏引号等 shell 语法。
func runShellCommand(source string, captured *bytes.Buffer) (int, error) {
	var command *exec.Cmd
	cleanup := func() {}
	if runtime.GOOS == "windows" {
		scriptPath, err := writeCommandScript(source)
		if err != nil {
			return 0, err
		}
		cleanup = func() { _ = os.Remove(scriptPath) }
		command = exec.Command("cmd", "/c", scriptPath)
	} else {
		command = exec.Command("sh", "-c", source)
	}
	defer cleanup()
	command.Stdout = io.MultiWriter(os.Stdout, captured)
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			return 0, fmt.Errorf("执行动作失败：%w", err)
		}
	}
	return command.ProcessState.ExitCode(), nil
}

func writeCommandScript(source string) (string, error) {
	file, err := os.CreateTemp("", "fast-window-dev-command-*.cmd")
	if err != nil {
		return "", fmt.Errorf("创建命令脚本失败：%w", err)
	}
	path := file.Name()
	if _, err := file.WriteString("@echo off\r\n" + source + "\r\n"); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("写入命令脚本失败：%w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("关闭命令脚本失败：%w", err)
	}
	return path, nil
}

func parseJSONOutput(payload []byte) any {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil
	}
	var parsed any
	if err := json.Unmarshal(trimmed, &parsed); err != nil {
		return nil
	}
	return parsed
}

func extractArtifact(declaration map[string]string, result any) map[string]string {
	artifact := map[string]string{}
	for field, sourcePath := range declaration {
		if value, ok := valueAtPath(result, sourcePath); ok {
			artifact[field] = value
		}
	}
	return artifact
}

func valueAtPath(root any, dotted string) (string, bool) {
	current := root
	for _, segment := range strings.Split(dotted, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		value, ok := object[segment]
		if !ok {
			return "", false
		}
		current = value
	}
	text, ok := current.(string)
	if !ok {
		return "", false
	}
	return text, true
}

// buildStorePackage 把基础成品包加工成商店包：解包后写入运行清单与图标，再重打包到协议目录的产出区。
// 运行清单内容全部来自同目录的商店打包清单与应用的版本文件，不依赖应用本体的任何改造。
func buildStorePackage(protocolDir string, artifact map[string]string) (map[string]string, error) {
	protocolDir, err := filepath.Abs(strings.TrimSpace(protocolDir))
	if err != nil {
		return nil, fmt.Errorf("确定协议目录失败：%w", err)
	}
	sourcePath := strings.TrimSpace(artifact["path"])
	if sourcePath == "" {
		return nil, errors.New("商店化需要成品路径：命令输出没有提供 artifact.path")
	}
	sourcePath, err = filepath.Abs(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("确定成品路径失败：%w", err)
	}
	root := filepath.Dir(protocolDir)
	manifest, err := readStoreManifest(filepath.Join(protocolDir, storeManifestFileName))
	if err != nil {
		return nil, err
	}
	version, err := readStoreVersion(root, manifest.VersionSource)
	if err != nil {
		return nil, err
	}
	executable, err := resolveManifestPath(manifest.Package.WindowsExecutable, "package.windowsExecutable")
	if err != nil {
		return nil, err
	}
	iconRelative, err := resolveManifestPath(manifest.Package.Icon, "package.icon")
	if err != nil {
		return nil, err
	}
	iconSource := filepath.Join(root, filepath.FromSlash(iconRelative))
	if info, statErr := os.Stat(iconSource); statErr != nil || info.IsDir() {
		return nil, fmt.Errorf("图标文件不存在：%s", iconRelative)
	}

	tempDir, err := os.MkdirTemp("", "fast-window-dev-store-*")
	if err != nil {
		return nil, fmt.Errorf("建立商店化临时区失败：%w", err)
	}
	defer os.RemoveAll(tempDir)

	if err := unpackZip(sourcePath, tempDir); err != nil {
		return nil, err
	}
	iconTarget := path.Join(storeIconDirName, path.Base(iconRelative))
	if err := copyFileTo(filepath.Join(tempDir, filepath.FromSlash(iconTarget)), iconSource); err != nil {
		return nil, err
	}
	serviceTarget := ""
	if strings.TrimSpace(manifest.Service) != "" {
		serviceRelative, err := resolveManifestPath(manifest.Service, "service")
		if err != nil {
			return nil, err
		}
		serviceSource := filepath.Join(root, filepath.FromSlash(serviceRelative))
		if info, statErr := os.Stat(serviceSource); statErr != nil || info.IsDir() {
			return nil, fmt.Errorf("服务声明文件不存在：%s", serviceRelative)
		}
		serviceTarget = path.Base(serviceRelative)
		if err := copyFileTo(filepath.Join(tempDir, serviceTarget), serviceSource); err != nil {
			return nil, err
		}
	}
	runtimeManifest := storeRuntimeManifest{
		ID:                manifest.ID,
		Name:              manifest.Name,
		Version:           version,
		WindowsExecutable: executable,
		Icon:              iconTarget,
		Service:           serviceTarget,
		DisplayMode:       manifest.DisplayMode,
		Commands:          manifest.Commands,
	}
	if err := writeStoreRuntimeManifest(tempDir, runtimeManifest); err != nil {
		return nil, err
	}

	distDir := filepath.Join(protocolDir, storePackageDirName)
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		return nil, fmt.Errorf("建立商店包产出目录失败：%w", err)
	}
	targetPath := filepath.Join(distDir, filepath.Base(sourcePath))
	if err := packZipDirectory(tempDir, targetPath); err != nil {
		return nil, err
	}
	sum, err := sha256OfFile(targetPath)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"path":   targetPath,
		"name":   filepath.Base(targetPath),
		"sha256": sum,
	}, nil
}

func readStoreManifest(file string) (storeManifest, error) {
	payload, err := os.ReadFile(file)
	if err != nil {
		return storeManifest{}, fmt.Errorf("读取商店清单失败：%w", err)
	}
	var manifest storeManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return storeManifest{}, fmt.Errorf("解析商店清单失败：%w", err)
	}
	if strings.TrimSpace(manifest.ID) == "" {
		return storeManifest{}, errors.New("商店清单缺少 id")
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return storeManifest{}, errors.New("商店清单缺少 name")
	}
	if strings.TrimSpace(manifest.VersionSource) == "" {
		return storeManifest{}, errors.New("商店清单缺少 versionSource")
	}
	return manifest, nil
}

func readStoreVersion(root string, versionSource string) (string, error) {
	relative, err := resolveManifestPath(versionSource, "versionSource")
	if err != nil {
		return "", err
	}
	payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return "", fmt.Errorf("读取 versionSource 失败：%w", err)
	}
	var info struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(payload, &info); err != nil {
		return "", fmt.Errorf("解析 versionSource 失败：%w", err)
	}
	version := strings.TrimSpace(info.Version)
	if version == "" {
		return "", errors.New("versionSource 缺少 version")
	}
	return version, nil
}

func resolveManifestPath(value string, field string) (string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if normalized == "" {
		return "", fmt.Errorf("%s 不能为空", field)
	}
	if path.IsAbs(normalized) || filepath.IsAbs(normalized) || (len(normalized) >= 2 && normalized[1] == ':') {
		return "", fmt.Errorf("%s 不允许是绝对路径：%s", field, normalized)
	}
	for _, segment := range strings.Split(normalized, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("%s 不安全：%s", field, normalized)
		}
	}
	return normalized, nil
}

func unpackZip(archivePath string, targetDir string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("打开成品包失败：%w", err)
	}
	defer reader.Close()
	cleanTarget := filepath.Clean(targetDir)
	for _, entry := range reader.File {
		cleanName := path.Clean(strings.ReplaceAll(entry.Name, "\\", "/"))
		if cleanName == "." || cleanName == "" {
			continue
		}
		if path.IsAbs(cleanName) || cleanName == ".." || strings.HasPrefix(cleanName, "../") || (len(cleanName) >= 2 && cleanName[1] == ':') {
			return fmt.Errorf("成品包含不安全路径：%s", entry.Name)
		}
		destination := filepath.Join(cleanTarget, filepath.FromSlash(cleanName))
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return fmt.Errorf("解包目录失败：%w", err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fmt.Errorf("解包目录失败：%w", err)
		}
		if err := extractZipEntry(entry, destination); err != nil {
			return err
		}
	}
	return nil
}

func extractZipEntry(entry *zip.File, destination string) error {
	source, err := entry.Open()
	if err != nil {
		return fmt.Errorf("读取成品包条目失败：%w", err)
	}
	defer source.Close()
	target, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("写入成品包条目失败：%w", err)
	}
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		return fmt.Errorf("写入成品包条目失败：%w", err)
	}
	if err := target.Close(); err != nil {
		return fmt.Errorf("写入成品包条目失败：%w", err)
	}
	return nil
}

func packZipDirectory(sourceDir string, targetPath string) error {
	file, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("创建商店包失败：%w", err)
	}
	writer := zip.NewWriter(file)
	walkErr := filepath.WalkDir(sourceDir, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(sourceDir, filePath)
		if err != nil {
			return err
		}
		entryWriter, err := writer.Create(filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		payload, err := os.Open(filePath)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(entryWriter, payload)
		closeErr := payload.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if walkErr == nil {
		walkErr = writer.Close()
	} else {
		_ = writer.Close()
	}
	closeErr := file.Close()
	if walkErr != nil {
		_ = os.Remove(targetPath)
		return fmt.Errorf("打包商店包失败：%w", walkErr)
	}
	if closeErr != nil {
		_ = os.Remove(targetPath)
		return fmt.Errorf("保存商店包失败：%w", closeErr)
	}
	return nil
}

func copyFileTo(target string, source string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("建立图标目录失败：%w", err)
	}
	payload, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("读取图标文件失败：%w", err)
	}
	if err := os.WriteFile(target, payload, 0o644); err != nil {
		return fmt.Errorf("写入图标文件失败：%w", err)
	}
	return nil
}

func writeStoreRuntimeManifest(packageRoot string, manifest storeRuntimeManifest) error {
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("生成 fw-app.json 失败：%w", err)
	}
	payload = append(payload, '\n')
	target := filepath.Join(packageRoot, storeRuntimeManifestFileName)
	if err := os.WriteFile(target, payload, 0o644); err != nil {
		return fmt.Errorf("写入 fw-app.json 失败：%w", err)
	}
	return nil
}

func sha256OfFile(file string) (string, error) {
	source, err := os.Open(file)
	if err != nil {
		return "", fmt.Errorf("读取商店包失败：%w", err)
	}
	defer source.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, source); err != nil {
		return "", fmt.Errorf("计算商店包摘要失败：%w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeReceipt(output io.Writer, value receipt) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("生成回执失败：%w", err)
	}
	_, err = fmt.Fprintln(output, receiptPrefix+string(payload))
	return err
}
