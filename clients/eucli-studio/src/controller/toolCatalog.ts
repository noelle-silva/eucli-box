import { now } from '../core/utils'

export function createToolCatalog(deps: {
  getState: () => any
  netRequest: (req: any) => Promise<any>
  emit: () => void
  showToast?: (msg: string) => void
}) {
  async function refreshTools(force = false) {
    const state = deps.getState()
    const catalog = state.tools && typeof state.tools === 'object' ? state.tools : { loading: false, error: '', items: [], fetchedAt: 0 }
    const age = now() - Number(catalog.fetchedAt || 0)
    if (!force && Array.isArray(catalog.items) && catalog.items.length && age < 60 * 1000) return catalog.items

    state.tools = { loading: true, error: '', items: Array.isArray(catalog.items) ? catalog.items : [], fetchedAt: Number(catalog.fetchedAt || 0) }
    deps.emit()

    try {
      const response = await deps.netRequest({ method: 'GET', path: '/api/tools', timeoutMs: 15000 })
      const status = Number(response?.status || 0)
      if (status < 200 || status >= 300) throw new Error(`HTTP ${status}`)
      const items = normalizeToolSummaries(response?.body)
      state.tools = { loading: false, error: '', items, fetchedAt: now() }
      return items
    } catch (e: any) {
      const error = String(e?.message || e || '工具列表加载失败')
      state.tools = { loading: false, error, items: Array.isArray(catalog.items) ? catalog.items : [], fetchedAt: Number(catalog.fetchedAt || 0) }
      deps.showToast?.(error)
      return state.tools.items
    } finally {
      deps.emit()
    }
  }

  return { refreshTools }
}

function normalizeToolSummaries(value: any): any[] {
  const list = Array.isArray(value) ? value : Array.isArray(value?.data) ? value.data : []
  const seen = new Set<string>()
  const out: any[] = []
  for (const item of list) {
    if (!item || typeof item !== 'object') continue
    const name = String((item as any).name || (item as any).id || '').trim()
    const id = String((item as any).id || name).trim()
    if (!name || seen.has(name)) continue
    seen.add(name)
    out.push({
      id,
      name,
      description: String((item as any).description || '').trim(),
      type: String((item as any).type || '').trim(),
      updatedAt: (item as any).updatedAt,
    })
  }
  return out.sort((a, b) => String(a.name || '').localeCompare(String(b.name || '')))
}
