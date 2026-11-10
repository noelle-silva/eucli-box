package gateway

import (
	"net/http"
	"testing"

	"eucli-box/pkg/installsource"
)

func sourceList(t *testing.T, payload map[string]any) []map[string]any {
	t.Helper()
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v", payload)
	}
	raw, ok := data["sources"].([]any)
	if !ok {
		t.Fatalf("data = %#v", data)
	}
	result := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		source, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("source = %#v", item)
		}
		result = append(result, source)
	}
	return result
}

func TestSourceRoutesManageRemoteSource(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)

	recorder, payload := requestGateway(t, system, http.MethodGet, "/api/sources/tool", "")
	if recorder.Code != http.StatusOK || len(sourceList(t, payload)) != 0 {
		t.Fatalf("GET sources status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder, payload = requestGateway(t, system, http.MethodPost, "/api/sources/tool", `{"name":"社区商店","kind":"remote","indexUrl":"https://store.example.com/index.json"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST source status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	sources := sourceList(t, payload)
	if len(sources) != 1 || sources[0]["kind"] != installsource.SourceKindRemote || sources[0]["indexUrl"] != "https://store.example.com/index.json" {
		t.Fatalf("sources = %#v", sources)
	}

	recorder, _ = requestGateway(t, system, http.MethodPut, "/api/install-source/tool", `{"source":"社区商店"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT source status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder, payload = requestGateway(t, system, http.MethodPatch, "/api/sources/tool", `{"name":"社区商店","newIndexUrl":"https://mirror.example.com/index.json"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PATCH source status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	sources = sourceList(t, payload)
	if len(sources) != 1 || sources[0]["indexUrl"] != "https://mirror.example.com/index.json" {
		t.Fatalf("patched sources = %#v", sources)
	}

	recorder, payload = requestGateway(t, system, http.MethodDelete, "/api/sources/tool", `{"name":"社区商店"}`)
	if recorder.Code != http.StatusOK || len(sourceList(t, payload)) != 0 {
		t.Fatalf("DELETE source status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if _, payload = requestGateway(t, system, http.MethodGet, "/api/install-source/tool", ""); sourceData(t, payload) != installsource.OfficialSource {
		t.Fatalf("source after delete = %s", recorder.Body.String())
	}
}

func TestSourceRoutesRejectInvalidRequests(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)

	if recorder, _ := requestGateway(t, system, http.MethodPost, "/api/sources/tool", `{"kind":"remote"}`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("POST without name status = %d", recorder.Code)
	}
	if recorder, _ := requestGateway(t, system, http.MethodPost, "/api/sources/plugin", `{"name":"甲","kind":"remote","indexUrl":"http://store.example.com/index.json"}`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("POST invalid address status = %d", recorder.Code)
	}
	if recorder, _ := requestGateway(t, system, http.MethodPatch, "/api/sources/tool", `{"newPath":"D:/a"}`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("PATCH without name status = %d", recorder.Code)
	}
	if recorder, _ := requestGateway(t, system, http.MethodDelete, "/api/sources/tool", `{}`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("DELETE without name status = %d", recorder.Code)
	}
	if recorder, _ := requestGateway(t, system, http.MethodGet, "/api/sources/box", ""); recorder.Code != http.StatusBadRequest {
		t.Fatalf("GET unknown kind status = %d", recorder.Code)
	}
}
