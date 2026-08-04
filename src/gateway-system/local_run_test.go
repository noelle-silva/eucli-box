package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLocalRunRoutesRequireLoopbackAndBearerCredential(t *testing.T) {
	var stops atomic.Int32
	system := newLocalRunTestGateway(t, func() { stops.Add(1) })

	request := httptest.NewRequest(http.MethodGet, "/api/local-run", nil)
	request.RemoteAddr = "127.0.0.1:43210"
	request.Header.Set("Authorization", "Bearer session-test")
	recorder := httptest.NewRecorder()
	system.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"runIdentity":"run-test"`) {
		t.Fatalf("local facts status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/local-run?token=session-test", nil)
	request.RemoteAddr = "127.0.0.1:43210"
	request.Header.Set("Authorization", "Bearer session-test")
	recorder = httptest.NewRecorder()
	system.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "local gateway does not accept query credentials") {
		t.Fatalf("query credential status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/local-run", nil)
	request.RemoteAddr = "192.0.2.10:43210"
	request.Header.Set("Authorization", "Bearer session-test")
	recorder = httptest.NewRecorder()
	system.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "local gateway only accepts loopback requests") {
		t.Fatalf("non-loopback status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestLocalRunStopIsIdentityBoundAndIdempotent(t *testing.T) {
	var stops atomic.Int32
	system := newLocalRunTestGateway(t, func() { stops.Add(1) })
	request := httptest.NewRequest(http.MethodPost, "/api/local-run/stop", strings.NewReader(`{"runIdentity":"wrong"}`))
	request.RemoteAddr = "127.0.0.1:43210"
	request.Header.Set("Authorization", "Bearer session-test")
	recorder := httptest.NewRecorder()
	system.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "local run identity mismatch") {
		t.Fatalf("wrong identity status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	for range 2 {
		request = httptest.NewRequest(http.MethodPost, "/api/local-run/stop", strings.NewReader(`{"runIdentity":"run-test"}`))
		request.RemoteAddr = "127.0.0.1:43210"
		request.Header.Set("Authorization", "Bearer session-test")
		recorder = httptest.NewRecorder()
		system.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("stop status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}
	deadline := time.Now().Add(time.Second)
	for stops.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if stops.Load() != 1 {
		t.Fatalf("stop callback count=%d, want one", stops.Load())
	}
}

func TestOrdinaryGatewayDoesNotExposeLocalRunRoutes(t *testing.T) {
	system := newTestGateway(t, newGatewayFakes())
	request := httptest.NewRequest(http.MethodGet, "/api/local-run", nil)
	recorder := httptest.NewRecorder()
	system.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("ordinary local route status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func newLocalRunTestGateway(t *testing.T, stop func()) System {
	t.Helper()
	fakes := newGatewayFakes()
	system, err := NewSystem(Config{
		Addr: "127.0.0.1:0", LocalRun: true, LocalInstallID: "install-test", LocalDataID: "data-test",
		LocalRunID: "run-test", LocalCredential: "session-test", LocalProcessID: 1234,
		LocalProcessStart: time.Now().UTC(), LocalStop: stop,
	}, fakes.runtime, fakes.roles, fakes.groups, fakes.workspaces, fakes.providers, fakes.tools, fakes.sessions, fakes.stickers, fakes.hooks, fakes.placeholders, fakes.systemPlugins, fakes.assist, fakes.releaseChecks)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	return system
}
