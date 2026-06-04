import * as React from 'react'
import { Box, CircularProgress, Typography } from '@mui/material'

export function AssistantReplyPendingIndicator() {
  return (
    <Box
      role="status"
      aria-live="polite"
      aria-label="AI 回复加载中"
      sx={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 1,
        minHeight: 32,
        px: 0.25,
        color: 'text.secondary',
      }}
    >
      <CircularProgress size={18} thickness={4.4} color="inherit" />
      <Typography variant="body2" color="text.secondary">
        正在思考…
      </Typography>
    </Box>
  )
}
