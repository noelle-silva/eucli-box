export function normalizeChatModelOverride(chat: unknown): { kind: string; providerId: string; groupId: string; modelId: string } | null {
  const c = chat && typeof chat === 'object' ? (chat as Record<string, unknown>) : null
  const o = c && c.modelOverride && typeof c.modelOverride === 'object' ? (c.modelOverride as Record<string, unknown>) : null
  const kind = String(o?.kind || '').trim() === 'model_group' || String(o?.groupId || '').trim() ? 'model_group' : 'provider'
  const providerId = String(o?.providerId || '').trim()
  const groupId = String(o?.groupId || '').trim()
  const modelId = String(o?.modelId || '').trim()
  if (!modelId) return null
  if (kind === 'model_group') {
    if (!groupId) return null
    return { kind, providerId: '', groupId, modelId }
  }
  if (!providerId) return null
  return { kind, providerId, groupId: '', modelId }
}

export function normalizeMessageModelRef(message: unknown): { kind: string; providerId: string; groupId: string; modelId: string } | null {
  const m = message && typeof message === 'object' ? (message as Record<string, unknown>) : null
  const r = m && (m as any).modelRef && typeof (m as any).modelRef === 'object' ? (m as any).modelRef as Record<string, unknown> : null
  const kind = String(r?.kind || '').trim() === 'model_group' || String(r?.groupId || '').trim() ? 'model_group' : 'provider'
  const providerId = String(r?.providerId || '').trim()
  const groupId = String(r?.groupId || '').trim()
  const modelId = String(r?.modelId || '').trim()
  if (!modelId) return null
  if (kind === 'model_group') {
    if (!groupId) return null
    return { kind, providerId: '', groupId, modelId }
  }
  if (!providerId) return null
  return { kind, providerId, groupId: '', modelId }
}

export function buildMessageModelRef(providerId: unknown, modelId: unknown): { kind: string; providerId: string; groupId: string; modelId: string } | null {
  const pid = String(providerId || '').trim()
  const mid = String(modelId || '').trim()
  if (!pid || !mid) return null
  return { kind: 'provider', providerId: pid, groupId: '', modelId: mid }
}

export function resolveProviderDisplayName(providers: unknown, providerId: unknown): string {
  const pid = String(providerId || '').trim()
  if (!pid) return ''
  const list = Array.isArray(providers) ? providers : []
  const provider = list.find((item: any) => String(item?.id || '').trim() === pid)
  return String((provider as any)?.name || '').trim()
}

export function resolveModelGroupDisplayName(modelGroups: unknown, groupId: unknown): string {
  const gid = String(groupId || '').trim()
  if (!gid) return ''
  const list = Array.isArray(modelGroups) ? modelGroups : []
  const group = list.find((item: any) => String(item?.id || '').trim() === gid)
  return String((group as any)?.name || '').trim()
}

export function formatModelRefDisplayText(modelRef: unknown, providers: unknown, modelGroups?: unknown): string {
  const ref = modelRef && typeof modelRef === 'object' ? (modelRef as Record<string, unknown>) : null
  const kind = String(ref?.kind || '').trim() === 'model_group' || String(ref?.groupId || '').trim() ? 'model_group' : 'provider'
  const modelId = String(ref?.modelId || '').trim()
  if (kind === 'model_group') {
    const groupText = resolveModelGroupDisplayName(modelGroups, ref?.groupId)
    if (!groupText && !modelId) return ''
    return groupText && modelId ? `模型组 / ${groupText} / ${modelId}` : groupText || modelId
  }
  const providerText = resolveProviderDisplayName(providers, ref?.providerId)
  if (!providerText && !modelId) return ''
  return providerText && modelId ? `${providerText} / ${modelId}` : providerText || modelId
}
