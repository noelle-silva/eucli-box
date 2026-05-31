import { now, uid, clamp, clampTemp, normImagePaths } from '../core/utils'
import { createStateAccessors } from '../state/stateAccessors'
import { chatMetaFromChat, removeChatMeta, upsertChatMeta } from '../domain/chatMeta'
import { NEW_ROLE_ID } from '../domain/constants'
import { isAssistantGenerating } from '../domain/assistantRunState'

function looksLikeImageDataUrl(s: any): boolean {
  const t = String(s || '')
  return t.startsWith('data:image/')
}

function shrinkImageDataUrl(dataUrl: string, maxSide: number): Promise<string> {
  return new Promise((resolve) => {
    try {
      const u = String(dataUrl || '').trim()
      if (!looksLikeImageDataUrl(u)) return resolve('')

      const max = clamp(Math.round(Number(maxSide || 0)), 64, 4096)
      const img = new Image()
      img.decoding = 'async'
      img.onload = () => {
        try {
          const w0 = Number(img.naturalWidth || 0)
          const h0 = Number(img.naturalHeight || 0)
          if (!w0 || !h0) return resolve('')

          const s = Math.min(1, max / Math.max(w0, h0))
          const w = Math.max(1, Math.round(w0 * s))
          const h = Math.max(1, Math.round(h0 * s))

          const canvas = document.createElement('canvas')
          canvas.width = w
          canvas.height = h
          const ctx = canvas.getContext('2d')
          if (!ctx) return resolve('')
          ctx.clearRect(0, 0, w, h)
          ctx.drawImage(img, 0, 0, w, h)

          const out = canvas.toDataURL('image/png')
          resolve(looksLikeImageDataUrl(out) ? out : '')
        } catch (_) {
          resolve('')
        }
      }
      img.onerror = () => resolve('')
      img.src = u
    } catch (_) {
      resolve('')
    }
  })
}

function chatHasPendingAssistant(chat: any): boolean {
  const msgs = Array.isArray(chat?.messages) ? chat.messages : []
  for (const m of msgs) {
    if (!m || typeof m !== 'object') continue
    if (isAssistantGenerating(m)) return true
  }
  return false
}

function modelCacheId(item: any): string {
  return item && typeof item === 'object' ? String(item.id || '').trim() : String(item || '').trim()
}

function imageBasename(p: string): string {
  const s = String(p || '')
  const a = s.lastIndexOf('/')
  const b = s.lastIndexOf('\\')
  const i = Math.max(a, b)
  return i >= 0 ? s.slice(i + 1) : s
}

