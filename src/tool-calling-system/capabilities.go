package toolcalling

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
)

// 会话附件写入防线：单张数据上限与单次执行写入总量上限，
// 失控的工具不能靠大量或超大图片占满磁盘。
const (
	maxSessionAttachmentDataURLBytes = 16 << 20
	maxSessionAttachmentWriteBytes   = 64 << 20
)

// capabilitySession 承载一次工具执行期间的运行时能力服务：
// 宿主是服务方，只应答工具主动发起的请求；写入类请求由宿主代写，
// 产物登记进本次执行结果，工具不持有任何直接写句柄。
type capabilitySession struct {
	plan    types.ToolRunPlan
	storage StorageSystem

	mu               sync.Mutex
	produced         []types.MessageAttachment
	attachmentBudget int
	attachmentQuota  int
}

func newCapabilitySession(plan types.ToolRunPlan, storage StorageSystem) *capabilitySession {
	return &capabilitySession{plan: plan, storage: storage, attachmentBudget: maxSessionAttachmentWriteBytes, attachmentQuota: maxSessionAttachmentDataURLBytes}
}

func (c *capabilitySession) handle(ctx context.Context, request toolcontrol.CapabilityRequest) toolcontrol.CapabilityResult {
	if c == nil {
		return capabilityFailed("工具运行能力服务不可用")
	}
	capability := strings.TrimSpace(request.Capability)
	access := strings.TrimSpace(request.Access)
	if !types.ToolDeclaresCapability(c.plan.Tool, capability, access) {
		return capabilityFailed("工具未声明此能力：" + capability + ":" + access)
	}
	if !types.ToolCapabilityGranted(c.plan.Tool, capability, access) {
		return capabilityDenied()
	}
	switch {
	case capability == types.ToolCapabilityWorkspace && access == types.ToolCapabilityAccessRead:
		return c.readWorkspace(ctx)
	case capability == types.ToolCapabilitySessionAttachments && access == types.ToolCapabilityAccessRead:
		return c.readSessionAttachments(ctx, request.Payload)
	case capability == types.ToolCapabilitySessionAttachments && access == types.ToolCapabilityAccessWrite:
		return c.writeSessionAttachment(ctx, request.Payload)
	case capability == types.ToolCapabilitySessionState && access == types.ToolCapabilityAccessRead:
		return c.readSessionState(ctx, request.Payload)
	default:
		return capabilityFailed("能力服务不支持该请求：" + capability + ":" + access)
	}
}

// attachments 返回本次执行期间由工具提请写入、宿主代写完成的产出附件。
func (c *capabilitySession) attachments() []types.MessageAttachment {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.produced) == 0 {
		return nil
	}
	return append([]types.MessageAttachment(nil), c.produced...)
}

func capabilitySuccess(payload any) toolcontrol.CapabilityResult {
	return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: payload}
}

func capabilityDenied() toolcontrol.CapabilityResult {
	return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusDenied, Error: toolcontrol.CapabilityDeniedMessage}
}

func capabilityFailed(message string) toolcontrol.CapabilityResult {
	return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusFailed, Error: message}
}

func (c *capabilitySession) readWorkspace(ctx context.Context) toolcontrol.CapabilityResult {
	workspaceID := strings.TrimSpace(c.plan.Scope.WorkspaceID)
	if workspaceID == "" {
		return capabilityFailed("当前执行不在工作区会话中")
	}
	workspace, err := c.storage.LoadWorkspace(ctx, workspaceID)
	if err != nil {
		return capabilityFailed("读取工作区失败：" + err.Error())
	}
	return capabilitySuccess(types.ToolWorkspaceContext{ID: workspace.ID, Name: workspace.Name, Directories: workspace.Directories})
}

