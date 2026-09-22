package releaseartifact

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/systemplugin"
	"eucli-box/pkg/types"
)

type VerifyOptions struct {
	ArchivePath  string
	ManifestPath string
	Workspace    string
	Timeout      time.Duration
}

type VerifyResult struct {
	Manifest types.ReleaseManifest
	Evidence string
}

// VerifyProductOptions 是只使用官方索引产品记录的远端复核参数。
type VerifyProductOptions struct {
	ArchivePath string
	Product     types.ReleaseProductRecord
	Workspace   string
	Timeout     time.Duration
}

type VerifyProductResult struct {
	Product  types.ReleaseProductRecord
	Evidence string
}

func Verify(ctx context.Context, options VerifyOptions) (VerifyResult, error) {
	if ctx == nil {
		return VerifyResult{}, fmt.Errorf("验收上下文不能为空")
	}
	if options.Timeout <= 0 {
		options.Timeout = 45 * time.Second
	}
	archivePath, err := absoluteRegularFile(options.ArchivePath, "压缩包")
	if err != nil {
		return VerifyResult{}, err
	}
	manifestPath, err := absoluteRegularFile(options.ManifestPath, "发行清单")
	if err != nil {
		return VerifyResult{}, err
	}
	workspace, evidenceDir, environmentDir, tempDir, err := verifyWorkspace(options.Workspace)
	if err != nil {
		return VerifyResult{}, err
	}
	_ = workspace
	manifestPayload, err := os.ReadFile(manifestPath)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("读取发行清单失败：%w", err)
	}
	manifest, err := release.DecodeReleaseManifest(manifestPayload)
	if err != nil {
		return VerifyResult{}, err
	}
	archiveRecord, err := release.CollectFileRecords(filepath.Dir(archivePath))
	if err != nil {
		return VerifyResult{}, fmt.Errorf("读取压缩包资料失败：%w", err)
	}
	foundArchive := false
	for _, record := range archiveRecord {
		if record.Name == filepath.Base(archivePath) {
			foundArchive = true
			if record.Size != manifest.Archive.Size || record.SHA256 != manifest.Archive.SHA256 {
				return VerifyResult{}, fmt.Errorf("压缩包完整性与发行清单不一致")
			}
		}
	}
	if !foundArchive {
		return VerifyResult{}, fmt.Errorf("压缩包资料缺失")
	}
	product := types.ReleaseProductRecord{
		SchemaVersion:  manifest.SchemaVersion,
		Artifact:       manifest.Artifact,
		Version:        manifest.Version,
		Platform:       manifest.Platform,
		OfficialSource: manifest.OfficialSource,
		Compatibility:  manifest.Compatibility,
		Source:         manifest.Source,
		DataVersion:    manifest.DataVersion,
		ExternalAssets: manifest.ExternalAssets,
	}
	evidence, err := verifyProductContent(ctx, archivePath, product, evidenceDir, environmentDir, tempDir, options.Timeout)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{Manifest: manifest, Evidence: evidence}, nil
}

// VerifyProduct 只依据官方索引的产品记录复核一个已下载压缩包：
// 解包、包内身份核对和真实启动验收。压缩包摘要已在下载时核对完成。
func VerifyProduct(ctx context.Context, options VerifyProductOptions) (VerifyProductResult, error) {
	if ctx == nil {
		return VerifyProductResult{}, fmt.Errorf("验收上下文不能为空")
	}
	if err := release.ValidateReleaseProductRecord(options.Product); err != nil {
		return VerifyProductResult{}, err
	}
	if options.Timeout <= 0 {
		options.Timeout = 45 * time.Second
	}
	archivePath, err := absoluteRegularFile(options.ArchivePath, "压缩包")
	if err != nil {
		return VerifyProductResult{}, err
	}
	_, evidenceDir, environmentDir, tempDir, err := verifyWorkspace(options.Workspace)
	if err != nil {
		return VerifyProductResult{}, err
	}
	evidence, err := verifyProductContent(ctx, archivePath, options.Product, evidenceDir, environmentDir, tempDir, options.Timeout)
	if err != nil {
		return VerifyProductResult{}, err
	}
	return VerifyProductResult{Product: options.Product, Evidence: evidence}, nil
}

// verifyWorkspace 建立一次验收的隔离工作区并返回关键子目录。
func verifyWorkspace(workspaceValue string) (string, string, string, string, error) {
	workspace, err := filepath.Abs(strings.TrimSpace(workspaceValue))
	if err != nil || strings.TrimSpace(workspaceValue) == "" {
		return "", "", "", "", fmt.Errorf("验收工作区无效")
	}
	if err := os.RemoveAll(workspace); err != nil {
		return "", "", "", "", fmt.Errorf("清理验收工作区失败：%w", err)
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return "", "", "", "", fmt.Errorf("建立验收工作区失败：%w", err)
	}
	// 解包内容直接落在工作区根，因此工作区根必须先保持为空；
	// evidence/environment/temp 三个辅助子目录在解包完成后建立。
	evidenceDir := filepath.Join(workspace, "evidence")
	environmentDir := filepath.Join(workspace, "environment")
	tempDir := filepath.Join(workspace, "temp")
	return workspace, evidenceDir, environmentDir, tempDir, nil
}