export function createEntityEditors(deps: {
  getState: () => any
  save: () => Promise<void>
  render: () => void
  closeModal: () => void
  showToast?: (msg: string) => void
  pickImageFiles?: (maxCount?: number) => Promise<any[]>
  filesImages: { delete?: (req: any) => Promise<any> }
  ensureChatLoaded?: (rid: string, cid: string) => Promise<any>
  ensureGroupChatLoaded?: (gid: string, cid: string) => Promise<any>
  renameRoleChatInStore?: (rid: string, cid: string, title: string) => Promise<void>
  renameGroupChatInStore?: (gid: string, cid: string, title: string) => Promise<void>
  removeChatInStore?: (kind: 'role' | 'group', targetId: string, chatId: string) => Promise<void>
  removeLoadedChat?: (kind: 'role' | 'group', targetId: string, chatId: string) => void
  cleanupFavoriteRefsForTarget: (kind: string, targetId: string) => void
  cleanupFavoriteRefsForChat: (targetKind: string, targetId: string, chatId: string) => void
  pushBoxRole?: (role: Record<string, unknown>) => Promise<void>
  pushBoxProvider?: (provider: Record<string, unknown>) => Promise<void>
  deleteBoxRole?: (id: string) => Promise<void>
  deleteBoxProvider?: (id: string) => Promise<void>
  syncBoxCatalog?: () => Promise<{ roles?: any[]; providers?: any[] } | void>
  listBoxSessions?: (roleId: string) => Promise<any[]>
  createBoxSession?: (roleId: string) => Promise<any>
  getBoxSession?: (id: string) => Promise<any>
  deleteBoxSession?: (id: string) => Promise<void>
}) {
  const { getState, save, render, closeModal, showToast, pickImageFiles, filesImages, ensureChatLoaded, ensureGroupChatLoaded, renameRoleChatInStore, renameGroupChatInStore, removeChatInStore, removeLoadedChat, cleanupFavoriteRefsForTarget, cleanupFavoriteRefsForChat, pushBoxRole, pushBoxProvider, deleteBoxRole, deleteBoxProvider, syncBoxCatalog, listBoxSessions, createBoxSession, getBoxSession, deleteBoxSession } = deps
  const sa = createStateAccessors({ getState })

  function applyBoxCatalogSnapshot(snapshot: any) {
    if (!snapshot || typeof snapshot !== 'object') return
    const state = getState()
    if (!state.data) return
    if (Array.isArray(snapshot.roles)) state.data.roles = snapshot.roles
    if (Array.isArray(snapshot.providers)) {
      if (!state.data.settings || typeof state.data.settings !== 'object') state.data.settings = {} as any
      state.data.settings.providers = snapshot.providers
    }
  }

  async function refreshBoxCatalogView() {
    if (!syncBoxCatalog) throw new Error('eucli-box 目录同步通道不可用')
    const snapshot = await syncBoxCatalog()
    applyBoxCatalogSnapshot(snapshot)
    const err = snapshot && typeof snapshot === 'object' ? String((snapshot as any).error || '') : ''
    if (err) showToast?.('eucli-box 目录部分同步失败: ' + err)
  }

  function boxSessionToChatMeta(session: any): any {
    const id = String(session?.id || '').trim()
    const t = Number(Date.parse(String(session?.createdAt || session?.lastActive || ''))) || now()
    const updatedAt = Number(Date.parse(String(session?.lastActive || session?.updatedAt || ''))) || t
    const messageCount = Array.isArray(session?.messages) ? session.messages.length : Number(session?.messageCount || 0)
    const lastMessage = Array.isArray(session?.messages) ? session.messages[session.messages.length - 1] : null
    return chatMetaFromChat({
      id,
      title: String(session?.title || '新聊天'),
      createdAt: t,
      updatedAt,
      messages: [],
      lastMessagePreview: String(lastMessage?.content || lastMessage?.reason || ''),
      messageCount,
    }, '新聊天')
  }

  function upsertRoleSessionMeta(roleId: string, session: any) {
    const state = getState()
    if (!state.data) return null
    const meta = boxSessionToChatMeta(session)
    if (!meta.id) return null
    const box = sa.ensureChatsBoxBare(roleId)
    if (!box) return null
    box.chats = []
    box.chatMetas = upsertChatMeta(box.chatMetas, meta, '新聊天')
    box.activeChatId = meta.id
    return meta
  }

  async function syncRoleSessionsFromBox(roleId: string) {
    const rid = String(roleId || '').trim()
    if (!rid) return
    if (!listBoxSessions) throw new Error('eucli-box 会话列表通道不可用')
    const state = getState()
    if (!state.data) return
    if (!getBoxSession) throw new Error('eucli-box 会话读取通道不可用')
    const sessions = await listBoxSessions(rid)
    const box = sa.ensureChatsBoxBare(rid)
    if (!box) return
    const metas = (Array.isArray(sessions) ? sessions : []).map((session: any) => boxSessionToChatMeta(session)).filter((meta: any) => !!meta.id)
    box.chats = []
    box.chatMetas = metas
    const current = String(box.activeChatId || '')
    box.activeChatId = current && metas.some((meta: any) => String(meta.id || '') === current) ? current : String(metas[0]?.id || '')
  }

  function scrollToBottomSoon() {
    // UI 负责滚动逻辑（React）
  }

  // ===== Avatar =====

  async function pickRoleAvatarImage() {
    const state = getState()
    if (state.loading) return
    if (typeof pickImageFiles !== 'function') return showToast?.('未授权：files.pickImages')

    try {
      const items = await pickImageFiles(1)
      const list = Array.isArray(items) ? items : []
      const it = list.length ? list[0] : null
      const u0 = String(it?.dataUrl || '')
      if (!looksLikeImageDataUrl(u0)) return showToast?.('未选择图片')

      const shrunk = await shrinkImageDataUrl(u0, 1024)
      const u = shrunk || u0
      if (!looksLikeImageDataUrl(u)) return showToast?.('头像图片无效')

      state.draft.roleAvatarImageCropSrc = u
      render()
    } catch (e: any) {
      showToast?.(String(e?.message || e || '选择头像失败'))
    }
  }

  function clearRoleAvatarImage() {
    const state = getState()
    state.draft.roleAvatarImage = ''
    state.draft.roleAvatarImageCropSrc = ''
    render()
  }

  async function pickGroupAvatarImage() {
    const state = getState()
    if (state.loading) return
    if (typeof pickImageFiles !== 'function') return showToast?.('未授权：files.pickImages')

    try {
      const items = await pickImageFiles(1)
      const list = Array.isArray(items) ? items : []
      const it = list.length ? list[0] : null
      const u0 = String(it?.dataUrl || '')
      if (!looksLikeImageDataUrl(u0)) return showToast?.('未选择图片')

      const shrunk = await shrinkImageDataUrl(u0, 1024)
      const u = shrunk || u0
      if (!looksLikeImageDataUrl(u)) return showToast?.('头像图片无效')

      ;(state.draft as any).groupAvatarImageCropSrc = u
      render()
    } catch (e: any) {
      showToast?.(String(e?.message || e || '选择头像失败'))
    }
  }

  function clearGroupAvatarImage() {
    const state = getState()
    ;(state.draft as any).groupAvatarImage = ''
    ;(state.draft as any).groupAvatarImageCropSrc = ''
    render()
  }

  // ===== Role CRUD =====

  function openNewRoleEditor() {
    const state = getState()
    if (!state.data) return
    const fallbackPid = String(state.data.settings.providers?.[0]?.id || '')

    state.draft.editRoleId = NEW_ROLE_ID
    state.draft.roleName = '新角色'
    state.draft.roleAvatar = '🙂'
    state.draft.roleAvatarImage = ''
    state.draft.roleAvatarImageCropSrc = ''
    state.draft.roleSystemPrompt = ''
    state.draft.roleTemperature = '0.7'
    state.draft.roleProviderId = fallbackPid

    const p = sa.getProvider(fallbackPid)
    const cachedItems = Array.isArray(p?.modelsCache?.items) ? p.modelsCache.items : []
    state.models = { loading: false, error: '', items: cachedItems.slice(0, 300) }
    state.draft.roleModelId = ''
    state.draft.roleCustomModelId = ''

    state.modal = 'role'
    render()
  }

  function createRole() {
    openNewRoleEditor()
  }

  function openRoleEditor(roleId: any) {
    const state = getState()
    if (!state.data) return
    const rid = String(roleId || '')
    const role = state.data.roles.find((r: any) => String(r?.id) === rid)
    if (!role) return
    sa.ensureRoleDefaults(role)

    state.draft.editRoleId = rid
    state.draft.roleName = String(role.name || '')
    state.draft.roleAvatar = String(role.avatar || '')
    state.draft.roleAvatarImage = looksLikeImageDataUrl(role.avatarImage) ? String(role.avatarImage || '') : ''
    state.draft.roleAvatarImageCropSrc = ''
    state.draft.roleSystemPrompt = String(role.systemPrompt || '')
    state.draft.roleTemperature = String(role.temperature ?? 0.7)
    state.draft.roleProviderId = String(role.modelRef?.providerId || '')
    const curModelId = String(role.modelRef?.modelId || '').trim()

    const p = sa.getProvider(state.draft.roleProviderId)
    const cachedItems = Array.isArray(p?.modelsCache?.items) ? p.modelsCache.items : []
    state.models = { loading: false, error: '', items: cachedItems.slice(0, 300) }

    const inCache = !!curModelId && cachedItems.some((x: any) => modelCacheId(x) === curModelId)
    state.draft.roleModelId = inCache ? curModelId : curModelId ? '__custom__' : ''
    state.draft.roleCustomModelId = inCache ? '' : curModelId

    state.modal = 'role'
    render()
  }

  async function saveRoleEditor() {
    const state = getState()
    if (!state.data) return
    const rid = String(state.draft.editRoleId || '')

    const name = String(state.draft.roleName || '').trim() || '未命名角色'
    const avatar = String(state.draft.roleAvatar || '').trim() || '🙂'
    const avatarImage = looksLikeImageDataUrl(state.draft.roleAvatarImage) ? String(state.draft.roleAvatarImage || '') : ''
    const sys = String(state.draft.roleSystemPrompt || '').trim()
    const temperature = clampTemp(state.draft.roleTemperature)
    const providerId = String(state.draft.roleProviderId || '').trim()
    let modelId = String(state.draft.roleModelId || '').trim()
    if (modelId === '__custom__') modelId = String(state.draft.roleCustomModelId || '').trim()

    if (rid === NEW_ROLE_ID) {
      const newRid = uid('r')
      const role = {
        id: newRid,
        name,
        avatar,
        avatarImage,
        systemPrompt: sys,
        temperature,
        modelRef: { providerId, modelId },
        createdAt: now(),
        updatedAt: now(),
      }
      sa.ensureRoleDefaults(role)
      try {
        if (!pushBoxRole) throw new Error('eucli-box 角色写入通道不可用')
        await pushBoxRole({ id: newRid, name, avatar, avatarImage, systemPrompt: sys, temperature, modelRef: { providerId, modelId }, createdAt: role.createdAt, updatedAt: role.updatedAt })
        await refreshBoxCatalogView()
      } catch (e: any) {
        showToast?.('保存角色失败: ' + String(e?.message || e))
        return
      }
      if (!state.data.chatsByRole || typeof state.data.chatsByRole !== 'object') state.data.chatsByRole = {}
      if (!state.data.chatsByRole[newRid]) state.data.chatsByRole[newRid] = { activeChatId: '', chatMetas: [], chats: [] }
      state.draft.activeRoleId = newRid
      closeModal()
      return
    }

    const role = state.data.roles.find((r: any) => String(r?.id) === rid)
    if (!role) return

    const nextRole = { id: role.id, name, avatar, avatarImage, systemPrompt: sys, temperature, modelRef: { providerId, modelId }, createdAt: role.createdAt, updatedAt: now() }
    try {
      if (!pushBoxRole) throw new Error('eucli-box 角色写入通道不可用')
      await pushBoxRole(nextRole)
      await refreshBoxCatalogView()
    } catch (e: any) {
      showToast?.('保存角色失败: ' + String(e?.message || e))
      return
    }

    closeModal()
  }

  async function deleteRole(roleId: any) {
    const state = getState()
    if (!state.data) return
    const rid = String(roleId || '')
    try {
      if (!deleteBoxRole) throw new Error('eucli-box 角色删除通道不可用')
      await deleteBoxRole(rid)
      await refreshBoxCatalogView()
    } catch (e: any) {
      showToast?.('删除角色失败: ' + String(e?.message || e))
      return
    }
    if (state.data.chatsByRole && typeof state.data.chatsByRole === 'object') delete state.data.chatsByRole[rid]
    cleanupFavoriteRefsForTarget('role', rid)

    state.draft.activeRoleId = String(state.data.roles[0]?.id || '')
    if (!Array.isArray((state.data as any).groups) || !(state.data as any).groups.length) {
      ;(state.draft as any).activeTargetKind = 'role'
      ;(state.draft as any).activeGroupId = ''
    }
    render()
  }

  function openNewGroupEditor() {
    showToast?.('群聊能力已交由 eucli-box 接管，客户端本地群聊编辑已禁用')
  }

  function createGroup() {
    openNewGroupEditor()
  }

  function openGroupEditor(groupId: any) {
    showToast?.('群聊能力已交由 eucli-box 接管，客户端本地群聊编辑已禁用')
  }

  function saveGroupEditor() {
    showToast?.('群聊能力已交由 eucli-box 接管，客户端本地群聊编辑已禁用')
  }

  function deleteGroup(groupId: any) {
    showToast?.('群聊能力已交由 eucli-box 接管，客户端本地群聊删除已禁用')
  }

  // ===== Provider CRUD =====

  function openProvidersEditor() {
    const state = getState()
    state.draft.editProviderId = ''
    state.modal = 'providers'
    render()
  }

  function openProviderInlineEditor(providerId: any) {
    const state = getState()
    const p = sa.getProvider(providerId)
    if (!p) return
    state.draft.editProviderId = String(p.id)
    state.draft.providerName = String(p.name || '')
    state.draft.providerBaseUrl = String(p.baseUrl || '')
    state.draft.providerApiKey = String(p.apiKey || '')
    render()
  }

  async function saveProviderInlineEditor() {
    const state = getState()
    const pid = String(state.draft.editProviderId || '')
    const p = sa.getProvider(pid)
    if (!p) return

    const nextName = String(state.draft.providerName || '').replace(/\s+/g, ' ').trim() || '未命名供应商'

    const nextBaseUrl = String(state.draft.providerBaseUrl || '').trim() || 'http://'
    const nextApiKey = String(state.draft.providerApiKey || '').trim()

    const nextProvider = {
      id: p.id,
      name: nextName,
      baseUrl: nextBaseUrl,
      apiKey: nextApiKey,
      protocol: (p as any).protocol || 'openai',
      modelsCache: { items: [], fetchedAt: 0 },
      createdAt: p.createdAt,
      updatedAt: now(),
    }
    try {
      if (!pushBoxProvider) throw new Error('eucli-box 供应商写入通道不可用')
      await pushBoxProvider(nextProvider)
      await refreshBoxCatalogView()
    } catch (e: any) {
      showToast?.('保存供应商失败: ' + String(e?.message || e))
      return
    }

    state.draft.editProviderId = ''
    render()
  }

  async function createProvider() {
    const state = getState()
    if (!state.data) return
    const name = '新供应商（OpenAI 兼容）'
    const pid = uid('p')
    const provider = {
      id: pid,
      name,
      baseUrl: 'http://',
      apiKey: '',
      modelsCache: { items: [], fetchedAt: 0 },
      createdAt: now(),
      updatedAt: now(),
    }
    try {
      if (!pushBoxProvider) throw new Error('eucli-box 供应商写入通道不可用')
      await pushBoxProvider(provider)
      await refreshBoxCatalogView()
    } catch (e: any) {
      showToast?.('创建供应商失败: ' + String(e?.message || e))
      return
    }
    openProviderInlineEditor(pid)
  }

  async function deleteProvider(providerId: any) {
    const state = getState()
    if (!state.data) return
    const pid = String(providerId || '')
    try {
      if (!deleteBoxProvider) throw new Error('eucli-box 供应商删除通道不可用')
      await deleteBoxProvider(pid)
      await refreshBoxCatalogView()
    } catch (e: any) {
      showToast?.('删除供应商失败: ' + String(e?.message || e))
      return
    }
    render()
  }

  // ===== Create chat for active =====

  async function createChatForActiveRole() {
    const state = getState()
    const role = sa.activeRole()
    if (!role) return showToast?.('请先选择角色')
    const rid = String(role.id || '')
    if (!createBoxSession) return showToast?.('eucli-box 会话创建通道不可用')
    let chat: any = null
    try {
      const session = await createBoxSession(rid)
      chat = upsertRoleSessionMeta(rid, session)
      if (!chat) throw new Error('会话数据无效')
    } catch (e: any) {
      showToast?.('创建会话失败: ' + String(e?.message || e))
      return
    }
    state.pendingChat = null
    state.sideTab = 'chats'
    state.draft.input = ''
    state.draft.images = []
    render()
    scrollToBottomSoon()
  }

  function createChatForActiveGroup() {
    showToast?.('群聊能力已交由 eucli-box 接管，客户端本地群聊会话已禁用')
  }

  function createChatForActiveTarget() {
    if (sa.activeTargetKind() === 'group') return createChatForActiveGroup()
    return createChatForActiveRole()
  }

  // ===== Pick chat for active =====

  async function pickChatForActiveRole(chatId: any) {
    const state = getState()
    const role = sa.activeRole()
    if (!role || !state.data) return
    sa.clearPendingChat()
    const box = sa.ensureChatsBoxBare(String(role.id))
    if (!box) return
    const cid = String(chatId || '')
    const exists =
      Array.isArray(box.chatMetas) && box.chatMetas.some((c: any) => String(c?.id || '') === cid) ||
      Array.isArray(box.chats) && box.chats.some((c: any) => String(c?.id || '') === cid)
    if (!cid || !exists) return
    try {
      if (!getBoxSession) throw new Error('eucli-box 会话读取通道不可用')
      const session = await getBoxSession(cid)
      upsertRoleSessionMeta(String(role.id || ''), session)
    } catch (e: any) {
      showToast?.('读取会话失败: ' + String(e?.message || e))
      return
    }
    box.activeChatId = cid
    render()
    scrollToBottomSoon()
  }

  async function pickChatForActiveGroup(chatId: any) {
    void chatId
    showToast?.('群聊能力已交由 eucli-box 接管，客户端本地群聊会话已禁用')
  }

  function pickChatForActiveTarget(chatId: any) {
    if (sa.activeTargetKind() === 'group') return pickChatForActiveGroup(chatId)
    return pickChatForActiveRole(chatId)
  }

  // ===== Rename =====

  function renameChatTitle(roleId: any, chatId: any, title: any) {
    showToast?.('会话重命名必须由 eucli-box 提供，客户端本地会话修改已禁用')
    return

    const state = getState()
    if (!state.data) return
    const rid = String(roleId || '')
    const cid = String(chatId || '')
    if (!rid || !cid) return

    const box = sa.ensureChatsBoxBare(rid)
    if (!box) return
    let t = String(title ?? '').replace(/\s+/g, ' ').trim()
    if (t.length > 80) t = t.slice(0, 80).trim()
    t = t || '新聊天'
    const chats = Array.isArray(box.chats) ? box.chats : []
    const chat = chats.find((c: any) => String(c?.id) === cid) || null
    if (chat) {
      chat.title = t
      chat.updatedAt = now()
      box.chatMetas = upsertChatMeta(box.chatMetas, chatMetaFromChat(chat, '新聊天'), '新聊天')
    } else {
      const old = Array.isArray(box.chatMetas) ? box.chatMetas.find((m: any) => String(m?.id || '') === cid) : null
      box.chatMetas = upsertChatMeta(box.chatMetas, {
        id: cid,
        title: t,
        createdAt: Number(old?.createdAt || now()),
        updatedAt: now(),
        lastMessagePreview: String(old?.lastMessagePreview || ''),
        messageCount: Number(old?.messageCount || 0),
        hasPending: !!old?.hasPending,
      }, '新聊天')
    }

    ;(renameRoleChatInStore?.(rid, cid, t) || save()).catch(() => {})
    render()
  }

  function renameGroupChatTitle(groupId: any, chatId: any, title: any) {
    void groupId
    void chatId
    void title
    showToast?.('群聊能力已交由 eucli-box 接管，客户端本地群聊会话已禁用')
  }

  // ===== Image path collection =====

  function collectChatImagePathSet(chat: any): Set<string> {
    const out = new Set<string>()
    const msgs = Array.isArray(chat?.messages) ? chat.messages : []
    for (const m of msgs) {
      const paths = normImagePaths(m?.images)
      for (const p of paths) {
        const s = String(p || '').trim()
        if (s) out.add(s)
      }
    }
    return out
  }

  function collectOtherChatsImagePathSet(excludeRoleId: string, excludeChatId: string): Set<string> {
    const state = getState()
    const out = new Set<string>()
    if (!state.data) return out
    const byRole: Record<string, any> = state.data.chatsByRole && typeof state.data.chatsByRole === 'object' ? (state.data.chatsByRole as any) : {}
    for (const [rid, box] of Object.entries(byRole)) {
      const chats = Array.isArray((box as any)?.chats) ? (box as any).chats : []
      for (const c of chats) {
        const cid = String((c as any)?.id || '')
        if (String(rid) === String(excludeRoleId || '') && cid === String(excludeChatId || '')) continue
        const paths = collectChatImagePathSet(c)
        for (const p of paths) {
          out.add(p)
          const base = imageBasename(p)
          if (base && base !== p) out.add(base)
        }
      }
    }
    return out
  }

  function collectOtherChatsImagePathSetForGroup(excludeGroupId: string, excludeChatId: string): Set<string> {
    const state = getState()
    const out = new Set<string>()
    if (!state.data) return out

    const byRole: Record<string, any> = state.data.chatsByRole && typeof state.data.chatsByRole === 'object' ? (state.data.chatsByRole as any) : {}
    for (const box of Object.values(byRole)) {
      const chats = Array.isArray((box as any)?.chats) ? (box as any).chats : []
      for (const c of chats) {
        const paths = collectChatImagePathSet(c)
        for (const p of paths) {
          out.add(p)
          const base = imageBasename(p)
          if (base && base !== p) out.add(base)
        }
      }
    }

    const byGroup = (state.data as any).chatsByGroup && typeof (state.data as any).chatsByGroup === 'object' ? (state.data as any).chatsByGroup : {}
    for (const [gid, box] of Object.entries(byGroup)) {
      const chats = Array.isArray((box as any)?.chats) ? (box as any).chats : []
      for (const c of chats) {
        const cid = String((c as any)?.id || '')
        if (String(gid) === String(excludeGroupId || '') && cid === String(excludeChatId || '')) continue
        const paths = collectChatImagePathSet(c)
        for (const p of paths) {
          out.add(p)
          const base = imageBasename(p)
          if (base && base !== p) out.add(base)
        }
      }
    }

    return out
  }

  // ===== Delete chat & images =====

  async function deleteChatImages(paths: string[]): Promise<void> {
    const list = Array.isArray(paths) ? paths : []
    if (!list.length) return
    if (typeof filesImages?.delete !== 'function') return
    for (const p of list) {
      const path = String(p || '').trim()
      if (!path) continue
      await filesImages.delete({ scope: 'data', path }).catch(() => {})
    }
  }

  async function deleteChatForRole(roleId: any, chatId: any) {
    const state = getState()
    if (!state.data) return
    const rid = String(roleId || '')
    const cid = String(chatId || '')
    if (!rid || !cid) return

    const box = sa.ensureChatsBoxBare(rid)
    if (!box) return
    const before = Array.isArray(box.chats) ? box.chats : []
    const target = before.find((c: any) => String(c?.id) === cid) || null
    if (target && chatHasPendingAssistant(target)) return showToast?.('正在生成中，不能删除该会话')
    try {
      if (!deleteBoxSession) throw new Error('eucli-box 会话删除通道不可用')
      await deleteBoxSession(cid)
    } catch (e: any) {
      showToast?.('删除会话失败: ' + String(e?.message || e))
      return
    }

    box.chats = before.filter((c: any) => String(c?.id) !== cid)
    box.chatMetas = removeChatMeta(box.chatMetas, cid, '新聊天')
    cleanupFavoriteRefsForChat('role', rid, cid)
    if (String(box.activeChatId || '') === cid) box.activeChatId = String(box.chatMetas[0]?.id || box.chats[0]?.id || '')

    removeLoadedChat?.('role', rid, cid)
    render()
  }

  function deleteChatForGroup(groupId: any, chatId: any) {
    void groupId
    void chatId
    showToast?.('群聊能力已交由 eucli-box 接管，客户端本地群聊删除已禁用')
  }

  return {
    pickRoleAvatarImage,
    clearRoleAvatarImage,
    pickGroupAvatarImage,
    clearGroupAvatarImage,
    openNewRoleEditor,
    createRole,
    openRoleEditor,
    saveRoleEditor,
    deleteRole,
    openNewGroupEditor,
    createGroup,
    openGroupEditor,
    saveGroupEditor,
    deleteGroup,
    openProvidersEditor,
    openProviderInlineEditor,
    saveProviderInlineEditor,
    createProvider,
    deleteProvider,
    createChatForActiveRole,
    createChatForActiveGroup,
    createChatForActiveTarget,
    pickChatForActiveRole,
    pickChatForActiveGroup,
    pickChatForActiveTarget,
    renameChatTitle,
    renameGroupChatTitle,
    collectChatImagePathSet,
    collectOtherChatsImagePathSet,
    collectOtherChatsImagePathSetForGroup,
    deleteChatImages,
    deleteChatForRole,
    deleteChatForGroup,
    syncRoleSessionsFromBox,
  }
}