func (c *capabilitySession) readSessionAttachments(ctx context.Context, payload json.RawMessage) toolcontrol.CapabilityResult {
	var request types.SessionAttachmentsReadRequest
	if len(bytes.TrimSpace(payload)) > 0 {
		if err := json.Unmarshal(payload, &request); err != nil {
			return capabilityFailed("会话附件读取请求格式无效")
		}
	}
	session, err := c.loadSession(ctx)
	if err != nil {
		return capabilityFailed("读取会话失败：" + err.Error())
	}
	switch strings.TrimSpace(request.Operation) {
	case types.SessionAttachmentOperationList:
		attachments := sessionImageAttachments(session)
		return capabilitySuccess(types.SessionAttachmentsListResult{Attachments: attachments})
	case types.SessionAttachmentOperationRead:
		attachmentID := strings.TrimSpace(request.AttachmentID)
		if attachmentID == "" {
			return capabilityFailed("会话附件读取请求缺少附件标识")
		}
		attachment, ok := findSessionAttachment(session, attachmentID)
		if !ok {
			return capabilityFailed("会话附件不存在：" + attachmentID)
		}
		dataURL, err := c.storage.LoadSessionAttachmentImage(ctx, attachment.Path)
		if err != nil {
			return capabilityFailed("读取会话附件失败：" + err.Error())
		}
		return capabilitySuccess(types.SessionAttachmentData{ID: attachment.ID, Name: attachment.Name, Mime: attachment.Mime, DataURL: dataURL})
	default:
		return capabilityFailed("会话附件读取操作无效：" + strings.TrimSpace(request.Operation))
	}
}

// sessionImageAttachments 收集会话中的全部图片附件逻辑信息。
func sessionImageAttachments(session types.Session) []types.SessionAttachmentInfo {
	attachments := []types.SessionAttachmentInfo{}
	for _, attachment := range sessionMessageImageAttachments(session) {
		attachments = append(attachments, types.SessionAttachmentInfo{ID: attachment.ID, Name: attachment.Name, Mime: attachment.Mime})
	}
	return attachments
}

// findSessionAttachment 按会话内逻辑标识定位附件；工具只持有标识，不持有路径。
func findSessionAttachment(session types.Session, attachmentID string) (types.MessageAttachment, bool) {
	for _, attachment := range sessionMessageImageAttachments(session) {
		if attachment.ID == attachmentID {
			return attachment, true
		}
	}
	return types.MessageAttachment{}, false
}

func sessionMessageImageAttachments(session types.Session) []types.MessageAttachment {
	attachments := []types.MessageAttachment{}
	for _, message := range session.Messages {
		for _, attachment := range message.Attachments {
			if strings.EqualFold(strings.TrimSpace(attachment.Kind), "image") {
				attachments = append(attachments, attachment)
			}
		}
	}
	return attachments
}

func (c *capabilitySession) writeSessionAttachment(ctx context.Context, payload json.RawMessage) toolcontrol.CapabilityResult {
	var request types.SessionAttachmentWriteRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return capabilityFailed("会话附件写入请求格式无效")
	}
	dataURL := strings.TrimSpace(request.DataURL)
	if dataURL == "" {
		return capabilityFailed("会话附件写入请求缺少图片数据")
	}
	if len(dataURL) > c.attachmentQuota {
		return capabilityFailed("会话附件超出单张大小上限")
	}
	scope := c.plan.Scope
	sessionID := strings.TrimSpace(scope.SessionID)
	if sessionID == "" {
		return capabilityFailed("当前执行没有会话身份")
	}
	if !c.reserveAttachmentBudget(len(dataURL)) {
		return capabilityFailed("会话附件写入总量超出上限")
	}
	attachment := types.RunAttachment{Kind: "image", Name: strings.TrimSpace(request.Name), DataURL: dataURL}
	var (
		saved types.MessageAttachment
		err   error
	)
	switch {
	case strings.TrimSpace(scope.GroupID) != "":
		saved, err = c.storage.SaveGroupSessionMessageAttachment(ctx, strings.TrimSpace(scope.GroupID), sessionID, attachment)
	case strings.TrimSpace(scope.WorkspaceID) != "":
		saved, err = c.storage.SaveWorkspaceSessionMessageAttachment(ctx, strings.TrimSpace(scope.WorkspaceID), strings.TrimSpace(scope.RoleID), sessionID, attachment)
	default:
		saved, err = c.storage.SaveSessionMessageAttachment(ctx, strings.TrimSpace(scope.RoleID), sessionID, attachment)
	}
	if err != nil {
		c.releaseAttachmentBudget(len(dataURL))
		return capabilityFailed("写入会话附件失败：" + err.Error())
	}
	c.mu.Lock()
	c.produced = append(c.produced, saved)
	c.mu.Unlock()
	return capabilitySuccess(types.SessionAttachmentInfo{ID: saved.ID, Name: saved.Name, Mime: saved.Mime})
}

