import { now, uid } from '../core/utils'
import { VERSION, SPLIT_SCHEMA_VERSION, SPLIT_META_KEY, SESSION_FAVORITES_KEY } from '../domain/constants'
import { chatMetaFromChat, chatMetaIds, chatMetaUpdatedAtMap, chatMetasFromBox, upsertChatMeta } from '../domain/chatMeta'
import { normalizeData } from '../domain/dataNormalizers'
import { normalizeFavorites } from '../domain/favorites'
import { resolveAssistantMessageForMerge } from '../domain/assistantRunState'
import { normalizeChatSaveIntent, type ChatSaveIntent } from '../domain/chatSaveIntent'
import {
  splitRoleKey,
  splitChatKey,
  splitGroupKey,
  splitGroupChatKey,
  splitChatsIndexKey,
  splitRoleChatIndexKey,
  splitGroupsIndexKey,
  splitGroupChatIndexKey,
  splitProvidersIndexKey,
  splitProviderKey,
  roleFolderName,
  groupFolderName,
  providerFolderName,
} from '../domain/storageKeys'
import { loadProvidersFromStorage, loadSplitMetaSnapshot } from './splitIndexes'
import { updateStoredChatIndexEntry, type ChatIndexKind } from './chatIndexUpdater'
import { initializeStickerSettingsStorage, loadStickerSettingsFromStorage } from './stickerSettingsPersistence'

let splitMetaCache: any = null
let splitMetaWriteChain: Promise<void> = Promise.resolve()
let sessionFavoritesWriteChain: Promise<void> = Promise.resolve()

function normalizeMessageIdSet(raw: any) {
  const out = new Set<string>()
  const list = Array.isArray(raw) ? raw : raw instanceof Set ? Array.from(raw) : []
  for (const item of list) {
    const id = String(item || '').trim()
    if (id) out.add(id)
  }
  return out
}

function normalizeDeletedMessageParentMap(raw: any) {
  const out = new Map<string, string>()
  const box = raw && typeof raw === 'object' ? raw : {}
  for (const [idRaw, parentRaw] of Object.entries(box)) {
    const id = String(idRaw || '').trim()
    if (!id) continue
    out.set(id, String(parentRaw || '').trim())
  }
  return out
}

function findExistingParentForDeletedMessage(messageId: string, deletedMessageParentById: Map<string, string>, localIds: Set<string>) {
  let cur = String(messageId || '').trim()
  const seen = new Set<string>()
  let guard = 0
  while (cur && !seen.has(cur) && guard < 6000) {
    guard++
    seen.add(cur)
    const parent = deletedMessageParentById.get(cur) || ''
    if (!parent) return ''
    if (localIds.has(parent)) return parent
    cur = parent
  }
  return ''
}

function stringList(value: any): string[] {
  return Array.isArray(value) ? value.map((item: any) => String(item || '').trim()).filter(Boolean) : []
}

function stringMap(value: any): Record<string, string> {
  const src = value && typeof value === 'object' && !Array.isArray(value) ? value : {}
  const out: Record<string, string> = {}
  for (const [key, raw] of Object.entries(src)) {
    const k = String(key || '').trim()
    const v = String(raw || '').trim()
    if (k && v) out[k] = v
  }
  return out
}

function uniqueFolder(baseRaw: string, used: Set<string>, id: string, prefix: string) {
  const base = String(baseRaw || prefix).trim() || prefix
  if (!used.has(base)) return base
  const tail = id.slice(Math.max(0, id.length - 8)) || uid(prefix)
  let folder = `${base}__${tail}`
  let suffix = 2
  while (used.has(folder)) folder = `${base}__${tail}_${suffix++}`
  return folder
}

async function writeRequired(storage: { set: (key: string, value: any) => Promise<any> }, key: string, value: any) {
  await storage.set(key, value)
}

async function removeRequired(storage: { remove: (key: string) => Promise<any> }, key: string) {
  await storage.remove(key)
}

