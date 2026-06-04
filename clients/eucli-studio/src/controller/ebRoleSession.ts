type EbNetRequest = (req: any) => Promise<any>

type RoleSessionInput = {
  roleId: string
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

export async function updateRoleSessionMessage(netRequest: EbNetRequest, input: RoleSessionInput & { messageId: string; content?: string; parts?: any[] }) {
  const { roleId, sessionId } = normalizeRoleSessionInput(input)
  const messageId = String(input.messageId || '').trim()
  if (!messageId) throw new Error('消息无效')
  const body: any = {}
  if (Object.prototype.hasOwnProperty.call(input, 'content')) body.content = String(input.content ?? '')
  if (Object.prototype.hasOwnProperty.call(input, 'parts')) body.parts = Array.isArray(input.parts) ? input.parts : []
  const response = await netRequest({
    method: 'PATCH',
    path: `/api/roles/${encodeURIComponent(roleId)}/sessions/${encodeURIComponent(sessionId)}/messages/${encodeURIComponent(messageId)}`,
    body,
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
