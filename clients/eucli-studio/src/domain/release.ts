import { normalizeLocalBoxState, type LocalBoxState } from './localBox'

export type EucliBoxCompatibility = {
  minimumVersion: string
  maximumVersionExclusive: string
}

export type CompatibilityStatus = {
  compatible: boolean
  reason: string
  currentEucliBoxVersion: string
  requiredEucliBoxCompatibility: EucliBoxCompatibility
}

export type ReleaseArtifactIdentity = {
  kind: 'eucli-box' | 'tool' | 'plugin' | string
  id: string
}

export type OfficialReleaseSource = {
  kind: string
  repository: string
  owner: string
  name: string
}

export type ReleaseCheckResult = {
  artifact: ReleaseArtifactIdentity
  source: OfficialReleaseSource
  installed: boolean
  currentVersion: string
  latestVersion: string
  status: 'not_checked' | 'checking' | 'completed' | 'failed' | string
  checkedAt: string
  updateAvailable: boolean
  releaseUrl: string
  releaseNotes: string
  downloadSize: number
  compatibility: CompatibilityStatus | null
  affectedArtifacts: ReleaseArtifactIdentity[]
  failureReason: string
}

export type ReleaseCheckSnapshot = {
  status: 'not_checked' | 'checking' | 'completed' | 'failed' | string
  source?: string
  startedAt: string
  checkedAt: string
  results: ReleaseCheckResult[]
  failureReason: string
}

export type ReleaseOperationProgress = {
  receivedBytes: number
  totalBytes: number
}

export type ReleaseOperationError = {
  code: string
  phase: string
  message: string
}

export type ArtifactInstallState = {
  operationId: string
  artifact: ReleaseArtifactIdentity
  installed: boolean
  currentVersion: string
  targetVersion: string
  status: string
  phase: string
  progress: ReleaseOperationProgress
  error: ReleaseOperationError
}

export type ArtifactActivityState = {
  artifact: ReleaseArtifactIdentity
  active: boolean
  activeRequests: number
  updating: boolean
  reason: string
}

export const artifactStatusLabels: Record<string, string> = {
  not_installed: '未安装',
  checking_release: '检查发行中',
  ready_to_install: '可安装',
  ready_to_update: '可更新',
  downloading: '下载中',
  verifying: '核对中',
  checking_activity: '检查活动中',
  preparing: '准备中',
  switching: '切换中',
  starting: '启动中',
  active: '已启用',
  unavailable: '不适用',
  failed: '失败',
  blocked: '被阻止',
  restoring: '恢复中',
}

export function normalizeArtifactInstallState(value: unknown): ArtifactInstallState {
  const source = objectValue(value)
  return {
    operationId: text(source.operationId),
    artifact: normalizeReleaseArtifactIdentity(source.artifact),
    installed: source.installed === true,
    currentVersion: text(source.currentVersion),
    targetVersion: text(source.targetVersion),
    status: text(source.status),
    phase: text(source.phase),
    progress: {
      receivedBytes: finiteNumber((source as any).progress?.receivedBytes),
      totalBytes: finiteNumber((source as any).progress?.totalBytes),
    },
    error: {
      code: text((source as any).error?.code),
      phase: text((source as any).error?.phase),
      message: text((source as any).error?.message),
    },
  }
}

export function normalizeArtifactActivityState(value: unknown): ArtifactActivityState {
  const source = objectValue(value)
  return {
    artifact: normalizeReleaseArtifactIdentity(source.artifact),
    active: source.active === true,
    activeRequests: finiteNumber(source.activeRequests),
    updating: source.updating === true,
    reason: text(source.reason),
  }
}

export type StudioBootstrap = {
  clientVersion: string
  clientEucliBoxCompatibility: EucliBoxCompatibility
  localBox: LocalBoxState
  eucliBoxConfigured: boolean
  eucliBoxReachable: boolean
  eucliBoxUrl: string
  eucliBoxVersion: string
  eucliBoxCompatibility: CompatibilityStatus | null
  businessAvailable: boolean
  eucliBoxIssue: string
  releaseChecks: ReleaseCheckSnapshot
}

