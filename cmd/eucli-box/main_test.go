package main

import (
	"os"
	"strings"
	"testing"

	"eucli-box/pkg/datapaths"
)

func writePortConfig(t *testing.T, dataDir string, content string) {
	t.Helper()
	if err := os.MkdirAll(datapaths.MetaDir(dataDir), 0o700); err != nil {
		t.Fatalf("准备数据区失败：%v", err)
	}
	if err := os.WriteFile(datapaths.PortFile(dataDir), []byte(content), 0o600); err != nil {
		t.Fatalf("写入端口配置失败：%v", err)
	}
}

func readPortConfigBytes(t *testing.T, dataDir string) string {
	t.Helper()
	payload, err := os.ReadFile(datapaths.PortFile(dataDir))
	if err != nil {
		t.Fatalf("读取端口配置失败：%v", err)
	}
	return string(payload)
}

func intPointer(value int) *int {
	return &value
}

func TestEnsurePortFileCreatesDefaultConfig(t *testing.T) {
	dataDir := t.TempDir()
	if err := ensurePortFile(dataDir); err != nil {
		t.Fatalf("ensurePortFile() error = %v", err)
	}
	const want = "{\n  \"port\": \"default\"\n}\n"
	if got := readPortConfigBytes(t, dataDir); got != want {
		t.Fatalf("首次生成的端口配置 = %q, want %q", got, want)
	}
	port, err := readPortFile(dataDir)
	if err != nil {
		t.Fatalf("readPortFile() error = %v", err)
	}
	if port != nil {
		t.Fatalf("readPortFile() = %d, want nil（default 走内置地址）", *port)
	}
}

func TestEnsurePortFileKeepsExistingConfig(t *testing.T) {
	dataDir := t.TempDir()
	custom := "{\n  \"port\": 9000\n}\n"
	writePortConfig(t, dataDir, custom)
	if err := ensurePortFile(dataDir); err != nil {
		t.Fatalf("ensurePortFile() error = %v", err)
	}
	if err := ensurePortFile(dataDir); err != nil {
		t.Fatalf("ensurePortFile() 第二次 error = %v", err)
	}
	if got := readPortConfigBytes(t, dataDir); got != custom {
		t.Fatalf("已存在的端口配置被改写 = %q", got)
	}
}

func TestReadPortFileAcceptedValues(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    *int
	}{
		{name: "default 走内置默认", content: `{"port": "default"}`, want: nil},
		{name: "数字端口", content: `{"port": 9000}`, want: intPointer(9000)},
		{name: "字符串数字端口", content: `{"port": "9000"}`, want: intPointer(9000)},
		{name: "最小边界端口", content: `{"port": 1}`, want: intPointer(1)},
		{name: "最大边界端口", content: `{"port": 65535}`, want: intPointer(65535)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			writePortConfig(t, dataDir, test.content)
			port, err := readPortFile(dataDir)
			if err != nil {
				t.Fatalf("readPortFile() error = %v", err)
			}
			if test.want == nil {
				if port != nil {
					t.Fatalf("readPortFile() = %d, want nil", *port)
				}
				return
			}
			if port == nil || *port != *test.want {
				t.Fatalf("readPortFile() = %v, want %d", port, *test.want)
			}
		})
	}
}

func TestReadPortFileRejectedValues(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "空文件", content: ""},
		{name: "非法 JSON", content: `{"port": `},
		{name: "顶层数组", content: `[9000]`},
		{name: "顶层 null", content: `null`},
		{name: "多余内容", content: `{"port": 9000} {}`},
		{name: "缺少 port 字段", content: `{}`},
		{name: "显式 null", content: `{"port": null}`},
		{name: "布尔值", content: `{"port": true}`},
		{name: "对象", content: `{"port": {}}`},
		{name: "数组", content: `{"port": []}`},
		{name: "非数字字符串", content: `{"port": "abc"}`},
		{name: "非整数数字", content: `{"port": 9000.5}`},
		{name: "零端口", content: `{"port": 0}`},
		{name: "负数端口", content: `{"port": -1}`},
		{name: "超出上限端口", content: `{"port": 65536}`},
		{name: "字符串超出上限端口", content: `{"port": "65536"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			writePortConfig(t, dataDir, test.content)
			if port, err := readPortFile(dataDir); err == nil {
				t.Fatalf("readPortFile() = %v, want error", port)
			}
		})
	}
}

func TestReadPortFileErrorMentionsPortField(t *testing.T) {
	dataDir := t.TempDir()
	writePortConfig(t, dataDir, `{"other": 1}`)
	_, err := readPortFile(dataDir)
	if err == nil || !strings.Contains(err.Error(), "port") {
		t.Fatalf("readPortFile() error = %v, want 包含 port 字段的中文错误", err)
	}
}

func TestReadPortFileRejectsMissingFile(t *testing.T) {
	if _, err := readPortFile(t.TempDir()); err == nil {
		t.Fatalf("readPortFile() 在文件缺失时未报错")
	}
}

func TestResolveListenAddr(t *testing.T) {
	tests := []struct {
		name    string
		content string
		envAddr string
		want    string
	}{
		{name: "配置文件 default 使用内置默认", content: `{"port": "default"}`, envAddr: "", want: "127.0.0.1:8765"},
		{name: "配置文件数字端口覆盖内置默认", content: `{"port": 9000}`, envAddr: "", want: "127.0.0.1:9000"},
		{name: "配置文件字符串端口覆盖内置默认", content: `{"port": "9000"}`, envAddr: "", want: "127.0.0.1:9000"},
		{name: "环境变量优先于配置文件", content: `{"port": 9000}`, envAddr: "127.0.0.1:1234", want: "127.0.0.1:1234"},
		{name: "环境变量为空白时回落到配置文件", content: `{"port": 9000}`, envAddr: "   ", want: "127.0.0.1:9000"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			writePortConfig(t, dataDir, test.content)
			t.Setenv("EUCLI_BOX_ADDR", test.envAddr)
			addr, err := resolveListenAddr(dataDir)
			if err != nil {
				t.Fatalf("resolveListenAddr() error = %v", err)
			}
			if addr != test.want {
				t.Fatalf("resolveListenAddr() = %q, want %q", addr, test.want)
			}
		})
	}
}

func TestResolveListenAddrRejectsBadFileEvenWithEnv(t *testing.T) {
	dataDir := t.TempDir()
	writePortConfig(t, dataDir, `{"port": 0}`)
	t.Setenv("EUCLI_BOX_ADDR", "127.0.0.1:1234")
	if addr, err := resolveListenAddr(dataDir); err == nil {
		t.Fatalf("resolveListenAddr() = %q, want error（坏值不因环境变量而放行）", addr)
	}
}
