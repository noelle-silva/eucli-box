type EbNetRequest = (req: any) => Promise<any>

type RoleSessionInput = {
  roleId: string
  sessionId: string
}

type GroupSessionInput = {
  groupId: string
  sessionId: string
}

export async function updateRoleSessionTitle(netRequest: EbNetRequest, input: RoleSessionInput & { title: string }) {
  const { roleId, sessionId } = normalizeRoleSessionInput(input)
  const response = await netRequest({
    method: 'PATCH',
    path: `/api/roles/${encodeURIComponent(roleId)}/sessions/${encodeURIComponent(sessionId)}/title`,
    body: { title: String(input.title || '').trim() || '新聊天' },
    timeoutMs: 15000,
  })
  return response?.body
}

export async function updateGroupSessionTitle(netRequest: EbNetRequest, input: GroupSessionInput & { title: string }) {
  const { groupId, sessionId } = normalizeGroupSessionInput(input)
  const response = await netRequest({
    method: 'PATCH',
    path: `/api/groups/${encodeURIComponent(groupId)}/sessions/${encodeURIComponent(sessionId)}/title`,
    body: { title: String(input.title || '').trim() || '群聊' },
    timeoutMs: 15000,
  })
  return response?.body
}

export async function updateRoleSessionMessage(netRequest: EbNetRequest, input: RoleSessionInput & { messageId: string; content?: string; parts?: any[] }) {
  const { roleId, sessionId } = normalizeRoleSessionInput(input)
  const messageId = String(input.messageId || '').trim()
  if (!messageId) throw new Error('消息无效')
  const body: any = {}
  if (Object.prototype.hasOwnProperty.call(input, 'content')) body.content = String(input.content ?? '')
  if (Object.prototype.hasOwnProperty.call(input, 'parts')) body.parts = serializeSessionMessagePartsForPatch(input.parts)
  const response = await netRequest({
    method: 'PATCH',
    path: `/api/roles/${encodeURIComponent(roleId)}/sessions/${encodeURIComponent(sessionId)}/messages/${encodeURIComponent(messageId)}`,
    body,
    timeoutMs: 15000,
  })
  return response?.body
}

export async function updateGroupSessionMessage(netRequest: EbNetRequest, input: GroupSessionInput & { messageId: string; content?: string; parts?: any[] }) {
  const { groupId, sessionId } = normalizeGroupSessionInput(input)
  const messageId = String(input.messageId || '').trim()
  if (!messageId) throw new Error('消息无效')
  const body: any = {}
  if (Object.prototype.hasOwnProperty.call(input, 'content')) body.content = String(input.content ?? '')
  if (Object.prototype.hasOwnProperty.call(input, 'parts')) body.parts = serializeSessionMessagePartsForPatch(input.parts)
  const response = await netRequest({
    method: 'PATCH',
    path: `/api/groups/${encodeURIComponent(groupId)}/sessions/${encodeURIComponent(sessionId)}/messages/${encodeURIComponent(messageId)}`,
    body,
    timeoutMs: 15000,
  })
  return response?.body
}

function serializeSessionMessagePartsForPatch(partsRaw: unknown) {
  const parts = Array.isArray(partsRaw) ? partsRaw : []
  const out: any[] = []
  for (const raw of parts) {
    const part = raw && typeof raw === 'object' ? (raw as any) : null
    if (!part) continue
    const type = String(part.type || '').trim()
    const id = String(part.id || '').trim()
    if (!id || (type !== 'text' && type !== 'tool')) continue
    const next: any = { id, type }
    if (type === 'text') {
      next.text = String(part.text ?? '')
    } else {
      next.source = String(part.source || '').trim()
      next.raw = String(part.raw || '')
      next.callId = String(part.callId || '').trim()
      next.toolName = String(part.toolName || '').trim()
      next.input = plainObjectCopy(part.input)
      next.state = String(part.state || '').trim()
      const decision = serializeToolDecisionForPatch(part.decision)
      if (decision) next.decision = decision
      const result = serializeToolResultForPatch(part.result)
      if (result) next.result = result
      const display = plainObjectCopy(part.display)
      if (Object.keys(display).length) next.display = display
    }
    assignTimeForPatch(next, 'createdAt', part.createdAt)
    assignTimeForPatch(next, 'updatedAt', part.updatedAt)
    out.push(next)
  }
  return out
}

function serializeToolDecisionForPatch(raw: unknown) {
  const decision = raw && typeof raw === 'object' ? (raw as any) : null
  if (!decision) return null
  const out: any = {
    id: String(decision.id || '').trim(),
    actionId: String(decision.actionId || '').trim(),
    toolName: String(decision.toolName || '').trim(),
    status: String(decision.status || '').trim(),
    reason: String(decision.reason || '').trim(),
  }
  assignTimeForPatch(out, 'createdAt', decision.createdAt)
  return out
}

