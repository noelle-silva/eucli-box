import * as React from 'react'
import { invoke } from '@tauri-apps/api/core'
import { listen } from '@tauri-apps/api/event'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { AiChatApp } from '../ui/App'
import { StandaloneWindowControls, type WindowControlActions } from '../ui/components/StandaloneWindowControls'
import type { AiChatController } from '../controller/types'
import type { AiChatToastKind, AiChatToastOptions } from '../gateway/capabilities'
import { AI_STUDIO_CHAT_ROOT_ID } from '../runtime/aiStudioGlobals'
import { createAiChatAppRuntime, type AiChatAppRuntime, type EucliBoxConfig } from './aiChatAppHost'
import { compatibilityRangeText, type ReleaseCheckSnapshot, type StudioBootstrap } from '../domain/release'
import { ReleaseChecksPanel } from '../ui/release/ReleaseChecksPanel'

type DataDirStatus = {
  dataDir: string
  defaultDataDir: string
  configuredDataDir?: string | null
  writable: boolean
  error?: string | null
}

type BootStatus = 'booting' | 'ready' | 'error'
type ToastMessage = {
  id: number
  text: string
  kind: AiChatToastKind
}

type FwLaunchInfo = {
  launched: boolean
  standalone: boolean
  mode: string
}

function getTauriWindowSafe() {
  try {
    return getCurrentWindow()
  } catch {
    return null
  }
}

const TAURI_WINDOW = getTauriWindowSafe()

const WINDOW_CONTROL_ACTIONS: WindowControlActions = {
  minimize: () => TAURI_WINDOW?.minimize?.(),
  toggleMaximize: () => TAURI_WINDOW?.toggleMaximize?.(),
  closeToTray: () => TAURI_WINDOW ? invoke('hide_to_tray') : Promise.resolve(),
}

function commandLabel(command: string | null | undefined) {
  const id = String(command || '').trim()
  if (!id) return ''
  return COMMAND_LABELS[id] || `未知命令：${id}`
}

const COMMAND_LABELS: Record<string, string> = {
  'new-chat': '新建对话',
  'open-studio': '打开 AI Studio',
  'provider-settings': '模型提供商设置',
  'open-settings': '打开设置',
}

