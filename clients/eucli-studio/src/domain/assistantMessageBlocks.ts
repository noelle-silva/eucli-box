export type AssistantMessageRenderSegment =
  | { type: 'text'; id: string; text: string; start: number; end: number }
  | { type: 'tool'; id: string; part: any; start: number; end: number }

export type AssistantMessageBlockKind = 'text' | 'tool_invocation' | 'tool_result' | 'diagnostic'

export type AssistantMessageBlock =
  | { kind: 'text'; id: string; text: string; start: number; end: number }
  | { kind: 'tool_invocation'; id: string; part: any; start?: number; end?: number }
  | { kind: 'tool_result'; id: string; part: any; start?: number; end?: number }
  | { kind: 'diagnostic'; id: string; reason: string; part?: any }

export type AssistantMessageRenderDiagnostic = {
  id: string
  part: any
  reason: string
}

export type AssistantMessageRenderPlan = {
  segments: AssistantMessageRenderSegment[]
  trailingToolParts: any[]
  diagnostics: AssistantMessageRenderDiagnostic[]
}

type IndexedText = {
  text: string
  originalIndexByNormalizedIndex: number[]
}

function normalizeLineEndingsWithIndex(input: string): IndexedText {
  let text = ''
  const originalIndexByNormalizedIndex: number[] = []

  for (let i = 0; i < input.length;) {
    const ch = input[i]
    if (ch === '\r') {
      text += '\n'
      originalIndexByNormalizedIndex.push(i)
      i += input[i + 1] === '\n' ? 2 : 1
      continue
    }
    text += ch
    originalIndexByNormalizedIndex.push(i)
    i++
  }

  return { text, originalIndexByNormalizedIndex }
}

function originalEndFromNormalizedEnd(source: string, indexed: IndexedText, normalizedEnd: number) {
  if (normalizedEnd >= indexed.originalIndexByNormalizedIndex.length) return source.length
  return indexed.originalIndexByNormalizedIndex[normalizedEnd]
}

function findToolRawRange(content: string, raw: string, start: number): { start: number; end: number } | null {
  if (!raw) return null

  const exactStart = content.indexOf(raw, start)
  if (exactStart >= 0) return { start: exactStart, end: exactStart + raw.length }

  if (!content.includes('\r') && !raw.includes('\r')) return null

  const indexedContent = normalizeLineEndingsWithIndex(content)
  const indexedRaw = normalizeLineEndingsWithIndex(raw)
  if (!indexedRaw.text) return null

  let searchFrom = 0
  for (;;) {
    const normalizedStart = indexedContent.text.indexOf(indexedRaw.text, searchFrom)
    if (normalizedStart < 0) return null

    const originalStart = indexedContent.originalIndexByNormalizedIndex[normalizedStart] ?? content.length
    const originalEnd = originalEndFromNormalizedEnd(content, indexedContent, normalizedStart + indexedRaw.text.length)
    if (originalStart >= start) return { start: originalStart, end: originalEnd }
    searchFrom = normalizedStart + Math.max(1, indexedRaw.text.length)
  }
}

function assistantToolParts(parts: any[]) {
  return (Array.isArray(parts) ? parts : []).filter((part: any) => String(part?.type || '') === 'tool')
}

export function assistantToolPartId(part: any, index = 0) {
  return String(part?.id || part?.callId || `tool:${index}`)
}

function toolPartDisplay(part: any) {
  return part?.display && typeof part.display === 'object' ? part.display : {}
}

export function isToolInvocationHidden(part: any) {
  return !!toolPartDisplay(part).hideInvocation
}

export function isToolResultHidden(part: any) {
  return !!toolPartDisplay(part).hideResult
}

export function isToolPartVisibleInPrompt(part: any) {
  return !isToolInvocationHidden(part) && !isToolResultHidden(part)
}

function pushToolBlocks(blocks: AssistantMessageBlock[], part: any, opts?: { start?: number; end?: number; index?: number }) {
  const id = assistantToolPartId(part, opts?.index || blocks.length)
  const start = typeof opts?.start === 'number' ? opts.start : undefined
  const end = typeof opts?.end === 'number' ? opts.end : undefined
  if (!isToolInvocationHidden(part)) blocks.push({ kind: 'tool_invocation', id: `tool-invocation:${id}`, part, start, end })
  if (part?.result && typeof part.result === 'object' && !isToolResultHidden(part)) blocks.push({ kind: 'tool_result', id: `tool-result:${id}`, part, start, end })
}

export function planAssistantMessageRender(contentRaw: unknown, partsRaw: any[]): AssistantMessageRenderPlan {
  const content = String(contentRaw ?? '')
  const toolParts = assistantToolParts(partsRaw)
  const segments: AssistantMessageRenderSegment[] = []
  const trailingToolParts: any[] = []
  const diagnostics: AssistantMessageRenderDiagnostic[] = []
  let cursor = 0

  toolParts.forEach((part: any, index: number) => {
    const source = String(part?.source || '').trim()
    const raw = String(part?.raw || '')
    const id = assistantToolPartId(part, index)

    if (source !== 'text_protocol') {
      trailingToolParts.push(part)
      return
    }

    if (!raw) {
      trailingToolParts.push(part)
      return
    }

    const range = findToolRawRange(content, raw, cursor)
    if (!range) {
      trailingToolParts.push(part)
      return
    }

    if (range.start > cursor) {
      segments.push({ type: 'text', id: `text:${cursor}:${range.start}`, text: content.slice(cursor, range.start), start: cursor, end: range.start })
    }
    segments.push({ type: 'tool', id, part, start: range.start, end: range.end })
    cursor = range.end
  })

  if (cursor < content.length) {
    segments.push({ type: 'text', id: `text:${cursor}:${content.length}`, text: content.slice(cursor), start: cursor, end: content.length })
  }

  return { segments, trailingToolParts, diagnostics }
}

export function planAssistantMessageBlocks(contentRaw: unknown, partsRaw: any[]): AssistantMessageBlock[] {
  const plan = planAssistantMessageRender(contentRaw, partsRaw)
  const blocks: AssistantMessageBlock[] = []

  for (const segment of plan.segments) {
    if (segment.type === 'text') {
      if (String(segment.text || '').trim()) blocks.push({ kind: 'text', id: segment.id, text: segment.text, start: segment.start, end: segment.end })
      continue
    }
    pushToolBlocks(blocks, segment.part, { start: segment.start, end: segment.end })
  }

  for (const diagnostic of plan.diagnostics) {
    blocks.push({ kind: 'diagnostic', id: `diagnostic:${diagnostic.id}`, reason: diagnostic.reason, part: diagnostic.part })
    pushToolBlocks(blocks, diagnostic.part)
  }

  plan.trailingToolParts.forEach((part: any, index: number) => pushToolBlocks(blocks, part, { index }))

  return blocks
}
