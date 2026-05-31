import { normalizeData } from '../domain/dataNormalizers'
import { VERSION, STICKERS_KEY } from '../domain/constants'
import { chatMetaFromChat, chatMetasFromBox, removeChatMeta, upsertChatMeta } from '../domain/chatMeta'
import {
  splitChatKey,
  splitGroupChatKey,
  splitGroupKey,
  splitRoleKey,
} from '../domain/storageKeys'
import { loadProvidersFromStorage, loadSplitMetaSnapshot } from './splitIndexes'

export type LazyChatKind = 'role' | 'group'

export type LazyChatStorage = {
  get: (key: string) => Promise<any>
  set: (key: string, value: any) => Promise<void>
  remove?: (key: string) => Promise<void>
}

function targetBox(data: any, kind: LazyChatKind, targetId: string) {
  if (!data) return null
  const root = kind === 'group' ? ((data as any).chatsByGroup ||= {}) : (data.chatsByRole ||= {})
  const fallbackTitle = kind === 'group' ? '群聊' : '新聊天'
  if (!root[targetId] || typeof root[targetId] !== 'object') root[targetId] = { activeChatId: '', chatMetas: [], chats: [] }
  const box = root[targetId]
  if (!Array.isArray(box.chats)) box.chats = []
  box.chatMetas = chatMetasFromBox(box, fallbackTitle)
  box.activeChatId = String(box.activeChatId || '')
  return box
}

function folderForTarget(meta: any, kind: LazyChatKind, targetId: string): string {
  const folders = kind === 'group' ? meta?.groupFolders : meta?.roleFolders
  return String(folders?.[targetId] || '').trim()
}

function chatKeyFor(kind: LazyChatKind, folder: string, chatId: string): string {
  return kind === 'group' ? splitGroupChatKey(folder, chatId) : splitChatKey(folder, chatId)
}

function normalizeLoadedChat(chat: any, kind: LazyChatKind) {
  const fallbackTitle = kind === 'group' ? '群聊' : '新聊天'
  const id = String(chat?.id || '').trim()
  if (!id) return null
  const data: any = {
    version: VERSION,
    settings: { providers: [{ id: '__lazy__', name: '__lazy__', baseUrl: 'http://', apiKey: '' }] },
    favorites: { folders: [], chatRefsByFolderId: {} },
    roles: [{ id: '__lazy_role__', name: '__lazy__', createdAt: 1, updatedAt: 1, modelRef: { providerId: '__lazy__', modelId: '' } }],
    chatsByRole: {
      __lazy_role__: {
        activeChatId: id,
        chats: [{ ...chat, title: String(chat?.title || '').trim() || fallbackTitle }],
      },
    },
    groups: [],
    chatsByGroup: {},
    ui: {},
  }
  return normalizeData(data).chatsByRole.__lazy_role__.chats[0] || null
}

