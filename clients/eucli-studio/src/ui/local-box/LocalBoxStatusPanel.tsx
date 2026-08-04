import * as React from 'react'
import { localBoxSourceLabel, type LocalBoxState } from '../../domain/localBox'

export function LocalBoxStatusPanel(props: { state: LocalBoxState }) {
  const { state } = props
  return (
    <div className="localBoxStatusPanel" data-status={state.status} data-source={state.source} role={state.status === 'failed' ? 'alert' : 'status'} aria-live="polite">
      <div className="localBoxStatusLine">{localBoxStatusDescription(state)}</div>
      {state.progress.phase ? <div className="localBoxStatusMeta">{localBoxProgressText(state)}</div> : null}
      {state.error.message ? <div className="bootFallbackIssue">{state.error.message}</div> : null}
    </div>
  )
}

export function localBoxStatusLabel(status: string) {
  switch (status) {
    case 'not_installed': return '未安装'
    case 'checking_release': return '读取成品'
    case 'ready_to_install': return '可安装'
    case 'downloading': return '下载中'
    case 'verifying': return '核对中'
    case 'installing': return '安装中'
    case 'starting': return '启动中'
    case 'connected': return '已连接'
    case 'failed': return '失败'
    case 'stopping': return '停止中'
    case 'stopped': return '已停止'
    default: return status || '未知'
  }
}

function localBoxStatusDescription(state: LocalBoxState) {
  const sourceLabel = localBoxSourceLabel(state.source)
  if (state.connected) return `${sourceLabel}已由客户端受托启动并连接。`
  if (state.status === 'ready_to_install') return `${sourceLabel} ${state.latestVersion || ''} 已就绪，可一键安装。`
  if (state.status === 'not_installed') return '当前客户端尚未安装本机业务端。'
  if (state.status === 'failed') {
    if (state.error.code === 'LOCAL_BOX_SOURCE_MISMATCH') return '已安装业务端与当前成品来源不匹配，需要重新安装。'
    return '本机业务端流程失败，请按失败原因处理后重试。'
  }
  return localBoxStatusLabel(state.status)
}

function localBoxProgressText(state: LocalBoxState) {
  const total = state.progress.totalBytes > 0 ? ` / ${formatBytes(state.progress.totalBytes)}` : ''
  const received = state.progress.receivedBytes > 0 ? formatBytes(state.progress.receivedBytes) : ''
  return `${localBoxStatusLabel(state.progress.phase)}${received || total ? `：${received}${total}` : ''}`
}

function formatBytes(value: number) {
  const bytes = Number(value)
  if (!Number.isFinite(bytes) || bytes <= 0) return '未知'
  if (bytes < 1024) return `${Math.round(bytes)} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}