function serializeToolResultForPatch(raw: unknown) {
  const result = raw && typeof raw === 'object' ? (raw as any) : null
  if (!result) return null
  const out: any = {
    id: String(result.id || '').trim(),
    actionId: String(result.actionId || '').trim(),
    toolName: String(result.toolName || '').trim(),
    status: String(result.status || '').trim(),
    content: String(result.content || ''),
    metadata: plainObjectCopy(result.metadata),
    error: String(result.error || '').trim(),
  }
  assignTimeForPatch(out, 'createdAt', result.createdAt)
  return out
}

function assignTimeForPatch(target: any, key: string, value: unknown) {
  const iso = isoTimeForPatch(value)
  if (iso) target[key] = iso
}

function isoTimeForPatch(value: unknown) {
  let time = 0
  if (value instanceof Date) {
    time = value.getTime()
  } else if (typeof value === 'number') {
    time = value
  } else {
    const text = String(value ?? '').trim()
    if (!text) return ''
    const numeric = Number(text)
    time = Number.isFinite(numeric) && numeric > 0 ? numeric : Date.parse(text)
  }
  if (!Number.isFinite(time) || time <= 0) return ''
  const date = new Date(time)
  return Number.isFinite(date.getTime()) ? date.toISOString() : ''
}

function plainObjectCopy(value: unknown) {
  const source = value && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>) : {}
  const out: Record<string, unknown> = {}
  for (const [key, item] of Object.entries(source)) {
    if (!key || typeof item === 'undefined') continue
    out[key] = item
  }
  return out
}

export async function deleteRoleSessionMessage(netRequest: EbNetRequest, input: RoleSessionInput & { messageId: string }) {
  const { roleId, sessionId } = normalizeRoleSessionInput(input)
  const messageId = String(input.messageId || '').trim()
  if (!messageId) throw new Error('消息无效')
  const response = await netRequest({
    method: 'DELETE',
    path: `/api/roles/${encodeURIComponent(roleId)}/sessions/${encodeURIComponent(sessionId)}/messages/${encodeURIComponent(messageId)}`,
    timeoutMs: 15000,
  })
  return response?.body
}

export async function deleteGroupSessionMessage(netRequest: EbNetRequest, input: GroupSessionInput & { messageId: string }) {
  const { groupId, sessionId } = normalizeGroupSessionInput(input)
  const messageId = String(input.messageId || '').trim()
  if (!messageId) throw new Error('消息无效')
  const response = await netRequest({
    method: 'DELETE',
    path: `/api/groups/${encodeURIComponent(groupId)}/sessions/${encodeURIComponent(sessionId)}/messages/${encodeURIComponent(messageId)}`,
    timeoutMs: 15000,
  })
  return response?.body
}

export async function deleteRoleSessionMessageSubtree(netRequest: EbNetRequest, input: RoleSessionInput & { messageId: string }) {
  const { roleId, sessionId } = normalizeRoleSessionInput(input)
  const messageId = String(input.messageId || '').trim()
  if (!messageId) throw new Error('消息无效')
  const response = await netRequest({
    method: 'DELETE',
    path: `/api/roles/${encodeURIComponent(roleId)}/sessions/${encodeURIComponent(sessionId)}/messages/${encodeURIComponent(messageId)}/subtree`,
    timeoutMs: 15000,
  })
  return response?.body
}

export async function deleteGroupSessionMessageSubtree(netRequest: EbNetRequest, input: GroupSessionInput & { messageId: string }) {
  const { groupId, sessionId } = normalizeGroupSessionInput(input)
  const messageId = String(input.messageId || '').trim()
  if (!messageId) throw new Error('消息无效')
  const response = await netRequest({
    method: 'DELETE',
    path: `/api/groups/${encodeURIComponent(groupId)}/sessions/${encodeURIComponent(sessionId)}/messages/${encodeURIComponent(messageId)}/subtree`,
    timeoutMs: 15000,
  })
  return response?.body
}

function normalizeRoleSessionInput(input: RoleSessionInput) {
  const roleId = String(input.roleId || '').trim()
  const sessionId = String(input.sessionId || '').trim()
  if (!roleId) throw new Error('角色无效')
  if (!sessionId) throw new Error('会话无效')
  return { roleId, sessionId }
}

function normalizeGroupSessionInput(input: GroupSessionInput) {
  const groupId = String(input.groupId || '').trim()
  const sessionId = String(input.sessionId || '').trim()
  if (!groupId) throw new Error('群组无效')
  if (!sessionId) throw new Error('会话无效')
  return { groupId, sessionId }
}
