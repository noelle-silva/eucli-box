package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var suffixPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type serviceProfile struct {
	Port int    `json:"port"`
	Key  string `json:"key"`
}

func main() {
	if len(os.Args) < 2 {
		fail("缺少适配器子命令")
	}
	switch os.Args[1] {
	case "deps":
		must(cmdDeps())
		ok(nil)
	case "build":
		data, err := cmdBuild()
		must(err)
		ok(data)
	case "start":
		data, err := cmdStart()
		must(err)
		ok(data)
	case "health":
		must(cmdHealth())
		ok(nil)
	case "stop":
		must(cmdStop())
		ok(nil)
	case "seed":
		emit(map[string]any{"status": "not-applicable"}, 0)
	case "test":
		must(cmdTest())
		ok(nil)
	default:
		fail("未知适配器子命令: " + os.Args[1])
	}
}

func emit(payload map[string]any, code int) {
	_ = json.NewEncoder(os.Stdout).Encode(payload)
	os.Exit(code)
}

func ok(data any) {
	out := map[string]any{"status": "ok"}
	if data != nil {
		out["data"] = data
	}
	emit(out, 0)
}

func fail(reason string) {
	emit(map[string]any{"status": "failed", "reason": reason}, 1)
}

func must(err error) {
	if err != nil {
		fail(err.Error())
	}
}

func requireEnv(name string) (string, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return "", fmt.Errorf("缺少必需环境变量 %s", name)
	}
	return v, nil
}

func requireSuffix() (string, error) {
	v, err := requireEnv("EUCLI_CF_INSTANCE_SUFFIX")
	if err != nil {
		return "", err
	}
	if !suffixPattern.MatchString(v) {
		return "", fmt.Errorf("实例身份后缀 %q 非法，拒绝启动", v)
	}
	return v, nil
}

func requirePort() (int, error) {
	raw, err := requireEnv("EUCLI_CF_PORT_BASE")
	if err != nil {
		return 0, err
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("端口基址 %q 非法", raw)
	}
	return port, nil
}

func goEnv(depsStore string, goworkOff bool) []string {
	env := append([]string(nil), os.Environ()...)
	if depsStore != "" {
		env = append(env,
			"GOMODCACHE="+filepath.Join(depsStore, "go", "pkg", "mod"),
			"GOCACHE="+filepath.Join(depsStore, "go", "build-cache"),
		)
	}
	if goworkOff {
		env = append(env, "GOWORK=off")
	}
	return env
}

func runGo(dir string, env []string, args ...string) error {
	fmt.Fprintf(os.Stderr, "[cf-adapter] (%s) go %s\n", dir, strings.Join(args, " "))
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("在 %s 执行 go %s 失败: %w", dir, strings.Join(args, " "), err)
	}
	return nil
}

func exeName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

func pidFile(dataRoot string) string {
	return filepath.Join(dataRoot, "eucli-box.pid")
}

func profileFile(dataRoot string) string {
	return filepath.Join(dataRoot, "service-profile.json")
}

func ensureProfile(dataRoot string, port int) (serviceProfile, error) {
	path := profileFile(dataRoot)
	var prof serviceProfile
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &prof)
	}
	if strings.TrimSpace(prof.Key) == "" {
		buf := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, buf); err != nil {
			return serviceProfile{}, fmt.Errorf("生成实例访问钥匙失败: %w", err)
		}
		prof.Key = hex.EncodeToString(buf)
	}
	prof.Port = port
	payload, err := json.MarshalIndent(prof, "", "  ")
	if err != nil {
		return serviceProfile{}, fmt.Errorf("序列化服务画像失败: %w", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		return serviceProfile{}, fmt.Errorf("写入服务画像失败: %w", err)
	}
	return prof, nil
}

func loadProfile(dataRoot string) (serviceProfile, error) {
	raw, err := os.ReadFile(profileFile(dataRoot))
	if err != nil {
		return serviceProfile{}, err
	}
	var prof serviceProfile
	if err := json.Unmarshal(raw, &prof); err != nil {
		return serviceProfile{}, err
	}
	if strings.TrimSpace(prof.Key) == "" {
		return serviceProfile{}, errors.New("服务画像中的 key 为空")
	}
	return prof, nil
}

func cmdDeps() error {
	worktree, err := requireEnv("EUCLI_CF_WORKTREE")
	if err != nil {
		return err
	}
	depsDir, err := requireEnv("EUCLI_CF_DEPS_DIR")
	if err != nil {
		return err
	}
	depsStore, err := requireEnv("EUCLI_CF_DEPS_STORE")
	if err != nil {
		return err
	}
	for _, dir := range []string{
		depsDir,
		filepath.Join(depsStore, "go", "pkg", "mod"),
		filepath.Join(depsStore, "go", "build-cache"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建依赖目录 %s 失败: %w", dir, err)
		}
	}
	env := goEnv(depsStore, true)
	if err := runGo(worktree, env, "mod", "download"); err != nil {
		return err
	}
	return nil
}

