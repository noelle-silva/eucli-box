import * as React from 'react'
import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, FormControl, IconButton, InputLabel, MenuItem, Select, Stack, Typography } from '@mui/material'
import CloseIcon from '@mui/icons-material/Close'
import StorefrontIcon from '@mui/icons-material/Storefront'
import RefreshIcon from '@mui/icons-material/Refresh'
import DownloadIcon from '@mui/icons-material/Download'
import UpdateIcon from '@mui/icons-material/Update'
import { artifactStatusLabels, compatibilityRangeText, type ReleaseArtifactIdentity, type ReleaseCheckResult } from '../../domain/release'
import { SettingsPill } from './SettingsSurfaces'

type ArtifactStoreDialogProps = {
  open: boolean
  onClose: () => void
  kind: 'tool' | 'plugin'
  title: string
  results: ReleaseCheckResult[]
  installState: any
  actionBusy: boolean
  onAction: (artifact: ReleaseArtifactIdentity, action: 'install' | 'update') => Promise<void> | void
  onRefresh: () => Promise<void> | void
  devBoxSourceEnabled?: boolean
  boxSourceKind?: string
  onChangeBoxSourceKind?: (value: string) => Promise<void> | void
}

export function ArtifactStoreDialog(props: ArtifactStoreDialogProps) {
  const { open, onClose, kind, title, results, installState, actionBusy, onAction, onRefresh, devBoxSourceEnabled, boxSourceKind, onChangeBoxSourceKind } = props
  const [refreshing, setRefreshing] = React.useState(false)
  const items = Array.isArray(results)
    ? results
        .filter((result) => String(result.artifact?.kind || '') === kind)
        .sort((a, b) => String(a.artifact?.id || '').localeCompare(String(b.artifact?.id || '')))
    : []

  const refresh = async () => {
    setRefreshing(true)
    try {
      await onRefresh()
    } finally {
      setRefreshing(false)
    }
  }

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="md">
      <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        <StorefrontIcon fontSize="small" />
        {title}
        <Box sx={{ flex: 1 }} />
        {devBoxSourceEnabled ? (
          <FormControl size="small" sx={{ minWidth: 140 }}>
            <InputLabel id="store-install-source-label">安装来源</InputLabel>
            <Select
              labelId="store-install-source-label"
              label="安装来源"
              value={boxSourceKind === 'development' ? 'development' : 'official'}
              onChange={(event) => onChangeBoxSourceKind?.(event.target.value)}
            >
              <MenuItem value="official">官方发行</MenuItem>
              <MenuItem value="development">本地开发版</MenuItem>
            </Select>
          </FormControl>
        ) : null}
        <Button startIcon={<RefreshIcon />} size="small" variant="text" onClick={refresh} disabled={refreshing}>
          {refreshing ? '刷新中…' : '刷新'}
        </Button>
        <IconButton onClick={onClose} size="small" aria-label="关闭商店">
          <CloseIcon fontSize="small" />
        </IconButton>
      </DialogTitle>
      <DialogContent sx={{ bgcolor: 'grey.50' }}>
        <Stack spacing={1}>
          {items.length ? (
            items.map((result) => (
              <StoreItem key={`${result.artifact.kind}:${result.artifact.id}`} result={result} installState={installState} actionBusy={actionBusy} onAction={onAction} />
            ))
          ) : (
            <Typography variant="body2" color="text.secondary" sx={{ p: 2, textAlign: 'center' }}>
              当前没有可显示的{itemTitle(kind)}。请先刷新官方发行记录。
            </Typography>
          )}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Typography variant="caption" color="text.secondary" sx={{ mr: 'auto', pl: 1 }}>
          安装或更新由业务端在后台完成，客户端只发起一次动作。
        </Typography>
        <Button onClick={onClose}>关闭</Button>
      </DialogActions>
    </Dialog>
  )
}

