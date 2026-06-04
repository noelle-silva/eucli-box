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
  part: any
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
  for (let index = 1; index < lines.length - 1; index++) {
    const line = String(lines[index] || '').trim()
    if (!line) continue
    const match = line.match(/^\[([A-Za-z0-9_-]+)\]:[ \t]?(.*)$/)
    if (!match) return null
    const key = String(match[1] || '').trim()
    const value = String(match[2] || '').trim()
    if (!key) return null
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

  for (const line of sourceLines(content)) {
    const marker = String(line.text || '').trim()
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
  if (extracted.requests.length > protocolParts.length) return { ok: false as const, error: 'TOOL_REQUEST 没有对应的工具记录' }
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

  matched.matches.forEach(({ request, part }, index) => {
    ranges.push({
      id: assistantToolPartId(part, index),
      part: { ...part, raw: request.raw, toolName: request.toolName || part?.toolName, input: request.input },
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
  const used = new Set<number>()
  const matches: TextProtocolToolRequestMatch[] = []
  const unmatchedRequests: TextProtocolToolRequestRange[] = []

  for (const request of requests) {
    const exactIndex = parts.findIndex((part: any, index: number) => !used.has(index) && parseTextToolRequest(String(part?.raw || ''))?.raw === request.raw)
    if (exactIndex >= 0) {
      used.add(exactIndex)
      matches.push({ request, part: parts[exactIndex] })
      continue
    }
    unmatchedRequests.push(request)
  }

  const unmatchedPartIndexes = parts.map((_part: any, index: number) => index).filter((index: number) => !used.has(index))
  if (unmatchedRequests.length === 1 && unmatchedPartIndexes.length === 1) {
    matches.push({ request: unmatchedRequests[0], part: parts[unmatchedPartIndexes[0]] })
    return { ok: true, matches }
  }

  if (unmatchedRequests.length) {
    return { ok: false, error: '多个文本协议工具调用不能在同一次正文编辑中改写未配对的 TOOL_REQUEST' }
  }

  return { ok: true, matches }
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
