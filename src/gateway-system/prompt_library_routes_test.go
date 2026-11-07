package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

// 三作用域共用一组端点：scope 走路径，归属 id 走查询参数，原样透传到业务层。
func TestPromptLibraryRoutesForwardScopeAndOwner(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)

	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, accessRequest(http.MethodGet, "/api/prompt-library/role?ownerId=developer", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/prompt-library/role status = %d body=%s", rec.Code, rec.Body.String())
	}
	fakes.promptLibraries.assertLastCall(t, types.PromptScopeRole, "developer")

	body := `{"prompts":[{"id":"p1","name":"行文风格","content":"多用短句"}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/prompt-library/global", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/prompt-library/global status = %d body=%s", rec.Code, rec.Body.String())
	}
	fakes.promptLibraries.assertLastCall(t, types.PromptScopeGlobal, "")
	if len(fakes.promptLibraries.library.Prompts) != 1 || fakes.promptLibraries.library.Prompts[0].ID != "p1" {
		t.Fatalf("saved library = %#v", fakes.promptLibraries.library)
	}

	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, accessRequest(http.MethodDelete, "/api/prompt-library/workspace/prompts/p1?ownerId=frontend", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /api/prompt-library/workspace/prompts/p1 status = %d body=%s", rec.Code, rec.Body.String())
	}
	fakes.promptLibraries.assertLastCall(t, types.PromptScopeWorkspace, "frontend")
	if fakes.promptLibraries.deletedID != "p1" {
		t.Fatalf("deleted prompt id = %q", fakes.promptLibraries.deletedID)
	}
}

func (f *fakeGatewayPromptLibraries) assertLastCall(t *testing.T, scope string, ownerID string) {
	t.Helper()
	if f.scope != scope || f.ownerID != ownerID {
		t.Fatalf("prompt library call = scope %q owner %q, want %q/%q", f.scope, f.ownerID, scope, ownerID)
	}
}