// verifyProductContent 完成解包、包内核对、真实启动验收并写下验收证据。
func verifyProductContent(ctx context.Context, archivePath string, product types.ReleaseProductRecord, evidenceDir string, environmentDir string, tempDir string, timeout time.Duration) (string, error) {
	// 解包目标直接是验收工作区根，避免深嵌套使 Windows 路径超过 260 字符限制
	// （scipy 等深层依赖在超长路径下无法访问，导致导入竞态）。
	extracted := filepath.Dir(evidenceDir)
	if err := os.MkdirAll(extracted, 0o755); err != nil {
		return "", err
	}
	if err := release.ExtractArchive(release.ExtractArchiveOptions{ArchivePath: archivePath, TargetDir: extracted}); err != nil {
		return "", err
	}
	for _, directory := range []string{evidenceDir, environmentDir, tempDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return "", err
		}
	}
	if _, err := release.ValidateExtractedPackage(release.ValidateExtractedPackageOptions{Directory: extracted, Product: product}); err != nil {
		return "", fmt.Errorf("解包后的成品边界无效：%w", err)
	}
	if err := launchCheck(ctx, product.Artifact, extracted, environmentDir, tempDir, evidenceDir, timeout); err != nil {
		return "", err
	}
	if err := writeJSON(filepath.Join(evidenceDir, "verification-result.json"), map[string]any{
		"status":   "passed",
		"artifact": product.Artifact,
		"version":  product.Version,
		"platform": product.Platform,
	}); err != nil {
		return "", err
	}
	return evidenceDir, nil
}

func launchCheck(parent context.Context, identity types.ReleaseArtifactIdentity, directory string, environment string, temp string, evidence string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	switch identity.Kind {
	case types.ReleaseArtifactKindTool:
		return launchTool(ctx, directory, environment, temp, evidence)
	case types.ReleaseArtifactKindPlugin:
		return launchPlugin(ctx, directory, identity.ID, evidence)
	default:
		return fmt.Errorf("无法验收未知发布物类别 %q", identity.Kind)
	}
}

func launchTool(ctx context.Context, directory string, environment string, temp string, evidence string) error {
	payload, err := os.ReadFile(filepath.Join(directory, "definition.json"))
	if err != nil {
		return err
	}
	var definition types.ToolDefinition
	if err := json.Unmarshal(payload, &definition); err != nil {
		return err
	}
	executable := ""
	for _, binary := range definition.Binaries {
		if binary.GOOS == "windows" && binary.GOARCH == "amd64" {
			executable = binary.Path
			break
		}
	}
	if executable == "" {
		return fmt.Errorf("工具没有 Windows x64 可执行文件")
	}
	cmd := exec.CommandContext(ctx, filepath.Join(directory, filepath.FromSlash(executable)))
	cmd.Dir = directory
	arguments := `{}`
	requireSuccess := false
	switch definition.ID {
	case "shell_command":
		arguments = `{"command":"printf release-verification","provider":"git-bash"}`
		requireSuccess = true
	}
	toolDataDir := filepath.Join(environment, "tool-data")
	for _, path := range []string{toolDataDir, temp} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	cmd.Env = replaceEnvironment(os.Environ(), map[string]string{"TEMP": temp, "TMP": temp})
	cmd.Stdin = strings.NewReader(`{"actionId":"release-verification","toolName":"` + definition.ID + `","arguments":` + arguments + `,"userConfig":{},"defaultConfig":{},"toolBodyDirectory":"` + escapeJSON(directory) + `","toolDataDirectory":"` + escapeJSON(toolDataDir) + `","hostWorkingDirectory":"` + escapeJSON(directory) + `"}`)
	return captureJSONProcess(cmd, filepath.Join(evidence, "tool.stdout.json"), filepath.Join(evidence, "tool.stderr.log"), requireSuccess)
}