export function normalizeEucliBoxCompatibility(value: unknown): EucliBoxCompatibility {
  const source = objectValue(value)
  return {
    minimumVersion: text(source.minimumVersion),
    maximumVersionExclusive: text(source.maximumVersionExclusive),
  }
}

export function normalizeCompatibilityStatus(value: unknown): CompatibilityStatus {
  const source = objectValue(value)
  return {
    compatible: source.compatible === true,
    reason: text(source.reason),
    currentEucliBoxVersion: text(source.currentEucliBoxVersion),
    requiredEucliBoxCompatibility: normalizeEucliBoxCompatibility(source.requiredEucliBoxCompatibility),
  }
}

export function normalizeStudioBootstrap(value: unknown): StudioBootstrap {
  const source = objectValue(value)
  return {
    clientVersion: text(source.clientVersion),
    clientEucliBoxCompatibility: normalizeEucliBoxCompatibility(source.clientEucliBoxCompatibility),
    localBox: normalizeLocalBoxState(source.localBox),
    eucliBoxConfigured: source.eucliBoxConfigured === true,
    eucliBoxReachable: source.eucliBoxReachable === true,
    eucliBoxUrl: text(source.eucliBoxUrl),
    eucliBoxVersion: text(source.eucliBoxVersion),
    eucliBoxCompatibility: source.eucliBoxCompatibility && typeof source.eucliBoxCompatibility === 'object'
      ? normalizeCompatibilityStatus(source.eucliBoxCompatibility)
      : null,
    businessAvailable: source.businessAvailable === true,
    eucliBoxIssue: text(source.eucliBoxIssue),
    releaseChecks: normalizeReleaseCheckSnapshot(source.releaseChecks),
  }
}

export function normalizeReleaseCheckSnapshot(value: unknown): ReleaseCheckSnapshot {
  const source = objectValue(value)
  return {
    status: text(source.status) || 'not_checked',
    source: text(source.source),
    startedAt: text(source.startedAt),
    checkedAt: text(source.checkedAt),
    results: Array.isArray(source.results) ? source.results.map(normalizeReleaseCheckResult) : [],
    failureReason: text(source.failureReason),
  }
}

function normalizeReleaseCheckResult(value: unknown): ReleaseCheckResult {
  const source = objectValue(value)
  return {
    artifact: normalizeReleaseArtifactIdentity(source.artifact),
    source: normalizeOfficialReleaseSource(source.source),
    installed: source.installed === true,
    currentVersion: text(source.currentVersion),
    latestVersion: text(source.latestVersion),
    status: text(source.status) || 'not_checked',
    checkedAt: text(source.checkedAt),
    updateAvailable: source.updateAvailable === true,
    releaseUrl: text(source.releaseUrl),
    releaseNotes: text(source.releaseNotes),
    downloadSize: finiteNumber(source.downloadSize),
    compatibility: source.compatibility && typeof source.compatibility === 'object'
      ? normalizeCompatibilityStatus(source.compatibility)
      : null,
    affectedArtifacts: Array.isArray(source.affectedArtifacts)
      ? source.affectedArtifacts.map(normalizeReleaseArtifactIdentity)
      : [],
    failureReason: text(source.failureReason),
  }
}

function normalizeReleaseArtifactIdentity(value: unknown): ReleaseArtifactIdentity {
  const source = objectValue(value)
  return { kind: text(source.kind), id: text(source.id) }
}

function normalizeOfficialReleaseSource(value: unknown): OfficialReleaseSource {
  const source = objectValue(value)
  return {
    kind: text(source.kind),
    repository: text(source.repository),
    owner: text(source.owner),
    name: text(source.name),
  }
}

export function compatibilityRangeText(value: EucliBoxCompatibility | null | undefined): string {
  const minimum = text(value?.minimumVersion)
  const maximum = text(value?.maximumVersionExclusive)
  if (!minimum || !maximum) return '范围资料无效'
  return `[${minimum}, ${maximum})`
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