// reserveAttachmentBudget 预占写入预算；写入失败时按预占量原额退回。
func (c *capabilitySession) reserveAttachmentBudget(length int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if length > c.attachmentBudget {
		return false
	}
	c.attachmentBudget -= length
	return true
}

func (c *capabilitySession) releaseAttachmentBudget(length int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.attachmentBudget += length
}

func (c *capabilitySession) readSessionState(ctx context.Context, payload json.RawMessage) toolcontrol.CapabilityResult {
	var request types.SessionStateRequest
	if len(bytes.TrimSpace(payload)) > 0 {
		if err := json.Unmarshal(payload, &request); err != nil {
			return capabilityFailed("会话状态读取请求格式无效")
		}
	}
	values := []types.SessionStateValue{
		{Key: types.SessionStateKeyCurrentTime, Description: "宿主当前时间", Value: time.Now().UTC().Format(time.RFC3339)},
	}
	if session, err := c.loadSession(ctx); err == nil {
		tokenEstimate := 0
		for _, message := range session.Messages {
			tokenEstimate += message.TokenEstimate
		}
		values = append(values,
			types.SessionStateValue{Key: types.SessionStateKeySessionTitle, Description: "当前会话标题", Value: session.Title},
			types.SessionStateValue{Key: types.SessionStateKeySessionMessageCount, Description: "当前会话消息数量", Value: len(session.Messages)},
			types.SessionStateValue{Key: types.SessionStateKeySessionTokenEstimate, Description: "当前会话累计 token 估算", Value: tokenEstimate},
		)
	}
	if roleID := strings.TrimSpace(c.plan.Scope.RoleID); roleID != "" {
		if role, err := c.storage.LoadRole(ctx, roleID); err == nil {
			values = append(values, types.SessionStateValue{Key: types.SessionStateKeyRoleName, Description: "当前角色名称", Value: role.Name})
		}
	}
	if groupID := strings.TrimSpace(c.plan.Scope.GroupID); groupID != "" {
		if group, err := c.storage.LoadChatGroup(ctx, groupID); err == nil {
			values = append(values, types.SessionStateValue{Key: types.SessionStateKeyGroupName, Description: "当前群组名称", Value: group.Name})
		}
	}
	if workspaceID := strings.TrimSpace(c.plan.Scope.WorkspaceID); workspaceID != "" {
		if workspace, err := c.storage.LoadWorkspace(ctx, workspaceID); err == nil {
			values = append(values, types.SessionStateValue{Key: types.SessionStateKeyWorkspaceName, Description: "当前工作区名称", Value: workspace.Name})
		}
	}
	if key := strings.TrimSpace(request.Key); key != "" {
		filtered := []types.SessionStateValue{}
		for _, value := range values {
			if value.Key == key {
				filtered = append(filtered, value)
			}
		}
		if len(filtered) == 0 {
			return capabilityFailed("会话状态键不存在：" + key)
		}
		values = filtered
	}
	return capabilitySuccess(types.SessionStateResult{Values: values})
}

func (c *capabilitySession) loadSession(ctx context.Context) (types.Session, error) {
	scope := c.plan.Scope
	sessionID := strings.TrimSpace(scope.SessionID)
	if sessionID == "" {
		return types.Session{}, toolInvalid("tool run scope is missing session id", nil)
	}
	if groupID := strings.TrimSpace(scope.GroupID); groupID != "" {
		return c.storage.LoadGroupSession(ctx, groupID, sessionID)
	}
	if workspaceID := strings.TrimSpace(scope.WorkspaceID); workspaceID != "" {
		return c.storage.LoadWorkspaceSession(ctx, workspaceID, strings.TrimSpace(scope.RoleID), sessionID)
	}
	return c.storage.LoadSession(ctx, strings.TrimSpace(scope.RoleID), sessionID)
}
