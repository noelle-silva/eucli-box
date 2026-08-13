package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

// fakeGatewayAccess 是访问系统的测试替身。
type fakeGatewayAccess struct {
	ports []types.PersistentPort
	keys  []types.PersistentKeyView
	verifyResult types.PersistentKeyVerifyResult
}

func (f *fakeGatewayAccess) ListPorts(ctx context.Context) ([]types.PersistentPort, error) {
	return f.ports, nil
}

func (f *fakeGatewayAccess) AddPort(ctx context.Context, name string, port int) (types.PersistentPort, error) {
	record := types.PersistentPort{ID: "port-1", Name: name, Port: port, DesiredState: types.PersistentPortDesiredDisabled, ActualState: types.PersistentPortActualStopped, CreatedAt: "2026-01-01T00:00:00Z"}
	f.ports = append(f.ports, record)
	return record, nil
}

func (f *fakeGatewayAccess) EnablePort(ctx context.Context, id string) (types.PersistentPort, error) {
	for i := range f.ports {
		if f.ports[i].ID == id {
			f.ports[i].DesiredState = types.PersistentPortDesiredEnabled
			f.ports[i].ActualState = types.PersistentPortActualRunning
			return f.ports[i], nil
		}
	}
	return types.PersistentPort{}, &accessNotFoundError{}
}

func (f *fakeGatewayAccess) DisablePort(ctx context.Context, id string) (types.PersistentPort, error) {
	for i := range f.ports {
		if f.ports[i].ID == id {
			f.ports[i].DesiredState = types.PersistentPortDesiredDisabled
			f.ports[i].ActualState = types.PersistentPortActualStopped
			return f.ports[i], nil
		}
	}
	return types.PersistentPort{}, &accessNotFoundError{}
}

func (f *fakeGatewayAccess) DeletePort(ctx context.Context, id string) error {
	for i := range f.ports {
		if f.ports[i].ID == id {
			f.ports = append(f.ports[:i], f.ports[i+1:]...)
			return nil
		}
	}
	return &accessNotFoundError{}
}

func (f *fakeGatewayAccess) ListKeys(ctx context.Context) ([]types.PersistentKeyView, error) {
	return f.keys, nil
}

func (f *fakeGatewayAccess) CreateKey(ctx context.Context, name string, expiresAt *string) (types.PersistentKeyCreated, error) {
	record := types.PersistentKeyCreated{ID: "key-1", Name: name, PlainKey: "plain-key-value", ExpiresAt: expiresAt, CreatedAt: "2026-01-01T00:00:00Z"}
	f.keys = append(f.keys, types.PersistentKeyView{ID: record.ID, Name: name, Enabled: true, ExpiresAt: expiresAt, CreatedAt: record.CreatedAt})
	return record, nil
}

func (f *fakeGatewayAccess) RevealKey(ctx context.Context, id string) (string, error) {
	if id != "key-1" {
		return "", &accessNotFoundError{}
	}
	return "plain-key-value", nil
}

func (f *fakeGatewayAccess) SetKeyEnabled(ctx context.Context, id string, enabled bool) error {
	for i := range f.keys {
		if f.keys[i].ID == id {
			f.keys[i].Enabled = enabled
			return nil
		}
	}
	return &accessNotFoundError{}
}

func (f *fakeGatewayAccess) SetKeyExpiration(ctx context.Context, id string, expiresAt *string) error {
	for i := range f.keys {
		if f.keys[i].ID == id {
			f.keys[i].ExpiresAt = expiresAt
			return nil
		}
	}
	return &accessNotFoundError{}
}

func (f *fakeGatewayAccess) DeleteKey(ctx context.Context, id string) error {
	for i := range f.keys {
		if f.keys[i].ID == id {
			f.keys = append(f.keys[:i], f.keys[i+1:]...)
			return nil
		}
	}
	return &accessNotFoundError{}
}

func (f *fakeGatewayAccess) VerifyKey(ctx context.Context, providedKey string) types.PersistentKeyVerifyResult {
	if f.verifyResult.Valid && f.verifyResult.KeyID != "" {
		return f.verifyResult
	}
	return types.PersistentKeyVerifyResult{Valid: false}
}

func (f *fakeGatewayAccess) RegisterConnection(keyID string, closer interface{ Close() error }) {}
func (f *fakeGatewayAccess) UnregisterConnection(keyID string, closer interface{ Close() error }) {}

type accessNotFoundError struct{}

func (e *accessNotFoundError) Error() string { return "not found" }

func newLocalAccessTestGateway(t *testing.T, fakes *gatewayFakes, access AccessSystem) System {
	t.Helper()
	system, err := NewSystem(Config{LocalRun: true, LocalCredential: "session-credential-0000000000000000000000000000000000000000000000000000000000000000", LocalStop: func() {}, Access: access}, fakes.runtime, fakes.roles, fakes.groups, fakes.workspaces, fakes.providers, fakes.tools, fakes.sessions, fakes.stickers, fakes.hooks, fakes.placeholders, fakes.systemPlugins, fakes.assist, fakes.releaseChecks)
	if err != nil {
		t.Fatalf("NewSystem() error = %v", err)
	}
	return system
}

