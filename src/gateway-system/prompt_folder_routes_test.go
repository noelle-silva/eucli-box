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
