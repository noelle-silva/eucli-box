export const TEXT_PROTOCOL_TOOL_SOURCE = 'text_protocol'
export const TOOL_REQUEST_START = '<<<TOOL_REQUEST>>>'
export const TOOL_REQUEST_END = '<<<END_TOOL_REQUEST>>>'

export type TextProtocolToolRequest = {
  raw: string
  toolName: string
  input: Record<string, string>
}

export type TextProtocolToolRequestRange = TextProtocolToolRequest & {
  start: number
  end: number
}

export type TextProtocolToolRange = {
  id: string
  request: TextProtocolToolRequestRange
  part: any | null
  raw: string
  start: number
  end: number
}

export type TextProtocolToolDiagnostic = {
  id: string
  part: any
  reason: string
}

export type TextProtocolToolRangePlan = {
  ranges: TextProtocolToolRange[]
  diagnostics: TextProtocolToolDiagnostic[]
}

export type TextProtocolToolRequestMatch = {
  request: TextProtocolToolRequestRange
  part: any
}

type SourceLine = {
  text: string
  start: number
  contentEnd: number
  end: number
}

export function assistantToolParts(parts: any[]) {
  return (Array.isArray(parts) ? parts : []).filter((part: any) => String(part?.type || '') === 'tool')
}

export function assistantToolPartId(part: any, index = 0) {
  return String(part?.id || part?.callId || `tool:${index}`)
}

export function isTextProtocolToolPart(part: any) {
  return String(part?.source || '').trim() === TEXT_PROTOCOL_TOOL_SOURCE
}

export function textProtocolToolParts(parts: any[]) {
  return assistantToolParts(parts).filter(isTextProtocolToolPart)
}

export function parseTextToolRequest(rawText: string): TextProtocolToolRequest | null {
  const raw = String(rawText || '').replace(/\r\n/g, '\n').replace(/\r/g, '\n').trim()
  const lines = raw.split('\n')
  if (String(lines[0] || '').trim() !== TOOL_REQUEST_START) return null
  if (String(lines[lines.length - 1] || '').trim() !== TOOL_REQUEST_END) return null

  const input: Record<string, string> = {}
  let toolName = ''
  const seen = new Set<string>()
  let entryCount = 0
  for (let index = 1; index < lines.length - 1; index++) {
    const line = String(lines[index] || '').trim()
    if (!line) continue
    const match = line.match(/^\[([A-Za-z0-9_-]+)\]:[ \t]?(.*)$/)
    if (!match) return null
    const key = String(match[1] || '').trim()
    const value = String(match[2] || '').trim()
    if (!key) return null
    if (seen.has(key)) return null
    seen.add(key)
    if (entryCount === 0 && key !== 'tool') return null
    entryCount++
    if (key === 'tool') toolName = value
    else input[key] = value
  }

  if (!toolName) return null
  return { raw, toolName, input }
}

export function extractTextToolRequests(contentRaw: unknown): { ok: true; requests: TextProtocolToolRequestRange[] } | { ok: false; error: string } {
  const content = String(contentRaw ?? '')
  const requests: TextProtocolToolRequestRange[] = []
  let blockStart: number | null = null
  let inFence = false

  for (const line of sourceLines(content)) {
    const marker = String(line.text || '').trim()
    if (blockStart === null && isMarkdownFenceLine(line.text)) {
      inFence = !inFence
      continue
    }
    if (inFence) continue

    if (marker === TOOL_REQUEST_START) {
      if (blockStart !== null) return { ok: false, error: 'TOOL_REQUEST 不能嵌套' }
      blockStart = line.start
      continue
    }

    if (marker !== TOOL_REQUEST_END) continue
    if (blockStart === null) return { ok: false, error: 'TOOL_REQUEST 结束标记缺少开始标记' }

    const rawText = content.slice(blockStart, line.contentEnd)
    const parsed = parseTextToolRequest(rawText)
    if (!parsed) return { ok: false, error: 'TOOL_REQUEST 格式无效' }
    requests.push({ ...parsed, start: blockStart, end: line.contentEnd })
    blockStart = null
  }

  if (blockStart !== null) return { ok: false, error: 'TOOL_REQUEST 缺少结束标记' }
  return { ok: true, requests }
}