function normalizeMessageAfterDeletionIntent(message: any, deletedMessageIds: Set<string>, deletedMessageParentById: Map<string, string>, localIds: Set<string>) {
  const m = message && typeof message === 'object' ? message : null
  if (!m || !deletedMessageIds.size) return message
  const parentMid = String((m as any)?.parentMid || '').trim()
  if (!parentMid || !deletedMessageIds.has(parentMid)) return message
  return { ...(m as any), parentMid: findExistingParentForDeletedMessage(parentMid, deletedMessageParentById, localIds) }
}

function isStoredMessageInDeletedSubtree(messageId: string, storedById: Map<string, any>, subtreeRootIds: Set<string>) {
  let cur = String(messageId || '').trim()
  if (!cur || !subtreeRootIds.size) return false
  const seen = new Set<string>()
  let guard = 0
  while (cur && !seen.has(cur) && guard < 6000) {
    guard++
    if (subtreeRootIds.has(cur)) return true
    seen.add(cur)
    const m = storedById.get(cur) || null
    cur = m ? String((m as any)?.parentMid || '').trim() : ''
  }
  return false
}

function sanitizeMergedBranchHeads(chat: any) {
  const c = chat && typeof chat === 'object' ? chat : null
  if (!c) return
  const messages = Array.isArray(c.messages) ? c.messages : []
  const ids = new Set<string>()
  for (const m of messages) {
    const id = String((m as any)?.id || '').trim()
    if (id) ids.add(id)
  }
  const fallbackHeadMid = messages.length ? String((messages[messages.length - 1] as any)?.id || '').trim() : ''
  const branches = Array.isArray(c.branching?.branches) ? c.branching.branches : []
  for (const b of branches) {
    if (!b || typeof b !== 'object') continue
    const headMid = String((b as any).headMid || '').trim()
    if (headMid && ids.has(headMid)) continue
    ;(b as any).headMid = fallbackHeadMid
    ;(b as any).updatedAt = Number(c.updatedAt || now())
  }
}

