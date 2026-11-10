package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

// 任务端点与提示词库共用同一套参数规则：scope 走路径、归属 id 走查询参数。
func TestTaskRoutesForwardScopeAndOwner(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)

	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, accessRequest(http.MethodGet, "/api/tasks/role?ownerId=developer", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/tasks/role status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.taskTrees.scope != types.TaskScopeRole || fakes.taskTrees.ownerID != "developer" {
		t.Fatalf("task call = scope %q owner %q", fakes.taskTrees.scope, fakes.taskTrees.ownerID)
	}

	body := `{"tasks":[{"id":"t1","name":"第一层","content":"复制我","children":[]}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/tasks/workspace?ownerId=frontend", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/tasks/workspace status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.taskTrees.scope != types.TaskScopeWorkspace || fakes.taskTrees.ownerID != "frontend" {
		t.Fatalf("task call = scope %q owner %q", fakes.taskTrees.scope, fakes.taskTrees.ownerID)
	}
	if len(fakes.taskTrees.tree.Tasks) != 1 || fakes.taskTrees.tree.Tasks[0].ID != "t1" {
		t.Fatalf("saved task tree = %#v", fakes.taskTrees.tree)
	}
}

// 结构动作只带必要参数到达业务层：新建、删除、移动、排序。
func TestTaskActionRoutesForwardParameters(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)

	req := httptest.NewRequest(http.MethodPost, "/api/tasks/role/nodes?ownerId=developer", strings.NewReader(`{"parentId":"a","node":{"id":"b","name":"子"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST nodes status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.taskTrees.parentID != "a" || fakes.taskTrees.node.ID != "b" {
		t.Fatalf("create call = parent %q node %#v", fakes.taskTrees.parentID, fakes.taskTrees.node)
	}

	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, accessRequest(http.MethodDelete, "/api/tasks/workspace/nodes/t9?ownerId=frontend", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE node status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.taskTrees.taskID != "t9" {
		t.Fatalf("deleted task id = %q", fakes.taskTrees.taskID)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/tasks/role/nodes/t1/move?ownerId=developer", strings.NewReader(`{"toParentId":"a"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST move status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.taskTrees.taskID != "t1" || fakes.taskTrees.toParentID != "a" {
		t.Fatalf("move call = task %q target %q", fakes.taskTrees.taskID, fakes.taskTrees.toParentID)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/tasks/role/order?ownerId=developer", strings.NewReader(`{"parentId":"a","taskIds":["d","c"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT order status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.taskTrees.parentID != "a" || len(fakes.taskTrees.order) != 2 || fakes.taskTrees.order[0] != "d" {
		t.Fatalf("order call = parent %q order %#v", fakes.taskTrees.parentID, fakes.taskTrees.order)
	}
}
