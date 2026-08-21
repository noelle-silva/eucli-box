package gateway

import (
	"errors"
	"net/http"
	"testing"

	"eucli-box/pkg/installsource"
)

func TestInstallSourceRoutes(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)

	recorder, payload := requestGateway(t, system, http.MethodGet, "/api/install-source", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if data, ok := payload["data"].(map[string]any); !ok || data["kind"] != string(installsource.KindOfficial) {
		t.Fatalf("GET data = %#v", payload)
	}

	recorder, payload = requestGateway(t, system, http.MethodPut, "/api/install-source", `{"kind":"development"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if data, ok := payload["data"].(map[string]any); !ok || data["kind"] != string(installsource.KindDevelopment) {
		t.Fatalf("PUT data = %#v", payload)
	}
	if fakes.installSource.current != installsource.KindDevelopment {
		t.Fatalf("current = %q, want development", fakes.installSource.current)
	}

	recorder, payload = requestGateway(t, system, http.MethodGet, "/api/install-source", "")
	if data, ok := payload["data"].(map[string]any); !ok || data["kind"] != string(installsource.KindDevelopment) {
		t.Fatalf("GET after switch data = %#v", payload)
	}
}

func TestInstallSourceRouteRejectsInvalidKind(t *testing.T) {
	system := newTestGateway(t, newGatewayFakes())
	recorder, _ := requestGateway(t, system, http.MethodPut, "/api/install-source", `{"kind":"internal"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	recorder, _ = requestGateway(t, system, http.MethodPut, "/api/install-source", `{"kind":123}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("bad body status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestInstallSourceRouteRejectsImmutableState(t *testing.T) {
	fakes := newGatewayFakes()
	fakes.installSource.mutable = false
	system := newTestGateway(t, fakes)
	recorder, _ := requestGateway(t, system, http.MethodPut, "/api/install-source", `{"kind":"development"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if fakes.installSource.current != installsource.KindOfficial {
		t.Fatalf("current = %q, want official (unchanged)", fakes.installSource.current)
	}
}

func TestInstallSourceRoutesAbsentWhenNotConfigured(t *testing.T) {
	fakes := newGatewayFakes()
	fakes.installSource = nil
	system := newTestGateway(t, fakes)
	recorder, _ := requestGateway(t, system, http.MethodGet, "/api/install-source", "")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	recorder, _ = requestGateway(t, system, http.MethodPut, "/api/install-source", `{"kind":"development"}`)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestInstallSourceRoutePropagatesSetError(t *testing.T) {
	fakes := newGatewayFakes()
	fakes.installSource.setErr = errors.New("不可切换")
	system := newTestGateway(t, fakes)
	recorder, _ := requestGateway(t, system, http.MethodPut, "/api/install-source", `{"kind":"development"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}
