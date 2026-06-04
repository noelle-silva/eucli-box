import { uid, clamp } from '../core/utils'
import { CHAT_ATTACHMENT_KINDS, CHAT_MSG_GROUP_ROLES } from './constants'

export function normalizeMessageError(input: any) {
  const raw = input && typeof input === 'object' ? input : null
  if (!raw) return null
  const message = String((raw as any).message || '').trim()
  if (!message) return null
  const out: any = { message }
  const code = String((raw as any).code || '').trim()
  const system = String((raw as any).system || '').trim()
  if (code) out.code = code
  if (system) out.system = system
  if (Object.prototype.hasOwnProperty.call(raw, 'details')) out.details = (raw as any).details
  return out
}

export function normalizeMessageAttachments(input: any) {
  const list = Array.isArray(input) ? input : []
  const out = []
  for (const raw of list) {
    if (!raw || typeof raw !== 'object') continue
    const id = String((raw as any).id || uid('att'))
    const name = String((raw as any).name || '文件')
    const kind0 = String((raw as any).kind || 'txt')
    const kind = CHAT_ATTACHMENT_KINDS.has(kind0) ? kind0 : 'txt'
    const lang0 = String((raw as any).lang || '')
    const lang = lang0 || (kind === 'md' ? 'markdown' : 'text')
    const text = String((raw as any).text || '')
    const fullLen = clamp(Number((raw as any).fullLen || text.length || 0), 0, 10_000_000)
    const sendLen = clamp(Number((raw as any).sendLen || text.length || 0), 0, fullLen || 0)
    const sendPct = clamp(Number((raw as any).sendPct ?? 100), 0, 100)
    out.push({ id, name, kind, lang, text, fullLen, sendLen, sendPct })
    if (out.length >= 20) break
  }
  return out
}

export function normalizeMessageGroup(m: any) {
  const g = m && typeof m === 'object' ? m : null
  const groupId = String(g?.groupId || '').trim()
  const groupRole0 = String(g?.groupRole || '').trim()
  const groupRole = CHAT_MSG_GROUP_ROLES.has(groupRole0) ? groupRole0 : ''
  const groupParentMid = String(g?.groupParentMid || '').trim()
  if (!groupId || !groupRole) return { groupId: '', groupRole: '', groupParentMid: '' }
  if (groupRole === 'attachment' && !groupParentMid) return { groupId: '', groupRole: '', groupParentMid: '' }
  return { groupId, groupRole, groupParentMid }
}

export function normalizeMessageParts(input: any) {
  const list = Array.isArray(input) ? input : []
  const out: any[] = []
  for (const raw of list) {
    if (!raw || typeof raw !== 'object') continue
    const type = String((raw as any).type || '').trim()
    const id = String((raw as any).id || uid('part')).trim()
    if (!id) continue
    if (type === 'text') {
      const text = String((raw as any).text || '')
      if (!text) continue
      out.push({ id, type: 'text', text, createdAt: (raw as any).createdAt, updatedAt: (raw as any).updatedAt })
      continue
    }
    if (type !== 'tool') continue
    const callId = String((raw as any).callId || '').trim()
    const toolName = String((raw as any).toolName || '').trim()
    const source = String((raw as any).source || '').trim()
    const rawText = String((raw as any).raw || '')
    const state = String((raw as any).state || '').trim()
    const inputValue = (raw as any).input && typeof (raw as any).input === 'object' && !Array.isArray((raw as any).input) ? (raw as any).input : {}
    const part: any = { id, type: 'tool', source, raw: rawText, callId, toolName, state, input: inputValue, createdAt: (raw as any).createdAt, updatedAt: (raw as any).updatedAt }
    const decision = (raw as any).decision && typeof (raw as any).decision === 'object' ? (raw as any).decision : null
    if (decision) {
      part.decision = {
        id: String((decision as any).id || '').trim(),
        actionId: String((decision as any).actionId || '').trim(),
        toolName: String((decision as any).toolName || '').trim(),
        status: String((decision as any).status || '').trim(),
        reason: String((decision as any).reason || '').trim(),
        createdAt: (decision as any).createdAt,
      }
    }
    const result = (raw as any).result && typeof (raw as any).result === 'object' ? (raw as any).result : null
    if (result) {
      part.result = {
        id: String((result as any).id || '').trim(),
        actionId: String((result as any).actionId || '').trim(),
        toolName: String((result as any).toolName || '').trim(),
        status: String((result as any).status || '').trim(),
        content: String((result as any).content || ''),
        error: String((result as any).error || '').trim(),
        metadata: (result as any).metadata && typeof (result as any).metadata === 'object' && !Array.isArray((result as any).metadata) ? (result as any).metadata : {},
        createdAt: (result as any).createdAt,
      }
    }
    out.push(part)
  }
  return out
}
