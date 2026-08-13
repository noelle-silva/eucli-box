package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func TestBoxActiveWorkReturnsRunningWork(t *testing.T) {
	fakes := newGatewayFakes()
	system := newLocalAccessTestGateway(t, fakes, &fakeGatewayAccess{})
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, accessRequest(http.MethodGet, "/api/box/active-work", "session-credential-0000000000000000000000000000000000000000000000000000000000000000"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			ActiveWork []types.RunState `json:"activeWork"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response error = %v", err)
	}
	if len(response.Data.ActiveWork) == 0 {
		t.Fatalf("active work must not be empty, body=%s", rec.Body.String())
	}
	if response.Data.ActiveWork[0].ID != "run-1" {
		t.Fatalf("active work = %#v", response.Data.ActiveWork)
	}
}

func TestBoxActiveWorkReturnsEmptyWhenNoWork(t *testing.T) {
	fakes := newGatewayFakes()
	fakes.runtime.runs = map[string]types.RunState{}
	system := newLocalAccessTestGateway(t, fakes, &fakeGatewayAccess{})
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, accessRequest(http.MethodGet, "/api/box/active-work", "session-credential-0000000000000000000000000000000000000000000000000000000000000000"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"activeWork":[]`) {
		t.Fatalf("active work must be empty list, body=%s", rec.Body.String())
	}
}

func TestBoxActiveWorkRejectsLongTermKey(t *testing.T) {
	fakes := newGatewayFakes()
	access := &fakeGatewayAccess{verifyResult: types.PersistentKeyVerifyResult{Valid: true, KeyID: "key-1"}}
	system := newLocalAccessTestGateway(t, fakes, access)
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, accessRequest(http.MethodGet, "/api/box/active-work", "long-term-key"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "长期 Key 无权管理访问设置") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestBoxActiveWorkRejectsWrongCredential(t *testing.T) {
	fakes := newGatewayFakes()
	system := newLocalAccessTestGateway(t, fakes, &fakeGatewayAccess{})
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, accessRequest(http.MethodGet, "/api/box/active-work", "wrong-credential"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}
