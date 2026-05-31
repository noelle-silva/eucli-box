export function createModelRefresh(deps: {
  getState: () => any
  getProvider: (pid: string) => any
  syncBoxCatalog?: () => Promise<{ roles?: any[]; providers?: any[] } | void>
  emit: () => void
  showToast?: (msg: string) => void
}) {
  async function refreshModels(providerId: string, force: boolean) {
    const s = deps.getState()
    const cachedProvider = deps.getProvider(providerId)
    const cachedItems = Array.isArray(cachedProvider?.modelsCache?.items) ? cachedProvider.modelsCache.items : []
    if (!force) {
      s.models = { loading: false, error: '', items: cachedItems.slice(0, 300) }
      deps.emit()
      return
    }

    s.models = { loading: true, error: '', items: [] }
    deps.emit()

    try {
      if (!deps.syncBoxCatalog) throw new Error('eucli-box 目录同步通道不可用')
      await deps.syncBoxCatalog()
      const p = deps.getProvider(providerId)
      const items = Array.isArray(p?.modelsCache?.items) ? p.modelsCache.items : []
      s.models = { loading: false, error: '', items: items.slice(0, 300) }
      deps.showToast?.('模型目录已从 eucli-box 同步')
    } catch (e: any) {
      s.models = { loading: false, error: String(e?.message || e || '同步模型目录失败'), items: cachedItems.slice(0, 300) }
      deps.showToast?.(s.models.error || '同步模型目录失败')
    } finally {
      deps.emit()
    }
  }

  function resolveAiModelId(modelPick: any, customModelId: any) {
    const pick = String(modelPick || '').trim()
    if (!pick) return ''
    if (pick === '__custom__') return String(customModelId || '').trim()
    return pick
  }

  return {
    refreshModels,
    resolveAiModelId,
  }
}
