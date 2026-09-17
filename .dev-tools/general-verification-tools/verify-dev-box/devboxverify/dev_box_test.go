//go:build eucli_devbox

package devboxverify

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/datapaths"
)

const devBoxKey = "devbox-verify-key"

type devBoxProcess struct {
	t       *testing.T
	cmd     *exec.Cmd
	baseURL string
	client  *http.Client
	logFile *os.File
}

func startDevBox(t *testing.T, boxPath string, envDir string) *devBoxProcess {
	t.Helper()
	unique := time.Now().UTC().Format("20060102T150405.000000000Z")
	boxData := filepath.Join(envDir, "box-data-"+unique)
	toolRoot := filepath.Join(envDir, "tool-package-"+unique)
	for _, dir := range []string{boxData, toolRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := writeServiceProfile(boxData, freePort(t), devBoxKey); err != nil {
		t.Fatalf("写入启动配置画像失败：%v", err)
	}
	logFile, err := os.Create(filepath.Join(envDir, "box-"+unique+".log"))
	if err != nil {
		t.Fatalf("create box log: %v", err)
	}
	cmd := exec.Command(boxPath)
	cmd.Env = append(os.Environ(),
		"EUCLI_BOX_DATA_DIR="+boxData,
		"EUCLI_DEV_TOOL_SOURCE=1",
		"EUCLI_DEV_TOOL_PACKAGE_ROOT="+toolRoot,
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		t.Fatalf("start box: %v", err)
	}
	box := &devBoxProcess{
		t: t, cmd: cmd, client: &http.Client{Timeout: 30 * time.Second}, logFile: logFile,
	}
	t.Cleanup(func() { box.stop() })
	box.waitReady(t)
	return box
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

// writeServiceProfile 预置启动配置画像，模拟平台侧按固定读法写入端口与钥匙。
func writeServiceProfile(boxData string, port int, key string) error {
	payload, err := json.Marshal(map[string]any{"port": port, "key": key})
	if err != nil {
		return err
	}
	return os.WriteFile(datapaths.ServiceProfileFile(boxData), append(payload, '\n'), 0o600)
}

var readyLinePattern = regexp.MustCompile("listening on (http://127\\.0\\.0\\.1:\\d+)")

func (b *devBoxProcess) waitReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if address := b.readyAddress(); address != "" {
			b.baseURL = address
			if status, _ := b.call(http.MethodGet, "/api/release", ""); status == http.StatusOK {
				return
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("业务端未在期限内就绪，日志：\n%s", b.logText())
}

func (b *devBoxProcess) readyAddress() string {
	if b.logFile == nil {
		return ""
	}
	payload, err := os.ReadFile(b.logFile.Name())
	if err != nil {
		return ""
	}
	match := readyLinePattern.FindStringSubmatch(string(payload))
	if match == nil {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func (b *devBoxProcess) stop() {
	if b.cmd == nil || b.cmd.Process == nil {
		return
	}
	_ = exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", b.cmd.Process.Pid)).Run()
	done := make(chan struct{})
	go func() {
		_ = b.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

func (b *devBoxProcess) logText() string {
	if b.logFile == nil {
		return ""
	}
	payload, err := os.ReadFile(b.logFile.Name())
	if err != nil {
		return ""
	}
	return string(payload)
}

func (b *devBoxProcess) call(method string, path string, body string) (int, []byte) {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, b.baseURL+path, reader)
	if err != nil {
		return 0, nil
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Authorization", "Bearer "+devBoxKey)
	response, err := b.client.Do(request)
	if err != nil {
		b.t.Logf("box.call %s %s 失败：%v", method, path, err)
		return 0, nil
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	return response.StatusCode, payload
}

func (b *devBoxProcess) dataJSON(status int, payload []byte) map[string]any {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if status >= 200 && status < 300 {
		_ = json.Unmarshal(payload, &envelope)
	}
	if len(envelope.Data) == 0 {
		return map[string]any{}
	}
	var value map[string]any
	_ = json.Unmarshal(envelope.Data, &value)
	return value
}

// TestDevBox 验证开发态链路：编译产物直接以普通模式启动、固定 Key 鉴权、工具开发源标记。
func TestDevBox(t *testing.T) {
	boxPath := strings.TrimSpace(os.Getenv("EUCLI_DEV_BOX_BOX"))
	runRoot := strings.TrimSpace(os.Getenv("EUCLI_DEV_BOX_RUN_ROOT"))
	if boxPath == "" || runRoot == "" {
		t.Fatalf("缺少 EUCLI_DEV_BOX_BOX 或 EUCLI_DEV_BOX_RUN_ROOT")
	}
	envDir := filepath.Join(runRoot, "environment", "dev-box")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatalf("mkdir env: %v", err)
	}
	box := startDevBox(t, boxPath, envDir)

	t.Run("业务端健康与版本", func(t *testing.T) {
		status, payload := box.call(http.MethodGet, "/api/release", "")
		if status != http.StatusOK {
			t.Fatalf("release status = %d body=%s", status, payload)
		}
		data := box.dataJSON(status, payload)
		if data["version"] == nil || data["version"] == "" {
			t.Fatalf("release 缺少版本：%s", payload)
		}
	})

	t.Run("工具开发源标记为 development", func(t *testing.T) {
		status, payload := box.call(http.MethodGet, "/api/install-source", "")
		if status != http.StatusOK {
			t.Fatalf("install-source status = %d body=%s", status, payload)
		}
		data := box.dataJSON(status, payload)
		if fmt.Sprint(data["kind"]) != "development" {
			t.Fatalf("install-source kind = %v，期望 development", data["kind"])
		}
	})

	t.Run("错误 Key 鉴权失败", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, box.baseURL+"/api/release", nil)
		request.Header.Set("Authorization", "Bearer wrong-key")
		response, err := box.client.Do(request)
		if err != nil {
			t.Fatalf("request error = %v", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("错误 Key status = %d，期望 401", response.StatusCode)
		}
	})
}
