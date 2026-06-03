import * as React from 'react'
import { Box, Chip, Paper, Stack, Typography } from '@mui/material'
import StorageIcon from '@mui/icons-material/Storage'

function safePrettyJson(value: any) {
  try {
    return JSON.stringify(value ?? {}, null, 2)
  } catch (_) {
    return String(value ?? '')
  }
}

function toolPartStateText(state0: any) {
  const state = String(state0 || '').trim()
  if (state === 'requested') return '已请求'
  if (state === 'needs_confirmation') return '等待确认'
  if (state === 'running') return '运行中'
  if (state === 'completed') return '已完成'
  if (state === 'error') return '失败'
  if (state === 'denied') return '已拒绝'
  if (state === 'cancelled') return '已取消'
  return state || '未知状态'
}

type AssistantToolCallCardProps = {
  part: any
}

export function AssistantToolCallCard(props: AssistantToolCallCardProps) {
  const { part } = props
  const name = String(part?.toolName || 'tool')
  const source = String(part?.source || '').trim()
  const isTextProtocol = source === 'text_protocol'
  const state = String(part?.state || '')
  const result = part?.result && typeof part.result === 'object' ? part.result : null
  const decision = part?.decision && typeof part.decision === 'object' ? part.decision : null
  const resultText = result ? String(result?.content || result?.error || '') : ''
  const rawText = String(part?.raw || '')

  return (
    <Paper
      data-stop="1"
      variant="outlined"
      sx={{ borderRadius: 2, borderColor: 'rgba(2, 132, 199, .22)', bgcolor: 'rgba(2, 132, 199, .045)', px: 1.25, py: 1 }}
    >
      <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 0.75, minWidth: 0 }}>
        <StorageIcon sx={{ fontSize: 18, color: 'rgba(2, 132, 199, .9)' }} />
        <Typography variant="body2" sx={{ fontWeight: 900 }}>
          {isTextProtocol ? '文本协议工具调用' : '原生工具调用'}
        </Typography>
        <Chip size="small" label={name} variant="outlined" sx={{ maxWidth: 260 }} />
        {source ? <Chip size="small" label={source} variant="outlined" /> : null}
        <Chip size="small" label={toolPartStateText(state)} color={state === 'completed' ? 'success' : state === 'error' || state === 'denied' ? 'error' : 'default'} />
        <Box sx={{ flex: 1 }} />
        {part?.callId ? (
          <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace' }}>
            {String(part.callId)}
          </Typography>
        ) : null}
      </Stack>
      <Stack spacing={0.75}>
        {isTextProtocol && rawText.trim() ? (
          <Box>
            <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 900 }}>
              原始 TOOL_REQUEST
            </Typography>
            <Box component="pre" sx={{ m: 0, mt: 0.25, p: 0.75, borderRadius: 1.5, bgcolor: 'rgba(88, 28, 135, .08)', border: '1px solid rgba(126, 34, 206, .18)', whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', fontSize: 12, fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace' }}>
              {rawText}
            </Box>
          </Box>
        ) : null}
        <Box>
          <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 900 }}>
            输入参数
          </Typography>
          <Box component="pre" sx={{ m: 0, mt: 0.25, p: 0.75, borderRadius: 1.5, bgcolor: 'rgba(15, 23, 42, .06)', whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', fontSize: 12, fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace' }}>
            {safePrettyJson(part?.input || {})}
          </Box>
        </Box>
        {decision ? (
          <Typography variant="caption" color="text.secondary">
            权限：{String(decision?.status || '') || '未知'}{decision?.reason ? `，原因：${String(decision.reason)}` : ''}
          </Typography>
        ) : null}
        {result ? (
          <Box>
            <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 900 }}>
              返回结果（{String(result?.status || '') || 'unknown'}）
            </Typography>
            <Box component="pre" sx={{ m: 0, mt: 0.25, p: 0.75, borderRadius: 1.5, bgcolor: 'rgba(15, 23, 42, .06)', whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', maxHeight: 260, overflow: 'auto', fontSize: 12, fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace' }}>
              {resultText || safePrettyJson(result)}
            </Box>
          </Box>
        ) : null}
      </Stack>
    </Paper>
  )
}
