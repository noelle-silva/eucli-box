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
import { normalizeStoredChat } from './normalizeStoredChat'

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

function isMissingChatError(error: unknown) {
  const message = String((error as any)?.message || error || '')
  return message.includes('json file does not exist') || message.includes('会话不存在') || message.includes('session does not exist')
}

function removeMissingChatRef(box: any, chatId: string, kind: LazyChatKind) {
  if (!box || !chatId) return
  box.chats = Array.isArray(box.chats) ? box.chats.filter((c: any) => String(c?.id || '') !== chatId) : []
  box.chatMetas = removeChatMeta(box.chatMetas, chatId, kind === 'group' ? '群聊' : '新聊天')
  if (String(box.activeChatId || '') === chatId) box.activeChatId = String(box.chatMetas[0]?.id || box.chats[0]?.id || '')
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

    const providers = await loadProvidersFromStorage(storage, meta)
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
    d.settings.providers = providers

    for (const rid of meta.roleOrder || []) {
      const folder = String(meta.roleFolders?.[rid] || '')
      if (!folder) throw new Error('存储索引损坏：roleFolders 缺失')
      const role = await storage.get(splitRoleKey(folder))
      if (!role || typeof role !== 'object') throw new Error('存储损坏：角色文件缺失或无效')
      d.roles.push(role)
      const idx = meta.chatIndexByRole?.[rid] && typeof meta.chatIndexByRole?.[rid] === 'object' ? meta.chatIndexByRole[rid] : {}
      d.chatsByRole[String(role.id || rid)] = {
        activeChatId: String(idx.activeChatId || ''),
        chatMetas: chatMetasFromBox(idx, '新聊天'),
        chats: [],
      }
    }

    for (const gid of (meta as any).groupOrder || []) {
      const folder = String((meta as any).groupFolders?.[gid] || '')
      if (!folder) throw new Error('存储索引损坏：groupFolders 缺失')
      const group = await storage.get(splitGroupKey(folder))
      if (!group || typeof group !== 'object') throw new Error('存储损坏：群组文件缺失或无效')
      d.groups.push(group)
      const idx = (meta as any).chatIndexByGroup?.[gid] && typeof (meta as any).chatIndexByGroup?.[gid] === 'object' ? (meta as any).chatIndexByGroup[gid] : {}
      d.chatsByGroup[String(group.id || gid)] = {
        activeChatId: String(idx.activeChatId || ''),
        chatMetas: chatMetasFromBox(idx, '群聊'),
        chats: [],
      }
    }

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

    const meta = await loadSplitMeta()
    const folder = folderForTarget(meta, kind, targetId)
    if (!folder) throw new Error(kind === 'group' ? '群组不存在' : '角色不存在')
    let raw: any = null
    try {
      raw = await storage.get(chatKeyFor(kind, folder, chatId))
    } catch (error) {
      if (isMissingChatError(error)) {
        removeMissingChatRef(box, chatId, kind)
        return null
      }
      throw error
    }
    const chat = normalizeStoredChat(raw, kind)
    if (!chat) {
      removeMissingChatRef(box, chatId, kind)
      return null
    }

    const index = box.chats.findIndex((c: any) => String(c?.id || '') === chatId)
    if (index >= 0) box.chats[index] = chat
    else box.chats.unshift(chat)
    box.chatMetas = upsertChatMeta(box.chatMetas, chatMetaFromChat(chat, kind === 'group' ? '群聊' : '新聊天'), kind === 'group' ? '群聊' : '新聊天')
    return chat
  }

  async function loadChat(kind: LazyChatKind, targetIdRaw: any, chatIdRaw: any) {
    const targetId = String(targetIdRaw || '').trim()
    const chatId = String(chatIdRaw || '').trim()
    if (!targetId || !chatId) return null
    const meta = await loadSplitMeta()
    const folder = folderForTarget(meta, kind, targetId)
    if (!folder) throw new Error(kind === 'group' ? '群组不存在' : '角色不存在')
    const raw = await storage.get(chatKeyFor(kind, folder, chatId))
    return normalizeStoredChat(raw, kind)
  }

  async function saveChat(kind: LazyChatKind, targetIdRaw: any, chatRaw: any) {
    const targetId = String(targetIdRaw || '').trim()
    const chatId = String(chatRaw?.id || '').trim()
    if (!targetId || !chatId) return
    const meta = await loadSplitMeta()
    const folder = folderForTarget(meta, kind, targetId)
    if (!folder) throw new Error(kind === 'group' ? '群组不存在' : '角色不存在')
    await storage.set(chatKeyFor(kind, folder, chatId), chatRaw)
  }

  async function removeChat(kind: LazyChatKind, targetIdRaw: any, chatIdRaw: any) {
    const targetId = String(targetIdRaw || '').trim()
    const chatId = String(chatIdRaw || '').trim()
    if (!targetId || !chatId || typeof storage.remove !== 'function') return
    const meta = await loadSplitMeta()
    const folder = folderForTarget(meta, kind, targetId)
    if (!folder) return
    await storage.remove(chatKeyFor(kind, folder, chatId))
  }

  async function ensureActiveChatLoaded() {
    const state = getState()
    if (!state.data) return null
    const kind = String(state.draft?.activeTargetKind || state.data?.ui?.activeTargetKind || '') === 'group' ? 'group' : 'role'
    const targetId = kind === 'group' ? String(state.draft?.activeGroupId || state.data?.ui?.activeGroupId || '') : String(state.draft?.activeRoleId || state.data?.ui?.activeRoleId || '')
    const box = targetBox(state.data, kind, targetId)
    const ids = [String(box?.activeChatId || ''), ...(Array.isArray(box?.chatMetas) ? box.chatMetas.map((m: any) => String(m?.id || '')) : [])]
      .map((id) => id.trim())
      .filter((id, index, list) => !!id && list.indexOf(id) === index)
    for (const chatId of ids) {
      box.activeChatId = chatId
      const chat = await ensureChatLoaded(kind, targetId, chatId)
      if (chat) return chat
    }
    box.activeChatId = ''
    return null
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
