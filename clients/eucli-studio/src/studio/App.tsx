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

type RuntimeBootstrap = {
  eucliBoxConfigured?: boolean
  eucliBoxReachable?: boolean
  eucliBoxUrl?: string
  eucliBoxIssue?: string
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
  const [runtimeBootstrap, setRuntimeBootstrap] = React.useState<RuntimeBootstrap | null>(null)
  const runtimeRef = React.useRef<AiChatAppRuntime | null>(null)
  const runtimeVersionRef = React.useRef(0)
  const mountedRef = React.useRef(false)
  const toastSeqRef = React.useRef(0)

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
      const bootstrap = runtime.bootstrap && typeof runtime.bootstrap === 'object' ? runtime.bootstrap as RuntimeBootstrap : null
      setRuntimeBootstrap(bootstrap)
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

  const runtimeBootstrapIssue = bootStatus === 'ready' && runtimeBootstrap?.eucliBoxConfigured && runtimeBootstrap?.eucliBoxReachable === false
    ? String(runtimeBootstrap?.eucliBoxIssue || 'eucli-box 当前不可达')
    : ''
  const issue = bootError || dataDirStatus?.error || runtimeBootstrapIssue || (dataDirStatus && !dataDirStatus.writable ? '数据目录不可写' : '')
  const needsEucliBoxConfig = !!controller && bootStatus === 'ready' && !String(eucliBoxConfig?.eucliBoxUrl || '').trim()

  const canRenderChatApp = !!controller && bootStatus === 'ready' && !issue && !needsEucliBoxConfig

  return (
    <div className="appShell">
      {canRenderChatApp ? (
        <div id={AI_STUDIO_CHAT_ROOT_ID} className="chatHost">
          <AiChatApp
            controller={controller}
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
          />
        </div>
      ) : needsEucliBoxConfig ? (
        <EucliBoxConfigScreen
          standalone={launchInfo.standalone}
          windowControlActions={WINDOW_CONTROL_ACTIONS}
          urlDraft={eucliBoxUrlDraft}
          keyDraft={eucliBoxKeyDraft}
          busy={eucliBoxConfigBusy}
          issue={runtimeBootstrapIssue}
          onUrlChange={setEucliBoxUrlDraft}
          onKeyChange={setEucliBoxKeyDraft}
          onSubmit={saveEucliBoxConfig}
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
  onUrlChange: (value: string) => void
  onKeyChange: (value: string) => void
  onSubmit: (event: React.FormEvent<HTMLFormElement>) => void
}) {
  const { standalone, windowControlActions, urlDraft, keyDraft, busy, issue, onUrlChange, onKeyChange, onSubmit } = props
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
        <div className="bootFallbackText">请填写当前手动启动的 eucli-box gateway 地址和 key。客户端只保存连接配置，业务事实仍由 eucli-box 负责。</div>
        {issue ? <div className="bootFallbackIssue">{issue}</div> : null}
        <form className="eucliConfigForm" onSubmit={onSubmit}>
          <label className="eucliConfigLabel">
            <span>Gateway 地址</span>
            <input value={urlDraft} onChange={event => onUrlChange(event.target.value)} placeholder="http://127.0.0.1:8765" disabled={busy} />
          </label>
          <label className="eucliConfigLabel">
            <span>Key</span>
            <input value={keyDraft} onChange={event => onKeyChange(event.target.value)} placeholder="如未启用 key 可留空" disabled={busy} type="password" />
          </label>
          <button type="submit" disabled={busy}>{busy ? '保存中…' : '保存并连接'}</button>
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
