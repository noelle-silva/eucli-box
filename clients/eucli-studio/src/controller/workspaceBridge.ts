import { normalizeStoredChat } from '../storage/normalizeStoredChat'
import { normalizeReasoningEffort } from '../domain/reasoning'
import { workspaceRoleTargetId } from '../domain/workspaceRoleTarget'
import { HOOK_PROMPT_SESSION_METADATA_KEY, HOOK_PROMPT_SESSION_METADATA_MODE_KEY, hookPromptSelectionFromMetadata, normalizeHookPromptSelection } from '../domain/hookPrompt'

type EbNetRequest = (req: any) => Promise<any>

type UiWorkspaceDirectory = {
  path: string
  alias: string
  description: string
}

export type UiWorkspace = {
  id: string
  name: string
  directories: UiWorkspaceDirectory[]
  prompt: string
  actualPrompt: string
  createdAt: number
  updatedAt: number
}

function text(value: unknown) {
  return String(value || '').trim()
}

function object(value: unknown): Record<string, any> {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, any>) : {}
}

function list(value: unknown): any[] {
  return Array.isArray(value) ? value : []
}

function timeMs(value: unknown, fallback = Date.now()) {
  if (typeof value === 'number' && isFinite(value) && value > 0) return Math.floor(value)
  const raw = text(value)
  if (!raw) return fallback
  const numeric = Number(raw)
  if (isFinite(numeric) && numeric > 0) return Math.floor(numeric)
  const parsed = Date.parse(raw)
  return isFinite(parsed) && parsed > 0 ? parsed : fallback
}

function timeIso(value: unknown, fallback = Date.now()) {
  return new Date(timeMs(value, fallback)).toISOString()
}

function workspaceFallbackTime(raw: unknown) {
  return timeMs(raw, Date.now())
}

function normalizeWorkspaceDirectory(raw: unknown): UiWorkspaceDirectory | null {
  const box = object(raw)
  const path = text(box.path)
  if (!path) return null
  return {
    path,
    alias: text(box.alias),
    description: text(box.description),
  }
}

export function normalizeWorkspace(raw: unknown): UiWorkspace | null {
  const box = object(raw)
  const id = text(box.id)
  if (!id) return null
  const createdAt = workspaceFallbackTime(box.createdAt)
  const updatedAt = timeMs(box.updatedAt, createdAt)
  return {
    id,
    name: text(box.name) || '未命名工作区',
    directories: list(box.directories)
      .map(normalizeWorkspaceDirectory)
      .filter(Boolean) as UiWorkspaceDirectory[],
    prompt: text(box.prompt),
    actualPrompt: String(box.actualPrompt ?? ''),
    createdAt,
    updatedAt,
  }
}

function normalizeMessageType(raw: unknown) {
  const value = text(raw)
  if (value === 'assistant' || value === 'tool' || value === 'tool_request' || value === 'tool_confirmation' || value === 'failure' || value === 'system_control') return value
  return 'user'
}

function serializeMessageParts(partsRaw: unknown) {
  return list(partsRaw)
    .map((raw) => {
      const part = object(raw)
      const id = text(part.id)
      const type = text(part.type)
      if (!id || !type) return null
      const next: any = { id, type }
      if (type === 'text') {
        next.text = String(part.text ?? '')
      } else if (type === 'reasoning') {
        next.text = String(part.text ?? '')
        next.source = text(part.source)
        next.signature = text(part.signature)
        next.data = String(part.data ?? '')
      } else if (type === 'tool') {
        next.source = text(part.source)
        next.raw = String(part.raw ?? '')
        next.callId = text(part.callId)
        next.toolName = text(part.toolName)
        next.input = object(part.input)
        next.state = text(part.state)
        if (Object.keys(object(part.display)).length) next.display = { ...object(part.display) }
        const decision = object(part.decision)
        if (Object.keys(decision).length) {
          next.decision = {
            id: text(decision.id),
            actionId: text(decision.actionId),
            toolName: text(decision.toolName),
            status: text(decision.status),
            reason: text(decision.reason),
            createdAt: decision.createdAt ? timeIso(decision.createdAt) : undefined,
          }
        }
        const result = object(part.result)
        if (Object.keys(result).length) {
          next.result = {
            id: text(result.id),
            actionId: text(result.actionId),
            toolName: text(result.toolName),
            status: text(result.status),
            content: String(result.content ?? ''),
            metadata: object(result.metadata),
            error: text(result.error),
            createdAt: result.createdAt ? timeIso(result.createdAt) : undefined,
          }
        }
      } else {
        return null
      }
      if (part.createdAt) next.createdAt = timeIso(part.createdAt)
      if (part.updatedAt) next.updatedAt = timeIso(part.updatedAt)
      return next
    })
    .filter(Boolean)
}

