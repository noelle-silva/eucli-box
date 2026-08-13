import * as React from 'react'
import { Box, Button, Dialog, DialogContent, DialogTitle, InputAdornment, Stack, Switch, TextField, Typography } from '@mui/material'
import AddIcon from '@mui/icons-material/Add'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import LockOpenIcon from '@mui/icons-material/LockOpen'
import LockOutlineIcon from '@mui/icons-material/LockOutline'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'
import RestartAltIcon from '@mui/icons-material/RestartAlt'
import StopIcon from '@mui/icons-material/Stop'
import VisibilityIcon from '@mui/icons-material/Visibility'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import { persistentPortStateLabel, type BoxShutdownResult, type PersistentKeyCreated } from '../../domain/accessSettings'
import { useEvent } from '../hooks/useEvent'
import { SettingsListItem, SettingsPill, SettingsSection, SettingsSurface } from './SettingsSurfaces'

type AccessSettingsPanelProps = {
  controller: any
  section: any
  localBoxState: any
  keepBoxRunningOnExit: boolean
  onKeepBoxRunningOnExitChange: (value: boolean) => Promise<void> | void
  onStartBox?: () => Promise<void> | void
  onRestartBox?: () => Promise<void> | void
  onStopBox?: () => Promise<void> | void
}

export function AccessSettingsPanel(props: AccessSettingsPanelProps) {
  const { controller, section: accessSection, localBoxState, keepBoxRunningOnExit, onKeepBoxRunningOnExitChange, onStartBox, onRestartBox, onStopBox } = props
  const section = accessSection && typeof accessSection === 'object' ? accessSection : {}
  const [portDialog, setPortDialog] = React.useState(false)
  const [portName, setPortName] = React.useState('')
  const [portNumber, setPortNumber] = React.useState('')
  const [keyDialog, setKeyDialog] = React.useState(false)
  const [keyName, setKeyName] = React.useState('')
  const [keyExpiryMode, setKeyExpiryMode] = React.useState<'never' | 'custom'>('never')
  const [keyExpiryValue, setKeyExpiryValue] = React.useState('')
  const [revealed, setRevealed] = React.useState<{ id: string; plainKey: string } | null>(null)
  const [created, setCreated] = React.useState<PersistentKeyCreated | null>(null)
  const [shutdownConfirm, setShutdownConfirm] = React.useState<BoxShutdownResult | null>(null)
  const busy = section.portsSaving || section.keysSaving || section.boxShutdownLoading

  React.useEffect(() => {
    controller?.actions?.refreshAccessPorts?.(true)
    controller?.actions?.refreshAccessKeys?.(true)
    controller?.actions?.loadBoxInfo?.()
  }, [controller])

  const refreshAll = useEvent(() => {
    controller?.actions?.refreshAccessPorts?.(true)
    controller?.actions?.refreshAccessKeys?.(true)
    controller?.actions?.loadBoxInfo?.()
  })

  const submitPort = useEvent(async () => {
    const port = Number(String(portNumber || '').trim())
    if (!String(portName || '').trim() || !Number.isInteger(port) || port < 1 || port > 65535) {
      controller?.capabilities?.ui?.showToast?.('请输入名称和 1-65535 的端口号', { kind: 'error' })
      return
    }
    const ok = await controller?.actions?.addAccessPort?.(portName, port)
    if (ok) {
      setPortDialog(false)
      setPortName('')
      setPortNumber('')
    }
  })

  const submitKey = useEvent(async () => {
    if (!String(keyName || '').trim()) {
      controller?.capabilities?.ui?.showToast?.('请输入 Key 名称', { kind: 'error' })
      return
    }
    let expiresAt: string | null = null
    if (keyExpiryMode === 'custom') {
      const value = String(keyExpiryValue || '').trim()
      if (!value) {
        controller?.capabilities?.ui?.showToast?.('请选择过期时间', { kind: 'error' })
        return
      }
      expiresAt = new Date(value).toISOString()
    }
    const created = await controller?.actions?.addAccessKey?.(keyName, expiresAt)
    if (created) {
      setKeyDialog(false)
      setKeyName('')
      setKeyExpiryValue('')
      setCreated(created)
    }
  })

  const copyText = useEvent(async (text: string) => {
    try {
      await controller?.capabilities?.clipboard?.writeText?.(text)
      controller?.capabilities?.ui?.showToast?.('已复制到剪贴板', { kind: 'success' })
    } catch (e: any) {
      controller?.capabilities?.ui?.showToast?.(String(e?.message || e || '复制失败'), { kind: 'error' })
    }
  })

  const handleStart = useEvent(async () => {
    if (onStartBox) await onStartBox()
  })

  const handleRestart = useEvent(async () => {
    shutdownConfirmActionRef.current = 'restart'
    const result = await controller?.actions?.requestBoxShutdown?.(false)
    if (result?.requiresConfirmation) {
      setShutdownConfirm(result)
      return
    }
    if (onRestartBox) await onRestartBox()
  })

  const handleStop = useEvent(async () => {
    shutdownConfirmActionRef.current = 'stop'
    const result = await controller?.actions?.requestBoxShutdown?.(false)
    if (result?.requiresConfirmation) {
      setShutdownConfirm(result)
      return
    }
    if (onStopBox) await onStopBox()
  })

  const confirmShutdown = useEvent(async () => {
    const result = await controller?.actions?.requestBoxShutdown?.(true)
    const action = shutdownConfirmActionRef.current
    setShutdownConfirm(null)
    if (result?.status === 'shutdown_requested') {
      if (action === 'restart' && onRestartBox) await onRestartBox()
      else if (action === 'stop' && onStopBox) await onStopBox()
    }
  })

  const shutdownConfirmActionRef = React.useRef<'restart' | 'stop'>('stop')

  const running = !!localBoxState?.connected
  const boxVersion = String(section.boxInfo?.version || localBoxState?.currentVersion || '')
  const ports = Array.isArray(section.ports) ? section.ports : []
  const keys = Array.isArray(section.keys) ? section.keys : []

  return (
    <SettingsSurface>
      <Stack spacing={1.5}>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}>
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Stack direction="row" spacing={0.75} alignItems="center" sx={{ flexWrap: 'wrap' }}>
              <Typography sx={{ fontWeight: 900 }}>业务端访问设置</Typography>
              <SettingsPill tone={running ? 'selected' : 'muted'}>{running ? '运行中' : '已停止'}</SettingsPill>
              {boxVersion ? <SettingsPill tone="info">v{boxVersion}</SettingsPill> : null}
            </Stack>
            <Typography variant="body2" color="text.secondary">
              管理长期端口与长期 Key，以及业务端的运行方式。
            </Typography>
          </Box>
          <Stack direction="row" spacing={1}>
            <Button startIcon={<PlayArrowIcon />} variant="contained" size="small" onClick={handleStart} disabled={busy || running}>
              启动业务端
            </Button>
            <Button startIcon={<RestartAltIcon />} variant="text" size="small" onClick={handleRestart} disabled={busy || !running}>
              重新启动
            </Button>
            <Button startIcon={<StopIcon />} variant="text" size="small" color="error" onClick={handleStop} disabled={busy || !running}>
              停止
            </Button>
          </Stack>
        </Stack>

        <SettingsSection tone="muted">
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'flex-start', sm: 'center' }} justifyContent="space-between">
            <Box sx={{ minWidth: 0, flex: 1 }}>
              <Typography sx={{ fontWeight: 900 }}>退出后继续运行业务端</Typography>
              <Typography variant="body2" color="text.secondary">
                开启后，关闭客户端时业务端将继续在后台运行
              </Typography>
            </Box>
            <Switch
              size="small"
              checked={keepBoxRunningOnExit}
              onChange={(event) => onKeepBoxRunningOnExitChange?.(event.target.checked)}
            />
          </Stack>
        </SettingsSection>

        <SettingsSection>
          <Stack spacing={1.25}>
            <Stack direction="row" spacing={1} alignItems="center" justifyContent="space-between">
              <Box>
                <Typography sx={{ fontWeight: 900 }}>长期端口</Typography>
                <Typography variant="body2" color="text.secondary">
                  接收本机和系统允许到达的外部连接，使用长期 Key 访问。
                </Typography>
              </Box>
              <Button startIcon={<AddIcon />} variant="contained" size="small" onClick={() => setPortDialog(true)} disabled={busy}>
                新增端口
              </Button>
            </Stack>
            {section.portsError ? <Typography variant="body2" color="error">{section.portsError}</Typography> : null}
            {ports.length === 0 ? (
              <Typography variant="body2" color="text.secondary">暂无长期端口</Typography>
            ) : (
              ports.map((port: any) => (
                <SettingsListItem key={String(port.id || '')} tone={port.actualState === 'running' ? 'selected' : undefined}>
                  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}>
                    <Box sx={{ minWidth: 0, flex: 1 }}>
                      <Stack direction="row" spacing={0.75} alignItems="center" sx={{ flexWrap: 'wrap' }}>
                        <Typography sx={{ fontWeight: 800 }}>{port.name}</Typography>
                        <SettingsPill tone="info">:{port.port}</SettingsPill>
                        <SettingsPill tone={port.actualState === 'failed' ? 'danger' : port.actualState === 'running' ? 'selected' : 'muted'}>
                          {persistentPortStateLabel(port)}
                        </SettingsPill>
                      </Stack>
                      {port.failureReason ? (
                        <Typography variant="caption" color="error">{port.failureReason}</Typography>
                      ) : null}
                    </Box>
                    <Stack direction="row" spacing={0.75} justifyContent={{ xs: 'flex-start', sm: 'flex-end' }}>
                      {port.desiredState === 'disabled' ? (
                        <Button startIcon={<LockOpenIcon />} size="small" variant="text" onClick={() => controller?.actions?.enableAccessPort?.(port.id)} disabled={busy}>
                          启用
                        </Button>
                      ) : (
                        <Button startIcon={<LockOutlineIcon />} size="small" variant="text" onClick={() => controller?.actions?.disableAccessPort?.(port.id)} disabled={busy}>
                          停用
                        </Button>
                      )}
                      <Button startIcon={<DeleteOutlineIcon />} size="small" color="error" variant="text" onClick={() => controller?.actions?.deleteAccessPort?.(port.id)} disabled={busy}>
                        删除
                      </Button>
                    </Stack>
                  </Stack>
                </SettingsListItem>
              ))
            )}
          </Stack>
        </SettingsSection>

        <SettingsSection>
          <Stack spacing={1.25}>
            <Stack direction="row" spacing={1} alignItems="center" justifyContent="space-between">
              <Box>
                <Typography sx={{ fontWeight: 900 }}>长期 Key</Typography>
                <Typography variant="body2" color="text.secondary">
                  用于长期端口访问的身份凭证，由当前 Windows 用户保护保存。
                </Typography>
              </Box>
              <Button startIcon={<AddIcon />} variant="contained" size="small" onClick={() => setKeyDialog(true)} disabled={busy}>
                新增 Key
              </Button>
            </Stack>
            {section.keysError ? <Typography variant="body2" color="error">{section.keysError}</Typography> : null}
            {keys.length === 0 ? (
              <Typography variant="body2" color="text.secondary">暂无长期 Key</Typography>
            ) : (
              keys.map((key: any) => (
                <SettingsListItem key={String(key.id || '')} tone={key.enabled ? undefined : 'muted'}>
                  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}>
                    <Box sx={{ minWidth: 0, flex: 1 }}>
                      <Stack direction="row" spacing={0.75} alignItems="center" sx={{ flexWrap: 'wrap' }}>
                        <Typography sx={{ fontWeight: 800 }}>{key.name}</Typography>
                        <SettingsPill tone={key.enabled ? 'selected' : 'muted'}>{key.enabled ? '已启用' : '已停用'}</SettingsPill>
                        <SettingsPill>{key.expiresAt ? `有效期至 ${formatDateTime(key.expiresAt)}` : '永不过期'}</SettingsPill>
                      </Stack>
                      {key.lastUsedAt ? (
                        <Typography variant="caption" color="text.secondary">最后使用：{formatDateTime(key.lastUsedAt)}</Typography>
                      ) : (
                        <Typography variant="caption" color="text.secondary">尚未使用</Typography>
                      )}
                    </Box>
                    <Stack direction="row" spacing={0.75} justifyContent={{ xs: 'flex-start', sm: 'flex-end' }} sx={{ flexWrap: 'wrap' }}>
                      <Button startIcon={<VisibilityIcon />} size="small" variant="text" onClick={async () => {
                        const plain = await controller?.actions?.revealAccessKey?.(key.id)
                        if (plain) setRevealed({ id: String(key.id || ''), plainKey: plain })
                      }} disabled={busy}>
                        查看
                      </Button>
                      <Button startIcon={<ContentCopyIcon />} size="small" variant="text" onClick={async () => {
                        const plain = await controller?.actions?.revealAccessKey?.(key.id)
                        if (plain) await copyText(plain)
                      }} disabled={busy}>
                        复制
                      </Button>
                      {key.enabled ? (
                        <Button size="small" variant="text" onClick={() => controller?.actions?.setAccessKeyEnabled?.(key.id, false)} disabled={busy}>
                          停用
                        </Button>
                      ) : (
                        <Button size="small" variant="text" onClick={() => controller?.actions?.setAccessKeyEnabled?.(key.id, true)} disabled={busy}>
                          启用
                        </Button>
                      )}
                      <Button size="small" variant="text" onClick={() => controller?.actions?.setAccessKeyExpiration?.(key.id, null)} disabled={busy}>
                        设为永不过期
                      </Button>
                      <Button startIcon={<DeleteOutlineIcon />} size="small" color="error" variant="text" onClick={() => controller?.actions?.deleteAccessKey?.(key.id)} disabled={busy}>
                        删除
                      </Button>
                    </Stack>
                  </Stack>
                </SettingsListItem>
              ))
            )}
          </Stack>
        </SettingsSection>

        <Button variant="text" size="small" onClick={refreshAll} disabled={section.portsLoading || section.keysLoading || section.boxInfoLoading}>
          刷新
        </Button>
      </Stack>

      <Dialog open={portDialog} onClose={() => setPortDialog(false)} maxWidth="xs" fullWidth>
        <DialogTitle>新增长期端口</DialogTitle>
        <DialogContent>
          <Stack spacing={1.25} sx={{ pt: 1 }}>
            <TextField size="small" label="名称" value={portName} onChange={(e) => setPortName(e.target.value)} autoFocus fullWidth />
            <TextField
              size="small"
              label="端口号"
              type="number"
              value={portNumber}
              onChange={(e) => setPortNumber(e.target.value)}
              inputProps={{ min: 1, max: 65535 }}
              fullWidth
              InputProps={{ endAdornment: <InputAdornment position="end">1-65535</InputAdornment> }}
            />
            <Button variant="contained" onClick={submitPort} disabled={busy}>创建</Button>
          </Stack>
        </DialogContent>
      </Dialog>

      <Dialog open={keyDialog} onClose={() => setKeyDialog(false)} maxWidth="xs" fullWidth>
        <DialogTitle>新增长期 Key</DialogTitle>
        <DialogContent>
          <Stack spacing={1.25} sx={{ pt: 1 }}>
            <TextField size="small" label="名称" value={keyName} onChange={(e) => setKeyName(e.target.value)} autoFocus fullWidth />
            <Stack direction="row" spacing={1} alignItems="center">
              <Button size="small" variant={keyExpiryMode === 'never' ? 'contained' : 'text'} onClick={() => setKeyExpiryMode('never')}>
                永不过期
              </Button>
              <Button size="small" variant={keyExpiryMode === 'custom' ? 'contained' : 'text'} onClick={() => setKeyExpiryMode('custom')}>
                指定过期
              </Button>
            </Stack>
            {keyExpiryMode === 'custom' ? (
              <TextField size="small" label="过期时间" type="datetime-local" value={keyExpiryValue} onChange={(e) => setKeyExpiryValue(e.target.value)} fullWidth />
            ) : null}
            <Button variant="contained" onClick={submitKey} disabled={busy}>创建</Button>
          </Stack>
        </DialogContent>
      </Dialog>

      <Dialog open={!!revealed} onClose={() => setRevealed(null)} maxWidth="sm" fullWidth>
        <DialogTitle>长期 Key</DialogTitle>
        <DialogContent>
          <Stack spacing={1.25} sx={{ pt: 1 }}>
            <Typography variant="body2" color="text.secondary">
              完整 Key 只在查看时显示，请妥善保管。
            </Typography>
            <Box
              sx={{
                p: 1.25,
                borderRadius: 2,
                bgcolor: 'var(--studio-paper-muted)',
                fontFamily: 'monospace',
                fontSize: 13,
                wordBreak: 'break-all',
                userSelect: 'all',
              }}
            >
              {revealed?.plainKey}
            </Box>
            <Button startIcon={<ContentCopyIcon />} variant="contained" onClick={() => revealed && copyText(revealed.plainKey)}>
              复制
            </Button>
          </Stack>
        </DialogContent>
      </Dialog>

      <Dialog open={!!created} onClose={() => setCreated(null)} maxWidth="sm" fullWidth>
        <DialogTitle>长期 Key 已创建</DialogTitle>
        <DialogContent>
          <Stack spacing={1.25} sx={{ pt: 1 }}>
            <Typography variant="body2" color="text.secondary">
              完整 Key 只在创建时显示一次，请立即复制并妥善保管。
            </Typography>
            <Box
              sx={{
                p: 1.25,
                borderRadius: 2,
                bgcolor: 'var(--studio-paper-muted)',
                fontFamily: 'monospace',
                fontSize: 13,
                wordBreak: 'break-all',
                userSelect: 'all',
              }}
            >
              {created?.plainKey}
            </Box>
            <Button startIcon={<ContentCopyIcon />} variant="contained" onClick={() => created && copyText(created.plainKey)}>
              复制
            </Button>
          </Stack>
        </DialogContent>
      </Dialog>

      <Dialog open={!!shutdownConfirm} onClose={() => setShutdownConfirm(null)} maxWidth="sm" fullWidth>
        <DialogTitle>存在未结束的真实工作</DialogTitle>
        <DialogContent>
          <Stack spacing={1.25} sx={{ pt: 1 }}>
            <Typography variant="body2" color="text.secondary">
              以下工作仍在进行，停止业务端会中断它们：
            </Typography>
            {(shutdownConfirm?.activeWork || []).map((work: any) => (
              <SettingsListItem key={String(work.id || '')}>
                <Typography variant="body2" sx={{ fontWeight: 700 }}>{String(work.roleId || work.id || '未知工作')}</Typography>
                <Typography variant="caption" color="text.secondary">{String(work.status || '')}</Typography>
              </SettingsListItem>
            ))}
            <Stack direction="row" spacing={1} justifyContent="flex-end">
              <Button variant="text" onClick={() => setShutdownConfirm(null)}>取消</Button>
              <Button variant="contained" color="error" onClick={confirmShutdown}>确认停止</Button>
            </Stack>
          </Stack>
        </DialogContent>
      </Dialog>
    </SettingsSurface>
  )
}

function formatDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}
