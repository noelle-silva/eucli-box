// AI Studio UI 轮询模块
// 由 V2 controller 组装使用
// 职责：定时轮询 stream 状态、同步 chat 索引、处理跨 tab 聊天更新通知

import { now } from '../core/utils'
import { chatMetaFromChat, chatMetasFromBox, upsertChatMeta } from '../domain/chatMeta'
import { UI_CHAT_UPDATED_NOTICE_KEY } from '../runtime/runtimeKeys'
import { splitChatKey, splitGroupChatKey, splitGroupChatIndexKey, splitRoleChatIndexKey } from '../domain/storageKeys'
import { normalizeStoredChat } from '../storage/normalizeStoredChat'
import { isStoredChatNewerThanCurrent, mergeChatFromStorage } from '../domain/chatStorageSync'
import { pendingChatForTarget } from '../domain/pendingChat'

export function createUiPolling(deps: {
  getState: () => any
  storage: { get: (key: string) => Promise<any>; set: (key: string, value: any) => Promise<void> }
  rtStorage: { get: (key: string) => Promise<any> }
  loadSplitMeta: () => Promise<any>
  getSplitMetaCache: () => any
  emit: () => void
  activeTargetKind: () => string
  activeChatFromData: () => any
  ensureActiveChatLoaded?: () => Promise<any>
  syncActiveGroupChatsFromStorage: (meta: any) => Promise<void>
}) {
  let uiPollTimer = 0
  let uiLastMetaCheckMs = 0
  let uiLastMetaUpdatedAt = 0
  let uiChatSyncing = false
  let uiLastChatUpdatedNoticeId = ''
  let uiPollingDisposed = false

  async function loadActiveTargetIndexMeta(activeKind: 'role' | 'group', activeTid: string) {
    if (!activeTid) return null
    const target = await loadTargetFolderMeta(activeKind, activeTid)
    const meta = target?.meta
    const folder = String(target?.folder || '').trim()
    if (!meta || !folder) return null

    const indexKey = activeKind === 'group' ? splitGroupChatIndexKey(folder) : splitRoleChatIndexKey(folder)
    const idx = await deps.storage.get(indexKey).catch(() => null)
    if (!idx || typeof idx !== 'object') return { meta, updatedAt: Number((meta as any)?.updatedAt || 0) }

    const updatedAt = Math.max(Number((meta as any)?.updatedAt || 0), Number((idx as any)?.updatedAt || 0))
    if (activeKind === 'group') {
      return {
        meta: { ...(meta as any), updatedAt, chatIndexByGroup: { ...((meta as any).chatIndexByGroup || {}), [activeTid]: idx } },
        updatedAt,
      }
    }
    return {
      meta: { ...(meta as any), updatedAt, chatIndexByRole: { ...((meta as any).chatIndexByRole || {}), [activeTid]: idx } },
      updatedAt,
    }
  }

  async function loadTargetFolderMeta(activeKind: 'role' | 'group', activeTid: string) {
    if (!activeTid) return null
    let meta = deps.getSplitMetaCache()
    let folder = activeKind === 'group' ? String((meta as any)?.groupFolders?.[activeTid] || '').trim() : String(meta?.roleFolders?.[activeTid] || '').trim()
    if (meta && folder) return { meta, folder }

    meta = await deps.loadSplitMeta()
    folder = activeKind === 'group' ? String((meta as any)?.groupFolders?.[activeTid] || '').trim() : String(meta?.roleFolders?.[activeTid] || '').trim()
    return meta && folder ? { meta, folder } : null
  }

  async function syncActiveRoleChatsFromStorage(metaOverride?: any) {
    const state = deps.getState()
    if (!state.data) return
    if (uiChatSyncing) return
    uiChatSyncing = true
    try {
      const rid = String(state.draft.activeRoleId || state.data?.ui?.activeRoleId || '')
      if (!rid) return

      const activeMeta = metaOverride ? null : await loadActiveTargetIndexMeta('role', rid)
      const meta = metaOverride || activeMeta?.meta || deps.getSplitMetaCache() || (await deps.loadSplitMeta())
      if (!meta || typeof meta !== 'object') return

      const updatedAt = Number((meta as any).updatedAt || 0)
      if (updatedAt) uiLastMetaUpdatedAt = Math.max(uiLastMetaUpdatedAt, updatedAt)

      const folder = String((meta as any).roleFolders?.[rid] || '')
      const idx = (meta as any).chatIndexByRole?.[rid]
      if (!folder || !idx || typeof idx !== 'object') return

      const chatMetas = chatMetasFromBox(idx, '新聊天')
      const desiredChatIds = chatMetas.map((x: any) => String(x?.id || '')).filter((x: any) => !!x)
      const desiredActiveChatId = String((idx as any).activeChatId || '')
      const wantUpdatedAt = (idx as any).chatUpdatedAt && typeof (idx as any).chatUpdatedAt === 'object' ? (idx as any).chatUpdatedAt : {}

      if (!state.data.chatsByRole || typeof state.data.chatsByRole !== 'object') state.data.chatsByRole = {}
      if (!state.data.chatsByRole[rid] || typeof state.data.chatsByRole[rid] !== 'object') state.data.chatsByRole[rid] = { activeChatId: '', chats: [] }
      const box = state.data.chatsByRole[rid]
      if (!Array.isArray(box.chats)) box.chats = []

      const keepChatNow = String(box.activeChatId || '')
      const activeChatId = keepChatNow || desiredActiveChatId || String(desiredChatIds[0] || '')

      const curChats = box.chats
      const curById = new Map<string, any>()
      for (const c of curChats) {
        const cid = String(c?.id || '')
        if (cid) curById.set(cid, c)
      }

      const nextChats: any[] = []
      for (const cid of desiredChatIds) {
        const cur = curById.get(cid) || null
        if (!cur) {
          continue
        }

        const metaUpdatedAt = Number((wantUpdatedAt as any)?.[cid] || 0)
        if (metaUpdatedAt && cid !== activeChatId && isStoredChatNewerThanCurrent(metaUpdatedAt, cur.updatedAt)) cur.updatedAt = metaUpdatedAt
        nextChats.push(cur)
      }

      if (activeChatId) {
        const metaUpdatedAt = Number((wantUpdatedAt as any)?.[activeChatId] || 0)
        const cur = curById.get(activeChatId) || null
        const curUpdatedAt = Number(cur?.updatedAt || 0)
        if (isStoredChatNewerThanCurrent(metaUpdatedAt, curUpdatedAt)) {
          const c0 = await deps.storage.get(splitChatKey(folder, activeChatId))
          const c1 = c0 && typeof c0 === 'object' ? normalizeStoredChat(c0, 'role') : null
          if (c1) {
            const idx0 = nextChats.findIndex((c: any) => String(c?.id || '') === activeChatId)
            if (idx0 >= 0) nextChats[idx0] = mergeChatFromStorage(c1, nextChats[idx0])
            else nextChats.unshift(c1)
          }
        }
      }

      box.chats = nextChats
      box.chatMetas = chatMetas

      if (keepChatNow && desiredChatIds.includes(keepChatNow)) box.activeChatId = keepChatNow
      else if (desiredActiveChatId && desiredChatIds.includes(desiredActiveChatId)) box.activeChatId = desiredActiveChatId
      else box.activeChatId = String(desiredChatIds[0] || '')
    } finally {
      uiChatSyncing = false
    }
  }

  async function syncActiveTargetChatsFromStorage(metaOverride?: any) {
    if (deps.activeTargetKind() === 'group') return deps.syncActiveGroupChatsFromStorage(metaOverride)
    return syncActiveRoleChatsFromStorage(metaOverride)
  }

  async function syncChatByIdFromStorage(roleId: any, chatId: any) {
    const state = deps.getState()
    if (!state.data) return false
    const rid = String(roleId || '').trim()
    const cid = String(chatId || '').trim()
    if (!rid || !cid) return false

    const target = await loadTargetFolderMeta('role', rid)
    const folder = String(target?.folder || '')
    if (!folder) return false

    const raw = await deps.storage.get(splitChatKey(folder, cid))
    const chat = raw && typeof raw === 'object' ? normalizeStoredChat(raw, 'role') : null
    if (!chat) return false

    if (!state.data.chatsByRole || typeof state.data.chatsByRole !== 'object') state.data.chatsByRole = {}
    if (!state.data.chatsByRole[rid] || typeof state.data.chatsByRole[rid] !== 'object') state.data.chatsByRole[rid] = { activeChatId: '', chats: [] }
    const box = state.data.chatsByRole[rid]
    if (!Array.isArray(box.chats)) box.chats = []

    const idx = box.chats.findIndex((c: any) => String(c?.id || '') === cid)
    const nextChat = mergeChatFromStorage(chat, idx >= 0 ? box.chats[idx] : null)
    if (idx >= 0) box.chats[idx] = nextChat
    else box.chats.unshift(nextChat)
    box.chatMetas = upsertChatMeta(box.chatMetas, chatMetaFromChat(nextChat, '新聊天'), '新聊天')

    return true
  }

  async function syncGroupChatByIdFromStorage(groupId: any, chatId: any) {
    const state = deps.getState()
    if (!state.data) return false
    const gid = String(groupId || '').trim()
    const cid = String(chatId || '').trim()
    if (!gid || !cid) return false

    const target = await loadTargetFolderMeta('group', gid)
    const folder = String(target?.folder || '')
    if (!folder) return false

    const raw = await deps.storage.get(splitGroupChatKey(folder, cid))
    const chat = raw && typeof raw === 'object' ? normalizeStoredChat(raw, 'group') : null
    if (!chat) return false

    if (!(state.data as any).chatsByGroup || typeof (state.data as any).chatsByGroup !== 'object') (state.data as any).chatsByGroup = {}
    if (!(state.data as any).chatsByGroup[gid] || typeof (state.data as any).chatsByGroup[gid] !== 'object') (state.data as any).chatsByGroup[gid] = { activeChatId: '', chats: [] }
    const box = (state.data as any).chatsByGroup[gid]
    if (!Array.isArray(box.chats)) box.chats = []

    const idx = box.chats.findIndex((c: any) => String(c?.id || '') === cid)
    const nextChat = mergeChatFromStorage(chat, idx >= 0 ? box.chats[idx] : null)
    if (idx >= 0) box.chats[idx] = nextChat
    else box.chats.unshift(nextChat)
    box.chatMetas = upsertChatMeta(box.chatMetas, chatMetaFromChat(nextChat, '群聊'), '群聊')

    return true
  }

  async function applyChatUpdatedNoticeOnce() {
    const state = deps.getState()
    if (state.loading || !state.data) return false
    let raw = null
    try {
      raw = await deps.rtStorage.get(UI_CHAT_UPDATED_NOTICE_KEY)
    } catch (_) {
      raw = null
    }
    if (!raw || typeof raw !== 'object') return false

    const nid = String((raw as any).id || '')
    if (!nid || nid === uiLastChatUpdatedNoticeId) return false
    uiLastChatUpdatedNoticeId = nid

    const kind = String((raw as any).targetKind || '').trim() === 'group' ? 'group' : 'role'
    const tid = String((raw as any).targetId || (raw as any).roleId || '').trim()
    const cid = String((raw as any).chatId || '').trim()
    const updatedAt = Number((raw as any).updatedAt || 0)
    if (!tid || !cid) return false

    const activeKind = deps.activeTargetKind()
    const activeTid =
      activeKind === 'group'
        ? String((state.draft as any).activeGroupId || (state.data?.ui as any)?.activeGroupId || '').trim()
        : String(state.draft.activeRoleId || state.data?.ui?.activeRoleId || '').trim()
    if (!activeTid || kind !== activeKind || tid !== activeTid) return false
    if (pendingChatForTarget(state, kind, tid)) return false

    const activeBox = kind === 'group' ? (state.data as any)?.chatsByGroup?.[tid] : state.data?.chatsByRole?.[tid]
    const activeChatId = String(deps.activeChatFromData()?.id || activeBox?.activeChatId || '').trim()
    if (activeChatId && cid === activeChatId) {
      const currentUpdatedAt = Number(deps.activeChatFromData()?.updatedAt || 0)
      if (updatedAt && currentUpdatedAt && updatedAt <= currentUpdatedAt) return false
      const ok = kind === 'group' ? await syncGroupChatByIdFromStorage(tid, cid) : await syncChatByIdFromStorage(tid, cid)
      return !!ok
    }

    const box = activeBox
    const metas = Array.isArray(box?.chatMetas) ? box.chatMetas : []
    const it = metas.find((c: any) => String(c?.id || '') === cid) || null
    if (it && updatedAt && Number(it.updatedAt || 0) !== updatedAt) {
      it.updatedAt = updatedAt
      return true
    }

    return false
  }

  async function uiPollTick() {
    if (uiPollingDisposed) return
    const state = deps.getState()
    if (state.loading || !state.data) return

    let chat = deps.activeChatFromData() || (await deps.ensureActiveChatLoaded?.().catch(() => null))
    if (!chat) return

    try {
      const changedByNotice = await applyChatUpdatedNoticeOnce()
      if (changedByNotice) {
        chat = deps.activeChatFromData()
        deps.emit()
      }
    } catch (_) {}

    const t2 = now()
    if (t2 - uiLastMetaCheckMs > 900) {
      uiLastMetaCheckMs = t2
      const activeKind = deps.activeTargetKind() === 'group' ? 'group' : 'role'
      const activeTid =
        activeKind === 'group'
          ? String((state.draft as any).activeGroupId || (state.data?.ui as any)?.activeGroupId || '').trim()
          : String(state.draft.activeRoleId || state.data?.ui?.activeRoleId || '').trim()
      if (!state.sending && !pendingChatForTarget(state, activeKind, activeTid)) {
        try {
          const activeMeta = await loadActiveTargetIndexMeta(activeKind, activeTid)
          const meta = activeMeta?.meta
          const updatedAt = Number(activeMeta?.updatedAt || 0)
          if (updatedAt && updatedAt !== uiLastMetaUpdatedAt) {
            await syncActiveTargetChatsFromStorage(meta)
            deps.emit()
          }
        } catch (_) {}
      }
    }

  }

  function startUiPollers() {
    if (uiPollingDisposed) return
    if (uiPollTimer) return
    uiPollTimer = window.setInterval(() => {
      uiPollTick().catch(() => {})
    }, 350)
  }

  function stopUiPollers() {
    uiPollingDisposed = true
    if (uiPollTimer) {
      window.clearInterval(uiPollTimer)
      uiPollTimer = 0
    }
  }

  return {
    startUiPollers,
    stopUiPollers,
    uiPollTick,
    syncActiveRoleChatsFromStorage,
    syncActiveTargetChatsFromStorage,
    syncChatByIdFromStorage,
    syncGroupChatByIdFromStorage,
    applyChatUpdatedNoticeOnce,
  }
}
