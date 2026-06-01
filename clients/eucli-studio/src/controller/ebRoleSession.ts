type EbNetRequest = (req: any) => Promise<any>

export type EbSession = {
  id: string
  roleId: string
  title: string
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
