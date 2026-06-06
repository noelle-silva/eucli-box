import * as React from 'react'
import { Box, Button, Paper, Stack, Typography } from '@mui/material'

export type SettingsTabValue = 'appearance' | 'attachments' | 'data' | 'groups' | 'roles' | 'providers' | 'modelGroups' | 'services' | 'tools' | 'stickers' | 'eb'

type SettingsNavigationItem = {
  value: SettingsTabValue
  label: string
}

const SETTINGS_NAVIGATION_ITEMS: SettingsNavigationItem[] = [
  { value: 'appearance', label: '外观' },
  { value: 'attachments', label: '附件' },
  { value: 'data', label: '数据' },
  { value: 'groups', label: '群组管理' },
  { value: 'roles', label: '角色管理' },
  { value: 'providers', label: '供应商管理' },
  { value: 'modelGroups', label: '模型组' },
  { value: 'services', label: 'AI 微服务' },
  { value: 'eb', label: 'e-b' },
  { value: 'tools', label: 'AI 工具' },
  { value: 'stickers', label: '表情包' },
]

const TAB_BUTTON_SX = { borderRadius: 999, minWidth: 0, px: 1.25, py: 0.25, flexShrink: 0 }

export function SettingsNavigationBar(props: { value: SettingsTabValue; onChange: (value: SettingsTabValue) => void }) {
  return (
    <Paper variant="outlined" sx={{ p: 1, borderRadius: 2, bgcolor: 'background.paper' }}>
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}>
        <Typography variant="body2" sx={{ fontWeight: 900, whiteSpace: 'nowrap' }}>
          设置分区
        </Typography>
        <Box sx={{ minWidth: 0, flex: 1, overflowX: 'auto', scrollbarWidth: 'thin' }}>
          <Stack direction="row" spacing={0.5} sx={{ minWidth: 'max-content' }}>
            {SETTINGS_NAVIGATION_ITEMS.map((item) => (
              <Button
                key={item.value}
                size="small"
                variant={props.value === item.value ? 'contained' : 'outlined'}
                onClick={() => props.onChange(item.value)}
                sx={TAB_BUTTON_SX}
              >
                {item.label}
              </Button>
            ))}
          </Stack>
        </Box>
      </Stack>
    </Paper>
  )
}

export function SettingsPageLayout(props: { topbarHeight: number; value: SettingsTabValue; onChange: (value: SettingsTabValue) => void; children: React.ReactNode }) {
  return (
    <Box sx={{ flex: 1, minWidth: 0, minHeight: 0, overflow: 'auto', px: 2, pt: `calc(${props.topbarHeight}px + 16px)`, pb: 2, bgcolor: 'grey.50' }}>
      <Stack spacing={1.5}>
        <SettingsNavigationBar value={props.value} onChange={props.onChange} />
        {props.children}
      </Stack>
    </Box>
  )
}