export function App() {
  const [dataDirStatus, setDataDirStatus] = React.useState<DataDirStatus | null>(null)
  const [dataDirBusy, setDataDirBusy] = React.useState(false)
  const [bootStatus, setBootStatus] = React.useState<BootStatus>('booting')
  const [bootError, setBootError] = React.useState('')
  const [pendingCommand, setPendingCommand] = React.useState<string | null>(null)
  const [controller, setController] = React.useState<AiChatController | null>(null)
  const [toast, setToast] = React.useState<ToastMessage | null>(null)
  const [launchInfo, setLaunchInfo] = React.useState<FwLaunchInfo>({ launched: false, standalone: true, mode: 'standalone' })
  const [eucliBoxConfig, setEucliBoxConfig] = React.useState<EucliBoxConfig | null>(null)
  const [eucliBoxUrlDraft, setEucliBoxUrlDraft] = React.useState('http://127.0.0.1:8765')
  const [eucliBoxKeyDraft, setEucliBoxKeyDraft] = React.useState('')
  const [eucliBoxConfigBusy, setEucliBoxConfigBusy] = React.useState(false)
  const [runtimeBootstrap, setRuntimeBootstrap] = React.useState<StudioBootstrap | null>(null)
  const [releaseCheckBusy, setReleaseCheckBusy] = React.useState(false)
  const runtimeRef = React.useRef<AiChatAppRuntime | null>(null)
  const runtimeVersionRef = React.useRef(0)
  const mountedRef = React.useRef(false)
  const toastSeqRef = React.useRef(0)
  const releaseCheckBusyRef = React.useRef(false)

  const showToast = React.useCallback((message: unknown, options?: AiChatToastOptions) => {
    const text = String((message as any)?.message || message || '').trim()
    if (!text) return
    const kind = options?.kind === 'success' || options?.kind === 'error' ? options.kind : 'info'
    setToast({ id: ++toastSeqRef.current, text, kind })
  }, [])

  const refreshDataDirStatus = React.useCallback(async (isCancelled: () => boolean = () => false) => {
    const status = await invoke<DataDirStatus>('data_dir_status').catch(error => ({
      dataDir: '',
      defaultDataDir: '',
      writable: false,
      error: String((error as any)?.message || error || '读取数据目录状态失败'),
    }))
    if (!isCancelled()) setDataDirStatus(status)
    return status
  }, [])

  const connectBackend = React.useCallback(async (isCancelled: () => boolean = () => false) => {
    if (isCancelled()) return null
    const runtimeVersion = runtimeVersionRef.current + 1
    runtimeVersionRef.current = runtimeVersion
    runtimeRef.current?.dispose()
    runtimeRef.current = null
    setController(null)
    setRuntimeBootstrap(null)
    releaseCheckBusyRef.current = false
    setReleaseCheckBusy(false)
    if (isCancelled()) return null
    const runtime = await createAiChatAppRuntime({
      showToast,
      onBack: () => getCurrentWindow().hide(),
    })
    if (isCancelled() || runtimeVersionRef.current !== runtimeVersion) {
      runtime.dispose()
      return null
    }
    runtimeRef.current = runtime
    setController(runtime.controller)
    setRuntimeBootstrap(runtime.bootstrap)
    const config = await runtime.getEucliBoxConfig().catch(() => null)
    if (config && !isCancelled() && runtimeVersionRef.current === runtimeVersion) {
      setEucliBoxConfig(config)
      setEucliBoxUrlDraft(config.eucliBoxUrl || 'http://127.0.0.1:8765')
      setEucliBoxKeyDraft(config.eucliBoxKey || '')
    }
    setBootStatus('ready')
    setBootError('')
    return runtime
  }, [showToast])

  const isAppUnmounted = React.useCallback(() => !mountedRef.current, [])

  const refreshMountedDataDirStatus = React.useCallback(() => {
    return refreshDataDirStatus(isAppUnmounted)
  }, [isAppUnmounted, refreshDataDirStatus])

  const connectMountedBackend = React.useCallback(async () => {
    return connectBackend(isAppUnmounted)
  }, [connectBackend, isAppUnmounted])

  const handleCommand = React.useCallback((command: string | null | undefined) => {
    const id = String(command || '').trim()
    if (!id) return
    setPendingCommand(id)
  }, [])

  React.useEffect(() => {
    if (!controller || bootStatus !== 'ready' || !pendingCommand) return
    const command = pendingCommand
    setPendingCommand(null)

    if (command === 'open-studio') return
    if (command === 'new-chat') {
      Promise.resolve(controller.actions.createChat?.()).catch(error => showToast(error, { kind: 'error' }))
      return
    }
    if (command === 'provider-settings' || command === 'open-settings') {
      Promise.resolve(controller.actions.openProviders?.()).catch(error => showToast(error, { kind: 'error' }))
      return
    }

    showToast(`未知命令：${command}`, { kind: 'error' })
  }, [bootStatus, controller, pendingCommand, showToast])

  React.useEffect(() => {
    let disposed = false
    let unlisten: (() => void) | null = null
    mountedRef.current = true

    async function boot() {
      try {
        const info = await invoke<FwLaunchInfo>('fw_launch_info').catch(() => null)
        if (disposed) return
        if (!disposed && info) setLaunchInfo(normalizeLaunchInfo(info))
        const command = await invoke<string | null>('fw_initial_command').catch(() => null)
        if (disposed) return
        if (!disposed) handleCommand(command)
        const removeCommandListener = await listen<{ command?: string }>('fw-app-command', event => handleCommand(event.payload?.command))
        if (disposed) {
          removeCommandListener()
          return
        }
        unlisten = removeCommandListener
        await invoke('app_ready').catch(() => {})
        if (disposed) return
        await refreshDataDirStatus(() => disposed)
        if (disposed) return
        if (!disposed) setBootStatus('booting')
        await connectBackend(() => disposed)
      } catch (error: any) {
        if (disposed) return
        setBootStatus('error')
        setBootError(String(error?.message || error || 'AI Studio 启动失败'))
        await refreshDataDirStatus(() => disposed)
        await invoke('app_ready').catch(() => {})
      }
    }

    boot()
    return () => {
      disposed = true
      mountedRef.current = false
      if (unlisten) unlisten()
      runtimeVersionRef.current += 1
      runtimeRef.current?.dispose()
      runtimeRef.current = null
    }
  }, [connectBackend, handleCommand, refreshDataDirStatus])

  React.useEffect(() => {
    const status = runtimeBootstrap?.releaseChecks.status
    if (bootStatus !== 'ready' || status !== 'not_checked' && status !== 'checking') return
    const runtime = runtimeRef.current
    const runtimeVersion = runtimeVersionRef.current
    if (!runtime) return
    const activeRuntime = runtime
    let cancelled = false

    async function syncPendingCheck() {
      for (let attempt = 0; attempt < 12; attempt += 1) {
        await new Promise(resolve => window.setTimeout(resolve, attempt === 0 ? 400 : 1000))
        if (cancelled || runtimeVersionRef.current !== runtimeVersion) return
        const next = await activeRuntime.getBootstrap().catch(() => null)
        if (!next || cancelled || runtimeVersionRef.current !== runtimeVersion) return
        setRuntimeBootstrap(current => current ? { ...current, releaseChecks: next.releaseChecks } : current)
        if (next.releaseChecks.status !== 'not_checked' && next.releaseChecks.status !== 'checking') return
      }
    }

    void syncPendingCheck()
    return () => {
      cancelled = true
    }
  }, [bootStatus, runtimeBootstrap?.releaseChecks.status])

  React.useEffect(() => {
    if (!toast) return
    const timer = window.setTimeout(() => setToast(current => current?.id === toast.id ? null : current), 3200)
    return () => window.clearTimeout(timer)
  }, [toast])

  async function pickDataDir() {
    if (!mountedRef.current) return
    const previousStatus = bootStatus
    try {
      setDataDirBusy(true)
      setBootError('')
      const picked = await invoke<DataDirStatus | null>('pick_data_dir')
      if (!mountedRef.current) return
      if (!picked) {
        setBootStatus(controller ? 'ready' : previousStatus)
        return
      }
      setDataDirStatus(picked)
      await connectMountedBackend()
    } catch (error: any) {
      if (!mountedRef.current) return
      setBootStatus('error')
      setBootError(String(error?.message || error || '选择数据目录失败'))
      await refreshMountedDataDirStatus()
    } finally {
      if (mountedRef.current) setDataDirBusy(false)
    }
  }

  async function saveEucliBoxConfig(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const runtime = runtimeRef.current
    if (!runtime || eucliBoxConfigBusy) return
    const eucliBoxUrl = eucliBoxUrlDraft.trim().replace(/\/+$/, '')
    const eucliBoxKey = eucliBoxKeyDraft.trim()
    if (!eucliBoxUrl) {
      showToast('请填写 eucli-box 地址', { kind: 'error' })
      return
    }
    try {
      setEucliBoxConfigBusy(true)
      const config = await runtime.setEucliBoxConfig({ eucliBoxUrl, eucliBoxKey })
      setEucliBoxConfig(config)
      setEucliBoxUrlDraft(config.eucliBoxUrl || eucliBoxUrl)
      setEucliBoxKeyDraft(config.eucliBoxKey || '')
      showToast('eucli-box 连接配置已保存', { kind: 'success' })
      await connectMountedBackend()
    } catch (error: any) {
      showToast(String(error?.message || error || '保存 eucli-box 连接配置失败'), { kind: 'error' })
    } finally {
      if (mountedRef.current) setEucliBoxConfigBusy(false)
    }
  }

  const refreshReleaseChecks = React.useCallback(async () => {
    const runtime = runtimeRef.current
    if (!runtime || releaseCheckBusyRef.current) return
    const runtimeVersion = runtimeVersionRef.current
    releaseCheckBusyRef.current = true
    setReleaseCheckBusy(true)
    setRuntimeBootstrap(current => current ? {
      ...current,
      releaseChecks: {
        ...current.releaseChecks,
        status: 'checking',
        startedAt: new Date().toISOString(),
        failureReason: '',
      },
    } : current)
    try {
      const snapshot = await runtime.refreshReleaseChecks()
      if (runtimeVersionRef.current !== runtimeVersion || !mountedRef.current) return
      setRuntimeBootstrap(current => current ? { ...current, releaseChecks: snapshot } : current)
    } catch (error: any) {
      if (runtimeVersionRef.current !== runtimeVersion || !mountedRef.current) return
      const message = String(error?.message || error || '检查正式版本失败')
      const failed: ReleaseCheckSnapshot = {
        status: 'failed',
        startedAt: '',
        checkedAt: new Date().toISOString(),
        results: [],
        failureReason: message,
      }
      setRuntimeBootstrap(current => current ? { ...current, releaseChecks: failed } : current)
      showToast(message, { kind: 'error' })
    } finally {
      if (runtimeVersionRef.current === runtimeVersion && mountedRef.current) {
        releaseCheckBusyRef.current = false
        setReleaseCheckBusy(false)
      }
    }
  }, [showToast])

  const runtimeBootstrapIssue = bootStatus === 'ready' && runtimeBootstrap && !runtimeBootstrap.businessAvailable
    ? runtimeBootstrap.eucliBoxIssue
    : ''
  const issue = bootError || dataDirStatus?.error || (dataDirStatus && !dataDirStatus.writable ? '数据目录不可写' : '')
  const needsEucliBoxConnection = bootStatus === 'ready' && !!runtimeBootstrap && !runtimeBootstrap.businessAvailable
  const canRenderChatApp = !!controller && bootStatus === 'ready' && runtimeBootstrap?.businessAvailable === true && !issue

  return (
    <div className="appShell">
      {canRenderChatApp ? (
        <div id={AI_STUDIO_CHAT_ROOT_ID} className="chatHost">
          <AiChatApp
            controller={controller}
            bootstrap={runtimeBootstrap}
            dataDirectory={{
              status: dataDirStatus,
              busy: dataDirBusy,
              onPick: pickDataDir,
              onRefresh: refreshDataDirStatus,
            }}
             windowControls={{
              standalone: launchInfo.standalone,
               actions: WINDOW_CONTROL_ACTIONS,
             }}
             releaseCheckBusy={releaseCheckBusy}
             onRefreshReleaseChecks={refreshReleaseChecks}
           />
        </div>
      ) : needsEucliBoxConnection ? (
        <EucliBoxConfigScreen
          standalone={launchInfo.standalone}
          windowControlActions={WINDOW_CONTROL_ACTIONS}
          urlDraft={eucliBoxUrlDraft}
          keyDraft={eucliBoxKeyDraft}
          busy={eucliBoxConfigBusy}
          issue={runtimeBootstrapIssue}
          bootstrap={runtimeBootstrap}
          releaseCheckBusy={releaseCheckBusy}
          onUrlChange={setEucliBoxUrlDraft}
          onKeyChange={setEucliBoxKeyDraft}
          onSubmit={saveEucliBoxConfig}
          onRefreshReleaseChecks={refreshReleaseChecks}
        />
      ) : (
        <BootFallback
          status={bootStatus}
          issue={issue || ''}
          pendingCommand={commandLabel(pendingCommand)}
          standalone={launchInfo.standalone}
          windowControlActions={WINDOW_CONTROL_ACTIONS}
          onPickDataDir={pickDataDir}
        />
      )}
      {toast ? <div className="toast" data-kind={toast.kind} role={toast.kind === 'error' ? 'alert' : 'status'} aria-live={toast.kind === 'error' ? 'assertive' : 'polite'}>{toast.text}</div> : null}
    </div>
  )
}

