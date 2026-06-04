import {
  assistantToolPartId,
  assistantToolParts,
  isTextProtocolToolPart,
  planTextProtocolToolRanges,
  textProtocolToolParts,
} from './textProtocolTools'

export type AssistantMessageRenderSegment =
  | { type: 'text'; id: string; text: string; start: number; end: number }
  | { type: 'tool'; id: string; part: any; start: number; end: number }

export type AssistantMessageBlockKind = 'text' | 'tool_invocation' | 'tool_result' | 'diagnostic'

export type AssistantMessageBlock =
  | { kind: 'text'; id: string; text: string; start: number; end: number; parts: any[] }
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
  diagnostics: AssistantMessageRenderDiagnostic[]
}

export { assistantToolPartId } from './textProtocolTools'

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
  const segments: AssistantMessageRenderSegment[] = []
  const rangePlan = planTextProtocolToolRanges(content, partsRaw)
  let cursor = 0

  for (const range of rangePlan.ranges) {
    if (range.start > cursor) segments.push({ type: 'text', id: `text:${cursor}:${range.start}`, text: content.slice(cursor, range.start), start: cursor, end: range.start })
    segments.push({ type: 'tool', id: range.id, part: range.part, start: range.start, end: range.end })
    cursor = range.end
  }

  if (cursor < content.length) {
    segments.push({ type: 'text', id: `text:${cursor}:${content.length}`, text: content.slice(cursor), start: cursor, end: content.length })
  }

  return { segments, diagnostics: rangePlan.diagnostics }
}

export function planAssistantMessageBlocks(contentRaw: unknown, partsRaw: any[]): AssistantMessageBlock[] {
  const content = String(contentRaw ?? '')
  const toolParts = assistantToolParts(partsRaw)
  const inlineParts = textProtocolToolParts(partsRaw)
  const blocks: AssistantMessageBlock[] = []

  if (content.trim() || inlineParts.length) blocks.push({ kind: 'text', id: `text:0:${content.length}`, text: content, start: 0, end: content.length, parts: inlineParts })

  toolParts.forEach((part: any, index: number) => {
    if (isTextProtocolToolPart(part)) {
      if (part?.result && typeof part.result === 'object' && !isToolResultHidden(part)) blocks.push({ kind: 'tool_result', id: `tool-result:${assistantToolPartId(part, index)}`, part })
      return
    }
    pushToolBlocks(blocks, part, { index })
  })

  return blocks
}
