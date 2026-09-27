package gateway

import (
	"errors"
	"net/http"
	"testing"

	"eucli-box/pkg/installsource"
)

func sourceData(t *testing.T, payload map[string]any) string {
	t.Helper()
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v", payload)
	}
	source, _ := data["source"].(string)
	return source
}

func problemData(t *testing.T, payload map[string]any) string {
	t.Helper()
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v", payload)
	}
	problem, _ := data["problem"].(string)
	return problem
}

func shelfNames(t *testing.T, payload map[string]any) []map[string]any {
	t.Helper()
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v", payload)
	}
	raw, ok := data["shelves"].([]any)
	if !ok {
		t.Fatalf("data = %#v", data)
	}
	result := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		shelf, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("shelf = %#v", item)
		}
		result = append(result, shelf)
	}
	return result
}

func TestInstallSourceAndShelfRoutes(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)

	recorder, payload := requestGateway(t, system, http.MethodGet, "/api/install-source", "")
	if recorder.Code != http.StatusOK || sourceData(t, payload) != installsource.OfficialSource {
		t.Fatalf("GET status = %d payload=%s", recorder.Code, recorder.Body.String())
	}

	recorder, payload = requestGateway(t, system, http.MethodPost, "/api/shelves", `{"name":"甲","path":"D:/shelf-a"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	shelves := shelfNames(t, payload)
	if len(shelves) != 1 || shelves[0]["name"] != "甲" || shelves[0]["path"] != "D:/shelf-a" {
		t.Fatalf("shelves = %#v", shelves)
	}

	recorder, payload = requestGateway(t, system, http.MethodPut, "/api/install-source", `{"source":"甲"}`)
	if recorder.Code != http.StatusOK || sourceData(t, payload) != "甲" {
		t.Fatalf("PUT status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if fakes.installSource.source != "甲" || len(fakes.installSource.sets) != 1 || fakes.installSource.sets[0] != "甲" {
		t.Fatalf("fake source = %#v", fakes.installSource)
	}

	recorder, payload = requestGateway(t, system, http.MethodPatch, "/api/shelves", `{"name":"甲","newName":"甲改"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PATCH rename status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if shelves = shelfNames(t, payload); shelves[0]["name"] != "甲改" {
		t.Fatalf("renamed shelves = %#v", shelves)
	}
	_, payload = requestGateway(t, system, http.MethodGet, "/api/install-source", "")
	if sourceData(t, payload) != "甲改" {
		t.Fatalf("source after rename = %s", recorder.Body.String())
	}

	recorder, payload = requestGateway(t, system, http.MethodPatch, "/api/shelves", `{"name":"甲改","newPath":"D:/moved"}`)
	if recorder.Code != http.StatusOK || shelfNames(t, payload)[0]["path"] != "D:/moved" {
		t.Fatalf("PATCH path status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder, payload = requestGateway(t, system, http.MethodGet, "/api/shelves", "")
	if recorder.Code != http.StatusOK || len(shelfNames(t, payload)) != 1 {
		t.Fatalf("GET shelves status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder, payload = requestGateway(t, system, http.MethodDelete, "/api/shelves", `{"name":"甲改"}`)
	if recorder.Code != http.StatusOK || len(shelfNames(t, payload)) != 0 {
		t.Fatalf("DELETE status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	_, payload = requestGateway(t, system, http.MethodGet, "/api/install-source", "")
	if sourceData(t, payload) != installsource.OfficialSource {
		t.Fatalf("source after delete = %#v", payload)
	}
}

func TestInstallSourceRoutesRejectInvalidBodies(t *testing.T) {
	system := newTestGateway(t, newGatewayFakes())
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPut, path: "/api/install-source", body: `{}`},
		{method: http.MethodPut, path: "/api/install-source", body: `{"source":123}`},
		{method: http.MethodPut, path: "/api/install-source", body: `not-json`},
		{method: http.MethodPost, path: "/api/shelves", body: `{"name":"甲"}`},
		{method: http.MethodPost, path: "/api/shelves", body: `{}`},
		{method: http.MethodPost, path: "/api/shelves", body: `not-json`},
		{method: http.MethodPatch, path: "/api/shelves", body: `{}`},
		{method: http.MethodPatch, path: "/api/shelves", body: `not-json`},
		{method: http.MethodDelete, path: "/api/shelves", body: `{}`},
		{method: http.MethodDelete, path: "/api/shelves", body: `not-json`},
	}
	for _, item := range cases {
		recorder, _ := requestGateway(t, system, item.method, item.path, item.body)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s %s body=%s status = %d", item.method, item.path, item.body, recorder.Code)
		}
	}
}

func TestInstallSourceRoutesPropagateErrors(t *testing.T) {
	expected := errors.New("修改失败")
	cases := []struct {
		name   string
		setup  func(*fakeGatewayInstallSource)
		method string
		path   string
		body   string
	}{
		{name: "切换来源", setup: func(f *fakeGatewayInstallSource) { f.setErr = expected }, method: http.MethodPut, path: "/api/install-source", body: `{"source":"甲"}`},
		{name: "注册货架", setup: func(f *fakeGatewayInstallSource) { f.addErr = expected }, method: http.MethodPost, path: "/api/shelves", body: `{"name":"甲","path":"D:/a"}`},
		{name: "改货架", setup: func(f *fakeGatewayInstallSource) { f.updErr = expected }, method: http.MethodPatch, path: "/api/shelves", body: `{"name":"甲","newPath":"D:/b"}`},
		{name: "删货架", setup: func(f *fakeGatewayInstallSource) { f.delErr = expected }, method: http.MethodDelete, path: "/api/shelves", body: `{"name":"甲"}`},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			fakes := newGatewayFakes()
			item.setup(fakes.installSource)
			system := newTestGateway(t, fakes)
			recorder, _ := requestGateway(t, system, item.method, item.path, item.body)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d want 400, body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestInstallSourceRoutesCarryProblemAndAllowRepair(t *testing.T) {
	fakes := newGatewayFakes()
	fakes.installSource.problem = "旧格式无法识别"
	system := newTestGateway(t, fakes)

	recorder, payload := requestGateway(t, system, http.MethodGet, "/api/install-source", "")
	if recorder.Code != http.StatusOK || sourceData(t, payload) != "" || problemData(t, payload) != "旧格式无法识别" {
		t.Fatalf("GET status = %d payload=%s", recorder.Code, recorder.Body.String())
	}
	recorder, payload = requestGateway(t, system, http.MethodGet, "/api/shelves", "")
	if recorder.Code != http.StatusOK || problemData(t, payload) != "旧格式无法识别" {
		t.Fatalf("GET shelves status = %d payload=%s", recorder.Code, recorder.Body.String())
	}
	recorder, _ = requestGateway(t, system, http.MethodPut, "/api/install-source", `{"source":"甲"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("PUT shelf in problem mode status = %d, want 400", recorder.Code)
	}

	fakes = newGatewayFakes()
	fakes.installSource.problem = "旧格式无法识别"
	system = newTestGateway(t, fakes)
	recorder, payload = requestGateway(t, system, http.MethodPut, "/api/install-source", `{"source":"official"}`)
	if recorder.Code != http.StatusOK || sourceData(t, payload) != installsource.OfficialSource || problemData(t, payload) != "" {
		t.Fatalf("PUT official repair status = %d payload=%s", recorder.Code, recorder.Body.String())
	}

	fakes = newGatewayFakes()
	fakes.installSource.problem = "旧格式无法识别"
	system = newTestGateway(t, fakes)
	recorder, payload = requestGateway(t, system, http.MethodPost, "/api/shelves", `{"name":"甲","path":"D:/shelf-a"}`)
	if recorder.Code != http.StatusOK || len(shelfNames(t, payload)) != 1 || problemData(t, payload) != "" {
		t.Fatalf("POST shelf repair status = %d payload=%s", recorder.Code, recorder.Body.String())
	}
}

func TestInstallSourceRoutesAbsentWhenNotConfigured(t *testing.T) {
	fakes := newGatewayFakes()
	fakes.installSource = nil
	system := newTestGateway(t, fakes)
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/install-source", body: ""},
		{method: http.MethodPut, path: "/api/install-source", body: `{"source":"official"}`},
		{method: http.MethodGet, path: "/api/shelves", body: ""},
		{method: http.MethodPost, path: "/api/shelves", body: `{"name":"甲","path":"D:/a"}`},
		{method: http.MethodPatch, path: "/api/shelves", body: `{"name":"甲","newPath":"D:/b"}`},
		{method: http.MethodDelete, path: "/api/shelves", body: `{"name":"甲"}`},
	}
	for _, item := range cases {
		recorder, _ := requestGateway(t, system, item.method, item.path, item.body)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s %s status = %d, want 404", item.method, item.path, recorder.Code)
		}
	}
}