function normalizeLaunchInfo(raw: FwLaunchInfo): FwLaunchInfo {
  return {
    launched: !!raw?.launched,
    standalone: raw?.standalone !== false,
    mode: String(raw?.mode || (raw?.standalone === false ? 'default' : 'standalone')),
  }
}

function EucliBoxConfigScreen(props: {
  standalone: boolean
  windowControlActions: WindowControlActions
  urlDraft: string
  keyDraft: string
  busy: boolean
  issue: string
  bootstrap: StudioBootstrap
  releaseCheckBusy: boolean
  onUrlChange: (value: string) => void
  onKeyChange: (value: string) => void
  onSubmit: (event: React.FormEvent<HTMLFormElement>) => void
  onRefreshReleaseChecks: () => Promise<void> | void
}) {
  const { standalone, windowControlActions, urlDraft, keyDraft, busy, issue, bootstrap, releaseCheckBusy, onUrlChange, onKeyChange, onSubmit, onRefreshReleaseChecks } = props
  const onTopbarPointerDown = React.useCallback((event: React.PointerEvent<HTMLElement>) => {
    if (event.button !== 0) return
    const target = event.target
    if (!(target instanceof HTMLElement)) return
    if (target.closest('button, a, input, textarea, select, [role="button"], [data-window-controls="true"]')) return
    void TAURI_WINDOW?.startDragging?.().catch(() => {})
  }, [])

  return (
    <main className="bootFallback" role="main" aria-live="polite">
      <header className="bootFallbackTopbar" onPointerDown={onTopbarPointerDown}>
        <div className="bootFallbackBrand">AI Studio</div>
        {standalone ? <StandaloneWindowControls actions={windowControlActions} /> : null}
      </header>
      <section className="bootFallbackCard eucliConfigCard">
        <div className="bootFallbackTitle">连接 eucli-box</div>
        <dl className="releaseFacts">
          <div><dt>客户端版本</dt><dd>{bootstrap.clientVersion || '版本资料无效'}</dd></div>
          <div><dt>所需本体范围</dt><dd>{compatibilityRangeText(bootstrap.clientEucliBoxCompatibility)}</dd></div>
          <div><dt>本体版本</dt><dd>{bootstrap.eucliBoxVersion || (bootstrap.eucliBoxReachable ? '版本资料无效' : '尚未读取')}</dd></div>
          <div><dt>连接状态</dt><dd data-compatible={bootstrap.businessAvailable ? 'true' : 'false'}>{bootstrap.businessAvailable ? '适用' : bootstrap.eucliBoxReachable ? '不适用' : '未连接'}</dd></div>
        </dl>
        {issue ? <div className="bootFallbackIssue">{issue}</div> : null}
        <div className="eucliReleaseChecks">
          <ReleaseChecksPanel snapshot={bootstrap.releaseChecks} busy={releaseCheckBusy} onRefresh={onRefreshReleaseChecks} compact />
        </div>
        <form className="eucliConfigForm" onSubmit={onSubmit}>
          <label className="eucliConfigLabel">
            <span>Gateway 地址</span>
            <input value={urlDraft} onChange={event => onUrlChange(event.target.value)} placeholder="http://127.0.0.1:8765" disabled={busy} />
          </label>
          <label className="eucliConfigLabel">
            <span>Key</span>
            <input value={keyDraft} onChange={event => onKeyChange(event.target.value)} placeholder="如未启用 key 可留空" disabled={busy} type="password" />
          </label>
          <button type="submit" disabled={busy}>{busy ? '保存中…' : '保存并重新连接'}</button>
        </form>
      </section>
    </main>
  )
}