// launchPlugin 对插件执行真实交接验收：启动进程、完成身份与能力握手、
// 完成一次空接口能力调用，并按限时停机退出。插件对空配置返回结构化失败同样通过。
func launchPlugin(ctx context.Context, directory string, id string, evidence string) error {
	manifestPayload, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		return err
	}
	var manifest types.SystemPluginManifest
	if err := json.Unmarshal(manifestPayload, &manifest); err != nil {
		return fmt.Errorf("插件身份声明无效：%w", err)
	}
	expected := make([]systemplugin.Capability, 0, len(manifest.Capabilities))
	for _, capability := range manifest.Capabilities {
		interfaces := make([]string, 0, len(capability.Interfaces))
		for _, item := range capability.Interfaces {
			interfaces = append(interfaces, item.ID)
		}
		expected = append(expected, systemplugin.Capability{Type: capability.Type, Interfaces: interfaces})
	}
	dataDirectory, err := os.MkdirTemp("", "eucli-plugin-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dataDirectory)
	cmd := exec.CommandContext(ctx, filepath.Join(directory, "binary", id+".exe"))
	cmd.Dir = directory
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("进程启动验收失败：%w", err)
	}
	encoder := json.NewEncoder(stdin)
	decoder := json.NewDecoder(stdout)
	fail := func(cause error) error {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		writeEvidence(evidence, map[string]any{"status": "failed", "error": cause.Error()}, stderr.Bytes())
		return cause
	}
	hello := systemplugin.NewMessage(systemplugin.MessageHello)
	hello.PluginID = id
	hello.DataDirectory = dataDirectory
	if err := encoder.Encode(hello); err != nil {
		return fail(fmt.Errorf("插件控制通道写入失败：%w", err))
	}
	ready, err := readMessageWithContext(ctx, decoder)
	if err != nil {
		return fail(fmt.Errorf("插件未完成握手：%w", err))
	}
	if err := systemplugin.ValidateReady(ready, id, expected); err != nil {
		return fail(fmt.Errorf("插件握手校验失败：%w", err))
	}
	record := map[string]any{"status": "passed", "ready": ready}
	if len(manifestPlaceholderInterfaces(manifest)) > 0 {
		invoke := systemplugin.NewMessage(systemplugin.MessageInvoke)
		invoke.RequestID = "release-verification"
		invoke.Capability = systemplugin.CapabilityPlaceholderValues
		if err := encoder.Encode(invoke); err != nil {
			return fail(fmt.Errorf("插件能力调用写入失败：%w", err))
		}
		result, err := readMessageWithContext(ctx, decoder)
		if err != nil {
			return fail(fmt.Errorf("插件没有返回能力结果：%w", err))
		}
		if result.Type != systemplugin.MessageResult || result.RequestID != invoke.RequestID {
			return fail(fmt.Errorf("插件返回了不符合协议的能力结果"))
		}
		record["result"] = result
	}
	_ = encoder.Encode(systemplugin.NewMessage(systemplugin.MessageStop))
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
	}
	writeEvidence(evidence, record, stderr.Bytes())
	return nil
}

func manifestPlaceholderInterfaces(manifest types.SystemPluginManifest) []types.SystemPluginPlaceholderInterface {
	for _, capability := range manifest.Capabilities {
		if capability.Type == systemplugin.CapabilityPlaceholderValues {
			return capability.Interfaces
		}
	}
	return nil
}

func readMessageWithContext(ctx context.Context, decoder *json.Decoder) (systemplugin.Message, error) {
	type readResult struct {
		message systemplugin.Message
		err     error
	}
	result := make(chan readResult, 1)
	go func() {
		message, err := systemplugin.Read(decoder)
		result <- readResult{message: message, err: err}
	}()
	select {
	case item := <-result:
		return item.message, item.err
	case <-ctx.Done():
		return systemplugin.Message{}, ctx.Err()
	}
}

func writeEvidence(evidence string, record map[string]any, stderr []byte) {
	payload, err := json.MarshalIndent(record, "", "  ")
	if err == nil {
		_ = os.WriteFile(filepath.Join(evidence, "plugin.stdout.json"), payload, 0o644)
	}
	_ = os.WriteFile(filepath.Join(evidence, "plugin.stderr.log"), stderr, 0o644)
}

func captureJSONProcess(cmd *exec.Cmd, stdoutPath string, stderrPath string, requireSuccess bool) error {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil && len(bytes.TrimSpace(stdout.Bytes())) == 0 {
		_ = os.WriteFile(stdoutPath, stdout.Bytes(), 0o644)
		_ = os.WriteFile(stderrPath, stderr.Bytes(), 0o644)
		return fmt.Errorf("进程启动验收失败：%w", err)
	}
	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		_ = os.WriteFile(stdoutPath, stdout.Bytes(), 0o644)
		_ = os.WriteFile(stderrPath, stderr.Bytes(), 0o644)
		return fmt.Errorf("进程没有返回有效 JSON：%w", err)
	}
	if requireSuccess && result.Status != string(types.ToolStatusSuccess) {
		_ = os.WriteFile(stdoutPath, stdout.Bytes(), 0o644)
		_ = os.WriteFile(stderrPath, stderr.Bytes(), 0o644)
		return fmt.Errorf("工具随包能力没有通过真实启动验收")
	}
	if err := os.WriteFile(stdoutPath, stdout.Bytes(), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(stderrPath, stderr.Bytes(), 0o644); err != nil {
		return err
	}
	return nil
}

func absoluteRegularFile(path string, label string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("%s路径不能为空", label)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("读取%s失败：%w", label, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s不能是目录", label)
	}
	return absolute, nil
}

func replaceEnvironment(base []string, replacements map[string]string) []string {
	result := make([]string, 0, len(base)+len(replacements))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if ok {
			if _, replaced := replacements[strings.ToUpper(key)]; replaced {
				continue
			}
		}
		result = append(result, item)
	}
	for key, value := range replacements {
		result = append(result, key+"="+value)
	}
	return result
}

func escapeJSON(value string) string {
	payload, _ := json.Marshal(value)
	return strings.Trim(string(payload), `"`)
}
