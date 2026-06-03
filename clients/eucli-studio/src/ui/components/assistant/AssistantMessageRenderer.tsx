import * as React from 'react'
import { Alert, Stack } from '@mui/material'
import { assistantToolParts, buildAssistantToolRenderPlan } from '../../../domain/assistantToolRendering'
import { useEvent } from '../../hooks/useEvent'
import { AssistantToolCallCard } from './AssistantToolCallCard'

type AssistantTextContentProps = {
  controller: any
  className?: string
  text: string
  mid: string
  renderSafetyPolicyKey: string
  chatRootRef: React.RefObject<HTMLElement | null>
}

function AssistantTextContent(props: AssistantTextContentProps) {
  const { controller, className, text, mid, renderSafetyPolicyKey, chatRootRef } = props
  const ref = React.useRef<HTMLDivElement | null>(null)

  React.useLayoutEffect(() => {
    if (!ref.current) return
    controller.renderAssistantInto(ref.current, text)
  }, [controller, text, renderSafetyPolicyKey])

  const onClick = useEvent((e: React.MouseEvent) => {
    const t = e.target as any
    const root = chatRootRef.current
    if (!root || !(t instanceof Element)) return
    if (t.closest?.('[data-stop="1"]')) return
    const block = t.closest?.('.mermaid-block[data-mermaid="1"]')
    if (!block) return
    controller.actions.openMermaidViewer(root, block)
  })

  return <div className={className} data-mid={mid} ref={ref} onClick={onClick} />
}

function AssistantToolParts(props: { parts: any[] }) {
  const toolParts = assistantToolParts(props.parts)
  if (!toolParts.length) return null
  return (
    <Stack spacing={0.75} sx={{ mt: 1 }}>
      {toolParts.map((part: any, index: number) => {
        const id = String(part?.id || `${part?.callId || 'tool'}:${index}`)
        return <AssistantToolCallCard key={id} part={part} />
      })}
    </Stack>
  )
}

export function AssistantMessageRenderer(props: {
  controller: any
  className?: string
  text: string
  parts: any[]
  mid: string
  renderSafetyPolicyKey: string
  chatRootRef: React.RefObject<HTMLElement | null>
}) {
  const { controller, className, text, parts, mid, renderSafetyPolicyKey, chatRootRef } = props
  const plan = React.useMemo(() => buildAssistantToolRenderPlan(text, parts), [text, parts])

  return (
    <Stack spacing={0.75}>
      {plan.segments.map((segment: any, index: number) => {
        if (segment.type === 'tool') return <AssistantToolCallCard key={`tool:${segment.id}:${index}`} part={segment.part} />
        const textSegment = String(segment.text || '')
        if (!textSegment.trim()) return null
        return (
          <AssistantTextContent
            key={`text:${segment.id}:${index}`}
            controller={controller}
            className={className}
            text={textSegment}
            mid={`${mid}:text:${index}`}
            renderSafetyPolicyKey={renderSafetyPolicyKey}
            chatRootRef={chatRootRef}
          />
        )
      })}
      {plan.diagnostics.map((item) => {
        const id = String(item?.id || item?.part?.id || item?.part?.callId || 'tool-render-diagnostic')
        return (
          <Stack key={`diagnostic:${id}`} spacing={0.75}>
            <Alert severity="error" variant="outlined" sx={{ borderRadius: 2 }}>
              {item.reason}
            </Alert>
            <AssistantToolCallCard part={item.part} />
          </Stack>
        )
      })}
      <AssistantToolParts parts={plan.trailingToolParts} />
    </Stack>
  )
}
