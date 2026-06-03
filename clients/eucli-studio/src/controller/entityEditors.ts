import { now, uid, clamp, clampTemp, normImagePaths } from '../core/utils'
import { createStateAccessors } from '../state/stateAccessors'
import { chatMetaFromChat, removeChatMeta, upsertChatMeta } from '../domain/chatMeta'
import { NEW_ROLE_ID } from '../domain/constants'
import { isAssistantGenerating } from '../domain/assistantRunState'
import { emptyRoleToolPolicy, normalizeRoleToolPolicy } from '../domain/toolPolicy'

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
  saveRoleEntity?: (role: any) => Promise<void>
  removeRoleEntity?: (roleId: any) => Promise<void>
  saveProviderEntity?: (provider: any) => Promise<void>
  removeProviderEntity?: (providerId: any) => Promise<void>
  createRoleSession?: (roleId: string, title?: string) => Promise<{ id: string; title?: string }>
  render: () => void
  closeModal: () => void
  showToast?: (msg: string) => void
  pickImageFiles?: (maxCount?: number) => Promise<any[]>
  filesImages: { delete?: (req: any) => Promise<any> }
  ensureChatLoaded?: (rid: string, cid: string) => Promise<any>
  ensureGroupChatLoaded?: (gid: string, cid: string) => Promise<any>
  renameRoleChatInStore?: (rid: string, cid: string, title: string) => Promise<void>
  removeChatInStore?: (kind: 'role' | 'group', targetId: string, chatId: string) => Promise<void>
  setRoleActiveChatSelection?: (roleId: string, chatId: string) => Promise<void>
  removeLoadedChat?: (kind: 'role' | 'group', targetId: string, chatId: string) => void
  cleanupFavoriteRefsForTarget: (kind: string, targetId: string) => void
  cleanupFavoriteRefsForChat: (targetKind: string, targetId: string, chatId: string) => void
}) {
  const { getState, save, saveRoleEntity, removeRoleEntity, saveProviderEntity, removeProviderEntity, createRoleSession, render, closeModal, showToast, pickImageFiles, filesImages, ensureChatLoaded, ensureGroupChatLoaded, renameRoleChatInStore, removeChatInStore, setRoleActiveChatSelection, removeLoadedChat, cleanupFavoriteRefsForTarget, cleanupFavoriteRefsForChat } = deps
  const sa = createStateAccessors({ getState })

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
    state.draft.roleToolPolicy = emptyRoleToolPolicy()
    state.draft.roleToolWhitelistOpen = false
    state.draft.roleToolAddOpen = false
    state.draft.roleToolSearch = ''
    state.draft.roleToolAddSelected = []
    state.draft.roleToolMenuName = ''
    state.draft.roleToolPermissionName = ''

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
    state.draft.roleToolPolicy = normalizeRoleToolPolicy(role.toolPolicy)
    state.draft.roleToolWhitelistOpen = false
    state.draft.roleToolAddOpen = false
    state.draft.roleToolSearch = ''
    state.draft.roleToolAddSelected = []
    state.draft.roleToolMenuName = ''
    state.draft.roleToolPermissionName = ''
    const curModelId = String(role.modelRef?.modelId || '').trim()

    const p = sa.getProvider(state.draft.roleProviderId)
    const cachedItems = Array.isArray(p?.modelsCache?.items) ? p.modelsCache.items : []
    state.models = { loading: false, error: '', items: cachedItems.slice(0, 300) }

    const inCache = !!curModelId && cachedItems.some((x: any) => String(x) === curModelId)
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

    if (!sys) return showToast?.('请填写角色系统提示词')
    if (!providerId) return showToast?.('请选择角色供应商')
    if (!modelId) return showToast?.('请选择或填写角色模型')
    const toolPolicy = normalizeRoleToolPolicy(state.draft.roleToolPolicy)

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
        toolPolicy,
        createdAt: now(),
        updatedAt: now(),
      }
      try {
        if (typeof saveRoleEntity !== 'function') throw new Error('角色保存通道不可用')
        await saveRoleEntity(role)
        state.data.roles.unshift(role)
        if (!state.data.chatsByRole || typeof state.data.chatsByRole !== 'object') state.data.chatsByRole = {}
        state.data.chatsByRole[newRid] = { activeChatId: '', chatMetas: [], chats: [] }
        state.draft.activeRoleId = newRid
        showToast?.('角色已保存')
        closeModal()
      } catch (e: any) {
        showToast?.(String(e?.message || e || '角色保存失败'))
        render()
      }
      return
    }

    const role = state.data.roles.find((r: any) => String(r?.id) === rid)
    if (!role) return
    const previous = { ...role, modelRef: role.modelRef && typeof role.modelRef === 'object' ? { ...role.modelRef } : role.modelRef }

    role.name = name
    role.avatar = avatar
    role.avatarImage = avatarImage
    role.systemPrompt = sys
    role.temperature = temperature
    role.modelRef = { providerId, modelId }
    role.toolPolicy = toolPolicy
    role.updatedAt = now()

    try {
      await saveRoleEntity?.(role)
      showToast?.('角色已保存')
      closeModal()
    } catch (e: any) {
      Object.assign(role, previous)
      showToast?.(String(e?.message || e || '角色保存失败'))
      render()
    }
  }

  async function deleteRole(roleId: any) {
    const state = getState()
    if (!state.data) return false
    const rid = String(roleId || '')
    if (!rid) return false
    const previousRoles = Array.isArray(state.data.roles) ? state.data.roles.slice() : []
    const previousChatsByRole = state.data.chatsByRole && typeof state.data.chatsByRole === 'object' ? { ...state.data.chatsByRole } : {}
    const previousActiveRoleId = String(state.draft.activeRoleId || '')
    const previousActiveTargetKind = String((state.draft as any).activeTargetKind || '')
    const previousActiveGroupId = String((state.draft as any).activeGroupId || '')
    state.data.roles = state.data.roles.filter((r: any) => String(r?.id) !== rid)
    if (state.data.chatsByRole && typeof state.data.chatsByRole === 'object') delete state.data.chatsByRole[rid]
    cleanupFavoriteRefsForTarget('role', rid)

    state.draft.activeRoleId = String(state.data.roles[0]?.id || '')
    if (!Array.isArray((state.data as any).groups) || !(state.data as any).groups.length) {
      ;(state.draft as any).activeTargetKind = 'role'
      ;(state.draft as any).activeGroupId = ''
    }
    try {
      await removeRoleEntity?.(rid)
      showToast?.('角色已删除')
      render()
      return true
    } catch (e: any) {
      state.data.roles = previousRoles
      state.data.chatsByRole = previousChatsByRole
      state.draft.activeRoleId = previousActiveRoleId
      ;(state.draft as any).activeTargetKind = previousActiveTargetKind
      ;(state.draft as any).activeGroupId = previousActiveGroupId
      showToast?.(String(e?.message || e || '角色删除失败'))
      render()
      return false
    }
  }

  // ===== Group CRUD =====

  function openNewGroupEditor() {
    showToast?.('群组编辑尚未接入 e-b 真实群组根动作，已阻止本地假群组创建')
  }

  function createGroup() {
    openNewGroupEditor()
  }

  function openGroupEditor(groupId: any) {
    if (!String(groupId || '').trim()) return
    showToast?.('群组编辑尚未接入 e-b 真实群组根动作，已阻止本地假群组编辑')
  }

  async function saveGroupEditor() {
    showToast?.('群组保存尚未接入 e-b 真实群组根动作，已阻止本地假群组保存')
  }

  function deleteGroup(groupId: any) {
    const gid = String(groupId || '').trim()
    if (!gid) return false
    showToast?.('群组删除尚未接入 e-b 真实群组根动作，已阻止本地假群组删除')
    return false
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
    state.draft.providerProtocol = String(p.protocol || '')
    render()
  }

  async function saveProviderInlineEditor() {
    const state = getState()
    const pid = String(state.draft.editProviderId || '')
    const p = sa.getProvider(pid)
    if (!p) return

    const desiredName = String(state.draft.providerName || '').replace(/\s+/g, ' ').trim() || '未命名供应商'
    const used = new Set((state.data?.settings?.providers || []).filter((x: any) => x && typeof x === 'object').map((x: any) => String(x.name || '')).filter(Boolean))
    used.delete(String(p.name || ''))
    let nextName = desiredName
    if (used.has(nextName)) {
      let i = 2
      while (used.has(`${desiredName}（${i}）`)) i++
      nextName = `${desiredName}（${i}）`
    }

    const oldBaseUrl = String(p.baseUrl || '').trim()
    const oldApiKey = String(p.apiKey || '').trim()
    const nextBaseUrl = String(state.draft.providerBaseUrl || '').trim() || 'http://'
    const nextApiKey = String(state.draft.providerApiKey || '').trim()
    const nextProtocol = normalizeProviderProtocol(state.draft.providerProtocol)
    const previous = { ...p, modelsCache: p.modelsCache && typeof p.modelsCache === 'object' ? { ...p.modelsCache } : p.modelsCache }

    if (!nextProtocol) {
      showToast?.('请选择供应商协议')
      render()
      return
    }

    try {
      p.name = nextName
      p.baseUrl = nextBaseUrl
      p.apiKey = nextApiKey
      p.protocol = nextProtocol
      if (oldBaseUrl !== nextBaseUrl || oldApiKey !== nextApiKey) p.modelsCache = { items: [], fetchedAt: 0 }
      await saveProviderEntity?.(p)
      state.draft.editProviderId = ''
      showToast?.('供应商已保存')
      render()
    } catch (e: any) {
      Object.assign(p, previous)
      showToast?.(String(e?.message || e || '供应商保存失败'))
      render()
    }
  }

  function createProvider() {
    const state = getState()
    if (!state.data) return
    const desiredName = '新供应商（OpenAI 兼容）'
    const used = new Set(state.data.settings.providers.map((p: any) => String(p?.name || '')).filter(Boolean))
    let name = desiredName
    if (used.has(name)) {
      let i = 2
      while (used.has(`${desiredName}（${i}）`)) i++
      name = `${desiredName}（${i}）`
    }
    const pid = uid('p')
    state.data.settings.providers.unshift({
      id: pid,
      name,
      baseUrl: 'http://',
      apiKey: '',
      protocol: '',
      modelsCache: { items: [], fetchedAt: 0 },
    })
    openProviderInlineEditor(pid)
  }

  function normalizeProviderProtocol(value: any) {
    const protocol = String(value || '').trim()
    return protocol === 'openai' || protocol === 'anthropic' ? protocol : ''
  }

  async function deleteProvider(providerId: any) {
    const state = getState()
    if (!state.data) return false
    const pid = String(providerId || '')
    if (!pid) return false
    if (state.data.settings.providers.length <= 1) {
      showToast?.('至少保留一个供应商')
      return false
    }

    const previousProviders = Array.isArray(state.data.settings.providers) ? state.data.settings.providers.slice() : []
    state.data.settings.providers = state.data.settings.providers.filter((p: any) => String(p?.id) !== pid)

    try {
      await removeProviderEntity?.(pid)
      showToast?.('供应商已删除')
      render()
      return true
    } catch (e: any) {
      state.data.settings.providers = previousProviders
      showToast?.(String(e?.message || e || '供应商删除失败'))
      render()
      return false
    }
  }

  // ===== Create chat for active =====

  async function createChatForActiveRole() {
    const state = getState()
    const role = sa.activeRole()
    if (!role) return showToast?.('请先选择角色')
    const rid = String(role.id || '')
    try {
      if (typeof createRoleSession !== 'function') throw new Error('会话创建通道不可用')
      const session = await createRoleSession(rid, '新聊天')
      const cid = String(session.id || '').trim()
      if (!cid) throw new Error('e-b 未返回会话ID')
      const box = sa.ensureChatsBoxBare(rid)
      if (box) {
        box.activeChatId = cid
        await ensureChatLoaded?.(rid, cid)
      }
      state.sideTab = 'chats'
      state.draft.input = ''
      state.draft.images = []
      ;(state.draft as any).files = []
      render()
      scrollToBottomSoon()
    } catch (e: any) {
      showToast?.(String(e?.message || e || '新建聊天失败'))
      render()
    }
  }

  function createChatForActiveGroup() {
    const group = sa.activeGroup()
    if (!group) return showToast?.('请先选择群组')
    showToast?.('群组会话尚未接入 e-b 真实会话根动作，已阻止本地假会话创建')
  }

  async function createChatForActiveTarget() {
    if (sa.activeTargetKind() === 'group') return createChatForActiveGroup()
    return await createChatForActiveRole()
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
    await ensureChatLoaded?.(String(role.id || ''), cid)
    box.activeChatId = cid
    ;(setRoleActiveChatSelection?.(String(role.id || ''), cid) || save()).catch(() => {})
    render()
    scrollToBottomSoon()
  }

  async function pickChatForActiveGroup(chatId: any) {
    const state = getState()
    const group = sa.activeGroup()
    if (!group || !state.data) return
    sa.clearPendingGroupChat()
    const box = sa.ensureGroupChatsBoxBare(String((group as any).id || ''))
    if (!box) return
    const cid = String(chatId || '')
    const exists =
      Array.isArray(box.chatMetas) && box.chatMetas.some((c: any) => String(c?.id || '') === cid) ||
      Array.isArray(box.chats) && box.chats.some((c: any) => String(c?.id || '') === cid)
    if (!cid || !exists) return
    await ensureGroupChatLoaded?.(String((group as any).id || ''), cid)
    box.activeChatId = cid
    save().catch(() => {})
    render()
    scrollToBottomSoon()
  }

  function pickChatForActiveTarget(chatId: any) {
    if (sa.activeTargetKind() === 'group') return pickChatForActiveGroup(chatId)
    return pickChatForActiveRole(chatId)
  }

  // ===== Rename =====

  async function renameChatTitle(roleId: any, chatId: any, title: any) {
    const state = getState()
    if (!state.data) return false
    const rid = String(roleId || '')
    const cid = String(chatId || '')
    if (!rid || !cid) return false

    const box = sa.ensureChatsBoxBare(rid)
    if (!box) return false
    let t = String(title ?? '').replace(/\s+/g, ' ').trim()
    if (t.length > 80) t = t.slice(0, 80).trim()
    t = t || '新聊天'

    try {
      if (typeof renameRoleChatInStore !== 'function') throw new Error('会话标题保存通道不可用')
      await renameRoleChatInStore(rid, cid, t)
    } catch (e: any) {
      showToast?.(String(e?.message || e || '会话标题保存失败'))
      render()
      return false
    }

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

    render()
    return true
  }

  async function renameGroupChatTitle(groupId: any, chatId: any, _title: any) {
    const state = getState()
    if (!state.data) return false
    const gid = String(groupId || '').trim()
    const cid = String(chatId || '').trim()
    if (!gid || !cid) return false

    showToast?.('群组会话标题尚未接入 e-b 真实群组会话根动作，已阻止本地假修改')
    return false
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

  function deleteChatForRole(roleId: any, chatId: any) {
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

    box.chats = before.filter((c: any) => String(c?.id) !== cid)
    box.chatMetas = removeChatMeta(box.chatMetas, cid, '新聊天')
    cleanupFavoriteRefsForChat('role', rid, cid)
    if (String(box.activeChatId || '') === cid) box.activeChatId = String(box.chatMetas[0]?.id || box.chats[0]?.id || '')

    removeLoadedChat?.('role', rid, cid)
    void removeChatInStore?.('role', rid, cid).catch(() => {})
    void (setRoleActiveChatSelection?.(rid, String(box.activeChatId || '')) || save()).catch(() => {})
    render()
  }

  function deleteChatForGroup(groupId: any, chatId: any) {
    const state = getState()
    if (!state.data) return
    const gid = String(groupId || '').trim()
    const cid = String(chatId || '').trim()
    if (!gid || !cid) return

    const box = sa.ensureGroupChatsBoxBare(gid)
    if (!box) return
    const before = Array.isArray(box.chats) ? box.chats : []
    const target = before.find((c: any) => String(c?.id) === cid) || null
    if (target && chatHasPendingAssistant(target)) return showToast?.('正在生成中，不能删除该会话')

    box.chats = before.filter((c: any) => String(c?.id) !== cid)
    box.chatMetas = removeChatMeta(box.chatMetas, cid, '群聊')
    cleanupFavoriteRefsForChat('group', gid, cid)
    if (String(box.activeChatId || '') === cid) box.activeChatId = String(box.chatMetas[0]?.id || box.chats[0]?.id || '')

    removeLoadedChat?.('group', gid, cid)
    void removeChatInStore?.('group', gid, cid).catch(() => {})
    void save().catch(() => {})
    render()
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
  }
}
