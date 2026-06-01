type EbNetRequest = (req: any) => Promise<any>

export type EbRunState = {
  id: string
  roleId: string
  sessionId: string
  status: string
  reason: string
}

export function isTerminalRunStatus(status: unknown) {
  const value = String(status || '').trim()
  return value === 'completed' || value === 'failed' || value === 'cancelled'
}

export async function startRoleRun(netRequest: EbNetRequest, input: { roleId: string; sessionId?: string; message?: string; parentMessageId?: string; userMessageId?: string }) {
  const body = {
    roleId: String(input.roleId || '').trim(),
    sessionId: String(input.sessionId || '').trim(),
    message: String(input.message || '').trim(),
    parentMessageId: String(input.parentMessageId || '').trim(),
    userMessageId: String(input.userMessageId || '').trim(),
  }
  if (!body.roleId) throw new Error('角色无效')
  const hasMessage = !!body.message
  const hasUserMessageId = !!body.userMessageId
  if (hasMessage === hasUserMessageId) throw new Error('必须且只能指定输入内容或用户消息')
  if (body.parentMessageId && hasUserMessageId) throw new Error('父消息和用户消息不能同时指定')
  if (body.parentMessageId && !body.sessionId) throw new Error('会话无效')
  if (hasUserMessageId && !body.sessionId) throw new Error('会话无效')
  if (!body.parentMessageId) delete (body as any).parentMessageId
  if (!body.userMessageId) delete (body as any).userMessageId
  if (!body.message) delete (body as any).message
  if (!body.sessionId) delete (body as any).sessionId
  const response = await netRequest({ method: 'POST', path: '/api/runs', body, timeoutMs: 15000 })
  return normalizeRunState(response?.body)
}

export async function getRunState(netRequest: EbNetRequest, runId: string) {
  const id = String(runId || '').trim()
  if (!id) throw new Error('run id 无效')
  const response = await netRequest({ method: 'GET', path: `/api/runs/${encodeURIComponent(id)}`, timeoutMs: 15000 })
  return normalizeRunState(response?.body)
}

export async function cancelRoleRun(netRequest: EbNetRequest, runId: string) {
  const id = String(runId || '').trim()
  if (!id) throw new Error('run id 无效')
  await netRequest({ method: 'POST', path: `/api/runs/${encodeURIComponent(id)}/cancel`, timeoutMs: 15000 })
}

export function normalizeRunState(value: any): EbRunState {
  const state = value && typeof value === 'object' ? value : {}
  return {
    id: String(state.id || '').trim(),
    roleId: String(state.roleId || '').trim(),
    sessionId: String(state.sessionId || '').trim(),
    status: String(state.status || '').trim(),
    reason: String(state.reason || '').trim(),
  }
}

export function sleepMs(ms: number) {
  return new Promise<void>((resolve) => window.setTimeout(resolve, Math.max(0, Math.floor(Number(ms || 0)))))
}
