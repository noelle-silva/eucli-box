import * as React from 'react'
import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, IconButton, Paper, Stack, TextField, Tooltip, Typography } from '@mui/material'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import EditOutlinedIcon from '@mui/icons-material/EditOutlined'
import SaveOutlinedIcon from '@mui/icons-material/SaveOutlined'
import CloseIcon from '@mui/icons-material/Close'
import { planAssistantMessageBlocks, type AssistantMessageBlock } from '../../render/assistantMessagePlan'
import { renderAssistantToolDiagnosticHtml, renderAssistantToolInvocationHtml, renderAssistantToolResultHtml } from '../../render/assistantToolHtml'
import { AssistantMessageHost } from '../../render/assistantMessageHost'
import type { AiChatToastOptions } from '../../gateway/capabilities'

type AssistantMessageBlocksProps = {
  controller: any
  mid: string
  text: string
  parts: any[]
  renderSafetyPolicyKey: string
  chatRootRef: React.RefObject<HTMLElement | null>
  disabled?: boolean
}

type EditingBlock = { id: string; text: string }

function blockTitle(block: AssistantMessageBlock) {
  if (block.kind === 'text') return '消息正文'
  if (block.kind === 'tool_invocation') return '工具调用'
  if (block.kind === 'tool_result') return '工具返回'
  return '渲染诊断'
}

function blockTone(block: AssistantMessageBlock) {
  if (block.kind === 'tool_invocation') return { borderColor: 'rgba(2,132,199,.22)', bgcolor: 'rgba(2,132,199,.035)' }
  if (block.kind === 'tool_result') return { borderColor: 'rgba(22,163,74,.24)', bgcolor: 'rgba(22,163,74,.035)' }
  if (block.kind === 'diagnostic') return { borderColor: 'rgba(220,38,38,.24)', bgcolor: 'rgba(220,38,38,.035)' }
  return { borderColor: 'rgba(15,23,42,.10)', bgcolor: 'rgba(255,255,255,.54)' }
}

function partIdentity(part: any) {
  return String(part?.id || part?.callId || '').trim()
}

function blockMutationRef(block: AssistantMessageBlock) {
  const ref: any = { kind: block.kind, blockId: block.id }
  if (block.kind === 'text') {
    ref.start = block.start
    ref.end = block.end
    return ref
  }
  if (block.kind === 'tool_invocation' || block.kind === 'tool_result') {
    ref.partId = partIdentity(block.part)
    if (typeof block.start === 'number') ref.start = block.start
    if (typeof block.end === 'number') ref.end = block.end
  }
  return ref
}

function prettyJson(value: any) {
  try {
    return JSON.stringify(value ?? {}, null, 2)
  } catch (_) {
    return String(value ?? '')
  }
}

function invocationEditText(part: any) {
  return prettyJson(part?.input && typeof part.input === 'object' ? part.input : {})
}

function blockEditText(block: AssistantMessageBlock) {
  if (block.kind === 'text') return block.text
  if (block.kind === 'tool_invocation') return invocationEditText(block.part)
  if (block.kind === 'tool_result') {
    const result = block.part?.result && typeof block.part.result === 'object' ? block.part.result : null
    return String(result?.content || result?.error || '')
  }
  return block.kind === 'diagnostic' ? block.reason : ''
}

function blockCopyText(block: AssistantMessageBlock) {
  if (block.kind === 'text') return block.text
  if (block.kind === 'tool_invocation') return invocationEditText(block.part)
  if (block.kind === 'tool_result') {
    const result = block.part?.result && typeof block.part.result === 'object' ? block.part.result : null
    if (!result) return ''
    const text = String(result.content || result.error || '')
    return text || prettyJson(result)
  }
  return block.reason
}

function renderToolBlockHtml(block: AssistantMessageBlock) {
  if (block.kind === 'tool_invocation') return renderAssistantToolInvocationHtml(block.part)
  if (block.kind === 'tool_result') return renderAssistantToolResultHtml(block.part)
  if (block.kind === 'diagnostic') return renderAssistantToolDiagnosticHtml(block.reason)
  return ''
}

function showToast(controller: any, message: string, options?: AiChatToastOptions) {
  controller?.capabilities?.ui?.showToast?.(message, options)
}

function writeClipboard(controller: any, text: string) {
  const writeText = controller?.capabilities?.clipboard?.writeText
  if (typeof writeText !== 'function') {
    showToast(controller, '未授权：clipboard.writeText', { kind: 'error' })
    return
  }
  Promise.resolve()
    .then(() => writeText(text))
    .then(() => showToast(controller, '已复制', { kind: 'success' }))
    .catch(() => showToast(controller, '复制失败', { kind: 'error' }))
}

