import * as React from 'react'
import { Box, Button, Stack, Typography } from '@mui/material'
import type { LocalBoxState } from '../../domain/localBox'
import type { ReleaseArtifactIdentity, ReleaseCheckResult } from '../../domain/release'

type LocalBoxUpdatePanelProps = {
  state: LocalBoxState
  check: ReleaseCheckResult | null
  busy: boolean
  onUpdate: () => Promise<void> | void
}

export function LocalBoxUpdatePanel(props: LocalBoxUpdatePanelProps) {
  const { state, check, busy, onUpdate } = props
  const updating = state.status === 'downloading'
    || state.status === 'verifying'
    || state.status === 'waiting_stop'
    || state.status === 'switching'
    || state.status === 'starting'
    || state.status === 'restoring'
  if (!check) return null
  const canUpdate = check.updateAvailable && state.installed && !busy && !updating
  return (
    <Box sx={{ p: 1.35, borderRadius: 2, bgcolor: 'action.hover' }}>
      <Stack spacing={0.75}>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={0.75} alignItems={{ xs: 'flex-start', sm: 'center' }}>
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Typography variant="body2" sx={{ fontWeight: 900 }}>业务端更新</Typography>
            <Typography variant="body2" color="text.secondary" sx={{ overflowWrap: 'anywhere' }}>
              {check.latestVersion ? `官方新版 ${check.latestVersion}` : '暂无正式发行'}
              {check.downloadSize > 0 ? ` · 下载大小 ${formatBytes(check.downloadSize)}` : ''}
            </Typography>
          </Box>
          <Button size="small" variant="contained" disabled={!canUpdate} onClick={onUpdate} sx={{ minWidth: 104 }}>
            {updating ? updateProgressLabel(state) : busy ? '处理中…' : '更新业务端'}
          </Button>
        </Stack>
        {check.releaseNotes ? (
          <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>
            {check.releaseNotes}
          </Typography>
        ) : null}
        {check.affectedArtifacts.length ? (
          <Typography variant="caption" color="warning.main" sx={{ overflowWrap: 'anywhere' }}>
            更新后这些工具/插件将不适用，仍显示但停用：{check.affectedArtifacts.map(artifactLabel).join('、')}
          </Typography>
        ) : null}
        {check.compatibility && !check.compatibility.compatible ? (
          <Typography variant="caption" color="error.main" sx={{ overflowWrap: 'anywhere' }}>
            当前客户端不适用新版业务端：更新后正常业务受限，版本状态与维护入口仍可用。
          </Typography>
        ) : null}
        {state.error.message ? (
          <Typography variant="caption" color="error" role="alert" sx={{ overflowWrap: 'anywhere' }}>
            {updateErrorText(state.error.code, state.error.message)}
          </Typography>
        ) : null}
      </Stack>
    </Box>
  )
}

function updateErrorText(code: string, message: string) {
  if (code === 'LOCAL_BOX_UPDATE_BLOCKED') return `${message}；需要先等待或结束正在进行的工作。`
  if (code === 'LOCAL_BOX_DATA_UNSAFE') return `${message}；需要人工处理，请不要重装或删除数据目录。`
  if (code === 'LOCAL_BOX_MIGRATION_RECOVERED') return message
  return message
}

function updateProgressLabel(state: LocalBoxState) {
  switch (state.status) {
    case 'waiting_stop': return '等待业务端结束'
    case 'switching': return '切换版本中'
    case 'restoring': return '恢复中'
    case 'starting': return '启动中'
    default: return '下载核对中'
  }
}

function artifactLabel(artifact: ReleaseArtifactIdentity) {
  if (artifact.kind === 'eucli-box') return '业务端'
  if (artifact.kind === 'tool') return `AI 工具 · ${artifact.id || '未知'}`
  if (artifact.kind === 'plugin') return `系统插件 · ${artifact.id || '未知'}`
  return artifact.id || '未知发布物'
}

function formatBytes(value: number) {
  const bytes = Number(value)
  if (!Number.isFinite(bytes) || bytes <= 0) return '未知'
  if (bytes < 1024) return `${Math.round(bytes)} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}
