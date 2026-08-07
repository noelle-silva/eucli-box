package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func requestGateway(t *testing.T, system System, method string, path string, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	system.Handler().ServeHTTP(recorder, request)
	var payload map[string]any
	if recorder.Body.Len() > 0 {
		_ = json.Unmarshal(recorder.Body.Bytes(), &payload)
	}
	return recorder, payload
}

func TestToolInstallStateRoute(t *testing.T) {
	system := newTestGateway(t, newGatewayFakes())
	recorder, payload := requestGateway(t, system, http.MethodGet, "/api/tools/demo/install-state", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v", payload)
	}
	if data["status"] != types.ArtifactStatusNotInstalled {
		t.Fatalf("data = %#v", data)
	}
}

func TestInstallToolRoute(t *testing.T) {
	system := newTestGateway(t, newGatewayFakes())
	recorder, payload := requestGateway(t, system, http.MethodPost, "/api/tools/demo/install", "{}")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v", payload)
	}
	if data["status"] != types.ArtifactStatusActive || data["currentVersion"] != "0.1.0" {
		t.Fatalf("data = %#v", data)
	}
}

func TestUpdateToolRoute(t *testing.T) {
	system := newTestGateway(t, newGatewayFakes())
	recorder, payload := requestGateway(t, system, http.MethodPost, "/api/tools/demo/update", "{}")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v", payload)
	}
	if data["currentVersion"] != "0.1.1" {
		t.Fatalf("data = %#v", data)
	}
}

func TestInstallToolRouteRejectsNonEmptyBody(t *testing.T) {
	system := newTestGateway(t, newGatewayFakes())
	recorder, _ := requestGateway(t, system, http.MethodPost, "/api/tools/demo/install", `{"url":"https://example.com/tool.zip"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestPluginInstallStateRoute(t *testing.T) {
	system := newTestGateway(t, newGatewayFakes())
	recorder, payload := requestGateway(t, system, http.MethodGet, "/api/system-plugins/demo/install-state", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v", payload)
	}
	if data["status"] != types.ArtifactStatusNotInstalled {
		t.Fatalf("data = %#v", data)
	}
}

func TestInstallPluginRoute(t *testing.T) {
	system := newTestGateway(t, newGatewayFakes())
	recorder, payload := requestGateway(t, system, http.MethodPost, "/api/system-plugins/demo/install", "{}")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v", payload)
	}
	if data["status"] != types.ArtifactStatusActive || data["currentVersion"] != "0.1.0" {
		t.Fatalf("data = %#v", data)
	}
}

func TestUpdatePluginRoute(t *testing.T) {
	system := newTestGateway(t, newGatewayFakes())
	recorder, payload := requestGateway(t, system, http.MethodPost, "/api/system-plugins/demo/update", "{}")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v", payload)
	}
	if data["currentVersion"] != "0.1.1" {
		t.Fatalf("data = %#v", data)
	}
}
