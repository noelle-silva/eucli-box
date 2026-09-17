//go:build eucli_background_access

package backgroundaccessverify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/datapaths"
)

// ---------- 业务端进程（普通模式） ----------

type boxProcess struct {
	t          *testing.T
	cmd        *exec.Cmd
	baseURL    string
	credential string
	runID      string
	client     *http.Client
	logFile    *os.File
}

func startRegularBox(t *testing.T, boxPath string, envDir string) *boxProcess {
	t.Helper()
	unique := time.Now().UTC().Format("20060102T150405.000000000Z")
	boxData := filepath.Join(envDir, "box-data-"+unique)
	programRoot := filepath.Join(envDir, "program-root-"+unique)
	for _, dir := range []string{boxData, programRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	const fixedKey = "background-access-fixed-key"
	port := freePort(t)
	if err := writeServiceProfile(boxData, port, fixedKey); err != nil {
		t.Fatalf("写入启动配置画像失败：%v", err)
	}
	logFile, err := os.Create(filepath.Join(envDir, "box-"+unique+".log"))
	if err != nil {
		t.Fatalf("create box log: %v", err)
	}
	cmd := exec.Command(boxPath)
	cmd.Env = append(os.Environ(),
		"EUCLI_BOX_DATA_DIR="+boxData,
		"EUCLI_BOX_PROGRAM_ROOT="+programRoot,
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		t.Fatalf("start box: %v", err)
	}
	box := &boxProcess{
		t: t, cmd: cmd, baseURL: "http://127.0.0.1:" + strconv.Itoa(port), credential: fixedKey,
		client: &http.Client{Timeout: 30 * time.Second}, logFile: logFile,
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

func (b *boxProcess) waitReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if status, _ := b.call(http.MethodGet, "/api/release", "", b.credential); status == http.StatusOK {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("业务端未在期限内就绪，日志：\n%s", b.logText())
}

func (b *boxProcess) stop() {
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

func (b *boxProcess) logText() string {
	if b.logFile == nil {
		return ""
	}
	payload, err := os.ReadFile(b.logFile.Name())
	if err != nil {
		return ""
	}
	return string(payload)
}

func (b *boxProcess) call(method string, path string, body string, credential string) (int, []byte) {
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
	if credential != "" {
		request.Header.Set("Authorization", "Bearer "+credential)
	}
	response, err := b.client.Do(request)
	if err != nil {
		b.t.Logf("box.call %s %s 失败：%v", method, path, err)
		return 0, nil
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	return response.StatusCode, payload
}

func (b *boxProcess) trustedCall(method string, path string, body string) (int, []byte) {
	return b.call(method, path, body, b.credential)
}

func (b *boxProcess) dataJSON(status int, payload []byte) map[string]any {
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

// longTermCall 通过长期端口发起请求（使用长期 Key 鉴权），
// 与网关直连入口的身份鉴权完全隔离。
func (b *boxProcess) longTermCall(method string, port int, path string, body string, key string) (int, []byte) {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	target := fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
	request, err := http.NewRequest(method, target, reader)
	if err != nil {
		return 0, nil
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	response, err := b.client.Do(request)
	if err != nil {
		b.t.Logf("longTermCall %s %s 失败：%v", method, target, err)
		return 0, nil
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	return response.StatusCode, payload
}

// ---------- 验证场景 ----------

// TestBackgroundAccess 后台运行与访问设置默认验证：
// 长期端口、长期 Key、权限边界与旧配置转换。
func TestBackgroundAccess(t *testing.T) {
	boxPath := strings.TrimSpace(os.Getenv("EUCLI_BACKGROUND_ACCESS_BOX"))
	runRoot := strings.TrimSpace(os.Getenv("EUCLI_BACKGROUND_ACCESS_RUN_ROOT"))
	if boxPath == "" || runRoot == "" {
		t.Fatalf("缺少 EUCLI_BACKGROUND_ACCESS_BOX 或 EUCLI_BACKGROUND_ACCESS_RUN_ROOT")
	}
	envDir := filepath.Join(runRoot, "environment", "background-access")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatalf("mkdir env: %v", err)
	}
	box := startRegularBox(t, boxPath, envDir)

	t.Run("长期端口列表初始为空", func(t *testing.T) {
		status, payload := box.trustedCall(http.MethodGet, "/api/access/persistent-ports", "")
		if status != http.StatusOK {
			t.Fatalf("status = %d body=%s", status, payload)
		}
		if !strings.Contains(string(payload), `"ports"`) && !strings.Contains(string(payload), `[]`) {
			t.Fatalf("端口列表响应异常：%s", payload)
		}
	})

	var portID string
	var keyID string
	var plainKey string
	var portNumber int

	t.Run("没有有效 Key 时拒绝启用端口", func(t *testing.T) {
		status, payload := box.trustedCall(http.MethodPost, "/api/access/persistent-ports", `{"name":"测试端口","port":18088}`)
		if status != http.StatusCreated {
			t.Fatalf("创建端口 status = %d body=%s", status, payload)
		}
		data := box.dataJSON(status, payload)
		portID = fmt.Sprint(data["id"])
		if number, parseErr := strconv.Atoi(fmt.Sprint(data["port"])); parseErr == nil && number > 0 {
			portNumber = number
		} else {
			portNumber = 18088
		}
		status, payload = box.trustedCall(http.MethodPut, "/api/access/persistent-ports/"+portID+"/enable", "")
		if status != http.StatusBadRequest {
			t.Fatalf("无 Key 启用端口 status = %d body=%s", status, payload)
		}
		if !strings.Contains(string(payload), "没有有效的长期 Key") {
			t.Fatalf("错误信息不明确：%s", payload)
		}
	})

	t.Run("创建长期 Key 并返回明文", func(t *testing.T) {
		status, payload := box.trustedCall(http.MethodPost, "/api/access/persistent-keys", `{"name":"测试 Key","expiresAt":null}`)
		if status != http.StatusCreated {
			t.Fatalf("创建 Key status = %d body=%s", status, payload)
		}
		data := box.dataJSON(status, payload)
		keyID = fmt.Sprint(data["id"])
		plainKey = fmt.Sprint(data["plainKey"])
		if keyID == "" || plainKey == "" {
			t.Fatalf("创建 Key 响应缺少 id/plainKey：%s", payload)
		}
	})

	t.Run("启用端口成功并开放监听", func(t *testing.T) {
		status, payload := box.trustedCall(http.MethodPut, "/api/access/persistent-ports/"+portID+"/enable", "")
		if status != http.StatusOK {
			t.Fatalf("启用端口 status = %d body=%s", status, payload)
		}
		data := box.dataJSON(status, payload)
		if fmt.Sprint(data["actualState"]) != "running" {
			t.Fatalf("端口实际状态 = %v", data["actualState"])
		}
	})

	t.Run("长期 Key 鉴权通过", func(t *testing.T) {
		status, payload := box.longTermCall(http.MethodGet, portNumber, "/api/release", "", plainKey)
		if status != http.StatusOK {
			t.Fatalf("长期端口 Key 访问 status = %d body=%s", status, payload)
		}
	})

	t.Run("错误 Key 鉴权失败", func(t *testing.T) {
		status, _ := box.longTermCall(http.MethodGet, portNumber, "/api/release", "", "wrong-key")
		if status != http.StatusUnauthorized {
			t.Fatalf("错误 Key status = %d，期望 401", status)
		}
	})

	t.Run("长期 Key 无权管理访问设置", func(t *testing.T) {
		status, payload := box.longTermCall(http.MethodGet, portNumber, "/api/access/persistent-keys", "", plainKey)
		if status != http.StatusForbidden {
			t.Fatalf("长期 Key 管理访问设置 status = %d body=%s", status, payload)
		}
		if !strings.Contains(string(payload), "长期 Key 无权管理访问设置") {
			t.Fatalf("错误信息不明确：%s", payload)
		}
	})

	t.Run("查看和复制完整 Key", func(t *testing.T) {
		status, payload := box.trustedCall(http.MethodGet, "/api/access/persistent-keys/"+keyID+"/reveal", "")
		if status != http.StatusOK {
			t.Fatalf("查看 Key status = %d body=%s", status, payload)
		}
		if !strings.Contains(string(payload), plainKey) {
			t.Fatalf("查看 Key 未返回明文：%s", payload)
		}
	})

	t.Run("停用 Key 后鉴权失败", func(t *testing.T) {
		status, _ := box.trustedCall(http.MethodPut, "/api/access/persistent-keys/"+keyID+"/disable", "")
		if status != http.StatusOK {
			t.Fatalf("停用 Key status = %d", status)
		}
		status, _ = box.longTermCall(http.MethodGet, portNumber, "/api/release", "", plainKey)
		if status != http.StatusUnauthorized {
			t.Fatalf("停用后 status = %d，期望 401", status)
		}
	})

	t.Run("重新启用 Key 并设置有效期", func(t *testing.T) {
		status, _ := box.trustedCall(http.MethodPut, "/api/access/persistent-keys/"+keyID+"/enable", "")
		if status != http.StatusOK {
			t.Fatalf("启用 Key status = %d", status)
		}
		expired := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
		status, _ = box.trustedCall(http.MethodPut, "/api/access/persistent-keys/"+keyID+"/expiration", `{"expiresAt":"`+expired+`"}`)
		if status != http.StatusOK {
			t.Fatalf("设置有效期 status = %d", status)
		}
		status, _ = box.longTermCall(http.MethodGet, portNumber, "/api/release", "", plainKey)
		if status != http.StatusUnauthorized {
			t.Fatalf("过期 Key status = %d，期望 401", status)
		}
	})

	t.Run("删除 Key 后鉴权失败", func(t *testing.T) {
		status, _ := box.trustedCall(http.MethodDelete, "/api/access/persistent-keys/"+keyID, "")
		if status != http.StatusNoContent {
			t.Fatalf("删除 Key status = %d", status)
		}
		status, _ = box.longTermCall(http.MethodGet, portNumber, "/api/release", "", plainKey)
		if status != http.StatusUnauthorized {
			t.Fatalf("删除后 status = %d，期望 401", status)
		}
	})

	t.Run("停用和删除端口", func(t *testing.T) {
		status, _ := box.trustedCall(http.MethodPut, "/api/access/persistent-ports/"+portID+"/disable", "")
		if status != http.StatusOK {
			t.Fatalf("停用端口 status = %d", status)
		}
		status, _ = box.trustedCall(http.MethodDelete, "/api/access/persistent-ports/"+portID, "")
		if status != http.StatusNoContent {
			t.Fatalf("删除端口 status = %d", status)
		}
	})

	t.Run("业务端信息接口", func(t *testing.T) {
		status, payload := box.trustedCall(http.MethodGet, "/api/box/info", "")
		if status != http.StatusOK {
			t.Fatalf("box info status = %d body=%s", status, payload)
		}
		if !strings.Contains(string(payload), "version") {
			t.Fatalf("box info 缺少版本：%s", payload)
		}
	})

	t.Run("业务端关闭接口已随客户端解耦移除", func(t *testing.T) {
		status, _ := box.trustedCall(http.MethodPost, "/api/box/shutdown", `{}`)
		if status != http.StatusNotFound {
			t.Fatalf("关闭接口 status = %d，期望 404", status)
		}
	})
}

// TestBackgroundAccessExperience 后台访问体验准备：访问设置界面链路。
func TestBackgroundAccessExperience(t *testing.T) {
	boxPath := strings.TrimSpace(os.Getenv("EUCLI_BACKGROUND_ACCESS_BOX"))
	runRoot := strings.TrimSpace(os.Getenv("EUCLI_BACKGROUND_ACCESS_RUN_ROOT"))
	if boxPath == "" || runRoot == "" {
		t.Fatalf("缺少 EUCLI_BACKGROUND_ACCESS_BOX 或 EUCLI_BACKGROUND_ACCESS_RUN_ROOT")
	}
	envDir := filepath.Join(runRoot, "environment", "background-access-experience")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatalf("mkdir env: %v", err)
	}
	box := startRegularBox(t, boxPath, envDir)

	t.Run("创建 Key 并查看", func(t *testing.T) {
		status, payload := box.trustedCall(http.MethodPost, "/api/access/persistent-keys", `{"name":"体验 Key","expiresAt":null}`)
		if status != http.StatusCreated {
			t.Fatalf("创建 Key status = %d body=%s", status, payload)
		}
		data := box.dataJSON(status, payload)
		plainKey := fmt.Sprint(data["plainKey"])
		if plainKey == "" {
			t.Fatalf("创建 Key 未返回明文")
		}
		status, _ = box.trustedCall(http.MethodGet, "/api/access/persistent-keys/"+fmt.Sprint(data["id"])+"/reveal", "")
		if status != http.StatusOK {
			t.Fatalf("查看 Key status = %d", status)
		}
	})

	t.Run("创建端口并启用", func(t *testing.T) {
		status, payload := box.trustedCall(http.MethodPost, "/api/access/persistent-ports", `{"name":"体验端口","port":18089}`)
		if status != http.StatusCreated {
			t.Fatalf("创建端口 status = %d body=%s", status, payload)
		}
		data := box.dataJSON(status, payload)
		status, _ = box.trustedCall(http.MethodPut, "/api/access/persistent-ports/"+fmt.Sprint(data["id"])+"/enable", "")
		if status != http.StatusOK {
			t.Fatalf("启用端口 status = %d", status)
		}
	})
}

var _ = context.Background
