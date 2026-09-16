package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActionDefinitionUnmarshal(t *testing.T) {
	var text actionDefinition
	if err := json.Unmarshal([]byte(`"echo helloworld"`), &text); err != nil {
		t.Fatalf("字符串形态解析失败：%v", err)
	}
	if text.Command != "echo helloworld" || len(text.Artifact) != 0 {
		t.Fatalf("字符串形态 = %#v", text)
	}

	var object actionDefinition
	if err := json.Unmarshal([]byte(`{"command":"go run demo","artifact":{"path":"output.path"}}`), &object); err != nil {
		t.Fatalf("对象形态解析失败：%v", err)
	}
	if object.Command != "go run demo" || object.Artifact["path"] != "output.path" {
		t.Fatalf("对象形态 = %#v", object)
	}
}

func TestValueAtPath(t *testing.T) {
	root := map[string]any{"outer": map[string]any{"inner": "value"}, "size": 42.0}
	if value, ok := valueAtPath(root, "outer.inner"); !ok || value != "value" {
		t.Fatalf("valueAtPath(outer.inner) = %q, %v", value, ok)
	}
	for _, path := range []string{"outer", "outer.inner.deep", "size", "missing"} {
		if _, ok := valueAtPath(root, path); ok {
			t.Fatalf("valueAtPath(%q) 应该取不到", path)
		}
	}
}

func TestRunMainProducesReceiptWithArtifact(t *testing.T) {
	protocolFile := filepath.Join(t.TempDir(), protocolFileName)
	content := `{
  "runner": "go run",
  "actions": {
    "demo": {
      "command": "echo {\"name\":\"box.zip\",\"size\":\"42\"}",
      "artifact": {"name": "name", "size": "size"}
    }
  }
}
`
	if err := os.WriteFile(protocolFile, []byte(content), 0o644); err != nil {
		t.Fatalf("写入协议文件失败：%v", err)
	}

	var output bytes.Buffer
	if status := runMain([]string{"demo"}, protocolFile, &output); status != 0 {
		t.Fatalf("runMain() = %d，输出：%s", status, output.String())
	}
	parsed := unmarshalReceipt(t, output.String())
	if parsed.ContractVersion != contractVersion || parsed.Status != statusSucceeded {
		t.Fatalf("receipt = %#v", parsed)
	}
	if parsed.ExitCode == nil || *parsed.ExitCode != 0 || parsed.Error != "" {
		t.Fatalf("receipt = %#v", parsed)
	}
	result, ok := parsed.Data.Result.(map[string]any)
	if !ok || result["name"] != "box.zip" {
		t.Fatalf("data.result = %#v", parsed.Data.Result)
	}
	if parsed.Data.Artifact["name"] != "box.zip" || parsed.Data.Artifact["size"] != "42" {
		t.Fatalf("data.artifact = %#v", parsed.Data.Artifact)
	}
}

func TestRunMainUnknownActionFails(t *testing.T) {
	protocolFile := filepath.Join(t.TempDir(), protocolFileName)
	if err := os.WriteFile(protocolFile, []byte(`{"actions":{"known":"echo ok"}}`), 0o644); err != nil {
		t.Fatalf("写入协议文件失败：%v", err)
	}

	var output bytes.Buffer
	if status := runMain([]string{"missing"}, protocolFile, &output); status == 0 {
		t.Fatal("未知动作应该以非零状态结束")
	}
	parsed := unmarshalReceipt(t, output.String())
	if parsed.Status != statusFailed || parsed.ExitCode != nil || parsed.Error == "" {
		t.Fatalf("receipt = %#v", parsed)
	}
	if len(parsed.Data.Artifact) != 0 || parsed.Data.Result != nil {
		t.Fatalf("data = %#v", parsed.Data)
	}
}

func unmarshalReceipt(t *testing.T, output string) receipt {
	t.Helper()
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := lines[index]
		if !strings.HasPrefix(line, receiptPrefix) {
			continue
		}
		var parsed receipt
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, receiptPrefix)), &parsed); err != nil {
			t.Fatalf("回执解析失败：%v（%q）", err, line)
		}
		return parsed
	}
	t.Fatalf("输出中没有回执行：%q", output)
	return receipt{}
}
