export type AssistantToolRenderSegment =
  | { type: 'text'; id: string; text: string }
  | { type: 'tool'; id: string; part: any }

export type AssistantToolRenderPlan = {
  segments: AssistantToolRenderSegment[]
  trailingToolParts: any[]
  diagnostics: AssistantToolRenderDiagnostic[]
}

export type AssistantToolRenderDiagnostic = {
  id: string
  part: any
  reason: string
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

export function assistantToolParts(parts: any[]) {
  return (Array.isArray(parts) ? parts : []).filter((part: any) => String(part?.type || '') === 'tool')
}

export function buildAssistantToolRenderPlan(contentRaw: unknown, partsRaw: any[]): AssistantToolRenderPlan {
  const content = String(contentRaw ?? '')
  const toolParts = assistantToolParts(partsRaw)
  const segments: AssistantToolRenderSegment[] = []
  const trailingToolParts: any[] = []
  const diagnostics: AssistantToolRenderDiagnostic[] = []
  let cursor = 0

  toolParts.forEach((part: any, index: number) => {
    const source = String(part?.source || '').trim()
    const raw = String(part?.raw || '')
    const id = String(part?.id || part?.callId || `tool:${index}`)

    if (source !== 'text_protocol') {
      trailingToolParts.push(part)
      return
    }

    if (!raw) {
      diagnostics.push({ id, part, reason: '文本协议工具调用缺少 raw TOOL_REQUEST，无法在正文中定位替换。' })
      return
    }

    const range = findToolRawRange(content, raw, cursor)
    if (!range) {
      diagnostics.push({ id, part, reason: '正文中找不到对应的 raw TOOL_REQUEST，工具调用卡片没有安全替换位置。' })
      return
    }

    if (range.start > cursor) {
      segments.push({ type: 'text', id: `text:${segments.length}`, text: content.slice(cursor, range.start) })
    }
    segments.push({ type: 'tool', id, part })
    cursor = range.end
  })

  if (cursor < content.length) {
    segments.push({ type: 'text', id: `text:${segments.length}`, text: content.slice(cursor) })
  }

  return { segments, trailingToolParts, diagnostics }
}
