import * as React from 'react'
import { Alert, Box, Stack, Typography } from '@mui/material'

function stringifyDetails(value: unknown) {
  if (value === undefined || value === null) return ''
  if (typeof value === 'string') return value
  try {
    return JSON.stringify(value, null, 2)
  } catch (_) {
    return String(value)
  }
}

export function AssistantErrorNotice(props: { error: any }) {
  const raw = props.error && typeof props.error === 'object' ? props.error : null
  const message = String(raw?.message || '').trim()
  if (!message) return null

  const code = String(raw?.code || '').trim()
  const system = String(raw?.system || '').trim()
  const details = stringifyDetails(raw?.details)

  return (
    <Alert severity="error" variant="outlined" sx={{ borderRadius: 2, alignItems: 'flex-start' }}>
      <Stack spacing={0.75} sx={{ minWidth: 0 }}>
        <Typography variant="subtitle2" sx={{ fontWeight: 900 }}>
          上游请求失败
        </Typography>
        <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>
          {message}
        </Typography>
        {code || system ? (
          <Typography variant="caption" color="text.secondary" sx={{ overflowWrap: 'anywhere' }}>
            {[system, code].filter(Boolean).join(' / ')}
          </Typography>
        ) : null}
        {details ? (
          <Box component="details" sx={{ mt: 0.25 }}>
            <Box component="summary" sx={{ cursor: 'pointer', fontSize: 12, fontWeight: 800 }}>
              原始错误详情
            </Box>
            <Box
              component="pre"
              sx={{
                mt: 0.75,
                mb: 0,
                maxHeight: 260,
                overflow: 'auto',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
                fontSize: 12,
                p: 1,
                borderRadius: 1.5,
                bgcolor: 'rgba(0,0,0,.04)',
              }}
            >
              {details}
            </Box>
          </Box>
        ) : null}
      </Stack>
    </Alert>
  )
}