function serializeMessageAttachments(message: Record<string, any>) {
  const out: any[] = []
  for (const image of list(message.images)) {
    const path = text(image)
    if (!path) continue
    out.push({ kind: 'image', name: '图片', path })
  }
  for (const raw of list(message.attachments)) {
    const attachment = object(raw)
    const textValue = String(attachment.text ?? '')
    if (!textValue) continue
    out.push({
      id: text(attachment.id),
      kind: text(attachment.kind) || 'txt',
      name: text(attachment.name) || '文件',
      lang: text(attachment.lang) || 'text',
      text: textValue,
      fullLen: Math.max(0, Math.floor(Number(attachment.fullLen || textValue.length))),
      sendLen: Math.max(0, Math.floor(Number(attachment.sendLen || textValue.length))),
      sendPct: Math.max(0, Math.min(100, Math.floor(Number(attachment.sendPct ?? 100)))),
    })
  }
  return out
}

function modelOverrideFromMetadata(metadataRaw: unknown) {
  const metadata = object(metadataRaw)
  const modelId = text(metadata['modelOverride.modelId'])
  if (!modelId) return null
  const groupId = text(metadata['modelOverride.groupId'])
  if (groupId) {
    return { kind: 'model_group', groupId, providerId: '', modelId }
  }
  const providerId = text(metadata['modelOverride.providerId'])
  if (!providerId) return null
  return { kind: 'provider', groupId: '', providerId, modelId }
}

export function workspaceSessionToChat(raw: unknown) {
  const session = object(raw)
  const id = text(session.id)
  if (!id) return null
  const createdAt = timeMs(session.createdAt, Date.now())
  const updatedAt = timeMs(session.updatedAt, timeMs(session.lastActive, createdAt))
  const metadata = object(session.metadata)
  const chat: any = {
    id,
    roleId: text(session.roleId),
    workspaceId: text(session.workspaceId),
    title: text(session.title) || '工作区会话',
    status: text(session.status),
    createdAt,
    updatedAt,
    messages: list(session.messages).map((message) => ({ ...object(message) })),
  }
  const reasoningEffort = normalizeReasoningEffort(metadata.reasoningEffort)
  if (reasoningEffort) chat.reasoningEffort = reasoningEffort
  const modelOverride = modelOverrideFromMetadata(metadata)
  if (modelOverride) chat.modelOverride = modelOverride
  const hookPromptSelection = hookPromptSelectionFromMetadata(metadata)
  if (hookPromptSelection.mode !== 'inherit') chat.hookPromptMode = hookPromptSelection.mode
  if (hookPromptSelection.mode === 'preset') chat.hookPromptPresetId = hookPromptSelection.presetId
  return normalizeStoredChat(chat, 'workspace')
}

export function workspaceSessionSummaryToMeta(raw: unknown) {
  const summary = object(raw)
  const id = text(summary.id)
  if (!id) return null
  const updatedAt = timeMs(summary.updatedAt, timeMs(summary.lastActive, Date.now()))
  const status = text(summary.status)
  return {
    id,
    roleId: text(summary.roleId),
    workspaceId: text(summary.workspaceId),
    title: text(summary.title) || '工作区会话',
    createdAt: timeMs(summary.createdAt, updatedAt),
    updatedAt,
    lastMessagePreview: '',
    messageCount: 0,
    hasPending: status === 'running' || status === 'waiting_confirmation',
    runStatus: status === 'running' || status === 'waiting_confirmation' ? 'running' : 'idle',
    runStatusChangedAt: updatedAt,
  }
}

export function workspaceSessionTargetId(workspaceIdRaw: unknown, roleIdRaw: unknown) {
  return workspaceRoleTargetId(workspaceIdRaw, roleIdRaw)
}

function workspaceToWire(workspace: UiWorkspace) {
  const createdAt = workspaceFallbackTime(workspace.createdAt)
  const updatedAt = timeMs(workspace.updatedAt, createdAt)
  return {
    id: text(workspace.id),
    name: text(workspace.name) || '未命名工作区',
    directories: workspace.directories.map((directory) => ({
      path: text(directory.path),
      alias: text(directory.alias),
      description: text(directory.description),
    })),
    prompt: text(workspace.prompt),
    createdAt: timeIso(createdAt),
    updatedAt: timeIso(updatedAt),
  }
}

