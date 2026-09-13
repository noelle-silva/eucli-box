import * as React from 'react'
import type { AiChatAppRuntime } from './aiChatAppHost'
import {
  composeReleaseCandidatesView,
  emptyReleaseCache,
  isReleaseCacheFresh,
  RELEASE_ARTIFACT_KINDS,
  writeReleaseCache,
  type ArtifactCandidateList,
  type ArtifactInstallation,
  type ReleaseArtifactKind,
  type ReleaseCache,
  type ReleaseSourceKind,
} from '../domain/release'

// useReleaseStore 是客户端发行数据的唯一编排与缓存中心：按「来源 × 分类」缓存，
// 刷新失败保留旧缓存并记录失败原因；业务端只提供按需读取的事实。
export type ReleaseStoreState = {
  cache: ReleaseCache
  source: ReleaseSourceKind
  installations: ArtifactInstallation[]
  busy: boolean
}

export function useReleaseStore(getRuntime: () => AiChatAppRuntime | null, showToast: (message: unknown, options?: { kind?: 'success' | 'error' | 'info' }) => void) {
  const [store, setStore] = React.useState<ReleaseStoreState>(() => ({
    cache: emptyReleaseCache(),
    source: 'official',
    installations: [],
    busy: false,
  }))
  const storeRef = React.useRef(store)
  storeRef.current = store
  const runtimeRef = React.useRef(getRuntime)
  runtimeRef.current = getRuntime
  const toastRef = React.useRef(showToast)
  toastRef.current = showToast

  const refreshKinds = React.useCallback(async (kinds: ReleaseArtifactKind[], force: boolean) => {
    const runtime = runtimeRef.current()
    if (!runtime) return
    setStore((current) => ({ ...current, busy: true }))
    try {
      const resolvedSource = (await resolveInstallSource(runtime)) || storeRef.current.source
      const fresh = kinds.filter((item) => force || !isReleaseCacheFresh(storeRef.current.cache, resolvedSource, item))
      if (!fresh.length) {
        setStore((current) => ({ ...current, source: resolvedSource, busy: false }))
        return
      }
      const installations = await runtime.listArtifactInstallations().catch(() => null)
      const results = await Promise.all(fresh.map((item) => loadReleaseCandidates(runtime, item)))
      setStore((current) => {
        let cache: ReleaseCache = current.cache
        for (const result of results) {
          cache = writeReleaseCache(cache, resolvedSource, result.kind, {
            candidates: result.candidates,
            failure: result.failure,
          })
        }
        return {
          cache,
          source: resolvedSource,
          installations: installations ? installations.artifacts : current.installations,
          busy: false,
        }
      })
    } catch (error: any) {
      setStore((current) => ({ ...current, busy: false }))
      toastRef.current(String(error?.message || error || '读取发行候选失败'), { kind: 'error' })
    }
  }, [])

  // syncSource 在打开商店或切换来源后读取当前来源，让缓存键与真实来源保持一致。
  const syncSource = React.useCallback(async (): Promise<ReleaseSourceKind | null> => {
    const runtime = runtimeRef.current()
    if (!runtime) return null
    const source = await resolveInstallSource(runtime)
    if (source) setStore((current) => (current.source === source ? current : { ...current, source }))
    return source
  }, [])

  // read 打开商店或按分类读取：同步当前来源，缓存新鲜时复用，不发起读取。
  const read = React.useCallback(async (kind?: string) => {
    await syncSource()
    await refreshKinds([(kind || 'tool') as ReleaseArtifactKind], false)
  }, [refreshKinds, syncSource])

  // refresh 手动刷新入口：无论缓存是否新鲜都重新读取。
  const refresh = React.useCallback(async (kind?: string) => {
    await syncSource()
    await refreshKinds(kind ? [kind as ReleaseArtifactKind] : RELEASE_ARTIFACT_KINDS, true)
  }, [refreshKinds, syncSource])

  const view = React.useMemo(() => composeReleaseCandidatesView(store.cache, store.source, {
    kinds: RELEASE_ARTIFACT_KINDS,
    checking: store.busy,
    installations: store.installations,
  }), [store])

  return { view, busy: store.busy, read, refresh }
}

async function resolveInstallSource(runtime: AiChatAppRuntime): Promise<ReleaseSourceKind | null> {
  try {
    const resolved = await runtime.controller?.actions?.getInstallSource?.()
    if (resolved === 'official' || resolved === 'local') return resolved
  } catch {
    // 来源读取失败时保留当前缓存键，不猜测。
  }
  return null
}

async function loadReleaseCandidates(runtime: AiChatAppRuntime, kind: ReleaseArtifactKind): Promise<{ kind: ReleaseArtifactKind; candidates: ArtifactCandidateList['candidates']; failure: string }> {
  try {
    const list = await runtime.listReleaseCandidates(kind)
    return { kind, candidates: list.candidates, failure: '' }
  } catch (error: any) {
    return { kind, candidates: [], failure: String(error?.message || error || `读取 ${kind} 发行候选失败`) }
  }
}
