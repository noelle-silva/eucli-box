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

export type StudioBootstrap = {
  clientVersion: string
  clientEucliBoxCompatibility: EucliBoxCompatibility
  eucliBoxConfigured: boolean
  eucliBoxReachable: boolean
  eucliBoxUrl: string
  eucliBoxVersion: string
  eucliBoxCompatibility: CompatibilityStatus | null
  businessAvailable: boolean
  eucliBoxIssue: string
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
    eucliBoxConfigured: source.eucliBoxConfigured === true,
    eucliBoxReachable: source.eucliBoxReachable === true,
    eucliBoxUrl: text(source.eucliBoxUrl),
    eucliBoxVersion: text(source.eucliBoxVersion),
    eucliBoxCompatibility: source.eucliBoxCompatibility && typeof source.eucliBoxCompatibility === 'object'
      ? normalizeCompatibilityStatus(source.eucliBoxCompatibility)
      : null,
    businessAvailable: source.businessAvailable === true,
    eucliBoxIssue: text(source.eucliBoxIssue),
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
