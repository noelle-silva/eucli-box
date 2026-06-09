import { assignErrorPayload, normalizeErrorPayload, type ErrorPayload } from '../domain/errorPayload'

type EbNetRequest = (req: any) => Promise<any>

export type EbRunState = {
  id: string
  roleId: string
  sessionId: string
  inputMessageId: string
  lastMessageId: string
  dependencyMessageIds: string[]
  stream: boolean
  status: string
  reason: string
  error: ErrorPayload | null
}

function normalizeTextList(value: unknown) {
  const list = Array.isArray(value) ? value : []
  const out: string[] = []
  const seen = new Set<string>()
  for (const item of list) {
    const id = String(item || '').trim()
    if (!id || seen.has(id)) continue
    seen.add(id)
    out.push(id)
  }
  return out
}

export function isTerminalRunStatus(status: unknown) {
  const value = String(status || '').trim()
  return value === 'completed' || value === 'failed' || value === 'cancelled' || value === 'canceled'
}

export async function startRoleRun(netRequest: EbNetRequest, input: { roleId: string; sessionId?: string; message?: string; attachments?: any[]; parentMessageId?: string; userMessageId?: string; stream?: boolean; reasoningEffort?: string }) {
  const body = {
    roleId: String(input.roleId || '').trim(),
    sessionId: String(input.sessionId || '').trim(),
    message: String(input.message || '').trim(),
    attachments: Array.isArray(input.attachments) ? input.attachments : [],
    parentMessageId: String(input.parentMessageId || '').trim(),
    userMessageId: String(input.userMessageId || '').trim(),
    reasoningEffort: String(input.reasoningEffort || '').trim(),
    stream: !!input.stream,
  }
  if (!body.roleId) throw new Error('角色无效')
  const hasAttachments = body.attachments.length > 0
  const hasMessage = !!body.message || hasAttachments
  const hasUserMessageId = !!body.userMessageId
  if (hasMessage === hasUserMessageId) throw new Error('必须且只能指定输入内容或用户消息')
  if (body.parentMessageId && hasUserMessageId) throw new Error('父消息和用户消息不能同时指定')
  if (hasUserMessageId && hasAttachments) throw new Error('从已有用户消息继续生成时不能携带新附件')
  if (body.parentMessageId && !body.sessionId) throw new Error('会话无效')
  if (hasUserMessageId && !body.sessionId) throw new Error('会话无效')
  if (!body.parentMessageId) delete (body as any).parentMessageId
  if (!body.userMessageId) delete (body as any).userMessageId
  if (!body.message) delete (body as any).message
  if (!body.attachments.length) delete (body as any).attachments
  if (!body.sessionId) delete (body as any).sessionId
  if (!body.reasoningEffort) delete (body as any).reasoningEffort
  const response = await netRequest({ method: 'POST', path: '/api/runs', body, timeoutMs: 30000 })
  return normalizeRunState(response?.body)
}

export async function getRunState(netRequest: EbNetRequest, runId: string) {
  const id = String(runId || '').trim()
  if (!id) throw new Error('run id 无效')
  const response = await netRequest({ method: 'GET', path: `/api/runs/${encodeURIComponent(id)}`, timeoutMs: 15000 })
  return normalizeRunState(response?.body)
}

export async function listActiveRoleRuns(netRequest: EbNetRequest) {
  const response = await netRequest({ method: 'GET', path: '/api/runs', timeoutMs: 15000 })
  const runs = Array.isArray(response?.body) ? response.body : []
  return runs.map(normalizeRunState).filter((run: EbRunState) => run.id && !isTerminalRunStatus(run.status))
}

export async function cancelRoleRun(netRequest: EbNetRequest, runId: string) {
  const id = String(runId || '').trim()
  if (!id) throw new Error('run id 无效')
  await netRequest({ method: 'POST', path: `/api/runs/${encodeURIComponent(id)}/cancel`, timeoutMs: 15000 })
}

export async function submitToolConfirmation(netRequest: EbNetRequest, input: { decisionId: string; approved: boolean; reason?: string }) {
  const decisionId = String(input.decisionId || '').trim()
  if (!decisionId) throw new Error('确认项无效')
  const body: any = {
    decisionId,
    approved: !!input.approved,
  }
  const reason = String(input.reason || '').trim()
  if (reason) body.reason = reason
  await netRequest({ method: 'POST', path: '/api/tool-confirmations', body, timeoutMs: 15000 })
}

export async function pollRunUntilTerminal(
  netRequest: EbNetRequest,
  initialRun: EbRunState,
  onState?: (run: EbRunState) => void | Promise<void>,
  options?: { shouldContinue?: () => boolean },
) {
  let state = initialRun
  const runId = String(state?.id || '').trim()
  if (!runId) throw new Error('run id 无效')
  while (!isTerminalRunStatus(state.status)) {
    if (options?.shouldContinue && !options.shouldContinue()) return state
    await sleepMs(450)
    if (options?.shouldContinue && !options.shouldContinue()) return state
    state = await getRunState(netRequest, runId)
    await onState?.(state)
  }
  return state
}

export function normalizeRunState(value: any): EbRunState {
  const state = value && typeof value === 'object' ? value : {}
  const error = normalizeErrorPayload(state.error)
  return {
    id: String(state.id || '').trim(),
    roleId: String(state.roleId || '').trim(),
    sessionId: String(state.sessionId || '').trim(),
    inputMessageId: String(state.inputMessageId || '').trim(),
    lastMessageId: String(state.lastMessageId || '').trim(),
    dependencyMessageIds: normalizeTextList(state.dependencyMessageIds),
    stream: !!state.stream,
    status: String(state.status || '').trim(),
    reason: String(state.reason || '').trim(),
    error,
  }
}

export function runStateFailureError(state: EbRunState) {
  const payload = state.error
  const err: any = new Error(String(payload?.message || state.reason || `e-b run ${state.status}`))
  return assignErrorPayload(err, payload)
}

export function sleepMs(ms: number) {
  return new Promise<void>((resolve) => window.setTimeout(resolve, Math.max(0, Math.floor(Number(ms || 0)))))
}
