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

export async function startRoleRun(netRequest: EbNetRequest, input: { roleId: string; sessionId?: string; message: string }) {
  const body = {
    roleId: String(input.roleId || '').trim(),
    sessionId: String(input.sessionId || '').trim(),
    message: String(input.message || '').trim(),
  }
  if (!body.roleId) throw new Error('角色无效')
  if (!body.message) throw new Error('输入不能为空')
  const response = await netRequest({ method: 'POST', path: '/api/runs', body, timeoutMs: 15000 })
  return normalizeRunState(response?.body)
}

export async function getRunState(netRequest: EbNetRequest, runId: string) {
  const id = String(runId || '').trim()
  if (!id) throw new Error('run id 无效')
  const response = await netRequest({ method: 'GET', path: `/api/runs/${encodeURIComponent(id)}`, timeoutMs: 15000 })
  return normalizeRunState(response?.body)
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
