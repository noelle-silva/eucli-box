import * as React from 'react'
import { Box, Button, Chip, Divider, List, ListItemButton, ListItemText, Popover, Stack, Tooltip, Typography } from '@mui/material'
import { hookPromptPresetName, type HookPromptLibrary } from '../../domain/hookPrompt'

type HookPromptSelectorProps = {
  library: HookPromptLibrary
  selectedPresetId: string
  disabled?: boolean
  disabledReason?: string
  onSelect: (presetId: string) => void | Promise<void>
}

export function HookPromptSelector(props: HookPromptSelectorProps) {
  const { library, selectedPresetId, disabled = false, disabledReason = '', onSelect } = props
  const [anchorEl, setAnchorEl] = React.useState<HTMLElement | null>(null)
  const presets = Array.isArray(library?.presets) ? library.presets : []
  const label = hookPromptPresetName(library, selectedPresetId)
  const open = !!anchorEl

  const close = () => setAnchorEl(null)
  const select = (presetId: string) => {
    close()
    onSelect(String(presetId || '').trim())
  }

  return (
    <>
      <Tooltip title={disabled ? disabledReason || '当前不可选择 hook 提示词' : `hook 提示词：${label}`}>
        <span>
          <Button
            aria-label="选择 hook 提示词"
            size="small"
            variant="text"
            disabled={disabled}
            onClick={(e) => setAnchorEl(e.currentTarget)}
            sx={{
              minWidth: 0,
              maxWidth: 180,
              px: 1,
              py: 0.25,
              borderRadius: 999,
              color: selectedPresetId ? 'primary.main' : 'text.secondary',
              textTransform: 'none',
              fontWeight: 900,
            }}
          >
            <Box component="span" sx={{ minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {label}
            </Box>
          </Button>
        </span>
      </Tooltip>

      <Popover
        open={open}
        anchorEl={anchorEl}
        onClose={close}
        anchorOrigin={{ vertical: 'top', horizontal: 'left' }}
        transformOrigin={{ vertical: 'bottom', horizontal: 'left' }}
      >
        <Box sx={{ width: 320, maxWidth: 'calc(100vw - 32px)', p: 1 }}>
          <Stack spacing={1}>
            <Stack direction="row" spacing={1} alignItems="center">
              <Typography variant="subtitle2" sx={{ fontWeight: 900 }}>hook 提示词</Typography>
              <Box sx={{ flex: 1 }} />
              <Chip size="small" variant="outlined" label={presets.length ? `${presets.length} 个预设` : '无预设'} />
            </Stack>
            <Typography variant="caption" color="text.secondary">只影响当前真实会话；不修改聊天内容。</Typography>
            <Divider />
            <List dense sx={{ py: 0 }}>
              <ListItemButton selected={!selectedPresetId} onClick={() => select('')} sx={{ borderRadius: 2 }}>
                <ListItemText primary="无预设" secondary="当前会话不使用 hook 提示词" primaryTypographyProps={{ fontWeight: 900 }} />
              </ListItemButton>
              {presets.map((preset) => {
                const id = String(preset?.id || '')
                if (!id) return null
                return (
                  <ListItemButton key={id} selected={id === selectedPresetId} onClick={() => select(id)} sx={{ borderRadius: 2 }}>
                    <ListItemText
                      primary={String(preset?.name || '未命名预设')}
                      secondary={`${Array.isArray(preset?.messages) ? preset.messages.length : 0} 条提示内容`}
                      primaryTypographyProps={{ fontWeight: 900, noWrap: true }}
                    />
                  </ListItemButton>
                )
              })}
            </List>
          </Stack>
        </Box>
      </Popover>
    </>
  )
}