export function AssistantMessageBlocks(props: AssistantMessageBlocksProps) {
  const { controller, mid, text, parts, renderSafetyPolicyKey, chatRootRef, disabled } = props
  const blocks = React.useMemo(() => planAssistantMessageBlocks(text, parts), [text, parts])
  const [editing, setEditing] = React.useState<EditingBlock>({ id: '', text: '' })
  const [deleting, setDeleting] = React.useState<AssistantMessageBlock | null>(null)

  React.useEffect(() => {
    setEditing({ id: '', text: '' })
    setDeleting(null)
  }, [mid, text, parts])

  const saveEdit = async (block: AssistantMessageBlock) => {
    if (!editing.id || editing.id !== block.id) return
    const ok = await Promise.resolve(controller.actions.editMessageBlock?.(mid, blockMutationRef(block), editing.text))
    if (ok === true) setEditing({ id: '', text: '' })
  }

  const deleteBlock = async () => {
    const block = deleting
    if (!block) return
    const ok = await Promise.resolve(controller.actions.deleteMessageBlock?.(mid, blockMutationRef(block)))
    if (ok === true) setDeleting(null)
  }

  if (!blocks.length) return null

  return (
    <Stack spacing={0.9} data-mid={mid}>
      {blocks.map((block) => {
        if (block.kind === 'text') {
          return (
            <Box key={block.id} data-mid={mid} data-assistant-block-kind={block.kind} sx={{ minWidth: 0 }}>
              <AssistantMessageHost controller={controller} className="prose" text={block.text} parts={block.parts} mid={mid} renderSafetyPolicyKey={renderSafetyPolicyKey} chatRootRef={chatRootRef} />
            </Box>
          )
        }
        const isEditing = editing.id === block.id
        const tone = blockTone(block)
        const canEdit = block.kind !== 'diagnostic' && !disabled
        const canDelete = block.kind !== 'diagnostic' && !disabled
        return (
          <Paper
            key={block.id}
            variant="outlined"
            data-mid={mid}
            data-assistant-block-kind={block.kind}
            sx={{
              borderColor: tone.borderColor,
              bgcolor: tone.bgcolor,
              borderRadius: 3,
              px: 1.1,
              py: 0.9,
              overflow: 'hidden',
            }}
          >
            <Stack direction="row" spacing={0.75} alignItems="center" sx={{ mb: 0.65, minWidth: 0 }}>
              <Typography variant="caption" sx={{ fontWeight: 900, color: 'rgba(15,23,42,.68)', minWidth: 0 }} noWrap>
                {blockTitle(block)}
              </Typography>
              <Box sx={{ flex: 1, minWidth: 8 }} />
              {isEditing ? (
                <>
                  <Tooltip title="保存">
                    <span>
                      <IconButton size="small" aria-label="保存消息块" disabled={!!disabled} onClick={() => saveEdit(block)}>
                        <SaveOutlinedIcon fontSize="inherit" />
                      </IconButton>
                    </span>
                  </Tooltip>
                  <Tooltip title="取消">
                    <IconButton size="small" aria-label="取消编辑消息块" onClick={() => setEditing({ id: '', text: '' })}>
                      <CloseIcon fontSize="inherit" />
                    </IconButton>
                  </Tooltip>
                </>
              ) : (
                <>
                  <Tooltip title="编辑">
                    <span>
                      <IconButton size="small" aria-label="编辑消息块" disabled={!canEdit} onClick={() => setEditing({ id: block.id, text: blockEditText(block) })}>
                        <EditOutlinedIcon fontSize="inherit" />
                      </IconButton>
                    </span>
                  </Tooltip>
                  <Tooltip title="复制">
                    <IconButton size="small" aria-label="复制消息块" onClick={() => writeClipboard(controller, blockCopyText(block))}>
                      <ContentCopyIcon fontSize="inherit" />
                    </IconButton>
                  </Tooltip>
                  <Tooltip title="删除">
                    <span>
                      <IconButton size="small" aria-label="删除消息块" disabled={!canDelete} onClick={() => setDeleting(block)}>
                        <DeleteOutlineIcon fontSize="inherit" />
                      </IconButton>
                    </span>
                  </Tooltip>
                </>
              )}
            </Stack>

            {isEditing ? (
              <TextField
                autoFocus
                fullWidth
                multiline
                minRows={5}
                size="small"
                value={editing.text}
                onChange={(event) => setEditing((current) => ({ ...current, text: event.target.value }))}
                onKeyDown={(event) => {
                  if (event.key === 'Escape') setEditing({ id: '', text: '' })
                  if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) saveEdit(block)
                }}
                sx={{ '& .MuiOutlinedInput-root': { bgcolor: '#fff' } }}
              />
            ) : (
              <Box className="prose" dangerouslySetInnerHTML={{ __html: renderToolBlockHtml(block) }} />
            )}
          </Paper>
        )
      })}

      <Dialog open={!!deleting} onClose={() => setDeleting(null)} fullWidth maxWidth="xs">
        <DialogTitle>删除这个消息块？</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary">
            只删除当前的{deleting ? blockTitle(deleting) : '消息块'}；同一条回复里的其他块会保留。
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleting(null)}>取消</Button>
          <Button color="error" variant="contained" disabled={!!disabled || !deleting} onClick={deleteBlock}>删除</Button>
        </DialogActions>
      </Dialog>
    </Stack>
  )
}
