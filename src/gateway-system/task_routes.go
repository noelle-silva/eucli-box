package gateway

import (
	"net/http"

	"eucli-box/pkg/types"
)

// 任务树：只有角色与工作区两个作用域，同一组端点承载两者。
// 作用域合法性、归属存在性与树结构校验都在存储层收口，网关只做搬运。

type taskNodeRequest struct {
	ParentID string         `json:"parentId"`
	Node     types.TaskNode `json:"node"`
}

type taskMoveRequest struct {
	ToParentID string `json:"toParentId"`
}

type taskOrderRequest struct {
	ParentID string   `json:"parentId"`
	TaskIDs  []string `json:"taskIds"`
}

func (s *system) handleLoadTaskTree(w http.ResponseWriter, r *http.Request) {
	tree, err := s.taskTrees.LoadTaskTree(r.Context(), taskScope(r), taskOwnerID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, tree)
}

func (s *system) handleSaveTaskTree(w http.ResponseWriter, r *http.Request) {
	tree, err := decodeJSON[types.TaskTree](r)
	if err != nil {
		writeError(w, err)
		return
	}
	saved, err := s.taskTrees.SaveTaskTree(r.Context(), taskScope(r), taskOwnerID(r), tree)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, saved)
}

// handleCreateTaskNode 新建节点：parentId 为空表示根级，node 携带节点自己的 id。
func (s *system) handleCreateTaskNode(w http.ResponseWriter, r *http.Request) {
	body, err := decodeJSON[taskNodeRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	tree, err := s.taskTrees.CreateTaskNode(r.Context(), taskScope(r), taskOwnerID(r), body.ParentID, body.Node)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, tree)
}

// handleDeleteTaskNode 删除节点连同它的整棵子树。
func (s *system) handleDeleteTaskNode(w http.ResponseWriter, r *http.Request) {
	tree, err := s.taskTrees.DeleteTaskNode(r.Context(), taskScope(r), taskOwnerID(r), r.PathValue("taskID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, tree)
}

// handleMoveTaskNode 移动节点：toParentId 为空表示移到根级。
// 来源父级由树唯一决定，请求只提交目标。
func (s *system) handleMoveTaskNode(w http.ResponseWriter, r *http.Request) {
	body, err := decodeJSON[taskMoveRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	tree, err := s.taskTrees.MoveTaskNode(r.Context(), taskScope(r), taskOwnerID(r), r.PathValue("taskID"), body.ToParentID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, tree)
}

// handleReorderTaskChildren 重排某一层：parentId 为空表示根级。
func (s *system) handleReorderTaskChildren(w http.ResponseWriter, r *http.Request) {
	body, err := decodeJSON[taskOrderRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	tree, err := s.taskTrees.ReorderTaskChildren(r.Context(), taskScope(r), taskOwnerID(r), body.ParentID, body.TaskIDs)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, tree)
}

func taskScope(r *http.Request) string {
	return r.PathValue("scope")
}

// taskOwnerID 返回作用域的归属 id：任务没有全局作用域，归属总是必填。
func taskOwnerID(r *http.Request) string {
	return r.URL.Query().Get("ownerId")
}