func accessRequest(method string, path string, credential string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "127.0.0.1:54321"
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	return req
}

func accessRequestWithBody(method string, path string, credential string, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:54321"
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	return req
}

func TestAccessRoutesRequireTrustedConnection(t *testing.T) {
	fakes := newGatewayFakes()
	access := &fakeGatewayAccess{verifyResult: types.PersistentKeyVerifyResult{Valid: true, KeyID: "key-1"}}
	system := newLocalAccessTestGateway(t, fakes, access)
	for _, path := range []string{
		"/api/access/persistent-ports",
		"/api/access/persistent-keys",
	} {
		rec := httptest.NewRecorder()
		system.Handler().ServeHTTP(rec, accessRequest(http.MethodGet, path, "long-term-key"))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("GET %s 长期 Key status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "长期 Key 无权管理访问设置") {
			t.Fatalf("GET %s 错误信息不明确：%s", path, rec.Body.String())
		}
	}
}

func TestAccessRoutesAllowSessionCredential(t *testing.T) {
	fakes := newGatewayFakes()
	access := &fakeGatewayAccess{}
	system := newLocalAccessTestGateway(t, fakes, access)
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, accessRequest(http.MethodGet, "/api/access/persistent-ports", "session-credential-0000000000000000000000000000000000000000000000000000000000000000"))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET persistent-ports status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAccessRoutesAddPortValidatesRange(t *testing.T) {
	fakes := newGatewayFakes()
	access := &fakeGatewayAccess{}
	system := newLocalAccessTestGateway(t, fakes, access)
	credential := "session-credential-0000000000000000000000000000000000000000000000000000000000000000"
	for _, body := range []string{`{"name":"a","port":0}`, `{"name":"a","port":65536}`, `{"name":"","port":8080}`} {
		req := accessRequestWithBody(http.MethodPost, "/api/access/persistent-ports", credential, body)
		rec := httptest.NewRecorder()
		system.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("POST persistent-ports %s status = %d body=%s", body, rec.Code, rec.Body.String())
		}
	}
}

func TestAccessRoutesKeyRevealAndExpiration(t *testing.T) {
	fakes := newGatewayFakes()
	access := &fakeGatewayAccess{}
	system := newLocalAccessTestGateway(t, fakes, access)
	credential := "session-credential-0000000000000000000000000000000000000000000000000000000000000000"

	// 先创建 Key
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, accessRequestWithBody(http.MethodPost, "/api/access/persistent-keys", credential, `{"name":"测试 Key"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create key status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, accessRequest(http.MethodGet, "/api/access/persistent-keys/key-1/reveal", credential))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "plain-key-value") {
		t.Fatalf("reveal status = %d body=%s", rec.Code, rec.Body.String())
	}

	req := accessRequestWithBody(http.MethodPut, "/api/access/persistent-keys/key-1/expiration", credential, `{"expiresAt":null}`)
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expiration status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBoxInfoAndShutdownRoutes(t *testing.T) {
	fakes := newGatewayFakes()
	system := newLocalAccessTestGateway(t, fakes, nil)

	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, accessRequest(http.MethodGet, "/api/box/info", "session-credential-0000000000000000000000000000000000000000000000000000000000000000"))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "version") {
		t.Fatalf("GET /api/box/info status = %d body=%s", rec.Code, rec.Body.String())
	}

	// 停止路由要求受托凭证：长期 Key 有效时明确返回 403
	rec = httptest.NewRecorder()
	fakes2 := newGatewayFakes()
	access2 := &fakeGatewayAccess{verifyResult: types.PersistentKeyVerifyResult{Valid: true, KeyID: "key-1"}}
	system2 := newLocalAccessTestGateway(t, fakes2, access2)
	system2.Handler().ServeHTTP(rec, accessRequest(http.MethodPost, "/api/box/shutdown", "long-term-key"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST /api/box/shutdown 长期 Key status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLongTermHandlerRejectsWithoutKey(t *testing.T) {
	fakes := newGatewayFakes()
	access := &fakeGatewayAccess{}
	system := newLocalAccessTestGateway(t, fakes, access)
	rec := httptest.NewRecorder()
	system.LongTermHandler().ServeHTTP(rec, accessRequest(http.MethodGet, "/api/release", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("LongTermHandler() 无 Key status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLongTermHandlerRejectsLocalRunPath(t *testing.T) {
	fakes := newGatewayFakes()
	access := &fakeGatewayAccess{verifyResult: types.PersistentKeyVerifyResult{Valid: true, KeyID: "key-1"}}
	system := newLocalAccessTestGateway(t, fakes, access)
	req := accessRequest(http.MethodGet, "/api/local-run", "valid-long-term-key")
	rec := httptest.NewRecorder()
	system.LongTermHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("LongTermHandler() local-run status = %d body=%s", rec.Code, rec.Body.String())
	}
}
