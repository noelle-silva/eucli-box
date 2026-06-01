type EbNetRequest = (req: any) => Promise<any>

export type EbSession = {
  id: string
  roleId: string
  title: string
}

type RoleSessionInput = {
  roleId: string
  sessionId: string
}

export async function createRoleSession(netRequest: EbNetRequest, input: { roleId: string; title?: string }) {
  const roleId = String(input.roleId || '').trim()
  if (!roleId) throw new Error('角色无效')
  const response = await netRequest({
    method: 'POST',
    path: `/api/roles/${encodeURIComponent(roleId)}/sessions/create`,
    body: { title: String(input.title || '').trim() || '新聊天' },
    timeoutMs: 15000,
  })
  return normalizeSession(response?.body)
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

export async function updateRoleSessionMessage(netRequest: EbNetRequest, input: RoleSessionInput & { messageId: string; content: string }) {
  const { roleId, sessionId } = normalizeRoleSessionInput(input)
  const messageId = String(input.messageId || '').trim()
  if (!messageId) throw new Error('消息无效')
  const response = await netRequest({
    method: 'PATCH',
    path: `/api/roles/${encodeURIComponent(roleId)}/sessions/${encodeURIComponent(sessionId)}/messages/${encodeURIComponent(messageId)}`,
    body: { content: String(input.content ?? '') },
    timeoutMs: 15000,
  })
  return response?.body
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

function normalizeRoleSessionInput(input: RoleSessionInput) {
  const roleId = String(input.roleId || '').trim()
  const sessionId = String(input.sessionId || '').trim()
  if (!roleId) throw new Error('角色无效')
  if (!sessionId) throw new Error('会话无效')
  return { roleId, sessionId }
}

function normalizeSession(value: any): EbSession {
  const session = value && typeof value === 'object' ? value : {}
  const id = String(session.id || '').trim()
  if (!id) throw new Error('e-b 未返回会话ID')
  return {
    id,
    roleId: String(session.roleId || '').trim(),
    title: String(session.title || '').trim() || '新聊天',
  }
}