function mergeChatForConcurrentWrite(localChat: any, storedChat: any, intentRaw?: ChatSaveIntent) {
  const local = localChat && typeof localChat === 'object' ? localChat : null
  const stored = storedChat && typeof storedChat === 'object' ? storedChat : null
  if (!local || !stored) return localChat

  const intent = normalizeChatSaveIntent(intentRaw)

  const out: any = { ...(local as any) }
  const localMsgs: any[] = Array.isArray(local.messages) ? local.messages.slice() : []
  const storedMsgs: any[] = Array.isArray((stored as any).messages) ? (stored as any).messages : []
  const deletedMessageIds = normalizeMessageIdSet(intent.deletedMessageIds)
  const deletedSubtreeRootIds = normalizeMessageIdSet(intent.deletedSubtreeRootIds)
  const deletedMessageParentById = normalizeDeletedMessageParentMap(intent.deletedMessageParentById)
  const storedById = new Map<string, any>()
  for (const sm of storedMsgs) {
    const sid = String((sm as any)?.id || '').trim()
    if (!sid || storedById.has(sid)) continue
    storedById.set(sid, sm)
  }

  const indexById = new Map<string, number>()
  const localIds = new Set<string>()
  const rebuildIndex = () => {
    indexById.clear()
    localIds.clear()
    for (let i = 0; i < localMsgs.length; i++) {
      const id = String((localMsgs[i] as any)?.id || '').trim()
      if (!id || indexById.has(id)) continue
      indexById.set(id, i)
      localIds.add(id)
    }
  }
  rebuildIndex()

  for (const sm of storedMsgs) {
    const sid = String((sm as any)?.id || '').trim()
    if (!sid || indexById.has(sid)) continue
    if (deletedMessageIds.has(sid)) continue
    if (isStoredMessageInDeletedSubtree(sid, storedById, deletedSubtreeRootIds)) continue
    const groupParentMid = String((sm as any)?.groupParentMid || '').trim()
    if (groupParentMid && deletedMessageIds.has(groupParentMid)) continue
    if (groupParentMid && isStoredMessageInDeletedSubtree(groupParentMid, storedById, deletedSubtreeRootIds)) continue

    const pm0 = String((sm as any)?.parentMid || '').trim()
    const parentWasDeleted = pm0 && deletedMessageIds.has(pm0)
    const nextMessage = parentWasDeleted ? normalizeMessageAfterDeletionIntent(sm, deletedMessageIds, deletedMessageParentById, localIds) : sm
    const pm = String((nextMessage as any)?.parentMid || '').trim()
    if (pm && indexById.has(pm)) {
      localMsgs.splice((indexById.get(pm) as number) + 1, 0, nextMessage)
    } else {
      localMsgs.push(nextMessage)
    }
    rebuildIndex()
  }

  for (const sm of storedMsgs) {
    const sid = String((sm as any)?.id || '').trim()
    if (!sid) continue
    const i = indexById.get(sid)
    if (typeof i !== 'number') continue
    const lm = localMsgs[i]
    if (!lm || typeof lm !== 'object') continue
    localMsgs[i] = normalizeMessageAfterDeletionIntent(resolveAssistantMessageForMerge(lm, sm, { storedChatStatus: (stored as any)?.status }), deletedMessageIds, deletedMessageParentById, localIds)
  }

  try {
    const lb = out.branching && typeof out.branching === 'object' ? out.branching : null
    const sb = (stored as any).branching && typeof (stored as any).branching === 'object' ? (stored as any).branching : null
    if (lb && sb) {
      const lList: any[] = Array.isArray((lb as any).branches) ? (lb as any).branches.slice() : []
      const sList: any[] = Array.isArray((sb as any).branches) ? (sb as any).branches : []
      const byId = new Map<string, any>()
      for (const b of lList) {
        const id = String(b?.id || '').trim()
        if (id && !byId.has(id)) byId.set(id, b)
      }
      for (const b of sList) {
        const id = String(b?.id || '').trim()
        if (!id) continue
        const cur = byId.get(id) || null
        if (!cur) {
          lList.push(b)
          byId.set(id, b)
          continue
        }
        const lu = Number(cur?.updatedAt || 0)
        const su = Number(b?.updatedAt || 0)
        if (su > lu) {
          cur.headMid = String(b?.headMid || cur.headMid || '')
          cur.updatedAt = su
        } else if (!String(cur?.headMid || '').trim() && String(b?.headMid || '').trim()) {
          cur.headMid = String(b?.headMid || '')
        }
        if (!String(cur?.forkFromMid || '').trim() && String(b?.forkFromMid || '').trim()) cur.forkFromMid = String(b?.forkFromMid || '')
      }
      out.branching = { ...(lb as any), ...(sb as any), branches: lList, activeBranchId: String((lb as any).activeBranchId || (sb as any).activeBranchId || '') }
    } else if (!out.branching && sb) {
      out.branching = sb
    }
  } catch (_) {}

  out.messages = localMsgs
  sanitizeMergedBranchHeads(out)
  return out
}

const noop = async () => {}

