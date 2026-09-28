package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"eucli-box/pkg/types"
)

func postImport(t *testing.T, system System, path string, payload []byte) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/zip")
	recorder := httptest.NewRecorder()
	system.Handler().ServeHTTP(recorder, request)
	var decoded map[string]any
	if recorder.Body.Len() > 0 {
		_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	}
	return recorder, decoded
}

// TestImportToolRoute 上传的压缩包原样交给工具系统并返回运行态。
func TestImportToolRoute(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)
	payload := []byte("PK\x03\x04 tool import payload")
	recorder, decoded := postImport(t, system, "/api/tools/import", payload)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Equal(fakes.tools.lastImportBytes, payload) {
		t.Fatalf("import bytes = %q", fakes.tools.lastImportBytes)
	}
	data, ok := decoded["data"].(map[string]any)
	if !ok || data["status"] != types.ArtifactStatusPreparing {
		t.Fatalf("payload = %#v", decoded)
	}
}

// TestImportPluginRoute 上传的压缩包原样交给插件系统并返回运行态。
func TestImportPluginRoute(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)
	payload := []byte("PK\x03\x04 plugin import payload")
	recorder, decoded := postImport(t, system, "/api/system-plugins/import", payload)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Equal(fakes.systemPlugins.lastImportBytes, payload) {
		t.Fatalf("import bytes = %q", fakes.systemPlugins.lastImportBytes)
	}
	data, ok := decoded["data"].(map[string]any)
	if !ok || data["status"] != types.ArtifactStatusPreparing {
		t.Fatalf("payload = %#v", decoded)
	}
}