func cmdBuild() (map[string]any, error) {
	worktree, err := requireEnv("EUCLI_CF_WORKTREE")
	if err != nil {
		return nil, err
	}
	buildDir, err := requireEnv("EUCLI_CF_BUILD_DIR")
	if err != nil {
		return nil, err
	}
	depsStore, err := requireEnv("EUCLI_CF_DEPS_STORE")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建构建目录失败: %w", err)
	}
	outBin := filepath.Join(buildDir, exeName("eucli-box"))
	env := append(goEnv(depsStore, true), "CGO_ENABLED=0")
	if err := runGo(worktree, env, "build", "-trimpath", "-ldflags=-s -w", "-o", outBin, "./cmd/eucli-box"); err != nil {
		return nil, err
	}
	return map[string]any{
		"binary": outBin,
	}, nil
}

func cmdStart() (map[string]any, error) {
	suffix, err := requireSuffix()
	if err != nil {
		return nil, err
	}
	port, err := requirePort()
	if err != nil {
		return nil, err
	}
	buildDir, err := requireEnv("EUCLI_CF_BUILD_DIR")
	if err != nil {
		return nil, err
	}
	dataRoot, err := requireEnv("EUCLI_CF_DATA_ROOT")
	if err != nil {
		return nil, err
	}
	binPath := filepath.Join(buildDir, exeName("eucli-box"))
	if _, err := os.Stat(binPath); err != nil {
		return nil, fmt.Errorf("正式构建产物不存在 (%s): %w", binPath, err)
	}
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		return nil, fmt.Errorf("创建实例数据目录失败: %w", err)
	}
	if _, err := ensureProfile(dataRoot, port); err != nil {
		return nil, err
	}

	logPath := filepath.Join(dataRoot, "eucli-box.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("打开实例日志失败: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(binPath)
	cmd.Dir = buildDir
	cmd.Env = append(os.Environ(), "EUCLI_BOX_DATA_DIR="+dataRoot)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 eucli-box 实例失败: %w", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	if err := os.WriteFile(pidFile(dataRoot), []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return nil, fmt.Errorf("写入 PID 文件失败: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[cf-adapter] started eucli-box (%s) pid=%d port=%d\n", suffix, pid, port)
	return map[string]any{
		"pid":    pid,
		"port":   port,
		"suffix": suffix,
		"log":    logPath,
	}, nil
}

func probeAPI(port int, key string, apiPath string) error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", port, apiPath), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s 返回 %d: %s", apiPath, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func cmdHealth() error {
	port, err := requirePort()
	if err != nil {
		return err
	}
	dataRoot, err := requireEnv("EUCLI_CF_DATA_ROOT")
	if err != nil {
		return err
	}
	prof, err := loadProfile(dataRoot)
	if err != nil {
		return fmt.Errorf("读取实例服务画像失败: %w", err)
	}
	if err := probeAPI(port, prof.Key, "/api/box/info"); err != nil {
		return fmt.Errorf("实例尚未就绪: %w", err)
	}
	return nil
}

func cmdStop() error {
	port, err := requirePort()
	if err != nil {
		return err
	}
	dataRoot, err := requireEnv("EUCLI_CF_DATA_ROOT")
	if err != nil {
		return err
	}
	pFile := pidFile(dataRoot)
	raw, err := os.ReadFile(pFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取 PID 文件失败: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		_ = os.Remove(pFile)
		return nil
	}
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
	}
	if proc, findErr := os.FindProcess(pid); findErr == nil {
		_ = proc.Kill()
	}
	prof, profErr := loadProfile(dataRoot)
	for i := 0; i < 50; i++ {
		if profErr != nil || probeAPI(port, prof.Key, "/api/box/info") != nil {
			_ = os.Remove(pFile)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("实例 pid=%d 在超时内未停止（端口 %d 仍有响应）", pid, port)
}

func cmdTest() error {
	worktree, err := requireEnv("EUCLI_CF_WORKTREE")
	if err != nil {
		return err
	}
	dataRoot, err := requireEnv("EUCLI_CF_DATA_ROOT")
	if err != nil {
		return err
	}
	depsStore, err := requireEnv("EUCLI_CF_DEPS_STORE")
	if err != nil {
		return err
	}
	port, err := requirePort()
	if err != nil {
		return err
	}

	// 1. 对运行中正式实例执行关键网关接口冒烟联调
	prof, err := loadProfile(dataRoot)
	if err != nil {
		return fmt.Errorf("读取实例服务画像失败: %w", err)
	}
	for _, apiPath := range []string{"/api/box/info", "/api/release", "/api/workspaces", "/api/roles"} {
		fmt.Fprintf(os.Stderr, "[cf-adapter] smoke: GET %s\n", apiPath)
		if err := probeAPI(port, prof.Key, apiPath); err != nil {
			return fmt.Errorf("运行中实例校验失败: %w", err)
		}
	}

	// 2. 业务端与开发工具双模块回归测试
	if err := runGo(worktree, goEnv(depsStore, true), "test", "./pkg/...", "./src/...", "./cmd/..."); err != nil {
		return err
	}
	devWorkspaceDir := filepath.Join(worktree, ".dev-workspace")
	_, existedBefore := os.Stat(devWorkspaceDir)
	err = runGo(filepath.Join(worktree, ".dev-tools"), goEnv(depsStore, false), "test", "./...")
	if errors.Is(existedBefore, fs.ErrNotExist) {
		_ = os.RemoveAll(devWorkspaceDir)
	}
	if err != nil {
		return err
	}
	return nil
}