function workspaceChatToWire(chatRaw: unknown, workspaceIdRaw: unknown, roleIdRaw?: unknown) {
  const chat = object(chatRaw)
  const roleId = text(roleIdRaw) || text(chat.roleId)
  const workspaceId = text(workspaceIdRaw) || text(chat.workspaceId)
  if (!roleId) throw new Error('工作区会话缺少角色')
  if (!workspaceId) throw new Error('工作区会话缺少工作区')
  const createdAt = timeMs(chat.createdAt, Date.now())
  const updatedAt = timeMs(chat.updatedAt, createdAt)
  const metadata: Record<string, any> = {}
  const reasoningEffort = normalizeReasoningEffort(chat.reasoningEffort)
  if (reasoningEffort) metadata.reasoningEffort = reasoningEffort
  const hookPromptSelection = normalizeHookPromptSelection(chat)
  if (hookPromptSelection.mode === 'none') {
    metadata[HOOK_PROMPT_SESSION_METADATA_MODE_KEY] = 'none'
  } else if (hookPromptSelection.mode === 'preset') {
    metadata[HOOK_PROMPT_SESSION_METADATA_MODE_KEY] = 'preset'
    metadata[HOOK_PROMPT_SESSION_METADATA_KEY] = hookPromptSelection.presetId
  }
  const modelOverride = object(chat.modelOverride)
  const modelId = text(modelOverride.modelId)
  if (modelId) {
    metadata['modelOverride.kind'] = text(modelOverride.kind)
    metadata['modelOverride.providerId'] = text(modelOverride.providerId)
    metadata['modelOverride.groupId'] = text(modelOverride.groupId)
    metadata['modelOverride.modelId'] = modelId
  }
  return {
    id: text(chat.id),
    roleId,
    workspaceId,
    title: text(chat.title) || '工作区会话',
    status: text(chat.status) || 'created',
    createdAt: timeIso(createdAt),
    updatedAt: timeIso(updatedAt),
    lastActive: timeIso(updatedAt),
    metadata,
    messages: list(chat.messages).map((rawMessage) => {
      const message = object(rawMessage)
      const createdAt = timeMs(message.createdAt, Date.now())
      const updatedAt = timeMs(message.updatedAt, createdAt)
      const next: any = {
        id: text(message.id),
        type: normalizeMessageType(message.type || message.role),
        content: String(message.content ?? ''),
        parentMessageId: text(message.parentMid || message.parentMessageId),
        branchId: text(message.branchId) || 'main',
        createdAt: timeIso(createdAt),
        updatedAt: timeIso(updatedAt),
      }
      if (text(message.speakerRoleId)) next.speakerRoleId = text(message.speakerRoleId)
      if (Object.keys(object(message.control)).length) next.control = object(message.control)
      if (Object.keys(object(message.error)).length) next.error = object(message.error)
      if (Array.isArray(message.parts) && message.parts.length) next.parts = serializeMessageParts(message.parts)
      const attachments = serializeMessageAttachments(message)
      if (attachments.length) next.attachments = attachments
      const tokenEstimate = Math.max(0, Math.floor(Number(message.tokenEstimate || 0)))
      if (tokenEstimate > 0) next.tokenEstimate = tokenEstimate
      return next
    }),
  }
}

export async function loadWorkspace(netRequest: EbNetRequest, workspaceId: string) {
  const id = text(workspaceId)
  if (!id) throw new Error('工作区无效')
  const response = await netRequest({ method: 'GET', path: `/api/workspaces/${encodeURIComponent(id)}`, timeoutMs: 15000 })
  return normalizeWorkspace(response?.body)
}

export async function listWorkspacesDetailed(netRequest: EbNetRequest) {
  const response = await netRequest({ method: 'GET', path: '/api/workspaces', timeoutMs: 15000 })
  const items = list(response?.body)
  const out: UiWorkspace[] = []
  for (const summary of items) {
    const workspace = await loadWorkspace(netRequest, text(object(summary).id)).catch(() => null)
    if (workspace) out.push(workspace)
  }
  return out
}