export function createLazyChatStore(deps: {
  storage: LazyChatStorage
  getState: () => any
  loadSplitMeta: () => Promise<any>
}) {
  const { storage, getState, loadSplitMeta } = deps

  async function loadShell() {
    const meta = await loadSplitMetaSnapshot(storage)
    if (!meta) return null

    let stickers = null
    try {
      stickers = await storage.get(STICKERS_KEY)
    } catch (_) {
      stickers = null
    }

    const d: any = {
      version: VERSION,
      settings: meta.settings && typeof meta.settings === 'object' ? meta.settings : {},
      favorites: (meta as any).favorites,
      roles: [],
      chatsByRole: {},
      groups: [],
      chatsByGroup: {},
      ui: meta.ui && typeof meta.ui === 'object' ? meta.ui : {},
    }
    d.settings.stickers = stickers && typeof stickers === 'object' ? stickers : {}
    d.settings.providers = []
    return normalizeData(d)
  }

  async function ensureChatLoaded(kind: LazyChatKind, targetIdRaw: any, chatIdRaw: any) {
    const state = getState()
    if (!state.data) return null
    const targetId = String(targetIdRaw || '').trim()
    const chatId = String(chatIdRaw || '').trim()
    if (!targetId || !chatId) return null
    const box = targetBox(state.data, kind, targetId)
    if (!box) return null
    const existing = box.chats.find((c: any) => String(c?.id || '') === chatId) || null
    if (existing) return existing

    void kind
    return null
  }

  async function loadChat(kind: LazyChatKind, targetIdRaw: any, chatIdRaw: any) {
    const targetId = String(targetIdRaw || '').trim()
    const chatId = String(chatIdRaw || '').trim()
    if (!targetId || !chatId) return null
    void kind
    void targetId
    void chatId
    return null
  }

  async function saveChat(kind: LazyChatKind, targetIdRaw: any, chatRaw: any) {
    const targetId = String(targetIdRaw || '').trim()
    const chatId = String(chatRaw?.id || '').trim()
    if (!targetId || !chatId) return
    void kind
    void targetId
    void chatId
    void chatRaw
  }

  async function removeChat(kind: LazyChatKind, targetIdRaw: any, chatIdRaw: any) {
    const targetId = String(targetIdRaw || '').trim()
    const chatId = String(chatIdRaw || '').trim()
    if (!targetId || !chatId || typeof storage.remove !== 'function') return
    void kind
    void targetId
    void chatId
  }

  async function ensureActiveChatLoaded() {
    const state = getState()
    if (!state.data) return null
    const kind = String(state.draft?.activeTargetKind || state.data?.ui?.activeTargetKind || '') === 'group' ? 'group' : 'role'
    const targetId = kind === 'group' ? String(state.draft?.activeGroupId || state.data?.ui?.activeGroupId || '') : String(state.draft?.activeRoleId || state.data?.ui?.activeRoleId || '')
    const box = targetBox(state.data, kind, targetId)
    const chatId = String(box?.activeChatId || box?.chatMetas?.[0]?.id || '')
    if (!chatId) return null
    box.activeChatId = chatId
    return ensureChatLoaded(kind, targetId, chatId)
  }

  function upsertLoadedChat(kind: LazyChatKind, targetIdRaw: any, chatRaw: any) {
    const state = getState()
    if (!state.data || !chatRaw || typeof chatRaw !== 'object') return null
    const targetId = String(targetIdRaw || '').trim()
    const chatId = String(chatRaw?.id || '').trim()
    if (!targetId || !chatId) return null
    const box = targetBox(state.data, kind, targetId)
    if (!box) return null
    const index = box.chats.findIndex((c: any) => String(c?.id || '') === chatId)
    if (index >= 0) box.chats[index] = chatRaw
    else box.chats.unshift(chatRaw)
    box.chatMetas = upsertChatMeta(box.chatMetas, chatMetaFromChat(chatRaw, kind === 'group' ? '群聊' : '新聊天'), kind === 'group' ? '群聊' : '新聊天')
    return chatRaw
  }

  function removeLoadedChat(kind: LazyChatKind, targetIdRaw: any, chatIdRaw: any) {
    const state = getState()
    if (!state.data) return
    const targetId = String(targetIdRaw || '').trim()
    const chatId = String(chatIdRaw || '').trim()
    if (!targetId || !chatId) return
    const box = targetBox(state.data, kind, targetId)
    if (!box) return
    box.chats = box.chats.filter((c: any) => String(c?.id || '') !== chatId)
    box.chatMetas = removeChatMeta(box.chatMetas, chatId, kind === 'group' ? '群聊' : '新聊天')
    if (String(box.activeChatId || '') === chatId) box.activeChatId = String(box.chatMetas[0]?.id || box.chats[0]?.id || '')
  }

  return {
    loadShell,
    loadChat,
    saveChat,
    removeChat,
    ensureChatLoaded,
    ensureActiveChatLoaded,
    upsertLoadedChat,
    removeLoadedChat,
  }
}
