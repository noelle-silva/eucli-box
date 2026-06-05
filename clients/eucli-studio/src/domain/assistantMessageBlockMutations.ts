import { assistantToolPartId, planAssistantMessageBlocks, type AssistantMessageBlockKind } from './assistantMessageBlocks'
import { syncMessageTextPart } from './message'
import { isTextProtocolToolPart, syncTextProtocolToolParts, validateTextProtocolToolRequestsForParts } from './textProtocolTools'

export type AssistantMessageBlockRef = {
  kind: AssistantMessageBlockKind
  blockId?: string
  partId?: string
  start?: number
  end?: number
}

type MutationResult = { ok: true } | { ok: false; error: string }

function plainObject(value: any) {
  return value && typeof value === 'object' && !Array.isArray(value) ? value : {}
}

function normalizeBlockRef(raw: any): AssistantMessageBlockRef | null {
  const kind = String(raw?.kind || '').trim() as AssistantMessageBlockKind
  if (kind !== 'text' && kind !== 'tool_invocation' && kind !== 'tool_result') return null
  const start = typeof raw?.start === 'number' && Number.isFinite(raw.start) ? Math.max(0, Math.floor(raw.start)) : undefined
  const end = typeof raw?.end === 'number' && Number.isFinite(raw.end) ? Math.max(0, Math.floor(raw.end)) : undefined
  return {
    kind,
    blockId: String(raw?.blockId || '').trim(),
    partId: String(raw?.partId || '').trim(),
    start,
    end,
  }
}

function findToolPart(message: any, ref: AssistantMessageBlockRef) {
  const parts = Array.isArray(message?.parts) ? message.parts : []
  const partId = String(ref.partId || '').trim()
  if (!partId) return null
  return parts.find((part: any, index: number) => String(part?.type || '') === 'tool' && assistantToolPartId(part, index) === partId) || null
}

function replaceContentRange(message: any, ref: AssistantMessageBlockRef, replacement: string): MutationResult {
  const content = String(message?.content ?? '')
  const start = Number(ref.start)
  const end = Number(ref.end)
  if (!Number.isFinite(start) || !Number.isFinite(end) || start < 0 || end < start || end > content.length) return { ok: false, error: '文本块范围无效' }
  const nextContent = content.slice(0, start) + replacement + content.slice(end)
  const validation = validateTextProtocolToolRequestsForParts(nextContent, message?.parts)
  if (!validation.ok) return { ok: false, error: validation.error }
  message.content = nextContent
  syncMessageTextPart(message)
  syncTextProtocolToolParts(message.parts, validation.matches)
  return { ok: true }
}

export function replaceMessageText(message: any, text: unknown): MutationResult {
  if (!message || typeof message !== 'object') return { ok: false, error: '消息无效' }
  const nextContent = String(text ?? '')
  const validation = validateTextProtocolToolRequestsForParts(nextContent, message?.parts)
  if (!validation.ok) return { ok: false, error: validation.error }
  message.content = nextContent
  syncMessageTextPart(message)
  syncTextProtocolToolParts(message.parts, validation.matches)
  return { ok: true }
}

function editTextBlock(message: any, ref: AssistantMessageBlockRef, text: string): MutationResult {
  const blocks = planAssistantMessageBlocks(message?.content, message?.parts)
  const block = blocks.find((item) => item.kind === 'text' && item.id === ref.blockId)
  if (!block || block.kind !== 'text') return { ok: false, error: '文本块不存在' }
  return replaceContentRange(message, { ...ref, start: block.start, end: block.end }, text)
}

function editInvocationBlock(message: any, ref: AssistantMessageBlockRef, text: string): MutationResult {
  const part = findToolPart(message, ref)
  if (!part) return { ok: false, error: '工具调用块不存在' }
  if (isTextProtocolToolPart(part)) return { ok: false, error: '文本协议工具调用属于消息正文，请编辑消息文本' }

  let parsed: any = null
  try {
    parsed = JSON.parse(String(text || '{}'))
  } catch (_) {
    return { ok: false, error: '原生工具调用必须是 JSON' }
  }
  const box = plainObject(parsed)
  const toolName = String(box.toolName || '').trim()
  if (toolName) part.toolName = toolName
  if (box.input && typeof box.input === 'object' && !Array.isArray(box.input)) part.input = box.input
  else if (!box.toolName) part.input = box
  part.raw = JSON.stringify(part.input || {})
  return { ok: true }
}

function editResultBlock(message: any, ref: AssistantMessageBlockRef, text: string): MutationResult {
  const part = findToolPart(message, ref)
  const result = part?.result && typeof part.result === 'object' ? part.result : null
  if (!part || !result) return { ok: false, error: '工具返回块不存在' }
  if (String(result.error || '').trim() && !String(result.content || '').trim()) result.error = String(text ?? '')
  else result.content = String(text ?? '')
  return { ok: true }
}

function ensureDisplay(part: any) {
  if (!part.display || typeof part.display !== 'object' || Array.isArray(part.display)) part.display = {}
  return part.display
}

function deleteTextBlock(message: any, ref: AssistantMessageBlockRef): MutationResult {
  const blocks = planAssistantMessageBlocks(message?.content, message?.parts)
  const block = blocks.find((item) => item.kind === 'text' && item.id === ref.blockId)
  if (!block || block.kind !== 'text') return { ok: false, error: '文本块不存在' }
  return replaceContentRange(message, { ...ref, start: block.start, end: block.end }, '')
}

function deleteInvocationBlock(message: any, ref: AssistantMessageBlockRef): MutationResult {
  const part = findToolPart(message, ref)
  if (!part) return { ok: false, error: '工具调用块不存在' }
  if (isTextProtocolToolPart(part)) return { ok: false, error: '文本协议工具调用属于消息正文，请删除消息文本' }
  ensureDisplay(part).hideInvocation = true
  return { ok: true }
}

function deleteResultBlock(message: any, ref: AssistantMessageBlockRef): MutationResult {
  const part = findToolPart(message, ref)
  if (!part || !part.result) return { ok: false, error: '工具返回块不存在' }
  ensureDisplay(part).hideResult = true
  return { ok: true }
}

export function editAssistantMessageBlock(message: any, refRaw: any, text: unknown): MutationResult {
  const ref = normalizeBlockRef(refRaw)
  if (!message || typeof message !== 'object') return { ok: false, error: '消息无效' }
  if (!ref) return { ok: false, error: '消息块无效' }
  if (ref.kind === 'text') return editTextBlock(message, ref, String(text ?? ''))
  if (ref.kind === 'tool_invocation') return editInvocationBlock(message, ref, String(text ?? ''))
  if (ref.kind === 'tool_result') return editResultBlock(message, ref, String(text ?? ''))
  return { ok: false, error: '消息块不可编辑' }
}

export function deleteAssistantMessageBlock(message: any, refRaw: any): MutationResult {
  const ref = normalizeBlockRef(refRaw)
  if (!message || typeof message !== 'object') return { ok: false, error: '消息无效' }
  if (!ref) return { ok: false, error: '消息块无效' }
  if (ref.kind === 'text') return deleteTextBlock(message, ref)
  if (ref.kind === 'tool_invocation') return deleteInvocationBlock(message, ref)
  if (ref.kind === 'tool_result') return deleteResultBlock(message, ref)
  return { ok: false, error: '消息块不可删除' }
}
