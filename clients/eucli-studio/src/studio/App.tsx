import * as React from 'react'
import { invoke } from '@tauri-apps/api/core'
import { listen } from '@tauri-apps/api/event'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { AiChatApp } from '../ui/App'
import { StandaloneWindowControls, type WindowControlActions } from '../ui/components/StandaloneWindowControls'
import type { AiChatController } from '../controller/types'
import type { AiChatToastKind, AiChatToastOptions } from '../gateway/capabilities'
import { AI_STUDIO_CHAT_ROOT_ID } from '../runtime/aiStudioGlobals'
import { createAiChatAppRuntime, type AiChatAppRuntime } from './aiChatAppHost'
import { compatibilityRangeText, type ReleaseCheckSnapshot, type StudioBootstrap } from '../domain/release'
import { ReleaseChecksPanel } from '../ui/release/ReleaseChecksPanel'
import { LocalBoxInstallPanel } from '../ui/local-box/LocalBoxInstallPanel'
import { LocalBoxStatusPanel, localBoxStatusLabel } from '../ui/local-box/LocalBoxStatusPanel'
import { localBoxSourceLabel } from '../domain/localBox'

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

const BASE_WINDOW_CONTROL_ACTIONS: WindowControlActions = {
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
  const [runtimeBootstrap, setRuntimeBootstrap] = React.useState<StudioBootstrap | null>(null)
  const [releaseCheckBusy, setReleaseCheckBusy] = React.useState(false)
  const [localBoxBusy, setLocalBoxBusy] = React.useState(false)
  const runtimeRef = React.useRef<AiChatAppRuntime | null>(null)
  const runtimeVersionRef = React.useRef(0)
  const mountedRef = React.useRef(false)
  const toastSeqRef = React.useRef(0)
  const releaseCheckBusyRef = React.useRef(false)
  const localBoxBusyRef = React.useRef(false)

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
      localBoxBusyRef.current = false
      setLocalBoxBusy(false)
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
    runtime.subscribeLocalBoxState(state => {
      if (runtimeVersionRef.current !== runtimeVersion || !mountedRef.current) return
      setRuntimeBootstrap(current => current ? { ...current, localBox: state } : current)
    })
    setController(runtime.controller)
    setRuntimeBootstrap(runtime.bootstrap)
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

  const installLocalBox = React.useCallback(async () => {
    const runtime = runtimeRef.current
    if (!runtime || localBoxBusyRef.current) return
    const runtimeVersion = runtimeVersionRef.current
    localBoxBusyRef.current = true
    setLocalBoxBusy(true)
    try {
      const currentState = runtimeRef.current ? await runtime.getLocalBoxStatus() : null
      const state = currentState?.installed
        ? currentState
        : await runtime.installLocalBox()
      if (runtimeVersionRef.current !== runtimeVersion || !mountedRef.current) return
      setRuntimeBootstrap(current => current ? { ...current, localBox: state } : current)
      if (state.connected) await connectMountedBackend()
    } catch (error: any) {
      if (runtimeVersionRef.current !== runtimeVersion || !mountedRef.current) return
      showToast(String(error?.message || error || '安装业务端失败'), { kind: 'error' })
    } finally {
      if (runtimeVersionRef.current === runtimeVersion && mountedRef.current) {
        localBoxBusyRef.current = false
        setLocalBoxBusy(false)
      }
    }
  }, [connectMountedBackend, showToast])

  const trueExit = React.useCallback(async () => {
    const runtime = runtimeRef.current
    if (!runtime || localBoxBusyRef.current) return
    try {
      localBoxBusyRef.current = true
      setLocalBoxBusy(true)
      const state = await runtime.exitLocalBox()
      setRuntimeBootstrap(current => current ? { ...current, localBox: state } : current)
      if (state.status !== 'stopped') {
        const message = state.error.message || '业务端没有确认停止，窗口保持打开。'
        showToast(message, { kind: 'error' })
        return
      }
      await invoke('exit_app')
    } catch (error: any) {
      showToast(String(error?.message || error || '真正退出失败'), { kind: 'error' })
    } finally {
      if (mountedRef.current) {
        localBoxBusyRef.current = false
        setLocalBoxBusy(false)
      }
    }
  }, [showToast])

  const windowControlActions = React.useMemo<WindowControlActions>(() => ({
    ...BASE_WINDOW_CONTROL_ACTIONS,
    trueExit,
  }), [trueExit])

  const runtimeBootstrapIssue = bootStatus === 'ready' && runtimeBootstrap && !runtimeBootstrap.businessAvailable
    ? runtimeBootstrap.eucliBoxIssue
    : ''
  const issue = bootError || dataDirStatus?.error || (dataDirStatus && !dataDirStatus.writable ? '数据目录不可写' : '')
  const needsEucliBoxConnection = bootStatus === 'ready' && !!runtimeBootstrap && !runtimeBootstrap.businessAvailable
  const canRenderChatApp = !!controller && bootStatus === 'ready' && runtimeBootstrap?.businessAvailable === true && runtimeBootstrap.localBox.connected && !issue

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
                actions: windowControlActions,
             }}
             releaseCheckBusy={releaseCheckBusy}
             onRefreshReleaseChecks={refreshReleaseChecks}
           />
        </div>
      ) : needsEucliBoxConnection ? (
        <LocalBoxGateScreen
          standalone={launchInfo.standalone}
          windowControlActions={windowControlActions}
          busy={localBoxBusy}
          issue={runtimeBootstrapIssue}
          bootstrap={runtimeBootstrap}
          releaseCheckBusy={releaseCheckBusy}
          onInstall={installLocalBox}
          onRefreshReleaseChecks={refreshReleaseChecks}
        />
      ) : (
        <BootFallback
          status={bootStatus}
          issue={issue || ''}
          pendingCommand={commandLabel(pendingCommand)}
          standalone={launchInfo.standalone}
          windowControlActions={windowControlActions}
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

function LocalBoxGateScreen(props: {
  standalone: boolean
  windowControlActions: WindowControlActions
  busy: boolean
  issue: string
  bootstrap: StudioBootstrap
  releaseCheckBusy: boolean
  onInstall: () => Promise<void> | void
  onRefreshReleaseChecks: () => Promise<void> | void
}) {
  const { standalone, windowControlActions, busy, issue, bootstrap, releaseCheckBusy, onInstall, onRefreshReleaseChecks } = props
  const localBox = bootstrap.localBox
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
        <div className="bootFallbackTitle">安装并连接业务端</div>
        <dl className="releaseFacts">
          <div><dt>客户端版本</dt><dd>{bootstrap.clientVersion || '版本资料无效'}</dd></div>
          <div><dt>所需本体范围</dt><dd>{compatibilityRangeText(bootstrap.clientEucliBoxCompatibility)}</dd></div>
          <div><dt>业务端状态</dt><dd data-compatible={localBox.connected ? 'true' : 'false'}>{localBoxStatusLabel(localBox.status)}</dd></div>
          <div><dt>业务端来源</dt><dd data-source={localBox.source}>{localBoxSourceLabel(localBox.source)}</dd></div>
          <div><dt>业务端版本</dt><dd>{localBox.currentVersion || localBox.latestVersion || '尚未安装'}</dd></div>
          <div><dt>下载大小</dt><dd>{localBox.downloadSize > 0 ? formatLocalBoxBytes(localBox.downloadSize) : '未知'}</dd></div>
        </dl>
        <LocalBoxStatusPanel state={localBox} />
        <LocalBoxInstallPanel state={localBox} busy={busy} onInstall={onInstall} />
        {issue ? <div className="bootFallbackIssue">{issue}</div> : null}
        <div className="eucliReleaseChecks">
          <ReleaseChecksPanel snapshot={bootstrap.releaseChecks} busy={releaseCheckBusy} onRefresh={onRefreshReleaseChecks} compact />
        </div>
      </section>
    </main>
  )
}

function formatLocalBoxBytes(value: number) {
  if (value < 1024) return `${Math.round(value)} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  if (value < 1024 * 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(1)} MB`
  return `${(value / (1024 * 1024 * 1024)).toFixed(2)} GB`
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
