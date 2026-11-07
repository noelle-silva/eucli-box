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

// 四个定点动作都只带必要参数到达业务层：删除、改名、兄弟排序、夹内条目排序。
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

	req = httptest.NewRequest(http.MethodPut, "/api/prompt-folders/global/order?parentId=p1", strings.NewReader(`{"folderIds":["y","x"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT folder order status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.promptFolders.parentID != "p1" || strings.Join(fakes.promptFolders.order, ",") != "y,x" {
		t.Fatalf("folder order call = parent %q order %v", fakes.promptFolders.parentID, fakes.promptFolders.order)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/prompt-folders/global/prompts/order?folderId=f3", strings.NewReader(`{"promptIds":["p2","p1"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT prompt order status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.promptFolders.folderID != "f3" || strings.Join(fakes.promptFolders.order, ",") != "p2,p1" {
		t.Fatalf("prompt order call = folder %q order %v", fakes.promptFolders.folderID, fakes.promptFolders.order)
	}
}
