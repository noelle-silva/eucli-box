package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

// 收藏夹端点与提示词库共用同一套作用域参数规则，scope 走路径、归属 id 走查询参数。
func TestPromptFolderRoutesForwardScopeAndOwner(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)

	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, accessRequest(http.MethodGet, "/api/prompt-folders/role?ownerId=developer", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/prompt-folders/role status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.promptFolders.scope != types.PromptScopeRole || fakes.promptFolders.ownerID != "developer" {
		t.Fatalf("folder call = scope %q owner %q", fakes.promptFolders.scope, fakes.promptFolders.ownerID)
	}

	body := `{"rootFolderIds":["work"],"folders":[{"id":"work","name":"工作","folderIds":[],"promptIds":["p1"]}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/prompt-folders/workspace?ownerId=frontend", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/prompt-folders/workspace status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.promptFolders.scope != types.PromptScopeWorkspace || fakes.promptFolders.ownerID != "frontend" {
		t.Fatalf("folder call = scope %q owner %q", fakes.promptFolders.scope, fakes.promptFolders.ownerID)
	}
	if len(fakes.promptFolders.library.RootFolderIDs) != 1 || fakes.promptFolders.library.RootFolderIDs[0] != "work" {
		t.Fatalf("saved folder library = %#v", fakes.promptFolders.library)
	}
}

// 定点动作只带必要参数到达业务层：删除、改名、混排排序、收藏、取消收藏。
func TestPromptFolderActionRoutesForwardParameters(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)

	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, accessRequest(http.MethodDelete, "/api/prompt-folders/global/folders/f1", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE folder status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.promptFolders.folderID != "f1" {
		t.Fatalf("deleted folder id = %q", fakes.promptFolders.folderID)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/prompt-folders/role/folders/f2?ownerId=developer", strings.NewReader(`{"name":"新名字"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH folder status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.promptFolders.folderID != "f2" || fakes.promptFolders.name != "新名字" {
		t.Fatalf("rename = id %q name %q", fakes.promptFolders.folderID, fakes.promptFolders.name)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/prompt-folders/global/order?parentId=p1", strings.NewReader(`{"entryOrder":["prompt:p2","folder:f9"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT entry order status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.promptFolders.parentID != "p1" || strings.Join(fakes.promptFolders.order, ",") != "prompt:p2,folder:f9" {
		t.Fatalf("entry order call = parent %q order %v", fakes.promptFolders.parentID, fakes.promptFolders.order)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/prompt-folders/workspace/folders/f3/prompts?ownerId=w1", strings.NewReader(`{"promptId":"p7"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST folder prompt status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.promptFolders.folderID != "f3" || fakes.promptFolders.promptID != "p7" {
		t.Fatalf("add prompt call = folder %q prompt %q", fakes.promptFolders.folderID, fakes.promptFolders.promptID)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/prompt-folders/global/folders/f5/move", strings.NewReader(`{"fromParentId":"p9","toParentId":"p10"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST folder move status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.promptFolders.folderID != "f5" || fakes.promptFolders.parentID != "p10" {
		t.Fatalf("move call = folder %q parent %q", fakes.promptFolders.folderID, fakes.promptFolders.parentID)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/prompt-folders/global/prompts/p1/move", strings.NewReader(`{"fromFolderId":"f6","toFolderId":"f7"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST prompt move status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.promptFolders.promptID != "p1" || fakes.promptFolders.folderID != "f6" || fakes.promptFolders.parentID != "f7" {
		t.Fatalf("prompt move call = prompt %q from %q to %q", fakes.promptFolders.promptID, fakes.promptFolders.folderID, fakes.promptFolders.parentID)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/prompt-folders/global/folders/f11/collect", strings.NewReader(`{"parentId":"f12"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST folder collect status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.promptFolders.folderID != "f11" || fakes.promptFolders.parentID != "f12" {
		t.Fatalf("collect call = folder %q parent %q", fakes.promptFolders.folderID, fakes.promptFolders.parentID)
	}

	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, accessRequest(http.MethodDelete, "/api/prompt-folders/global/folders/f4/prompts/p8", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE folder prompt status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.promptFolders.folderID != "f4" || fakes.promptFolders.promptID != "p8" {
		t.Fatalf("remove prompt call = folder %q prompt %q", fakes.promptFolders.folderID, fakes.promptFolders.promptID)
	}
}
