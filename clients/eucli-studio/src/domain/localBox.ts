export type LocalBoxStatus =
  | 'not_installed'
  | 'checking_release'
  | 'ready_to_install'
  | 'downloading'
  | 'verifying'
  | 'installing'
  | 'starting'
  | 'connected'
  | 'failed'
  | 'stopping'
  | 'stopped'
  | 'waiting_stop'
  | 'switching'
  | 'restoring'

export type LocalBoxSource = 'official' | 'development'

export type LocalBoxState = {
  status: LocalBoxStatus | string
  artifact: { kind: 'eucli-box'; id: 'eucli-box' }
  source: LocalBoxSource | string
  installed: boolean
  currentVersion: string
  latestVersion: string
  targetVersion: string
  releaseNotes: string
  downloadSize: number
  progress: { phase: string; receivedBytes: number; totalBytes: number }
  error: { code: string; message: string; phase: string }
  connected: boolean
}

export function normalizeLocalBoxState(value: unknown): LocalBoxState {
  const source = objectValue(value)
  const progress = objectValue(source.progress)
  const error = objectValue(source.error)
  return {
    status: text(source.status) || 'not_installed',
    artifact: { kind: 'eucli-box', id: 'eucli-box' },
    source: source.source === 'development' ? 'development' : 'official',
    installed: source.installed === true,
    currentVersion: text(source.currentVersion),
    latestVersion: text(source.latestVersion),
    targetVersion: text(source.targetVersion),
    releaseNotes: text(source.releaseNotes),
    downloadSize: finiteNumber(source.downloadSize),
    progress: {
      phase: text(progress.phase),
      receivedBytes: finiteNumber(progress.receivedBytes),
      totalBytes: finiteNumber(progress.totalBytes),
    },
    error: {
      code: text(error.code),
      message: text(error.message),
      phase: text(error.phase),
    },
    connected: source.connected === true,
  }
}

export function localBoxSourceLabel(source: LocalBoxSource | string): string {
  return source === 'development' ? '当前源码业务端' : '正式业务端'
}

function objectValue(value: unknown): Record<string, any> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, any> : {}
}

function text(value: unknown): string {
  return String(value ?? '').trim()
}

function finiteNumber(value: unknown): number {
  const number = Number(value)
  return Number.isFinite(number) && number >= 0 ? number : 0
}