function StoreItem(props: { result: ReleaseCheckResult; installState: any; actionBusy: boolean; onAction: (artifact: ReleaseArtifactIdentity, action: 'install' | 'update') => Promise<void> | void }) {
  const { result, installState, actionBusy, onAction } = props
  const artifact = result.artifact
  const id = String(artifact?.id || '')
  const compatibility = result.compatibility
  const failed = result.status === 'failed'
  const installed = result.installed === true
  const canInstall = !installed && !!result.latestVersion && !failed
  const canUpdate = installed && result.updateAvailable === true && !failed
  const stateForItem = installState && typeof installState === 'object' && String(installState.artifact?.id || '') === id ? installState : null
  const stateStatus = String(stateForItem?.status || '')
  const stateError = stateForItem?.error && typeof stateForItem.error === 'object' ? stateForItem.error : {}
  const busy = actionBusy || stateStatus === 'downloading' || stateStatus === 'verifying' || stateStatus === 'preparing' || stateStatus === 'switching' || stateStatus === 'starting' || stateStatus === 'restoring'

  return (
    <Box sx={{ p: 1.35, borderRadius: 2, bgcolor: 'background.paper' }}>
      <Stack spacing={0.75}>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={0.75} alignItems={{ xs: 'flex-start', sm: 'center' }}>
          <Typography variant="body2" sx={{ minWidth: 0, flex: 1, fontWeight: 900, overflowWrap: 'anywhere' }}>
            {id}
          </Typography>
          <Stack direction="row" spacing={0.75} sx={{ flexWrap: 'wrap' }}>
            <SettingsPill tone={installed ? 'selected' : 'info'}>{installed ? '已安装' : '未安装'}</SettingsPill>
            {canInstall ? (
              <Button size="small" variant="contained" startIcon={<DownloadIcon />} disabled={busy} onClick={() => onAction(artifact, 'install')}>
                {busy ? '处理中…' : '安装'}
              </Button>
            ) : null}
            {canUpdate ? (
              <Button size="small" variant="contained" startIcon={<UpdateIcon />} disabled={busy} onClick={() => onAction(artifact, 'update')}>
                {busy ? '处理中…' : '更新'}
              </Button>
            ) : null}
            {installed && !canUpdate && !failed ? <SettingsPill>已是最新版</SettingsPill> : null}
          </Stack>
        </Stack>
        <Stack direction="row" spacing={1.5} sx={{ flexWrap: 'wrap', rowGap: 0.5 }}>
          <Typography variant="caption" color="text.secondary">
            当前：<Box component="span" sx={{ color: 'text.primary', fontWeight: 800 }}>{installed ? result.currentVersion || '版本资料无效' : '未安装'}</Box>
          </Typography>
          <Typography variant="caption" color="text.secondary">
            官方：<Box component="span" sx={{ color: 'text.primary', fontWeight: 800 }}>{result.latestVersion || '暂无正式发行'}</Box>
          </Typography>
          {result.downloadSize > 0 ? (
            <Typography variant="caption" color="text.secondary">
              大小：<Box component="span" sx={{ color: 'text.primary', fontWeight: 800 }}>{formatBytes(result.downloadSize)}</Box>
            </Typography>
          ) : null}
        </Stack>
        {compatibility ? (
          <Typography variant="caption" color={compatibility.compatible ? 'success.main' : 'error.main'} sx={{ overflowWrap: 'anywhere' }}>
            {compatibility.compatible ? `适用于当前业务端（范围 ${compatibilityRangeText(compatibility.requiredEucliBoxCompatibility)}）` : compatibility.reason || '不适用于当前业务端'}
          </Typography>
        ) : null}
        {failed && result.failureReason ? (
          <Typography variant="caption" color="error" sx={{ overflowWrap: 'anywhere' }}>
            {result.failureReason}
          </Typography>
        ) : null}
        {stateForItem && String(stateError.code || '') ? (
          <Typography variant="caption" color={stateStatus === 'blocked' ? 'warning.main' : 'error'} sx={{ overflowWrap: 'anywhere' }}>
            上次操作：{String(stateError.code || '')}@{String(stateError.phase || '未知阶段')}：{String(stateError.message || '未知原因')}
          </Typography>
        ) : null}
        {result.releaseNotes ? (
          <Typography variant="caption" color="text.secondary" sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>
            {result.releaseNotes}
          </Typography>
        ) : null}
        {busy ? (
          <Typography variant="caption" color="info.main">
            处理中：{artifactStatusLabels[stateStatus] || stateStatus}
          </Typography>
        ) : null}
      </Stack>
    </Box>
  )
}

function itemTitle(kind: string) {
  return kind === 'plugin' ? '系统插件' : 'AI 工具'
}

function formatBytes(value: number) {
  const bytes = Number(value)
  if (!Number.isFinite(bytes) || bytes <= 0) return '未知'
  if (bytes < 1024) return `${Math.round(bytes)} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}
