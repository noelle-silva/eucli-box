package everything

import (
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func TestParseRequestDefaultsToSearchAction(t *testing.T) {
	request, err := parseRequest(types.ToolExecutionInput{Arguments: map[string]any{"query": "notes"}}, fixtureConfig())
	if err != nil {
		t.Fatal(err)
	}
	if request.Action != actionSearch || request.Query != "notes" {
		t.Fatalf("request = %#v", request)
	}
}

func TestParseRequestRejectsUnknownAction(t *testing.T) {
	_, err := parseRequest(types.ToolExecutionInput{Arguments: map[string]any{"action": "explode", "query": "notes"}}, fixtureConfig())
	if err == nil || !strings.Contains(err.Error(), "action") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseRequestRejectsSearchOnlyArgumentsForStewardActions(t *testing.T) {
	for _, action := range []actionName{actionAuthorize, actionIndex} {
		for _, key := range searchOnlyArgumentNames {
			args := map[string]any{"action": string(action), key: "value"}
			if _, err := parseRequest(types.ToolExecutionInput{Arguments: args}, fixtureConfig()); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("%s with %s: err = %v", action, key, err)
			}
		}
	}
}

func TestParseRequestAcceptsCommonArgumentsForStewardActions(t *testing.T) {
	config := fixtureConfig()
	authorize, err := parseRequest(types.ToolExecutionInput{Arguments: map[string]any{"action": "authorize", "description": "install steward", "timeoutMs": 9000}}, config)
	if err != nil {
		t.Fatal(err)
	}
	if authorize.Action != actionAuthorize || authorize.Description != "install steward" || authorize.TimeoutMs != 9000 {
		t.Fatalf("authorize request = %#v", authorize)
	}
	index, err := parseRequest(types.ToolExecutionInput{Arguments: map[string]any{"action": "index", "description": "build index", "timeoutMs": 9000}}, config)
	if err != nil {
		t.Fatal(err)
	}
	if index.Action != actionIndex || index.ScopeMode != scopeModeAllLocalDrives || index.InstanceName != config.Runtime.DefaultInstanceName {
		t.Fatalf("index request = %#v", index)
	}
	if index.Description != "build index" || index.TimeoutMs != 9000 {
		t.Fatalf("index request = %#v", index)
	}
}

func TestParseRequestBindsFullDiskSearchToDefaultInstance(t *testing.T) {
	config := fixtureConfig()
	if _, err := parseRequest(types.ToolExecutionInput{Arguments: map[string]any{"query": "notes", "instanceName": "another"}}, config); err == nil || !strings.Contains(err.Error(), "default Everything instance") {
		t.Fatalf("err = %v", err)
	}
	request, err := parseRequest(types.ToolExecutionInput{Arguments: map[string]any{"query": "notes", "instanceName": config.Runtime.DefaultInstanceName}}, config)
	if err != nil {
		t.Fatal(err)
	}
	if request.InstanceName != config.Runtime.DefaultInstanceName {
		t.Fatalf("request = %#v", request)
	}
}

func TestParseRequestAllowsScopedSearchWithCustomInstance(t *testing.T) {
	config := fixtureConfig()
	scopeDir := t.TempDir()
	request, err := parseRequest(types.ToolExecutionInput{
		Arguments:            map[string]any{"query": "notes", "scopePath": scopeDir, "instanceName": "custom"},
		HostWorkingDirectory: scopeDir,
	}, config)
	if err != nil {
		t.Fatal(err)
	}
	if request.InstanceName != "custom" || request.ScopeMode != scopeModeDirectory {
		t.Fatalf("request = %#v", request)
	}
}