export async function saveWorkspace(netRequest: EbNetRequest, workspace: UiWorkspace) {
  const body = workspaceToWire(workspace)
  if (!body.id) throw new Error('工作区无效')
  await netRequest({ method: 'POST', path: '/api/workspaces', body, timeoutMs: 15000 })
}

export async function previewWorkspacePrompt(netRequest: EbNetRequest, workspace: UiWorkspace) {
  const response = await netRequest({ method: 'POST', path: '/api/workspaces/prompt-preview', body: workspaceToWire(workspace), timeoutMs: 15000 })
  return String(object(response?.body).actualPrompt ?? '')
}

export async function deleteWorkspace(netRequest: EbNetRequest, workspaceId: string) {
  const id = text(workspaceId)
  if (!id) throw new Error('工作区无效')
  await netRequest({ method: 'DELETE', path: `/api/workspaces/${encodeURIComponent(id)}`, timeoutMs: 15000 })
}

export async function listWorkspaceSessionSummaries(netRequest: EbNetRequest, workspaceId: string, roleId: string) {
  const id = text(workspaceId)
  const rid = text(roleId)
  if (!id || !rid) return []
  const response = await netRequest({ method: 'GET', path: `/api/workspaces/${encodeURIComponent(id)}/roles/${encodeURIComponent(rid)}/sessions`, timeoutMs: 15000 })
  return list(response?.body)
}

export async function loadWorkspaceSession(netRequest: EbNetRequest, workspaceId: string, roleId: string, sessionId: string) {
  const wid = text(workspaceId)
  const rid = text(roleId)
  const sid = text(sessionId)
  if (!wid || !rid || !sid) throw new Error('工作区会话无效')
  const response = await netRequest({ method: 'GET', path: `/api/workspaces/${encodeURIComponent(wid)}/roles/${encodeURIComponent(rid)}/sessions/${encodeURIComponent(sid)}`, timeoutMs: 15000 })
  return workspaceSessionToChat(response?.body)
}

export async function createWorkspaceSession(netRequest: EbNetRequest, input: { workspaceId: string; roleId: string; title?: string }) {
  const workspaceId = text(input.workspaceId)
  const roleId = text(input.roleId)
  if (!workspaceId) throw new Error('工作区无效')
  if (!roleId) throw new Error('角色无效')
  const response = await netRequest({
    method: 'POST',
    path: `/api/workspaces/${encodeURIComponent(workspaceId)}/roles/${encodeURIComponent(roleId)}/sessions/create`,
    body: { title: text(input.title) || '工作区会话' },
    timeoutMs: 15000,
  })
  return workspaceSessionToChat(response?.body)
}

export async function saveWorkspaceSession(netRequest: EbNetRequest, input: { workspaceId: string; roleId: string; chat: any }) {
  const workspaceId = text(input.workspaceId)
  const roleId = text(input.roleId)
  if (!workspaceId || !roleId) throw new Error('工作区无效')
  const body = workspaceChatToWire(input.chat, workspaceId, input.roleId)
  await netRequest({
    method: 'POST',
    path: `/api/workspaces/${encodeURIComponent(workspaceId)}/roles/${encodeURIComponent(roleId)}/sessions`,
    body,
    timeoutMs: 15000,
  })
}

export async function deleteWorkspaceSession(netRequest: EbNetRequest, workspaceId: string, roleId: string, sessionId: string) {
  const wid = text(workspaceId)
  const rid = text(roleId)
  const sid = text(sessionId)
  if (!wid || !rid || !sid) throw new Error('工作区会话无效')
  await netRequest({ method: 'DELETE', path: `/api/workspaces/${encodeURIComponent(wid)}/roles/${encodeURIComponent(rid)}/sessions/${encodeURIComponent(sid)}`, timeoutMs: 15000 })
}

export async function updateWorkspaceSessionTitle(netRequest: EbNetRequest, input: { workspaceId: string; roleId: string; sessionId: string; title: string }) {
  const workspaceId = text(input.workspaceId)
  const roleId = text(input.roleId)
  const sessionId = text(input.sessionId)
  if (!workspaceId || !roleId || !sessionId) throw new Error('工作区会话无效')
  const response = await netRequest({
    method: 'PATCH',
    path: `/api/workspaces/${encodeURIComponent(workspaceId)}/roles/${encodeURIComponent(roleId)}/sessions/${encodeURIComponent(sessionId)}/title`,
    body: { title: text(input.title) || '工作区会话' },
    timeoutMs: 15000,
  })
  return response?.body
}
