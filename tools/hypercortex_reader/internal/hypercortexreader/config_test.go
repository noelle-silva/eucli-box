package hypercortexreader

import (
	"encoding/json"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func userConfigInput(t *testing.T, payload string) types.ToolExecutionInput {
	t.Helper()
	config := map[string]any{}
	if err := json.Unmarshal([]byte(payload), &config); err != nil {
		t.Fatal(err)
	}
	return types.ToolExecutionInput{UserConfig: config}
}

func TestLoadRepoConfigNormalizesEntries(t *testing.T) {
	input := userConfigInput(t, `{"endpoint":" http://127.0.0.1:1234/ ","defaultRepo":" 笔记 ","repos":[{"id":" 笔记 ","description":" 描述 ","key":" secret "}]}`)

	config, err := loadRepoConfig(input)
	if err != nil {
		t.Fatalf("loadRepoConfig failed: %v", err)
	}
	if config.Endpoint != "http://127.0.0.1:1234" {
		t.Fatalf("endpoint = %q", config.Endpoint)
	}
	if config.DefaultRepo != "笔记" || len(config.Repos) != 1 {
		t.Fatalf("config = %#v", config)
	}
	if config.Repos[0].ID != "笔记" || config.Repos[0].Description != "描述" || config.Repos[0].Key != "secret" {
		t.Fatalf("entry = %#v", config.Repos[0])
	}
	entry, err := config.resolve("")
	if err != nil || entry.Key != "secret" {
		t.Fatalf("resolve default = %#v, err = %v", entry, err)
	}
	if _, err := config.resolve("未注册"); err == nil || !strings.Contains(err.Error(), "已注册：笔记") {
		t.Fatalf("resolve unknown err = %v", err)
	}
}

func TestLoadRepoConfigRejectsInvalidPayloads(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{"missing endpoint", `{"defaultRepo":"notes","repos":[{"id":"notes","key":"k"}]}`, "缺少访问地址"},
		{"bad scheme", `{"endpoint":"ftp://127.0.0.1:1","defaultRepo":"notes","repos":[{"id":"notes","key":"k"}]}`, "只支持 http/https"},
		{"endpoint with path", `{"endpoint":"http://127.0.0.1:1/rpc","defaultRepo":"notes","repos":[{"id":"notes","key":"k"}]}`, "纯地址"},
		{"empty repos", `{"endpoint":"http://127.0.0.1:1","defaultRepo":"notes","repos":[]}`, "至少注册一条仓库条目"},
		{"duplicate id", `{"endpoint":"http://127.0.0.1:1","defaultRepo":"a","repos":[{"id":"a","key":"k"},{"id":"a","key":"k2"}]}`, "重复的仓库 id"},
		{"missing key", `{"endpoint":"http://127.0.0.1:1","defaultRepo":"a","repos":[{"id":"a"}]}`, "缺少访问钥匙"},
		{"missing default", `{"endpoint":"http://127.0.0.1:1","repos":[{"id":"a","key":"k"}]}`, "必须指定默认仓库"},
		{"unknown default", `{"endpoint":"http://127.0.0.1:1","defaultRepo":"b","repos":[{"id":"a","key":"k"}]}`, "未在仓库条目中注册"},
		{"unknown entry field", `{"endpoint":"http://127.0.0.1:1","defaultRepo":"a","repos":[{"id":"a","key":"k","oops":1}]}`, "格式无效"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := loadRepoConfig(userConfigInput(t, testCase.payload))
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("err = %v, want containing %q", err, testCase.want)
			}
		})
	}
}

func TestEffectiveMaxOutputCharsAllowsLowerUserBound(t *testing.T) {
	config := Config{Limits: LimitsConfig{MaxOutputChars: 50000}}
	base := types.ToolExecutionInput{}
	if limit, err := effectiveMaxOutputChars(base, config); err != nil || limit != 50000 {
		t.Fatalf("limit = %d, err = %v", limit, err)
	}
	lowered := types.ToolExecutionInput{UserConfig: map[string]any{"maxOutputChars": 1000}}
	if limit, err := effectiveMaxOutputChars(lowered, config); err != nil || limit != 1000 {
		t.Fatalf("limit = %d, err = %v", limit, err)
	}
	exceeding := types.ToolExecutionInput{UserConfig: map[string]any{"maxOutputChars": 60000}}
	if _, err := effectiveMaxOutputChars(exceeding, config); err == nil || !strings.Contains(err.Error(), "must not exceed") {
		t.Fatalf("err = %v", err)
	}
}