export function createSplitStorage(deps: {
  storage: { get: (k: string) => Promise<any>; set: (k: string, v: any) => Promise<void>; remove: (k: string) => Promise<void> }
  rtStorage?: { get: (k: string) => Promise<any>; set: (k: string, v: any) => Promise<void>; remove: (k: string) => Promise<void> }
  withChatWriteLock?: (kind: any, targetId: any, chatId: any, fn: () => Promise<any>) => Promise<any>
  writeChatUpdatedNotice?: (targetKind: any, targetId: any, chatId: any, updatedAt: any) => Promise<void>
  syncRoleAvatarFile?: (folder: any, role: any) => Promise<void>
  syncGroupAvatarFile?: (folder: any, group: any) => Promise<void>
  getState?: () => any
  setState?: (data: any) => void
  onError?: (msg: string) => void
}) {
  const {
    storage,
    withChatWriteLock: _withChatWriteLock,
    writeChatUpdatedNotice: _writeChatUpdatedNotice,
    syncRoleAvatarFile: _syncRoleAvatarFile = noop,
    syncGroupAvatarFile: _syncGroupAvatarFile = noop,
    getState,
    setState,
    onError,
  } = deps

  const withChatWriteLock = _withChatWriteLock || ((_k, _tid, _cid, fn) => fn())
  const writeChatUpdatedNotice = _writeChatUpdatedNotice || noop

  async function loadSplitMeta() {
    const meta = await loadSplitMetaSnapshot(storage)
    splitMetaCache = meta
    return meta
  }

  function withSplitMetaWrite<T>(fn: () => Promise<T>): Promise<T> {
    const run = () => Promise.resolve().then(fn)
    const p = splitMetaWriteChain.then(run, run) as Promise<T>
    splitMetaWriteChain = p.then(
      () => undefined,
      () => undefined,
    )
    return p
  }

  async function updateChatIndexEntry(kind: ChatIndexKind, targetId: any, chatId: any, patch: { chat?: any; updatedAt?: any; title?: any; remove?: boolean }) {
    await withSplitMetaWrite(async () => {
      const meta = (await loadSplitMeta()) || splitMetaCache
      const nextMeta = await updateStoredChatIndexEntry(storage, kind, targetId, chatId, patch, meta)
      splitMetaCache = nextMeta || meta
    })
  }

  async function touchChatUpdatedAt(roleId: any, chatId: any, updatedAt: any) {
    const rid = String(roleId || '').trim()
    const cid = String(chatId || '').trim()
    const ua0 = Number(updatedAt || 0)
    if (!rid || !cid) return

    await updateChatIndexEntry('role', rid, cid, { updatedAt: ua0 > 0 ? ua0 : now() })
  }

  async function touchGroupChatUpdatedAt(groupId: any, chatId: any, updatedAt: any) {
    const gid = String(groupId || '').trim()
    const cid = String(chatId || '').trim()
    const ua0 = Number(updatedAt || 0)
    if (!gid || !cid) return
    await updateChatIndexEntry('group', gid, cid, { updatedAt: ua0 > 0 ? ua0 : now() })
  }

  async function saveChatEntry(kind: ChatIndexKind, targetId: any, chat: any, intent?: ChatSaveIntent) {
    const tid = String(targetId || '').trim()
    const cid = String(chat?.id || '').trim()
    if (!tid || !cid || !chat || typeof chat !== 'object') return
    const meta = (await loadSplitMeta()) || splitMetaCache
    const folder = kind === 'group' ? String((meta as any)?.groupFolders?.[tid] || '').trim() : String(meta?.roleFolders?.[tid] || '').trim()
    if (!folder) throw new Error(kind === 'group' ? '群组不存在' : '角色不存在')
    const key = kind === 'group' ? splitGroupChatKey(folder, cid) : splitChatKey(folder, cid)
    let savedChat = chat
    await withChatWriteLock(kind, tid, cid, async () => {
      const raw0 = await storage.get(key)
      const stored = raw0 && typeof raw0 === 'object' ? raw0 : null
      const merged = mergeChatForConcurrentWrite(chat, stored, intent)
      savedChat = merged
      await storage.set(key, merged)
    })
    await updateChatIndexEntry(kind, tid, cid, { chat: savedChat })
    await writeChatUpdatedNotice(kind, tid, cid, Number(savedChat?.updatedAt || chat.updatedAt || now()))
  }

  async function saveRoleChat(roleId: any, chat: any, intent?: ChatSaveIntent) {
    await saveChatEntry('role', roleId, chat, intent)
  }

  async function setActiveRoleChatSelection(roleIdRaw: any, chatIdRaw: any) {
    const roleId = String(roleIdRaw || '').trim()
    const chatId = String(chatIdRaw || '').trim()
    if (!roleId) return
    await withSplitMetaWrite(async () => {
      const meta = (await loadSplitMeta()) || splitMetaCache
      if (!meta) throw new Error('存储未初始化')
      const folder = String(meta?.roleFolders?.[roleId] || '').trim()
      if (!folder) throw new Error('角色不存在')
      const key = splitRoleChatIndexKey(folder)
      const idx0 = await storage.get(key).catch(() => null)
      const idx = idx0 && typeof idx0 === 'object'
        ? { ...idx0 }
        : { schemaVersion: SPLIT_SCHEMA_VERSION, roleId, roleFolder: folder, chatIds: [], chatUpdatedAt: {}, chatMetas: [] }
      ;(idx as any).activeChatId = chatId
      ;(idx as any).updatedAt = now()
      await storage.set(key, idx)
      const chatIndexByRole = { ...(meta.chatIndexByRole || {}) }
      chatIndexByRole[roleId] = { ...(chatIndexByRole[roleId] || {}), ...(idx as any) }
      splitMetaCache = { ...meta, chatIndexByRole }
    })
  }

  async function removeRoleChatEntry(roleIdRaw: any, chatIdRaw: any) {
    const roleId = String(roleIdRaw || '').trim()
    const chatId = String(chatIdRaw || '').trim()
    if (!roleId || !chatId) return
    await withSplitMetaWrite(async () => {
      const meta = (await loadSplitMeta()) || splitMetaCache
      if (!meta) throw new Error('存储未初始化')
      const nextMeta = await updateStoredChatIndexEntry(storage, 'role', roleId, chatId, { remove: true }, meta)
      const folder = String(meta?.roleFolders?.[roleId] || '').trim()
      if (folder) await storage.remove(splitChatKey(folder, chatId)).catch(() => {})
      splitMetaCache = nextMeta || meta
    })
  }

  async function saveGroupChat(groupId: any, chat: any, intent?: ChatSaveIntent) {
    await saveChatEntry('group', groupId, chat, intent)
  }

  async function saveRoleEntity(role: any) {
    const roleId = String(role?.id || '').trim()
    if (!roleId || !role || typeof role !== 'object') throw new Error('角色无效')

    await withSplitMetaWrite(async () => {
      const meta = (await loadSplitMeta()) || splitMetaCache
      if (!meta) throw new Error('存储未初始化')

      const roleOrder = stringList(meta.roleOrder)
      if (!roleOrder.includes(roleId)) roleOrder.unshift(roleId)

      const roleFolders = stringMap(meta.roleFolders)
      if (!roleFolders[roleId]) {
        roleFolders[roleId] = uniqueFolder(roleFolderName(role), new Set(Object.values(roleFolders)), roleId, 'role')
      }
      const folder = roleFolders[roleId]

      await writeRequired(storage, splitRoleKey(folder), role)
      await _syncRoleAvatarFile(folder, role)
      await writeRequired(storage, splitChatsIndexKey(), { schemaVersion: SPLIT_SCHEMA_VERSION, updatedAt: now(), roleOrder, roleFolders })

      splitMetaCache = { ...meta, roleOrder, roleFolders }
    })
  }

  async function removeRoleEntity(roleIdRaw: any) {
    const roleId = String(roleIdRaw || '').trim()
    if (!roleId) throw new Error('角色无效')

    await withSplitMetaWrite(async () => {
      const meta = (await loadSplitMeta()) || splitMetaCache
      if (!meta) throw new Error('存储未初始化')

      const roleOrder = stringList(meta.roleOrder).filter((id) => id !== roleId)
      const roleFolders = stringMap(meta.roleFolders)
      const folder = roleFolders[roleId]
      if (!folder) throw new Error('角色不存在')

      const chatIndexByRole = { ...(meta.chatIndexByRole || {}) }
      const oldIndex = chatIndexByRole[roleId] || {}
      delete chatIndexByRole[roleId]
      delete roleFolders[roleId]

      await removeRequired(storage, splitRoleKey(folder))
      await storage.remove(splitRoleChatIndexKey(folder)).catch(() => {})
      for (const chatId of stringList(oldIndex.chatIds)) await storage.remove(splitChatKey(folder, chatId)).catch(() => {})
      await writeRequired(storage, splitChatsIndexKey(), { schemaVersion: SPLIT_SCHEMA_VERSION, updatedAt: now(), roleOrder, roleFolders })

      splitMetaCache = { ...meta, roleOrder, roleFolders, chatIndexByRole }
    })
  }

  async function saveProviderEntity(provider: any) {
    const providerId = String(provider?.id || '').trim()
    if (!providerId || !provider || typeof provider !== 'object') throw new Error('供应商无效')

    await withSplitMetaWrite(async () => {
      const meta = (await loadSplitMeta()) || splitMetaCache
      if (!meta) throw new Error('存储未初始化')

      const providerOrder = stringList((meta as any).providerOrder)
      if (!providerOrder.includes(providerId)) providerOrder.unshift(providerId)

      const providerFolders = stringMap((meta as any).providerFolders)
      if (!providerFolders[providerId]) {
        providerFolders[providerId] = uniqueFolder(providerFolderName(provider), new Set(Object.values(providerFolders)), providerId, 'provider')
      }

      await writeRequired(storage, splitProvidersIndexKey(), { schemaVersion: SPLIT_SCHEMA_VERSION, updatedAt: now(), providerOrder, providerFolders })
      await writeRequired(storage, splitProviderKey(providerFolders[providerId]), provider)

      splitMetaCache = { ...meta, providerOrder, providerFolders }
    })
  }

  async function removeProviderEntity(providerIdRaw: any) {
    const providerId = String(providerIdRaw || '').trim()
    if (!providerId) throw new Error('供应商无效')

    await withSplitMetaWrite(async () => {
      const meta = (await loadSplitMeta()) || splitMetaCache
      if (!meta) throw new Error('存储未初始化')

      const providerOrder = stringList((meta as any).providerOrder).filter((id) => id !== providerId)
      const providerFolders = stringMap((meta as any).providerFolders)
      const folder = providerFolders[providerId]
      if (!folder) throw new Error('供应商不存在')
      delete providerFolders[providerId]

      await removeRequired(storage, splitProviderKey(folder))
      await writeRequired(storage, splitProvidersIndexKey(), { schemaVersion: SPLIT_SCHEMA_VERSION, updatedAt: now(), providerOrder, providerFolders })

      splitMetaCache = { ...meta, providerOrder, providerFolders }
    })
  }

  async function saveRoleOrder(roleIdsRaw: any) {
    const roleIds = stringList(roleIdsRaw)
    await withSplitMetaWrite(async () => {
      const meta = (await loadSplitMeta()) || splitMetaCache
      if (!meta) throw new Error('存储未初始化')
      const roleFolders = stringMap(meta.roleFolders)
      const known = new Set(Object.keys(roleFolders))
      const filtered = roleIds.filter((id) => known.has(id))
      const remaining = stringList(meta.roleOrder).filter((id) => !filtered.includes(id) && known.has(id))
      const roleOrder = filtered.concat(remaining)
      await writeRequired(storage, splitChatsIndexKey(), { schemaVersion: SPLIT_SCHEMA_VERSION, updatedAt: now(), roleOrder, roleFolders })
      splitMetaCache = { ...meta, roleOrder, roleFolders }
    })
  }

  async function loadSplitData() {
    const meta = (await loadSplitMeta()) || splitMetaCache
    if (!meta) return null

    let stickers = null
    try {
      stickers = await loadStickerSettingsFromStorage(storage)
    } catch (_) {
      stickers = null
    }

    let favorites = null
    try {
      favorites = await storage.get(SESSION_FAVORITES_KEY)
    } catch (_) {
      favorites = null
    }

    const providers = await loadProvidersFromStorage(storage, meta)

    const d = {
      version: VERSION,
      settings: meta.settings && typeof meta.settings === 'object' ? meta.settings : {},
      favorites: normalizeFavorites(favorites),
      roles: [] as any[],
      chatsByRole: {} as Record<string, any>,
      groups: [] as any[],
      chatsByGroup: {} as Record<string, any>,
      ui: meta.ui && typeof meta.ui === 'object' ? meta.ui : {},
    }

    ;(d.settings as any).stickers = stickers && typeof stickers === 'object' ? stickers : {}
    ;(d.settings as any).providers = providers

    for (const rid of meta.roleOrder) {
      const folder = String(meta.roleFolders?.[rid] || '')
      if (!folder) throw new Error('存储索引损坏：roleFolders 缺失')

      const r = await storage.get(splitRoleKey(folder))
      const role = r && typeof r === 'object' ? r : null
      if (!role) throw new Error('存储损坏：角色文件缺失或无效')

      d.roles.push(role)

      const idx = meta.chatIndexByRole?.[rid]
      const box = idx && typeof idx === 'object' ? idx : {}
      const activeChatId = String(box.activeChatId || '')
      const chatIds = Array.isArray(box.chatIds) ? box.chatIds.map((x: any) => String(x || '')).filter((x: any) => !!x) : []

      const chats = []
      for (const cid of chatIds) {
        const c0 = await storage.get(splitChatKey(folder, cid))
        const c = c0 && typeof c0 === 'object' ? c0 : null
        if (!c) throw new Error('存储损坏：会话文件缺失或无效')
        chats.push(c)
      }

      d.chatsByRole[String(role.id || rid)] = {
        activeChatId,
        chats,
      }
    }

    for (const gid of (meta as any).groupOrder || []) {
      const folder = String((meta as any).groupFolders?.[gid] || '')
      if (!folder) throw new Error('存储索引损坏：groupFolders 缺失')

      const g0 = await storage.get(splitGroupKey(folder))
      const group = g0 && typeof g0 === 'object' ? g0 : null
      if (!group) throw new Error('存储损坏：群组文件缺失或无效')

      ;(d as any).groups.push(group)

      const idx = (meta as any).chatIndexByGroup?.[gid]
      const box = idx && typeof idx === 'object' ? idx : {}
      const activeChatId = String((box as any).activeChatId || '')
      const chatIds = Array.isArray((box as any).chatIds) ? (box as any).chatIds.map((x: any) => String(x || '')).filter((x: any) => !!x) : []

      const chats = []
      for (const cid of chatIds) {
        const c0 = await storage.get(splitGroupChatKey(folder, cid))
        const c = c0 && typeof c0 === 'object' ? c0 : null
        if (!c) throw new Error('存储损坏：群聊会话文件缺失或无效')
        chats.push(c)
      }

      ;(d as any).chatsByGroup[String(group.id || gid)] = { activeChatId, chats }
    }

    return normalizeData(d)
  }

  async function ensureSplitStoreReady() {
    const meta = (await loadSplitMeta()) || splitMetaCache
    if (meta) return
    const updatedAt = now()
    await initializeStickerSettingsStorage(storage)
    await storage.set(SESSION_FAVORITES_KEY, { folders: [], chatRefsByFolderId: {} })
    await writeRequired(storage, splitChatsIndexKey(), {
      schemaVersion: SPLIT_SCHEMA_VERSION,
      updatedAt,
      roleOrder: [],
      roleFolders: {},
    })
    await writeRequired(storage, splitGroupsIndexKey(), {
      schemaVersion: SPLIT_SCHEMA_VERSION,
      updatedAt,
      groupOrder: [],
      groupFolders: {},
    })
    await writeRequired(storage, splitProvidersIndexKey(), {
      schemaVersion: SPLIT_SCHEMA_VERSION,
      updatedAt,
      providerOrder: [],
      providerFolders: {},
    })
    await writeRequired(storage, SPLIT_META_KEY, {
      schemaVersion: SPLIT_SCHEMA_VERSION,
      dataVersion: VERSION,
      updatedAt,
      ui: {},
      settings: {},
      roleOrder: [],
      roleFolders: {},
      chatIndexByRole: {},
      groupOrder: [],
      groupFolders: {},
      chatIndexByGroup: {},
    })
    splitMetaCache = await loadSplitMetaSnapshot(storage)
  }

  async function saveMetaOnly() {
    const state = getState?.()
    if (!state?.data) return

    state.data.ui.activeRoleId = String(state.draft?.activeRoleId || '')
    ;(state.data.ui as any).activeGroupId = String(state.draft?.activeGroupId || '')
    ;(state.data.ui as any).activeTargetKind = String(state.draft?.activeTargetKind || '') === 'group' ? 'group' : 'role'

    const old = splitMetaCache || (await loadSplitMeta())
    if (!old) throw new Error('存储未初始化')

    const settingsMeta = state.data.settings && typeof state.data.settings === 'object' ? { ...(state.data.settings as any) } : {}
    try {
      delete (settingsMeta as any).stickers
      delete (settingsMeta as any).providers
    } catch (_) {}

    const meta = {
      schemaVersion: SPLIT_SCHEMA_VERSION,
      dataVersion: VERSION,
      updatedAt: now(),
      ui: state.data.ui && typeof state.data.ui === 'object' ? state.data.ui : {},
      settings: settingsMeta,
    }

    await storage.set(SPLIT_META_KEY, meta)
    splitMetaCache = { ...old, ...meta }
  }

  async function saveFavoritesOnly() {
    const state = getState?.()
    if (!state?.data) return
    const favorites = normalizeFavorites((state.data as any).favorites)
    const run = () => storage.set(SESSION_FAVORITES_KEY, favorites)
    const next = sessionFavoritesWriteChain.then(run, run)
    sessionFavoritesWriteChain = next.then(
      () => undefined,
      () => undefined,
    )
    await next
  }

  async function load() {
    const state = getState?.()
    try {
      await ensureSplitStoreReady()
      const split = await loadSplitData()
      if (!split) throw new Error('存储未初始化')
      setState?.(split)
      if (state) {
        state.draft.activeRoleId = String(split?.ui?.activeRoleId || '')
        state.draft.activeGroupId = String((split?.ui as any)?.activeGroupId || '')
        state.draft.activeTargetKind = String((split?.ui as any)?.activeTargetKind || 'role') === 'group' ? 'group' : 'role'
      }
    } catch (e: any) {
      setState?.(null)
      if (state) {
        state.draft.activeRoleId = ''
        state.draft.activeGroupId = ''
        state.draft.activeTargetKind = 'role'
      }
      onError?.(String(e?.message || e || '加载失败'))
    } finally {
      if (state) state.loading = false
    }
  }

  async function save() {
    const state = getState?.()
    if (!state?.data) return
    state.data.ui.activeRoleId = String(state.draft?.activeRoleId || '')
    ;(state.data.ui as any).activeGroupId = String(state.draft?.activeGroupId || '')
    ;(state.data.ui as any).activeTargetKind = String(state.draft?.activeTargetKind || '') === 'group' ? 'group' : 'role'
    await saveMetaOnly()
  }

  return {
    loadSplitMeta,
    withSplitMetaWrite,
    touchChatUpdatedAt,
    loadSplitData,
    ensureSplitStoreReady,
    saveRoleEntity,
    removeRoleEntity,
    saveProviderEntity,
    removeProviderEntity,
    saveRoleChat,
    setActiveRoleChatSelection,
    removeRoleChatEntry,
    saveRoleOrder,
    saveGroupChat,
    touchGroupChatUpdatedAt,
    saveMetaOnly,
    saveFavoritesOnly,
    load,
    save,
    writeChatUpdatedNotice,
  }
}
