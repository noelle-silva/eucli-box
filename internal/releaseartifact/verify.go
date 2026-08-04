package releaseartifact

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"eucli-box/pkg/release"
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
	workspace, err := filepath.Abs(strings.TrimSpace(options.Workspace))
	if err != nil || strings.TrimSpace(options.Workspace) == "" {
		return VerifyResult{}, fmt.Errorf("验收工作区无效")
	}
	if err := os.RemoveAll(workspace); err != nil {
		return VerifyResult{}, fmt.Errorf("清理验收工作区失败：%w", err)
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return VerifyResult{}, fmt.Errorf("建立验收工作区失败：%w", err)
	}
	evidenceDir := filepath.Join(workspace, "evidence")
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		return VerifyResult{}, err
	}
	environmentDir := filepath.Join(workspace, "environment")
	tempDir := filepath.Join(workspace, "temp")
	for _, directory := range []string{environmentDir, tempDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return VerifyResult{}, err
		}
	}
	manifestPayload, err := os.ReadFile(manifestPath)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("读取发行清单失败：%w", err)
	}
	manifest, err := release.DecodeReleaseManifest(manifestPayload)
	if err != nil {
		return VerifyResult{}, err
	}
	extracted := filepath.Join(workspace, "extracted")
	if err := os.MkdirAll(extracted, 0o755); err != nil {
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
	if err := release.ExtractArchive(release.ExtractArchiveOptions{ArchivePath: archivePath, TargetDir: extracted}); err != nil {
		return VerifyResult{}, err
	}
	if _, err := release.ValidateExtractedPackage(release.ValidateExtractedPackageOptions{Directory: extracted, Manifest: manifest}); err != nil {
		return VerifyResult{}, fmt.Errorf("解包后的成品边界无效：%w", err)
	}
	if err := launchCheck(ctx, manifest.Artifact, extracted, environmentDir, tempDir, evidenceDir, options.Timeout); err != nil {
		return VerifyResult{}, err
	}
	result := VerifyResult{Manifest: manifest, Evidence: evidenceDir}
	if err := writeJSON(filepath.Join(evidenceDir, "verification-result.json"), map[string]any{
		"status":   "passed",
		"artifact": manifest.Artifact,
		"version":  manifest.Version,
		"platform": manifest.Platform,
		"archive":  manifest.Archive,
	}); err != nil {
		return VerifyResult{}, err
	}
	return result, nil
}

func launchCheck(parent context.Context, identity types.ReleaseArtifactIdentity, directory string, environment string, temp string, evidence string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	switch identity.Kind {
	case types.ReleaseArtifactKindBox:
		return launchBox(ctx, directory, environment, temp, evidence)
	case types.ReleaseArtifactKindTool:
		return launchTool(ctx, directory, environment, temp, evidence)
	case types.ReleaseArtifactKindPlugin:
		return launchPlugin(ctx, directory, identity.ID, evidence)
	default:
		return fmt.Errorf("无法验收未知发布物类别 %q", identity.Kind)
	}
}

func launchBox(ctx context.Context, directory string, environment string, temp string, evidence string) error {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("业务端启动验收只能在 Windows x64 执行")
	}
	port, err := reservePort()
	if err != nil {
		return err
	}
	dataDir := filepath.Join(environment, "box-data")
	for _, path := range []string{dataDir, temp} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	stdout, err := os.OpenFile(filepath.Join(evidence, "box.stdout.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(filepath.Join(evidence, "box.stderr.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer stderr.Close()
	cmd := exec.CommandContext(ctx, filepath.Join(directory, "eucli-box.exe"))
	cmd.Dir = directory
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = replaceEnvironment(os.Environ(), map[string]string{
		"EUCLI_BOX_ADDR":     "127.0.0.1:" + port,
		"EUCLI_BOX_DATA_DIR": dataDir,
		"TEMP":               temp,
		"TMP":                temp,
	})
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("业务端启动失败：%w", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()
	client := &http.Client{Timeout: 750 * time.Millisecond}
	url := "http://127.0.0.1:" + port + "/api/release"
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("业务端启动验收超时：%w", err)
		}
		response, requestErr := client.Get(url)
		if requestErr == nil {
			payload, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr == nil && response.StatusCode == http.StatusOK {
				var envelope struct {
					Data types.EucliBoxReleaseInfo `json:"data"`
				}
				if json.Unmarshal(payload, &envelope) == nil && release.ValidateVersion(envelope.Data.Version) == nil && release.ValidateVersion(envelope.Data.DataVersion) == nil {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("业务端启动验收超时：%w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
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
	case "sci_calculator":
		arguments = `{"expression":"norm_cdf(0, 0, 1)"}`
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

func launchPlugin(ctx context.Context, directory string, id string, evidence string) error {
	cmd := exec.CommandContext(ctx, filepath.Join(directory, "binary", id+".exe"))
	cmd.Dir = directory
	cmd.Stdin = strings.NewReader(`{"action":"resolve_placeholders","pluginId":"` + escapeJSON(id) + `","placeholderInterfaces":[],"userConfig":{},"defaultConfig":{}}`)
	return captureJSONProcess(cmd, filepath.Join(evidence, "plugin.stdout.json"), filepath.Join(evidence, "plugin.stderr.log"), false)
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

func reservePort() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("准备验收端口失败：%w", err)
	}
	defer listener.Close()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok || address.Port <= 0 {
		return "", fmt.Errorf("系统没有返回有效验收端口")
	}
	return fmt.Sprint(address.Port), nil
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