export function validateTextProtocolToolRequestsForParts(contentRaw: unknown, partsRaw: any[]) {
  const extracted = extractTextToolRequests(contentRaw)
  if (!extracted.ok) return extracted
  const protocolParts = textProtocolToolParts(partsRaw)
  const matched = matchTextProtocolRequestsToParts(extracted.requests, protocolParts)
  if (!matched.ok) return matched
  return { ...extracted, matches: matched.matches }
}

export function syncTextProtocolToolParts(partsRaw: any[], matches: TextProtocolToolRequestMatch[]) {
  const protocolParts = textProtocolToolParts(partsRaw)
  const matchedParts = new Set<any>()
  matches.forEach(({ part, request }) => {
    matchedParts.add(part)
    part.raw = request.raw
    part.toolName = request.toolName
    part.input = request.input
  })
  protocolParts.forEach((part: any) => {
    if (!matchedParts.has(part)) part.raw = ''
  })
}

export function planTextProtocolToolRanges(contentRaw: unknown, partsRaw: any[]): TextProtocolToolRangePlan {
  const extracted = extractTextToolRequests(contentRaw)
  const protocolParts = textProtocolToolParts(partsRaw)
  const ranges: TextProtocolToolRange[] = []
  const diagnostics: TextProtocolToolDiagnostic[] = []

  if (!extracted.ok) {
    diagnostics.push({ id: 'text-protocol', part: null, reason: extracted.error })
    return { ranges, diagnostics }
  }

  const matched = matchTextProtocolRequestsToParts(extracted.requests, protocolParts)
  if (!matched.ok) {
    diagnostics.push({ id: 'text-protocol', part: null, reason: matched.error })
    return { ranges, diagnostics }
  }

  const partByRequest = new Map<TextProtocolToolRequestRange, any>()
  matched.matches.forEach(({ request, part }) => partByRequest.set(request, part))

  extracted.requests.forEach((request, index) => {
    const part = partByRequest.get(request) || null
    ranges.push({
      id: part ? assistantToolPartId(part, index) : `text-protocol:${request.start}:${request.end}`,
      request,
      part: part ? { ...part, raw: request.raw, toolName: request.toolName || part?.toolName, input: request.input } : null,
      raw: request.raw,
      start: request.start,
      end: request.end,
    })
  })

  const matchedParts = new Set(matched.matches.map((item) => item.part))
  protocolParts.forEach((part: any, index: number) => {
    if (matchedParts.has(part) || !String(part?.raw || '').trim()) return
    diagnostics.push({ id: assistantToolPartId(part, index), part, reason: '工具记录保留了文本协议调用，但正文中已经没有对应的 TOOL_REQUEST。' })
  })

  return { ranges, diagnostics }
}

function matchTextProtocolRequestsToParts(requests: TextProtocolToolRequestRange[], parts: any[]): { ok: true; matches: TextProtocolToolRequestMatch[] } | { ok: false; error: string } {
  const usedRequests = new Set<number>()
  const matches: TextProtocolToolRequestMatch[] = []

  const unmatchedParts: any[] = []
  for (const part of parts) {
    const exactIndex = requests.findIndex((request, index) => !usedRequests.has(index) && parseTextToolRequest(String(part?.raw || ''))?.raw === request.raw)
    if (exactIndex >= 0) {
      usedRequests.add(exactIndex)
      matches.push({ request: requests[exactIndex], part })
      continue
    }
    unmatchedParts.push(part)
  }

  const unmatchedRequests = requests.filter((_request, index) => !usedRequests.has(index))
  if (unmatchedParts.length > unmatchedRequests.length) {
    return { ok: false, error: '工具记录保留了文本协议调用，但正文中已经没有对应的 TOOL_REQUEST。' }
  }
  for (let index = 0; index < unmatchedParts.length; index++) {
    matches.push({ request: unmatchedRequests[index], part: unmatchedParts[index] })
  }

  return { ok: true, matches }
}

function isMarkdownFenceLine(line: string) {
  const trimmed = String(line || '').trim()
  return trimmed.startsWith('```') || trimmed.startsWith('~~~')
}

function sourceLines(source: string): SourceLine[] {
  const lines: SourceLine[] = []
  let start = 0

  while (start < source.length) {
    const newline = source.indexOf('\n', start)
    const end = newline >= 0 ? newline + 1 : source.length
    const contentEnd = newline >= 0 ? (newline > start && source[newline - 1] === '\r' ? newline - 1 : newline) : source.length
    lines.push({ text: source.slice(start, contentEnd), start, contentEnd, end })
    start = end
  }

  return lines
}