function BootFallback(props: {
  status: BootStatus
  issue: string
  pendingCommand: string | null
  standalone: boolean
  windowControlActions: WindowControlActions
  onPickDataDir: () => void
}) {
  const { status, issue, pendingCommand, standalone, windowControlActions, onPickDataDir } = props
  const title = issue ? 'AI Studio 启动遇到问题' : 'AI Studio 正在启动'
  const onTopbarPointerDown = React.useCallback((event: React.PointerEvent<HTMLElement>) => {
    if (event.button !== 0) return
    const target = event.target
    if (!(target instanceof HTMLElement)) return
    if (target.closest('button, a, input, textarea, select, [role="button"], [data-window-controls="true"]')) return
    void TAURI_WINDOW?.startDragging?.().catch(() => {})
  }, [])

  return (
    <main className="bootFallback" role={issue ? 'alert' : 'status'} aria-live="polite">
      <header className="bootFallbackTopbar" onPointerDown={onTopbarPointerDown}>
        <div className="bootFallbackBrand">AI Studio</div>
        {standalone ? <StandaloneWindowControls actions={windowControlActions} /> : null}
      </header>
      <section className="bootFallbackCard">
        <div className="bootFallbackTitle">{title}</div>
        <div className="bootFallbackText">{status === 'booting' ? '正在连接本机后台，请稍等。' : '请处理下面的问题后重试。'}</div>
      {pendingCommand ? (
          <div className="bootFallbackText">待处理命令：{pendingCommand}</div>
      ) : null}
      {issue ? (
          <>
            <div className="bootFallbackIssue">{issue}</div>
            <button type="button" onClick={onPickDataDir}>选择可写数据目录</button>
          </>
      ) : null}
      </section>
    </main>
  )
}
