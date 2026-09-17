package serviceprofile

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"eucli-box/pkg/datapaths"
)

func writeProfileFile(t *testing.T, dataDir string, content string) {
	t.Helper()
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("准备数据目录失败：%v", err)
	}
	if err := os.WriteFile(datapaths.ServiceProfileFile(dataDir), []byte(content), 0o600); err != nil {
		t.Fatalf("写入画像文件失败：%v", err)
	}
}

func readProfileBytes(t *testing.T, dataDir string) string {
	t.Helper()
	payload, err := os.ReadFile(datapaths.ServiceProfileFile(dataDir))
	if err != nil {
		t.Fatalf("读取画像文件失败：%v", err)
	}
	return string(payload)
}

var accessKeyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestEnsureBootstrapsDefaultProfile(t *testing.T) {
	dataDir := t.TempDir()
	profile, err := Ensure(dataDir)
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if profile.Port != DefaultPort {
		t.Fatalf("Ensure() port = %d, want %d", profile.Port, DefaultPort)
	}
	if !accessKeyPattern.MatchString(profile.Key) {
		t.Fatalf("Ensure() key = %q, want 64 位小写十六进制", profile.Key)
	}
	if got := readProfileBytes(t, dataDir); got != "{\n  \"port\": 8765,\n  \"key\": \""+profile.Key+"\"\n}\n" {
		t.Fatalf("自举画像内容 = %q", got)
	}
	loaded, err := Load(dataDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded != profile {
		t.Fatalf("Load() = %+v, want %+v", loaded, profile)
	}
}

func TestEnsureKeepsExistingProfile(t *testing.T) {
	dataDir := t.TempDir()
	custom := "{\n  \"port\": 9000,\n  \"key\": \"kept-key\"\n}\n"
	writeProfileFile(t, dataDir, custom)
	first, err := Ensure(dataDir)
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	second, err := Ensure(dataDir)
	if err != nil {
		t.Fatalf("Ensure() 第二次 error = %v", err)
	}
	if first != second {
		t.Fatalf("Ensure() 两次结果不同：%+v / %+v", first, second)
	}
	if first.Port != 9000 || first.Key != "kept-key" {
		t.Fatalf("Ensure() = %+v, want port 9000 key kept-key", first)
	}
	if got := readProfileBytes(t, dataDir); got != custom {
		t.Fatalf("已存在的画像被改写 = %q", got)
	}
}

func TestEnsureGeneratesFreshKeyEachBoot(t *testing.T) {
	first, err := generate()
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	second, err := generate()
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	if first.Key == second.Key {
		t.Fatalf("两次生成的钥匙相同：%q", first.Key)
	}
	if _, err := hex.DecodeString(first.Key); err != nil {
		t.Fatalf("生成的钥匙不是十六进制：%v", err)
	}
}

func TestLoadAcceptedValues(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    Profile
	}{
		{name: "整数端口", content: `{"port": 9000, "key": "abc"}`, want: Profile{Port: 9000, Key: "abc"}},
		{name: "最小边界端口", content: `{"port": 1, "key": "abc"}`, want: Profile{Port: 1, Key: "abc"}},
		{name: "最大边界端口", content: `{"port": 65535, "key": "abc"}`, want: Profile{Port: 65535, Key: "abc"}},
		{name: "钥匙去除首尾空白", content: `{"port": 9000, "key": "  abc  "}`, want: Profile{Port: 9000, Key: "abc"}},
		{name: "忽略未知字段", content: `{"extra": true, "port": 9000, "key": "abc"}`, want: Profile{Port: 9000, Key: "abc"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			writeProfileFile(t, dataDir, test.content)
			got, err := Load(dataDir)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("Load() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestLoadRejectedValues(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "空文件", content: ""},
		{name: "非法 JSON", content: `{"port": `},
		{name: "顶层数组", content: `[9000]`},
		{name: "顶层 null", content: `null`},
		{name: "顶层字符串", content: `"9000"`},
		{name: "多余内容", content: `{"port": 9000, "key": "a"} {}`},
		{name: "缺少 port 字段", content: `{"key": "abc"}`},
		{name: "显式 null 端口", content: `{"port": null, "key": "abc"}`},
		{name: "字符串端口", content: `{"port": "9000", "key": "abc"}`},
		{name: "非整数端口", content: `{"port": 9000.5, "key": "abc"}`},
		{name: "布尔端口", content: `{"port": true, "key": "abc"}`},
		{name: "零端口", content: `{"port": 0, "key": "abc"}`},
		{name: "负数端口", content: `{"port": -1, "key": "abc"}`},
		{name: "超出上限端口", content: `{"port": 65536, "key": "abc"}`},
		{name: "缺少 key 字段", content: `{"port": 9000}`},
		{name: "显式 null 钥匙", content: `{"port": 9000, "key": null}`},
		{name: "空钥匙", content: `{"port": 9000, "key": ""}`},
		{name: "空白钥匙", content: `{"port": 9000, "key": "   "}`},
		{name: "数字钥匙", content: `{"port": 9000, "key": 123}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			writeProfileFile(t, dataDir, test.content)
			if got, err := Load(dataDir); err == nil {
				t.Fatalf("Load() = %+v, want error", got)
			}
		})
	}
}

func TestLoadMissingFileKeepsErrNotExist(t *testing.T) {
	if _, err := Load(t.TempDir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load() error = %v, want os.ErrNotExist", err)
	}
}

func TestParseRejectsPortValueNotWithinRangeMessage(t *testing.T) {
	path := datapaths.ServiceProfileFile(t.TempDir())
	_, err := parse(path, []byte(`{"port": 65536, "key": "abc"}`))
	if err == nil || !strings.Contains(err.Error(), "65536") {
		t.Fatalf("parse() error = %v, want 提示越界端口", err)
	}
}

func TestListenAddr(t *testing.T) {
	profile := Profile{Port: 9123, Key: "abc"}
	if got := profile.ListenAddr(); got != "127.0.0.1:9123" {
		t.Fatalf("ListenAddr() = %q", got)
	}
}

func TestDocumentStructureStaysFixed(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := Ensure(dataDir); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	var document map[string]any
	payload, err := os.ReadFile(filepath.Join(dataDir, "service-profile.json"))
	if err != nil {
		t.Fatalf("读取画像文件失败：%v", err)
	}
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("画像不是有效 JSON：%v", err)
	}
	if len(document) != 2 {
		t.Fatalf("画像字段 = %v, want 只有 port/key", document)
	}
	if _, ok := document["port"].(float64); !ok {
		t.Fatalf("port 不是 JSON 数字：%T", document["port"])
	}
	if _, ok := document["key"].(string); !ok {
		t.Fatalf("key 不是 JSON 字符串：%T", document["key"])
	}
}
