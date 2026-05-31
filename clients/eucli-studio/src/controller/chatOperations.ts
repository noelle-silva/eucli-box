import { now, uid, clamp, trimSlash, isHttpBaseUrl } from '../core/utils'
import {
  MAX_DRAFT_IMAGES,
  MAX_DRAFT_FILES,
  CHAT_DEFAULT_BRANCH_ID,
} from '../domain/constants'
import {
  normalizeBranchId,
  ensureChatBranching,
  ensureChatBranch,
  setChatBranchHeadMid,
  genUniqueBranchId,
  repairChatLinearBranching,
  findChatMessageById,
  findPrevAssistantMidForAssistant,
  findChatBranch,
} from '../domain/branching'
import { chatMetaFromChat } from '../domain/chatMeta'
import { looksLikeImageDataUrl } from '../domain/textProcessing'
import { detectDraftFileKind, addDraftFilePlaceholder } from '../domain/draftFileUtils'
import type { DraftFileKind, DraftFileItem } from '../domain/draftFileUtils'
import { buildMessageModelRef, normalizeChatModelOverride } from '../domain/modelRefUtils'
import { createStateAccessors } from '../state/stateAccessors'
import type { AiChatInternalGateway } from '../gateway/types'
import { groupChatImageRelPath, roleFolderName, roleChatImageRelPath } from '../domain/storageKeys'
import {
  beginAssistantRun,
  checkpointAssistantRun,
  finishAssistantRun,
  isAssistantRunSignalCurrent,
  normalizeAssistantRunSignal,
  ASSISTANT_RUNNING_CONTENT,
} from '../domain/assistantRunState'
import { hasActiveAssistantMessages, listActiveAssistantMessages } from '../domain/chatRunState'
import type { ChatSaveIntent } from '../domain/chatSaveIntent'
import { createAssistantArtifactCleanup } from './assistantArtifactCleanup'

type ChatAttachmentItem = {
  id: string
  name: string
  kind: DraftFileKind
  lang: string
  text: string
  fullLen: number
  sendLen: number
  sendPct: number
}

type ChatMsgGroupRole = '' | 'root' | 'attachment'

function clonePlain<T>(value: T): T {
  const clone = (globalThis as any)?.structuredClone
  if (typeof clone === 'function') return clone(value)
  return JSON.parse(JSON.stringify(value))
}

function restoreObject(target: any, snapshot: any) {
  if (!target || typeof target !== 'object' || !snapshot || typeof snapshot !== 'object') return
  for (const key of Object.keys(target)) {
    try { delete target[key] } catch (_) {}
  }
  Object.assign(target, clonePlain(snapshot))
}

function buildUserTextForBox(rootMsg: any, attachMsgs: any[]): string {
  const base = String(rootMsg?.content || '').trim()
  const attachments = Array.isArray(attachMsgs) ? attachMsgs.flatMap((msg: any) => Array.isArray(msg?.attachments) ? msg.attachments : []) : []
  if (!attachments.length) return base
  const blocks = attachments
    .map((att: any) => {
      const name = String(att?.name || '文件').trim() || '文件'
      const lang = String(att?.lang || 'text').trim() || 'text'
      const text = String(att?.text || '').trim()
      if (!text) return ''
      return `附件：${name}\n\`\`\`${lang}\n${text.replaceAll('```', '``\u200b`')}\n\`\`\``
    })
    .filter(Boolean)
  const extra = blocks.join('\n\n').trim()
  if (!extra) return base
  return base ? `${base}\n\n${extra}` : extra
}

function imageExtFromDataUrl(dataUrl: unknown): string {
  const match = /^data:image\/([a-zA-Z0-9.+-]+);base64,/.exec(String(dataUrl || '').trim())
  if (!match) return 'png'
  const mime = String(match[1] || '').toLowerCase()
  if (mime === 'jpeg' || mime === 'jpg') return 'jpg'
  if (mime === 'png' || mime === 'gif' || mime === 'webp' || mime === 'bmp' || mime === 'svg+xml') return mime === 'svg+xml' ? 'svg' : mime
  return 'png'
}

function chatImageFileName(chatId: unknown, index: number, dataUrl: unknown): string {
  const random = Math.random().toString(36).slice(2, 8)
  return `image-${Date.now().toString(36)}-${random}-${String(chatId || 'chat')}-${Math.max(0, index + 1)}.${imageExtFromDataUrl(dataUrl)}`
}

export function createChatOperations(deps: {
  getState: () => any
  aiGateway: AiChatInternalGateway
  filesImages?: { writeBase64?: (...args: any[]) => Promise<any>; read?: any; delete?: any }
  pickImageFiles?: (maxCount: number) => Promise<any[]>
  loadSplitMeta?: () => Promise<any>
  showToast?: (msg: any) => void
  save: (intent?: ChatSaveIntent) => Promise<void>
  ensureActiveChatLoaded?: () => Promise<any>
  emit: () => void
  render: () => void
  renderComposer: () => void
  scrollToBottomSoon: () => void
  readImageFileAsDataUrl: (file: File) => Promise<string>
  extractTextFromFile: (file: File, kind: string) => Promise<string>
  uiStreamCache: Map<string, any>
}) {
  const { getState, aiGateway, filesImages, pickImageFiles, loadSplitMeta, showToast, save, ensureActiveChatLoaded, emit, render, renderComposer, scrollToBottomSoon, readImageFileAsDataUrl, extractTextFromFile, uiStreamCache } = deps

  const sa = createStateAccessors({ getState })
  const assistantArtifactCleanup = createAssistantArtifactCleanup({
    uiStreamCache,
    resetAssistantRuntime: (messageId) => aiGateway.resetAssistantRuntime(messageId),
  })

  function rejectLocalBusinessPath(message = '该能力必须由 eucli-box 提供，客户端本地旧业务路径已禁用') {
    showToast?.(message)
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

  async function ensureBoxBackedChat(roleId: string) {
    const rid = String(roleId || '').trim()
    if (!rid) throw new Error('请先选择角色')
    const state = getState()
    const box = sa.ensureChatsBoxBare(rid)
    if (!box) throw new Error('会话状态不可用')

    const activeSessionId = String(box.activeChatId || '').trim()
    if (activeSessionId && aiGateway.getBoxSession) {
      const session = await aiGateway.getBoxSession(activeSessionId)
      box.chats = []
      box.chatMetas = [boxSessionToChatMeta(session)]
      box.activeChatId = activeSessionId
      return { id: activeSessionId, boxSessionId: activeSessionId, messages: [] as any[], chatMetasOnly: true } as any
    }

    if (!aiGateway.createBoxSession) throw new Error('eucli-box 会话创建通道不可用')
    const session = await aiGateway.createBoxSession(rid)
    const meta = boxSessionToChatMeta(session)
    box.chats = []
    box.chatMetas = [meta]
    box.activeChatId = meta.id
    return { id: meta.id, boxSessionId: meta.id, messages: [] as any[], chatMetasOnly: true } as any
  }

  function userMessageTextForBox(message: any) {
    if (!message) return ''
    const text = buildUserTextForBox(message, [])
    return String(text || message?.content || '').trim()
  }

  function beginAssistantMessageRun(message: any, streamEnabled: boolean, mode: 'new' | 'regenerate' | 'tool-followup' = 'new') {
    return beginAssistantRun(message, {
      mode,
      stream: streamEnabled,
      resetContent: true,
    })
  }

  function assistantGenerationId(message: any) {
    return String(message?.assistantRun?.generationId || '').trim()
  }

  function checkpointAssistantMessage(message: any, content: unknown) {
    return checkpointAssistantRun(message, content, now())
  }

  function finishAssistantMessage(message: any, content: unknown, status: 'succeeded' | 'failed' | 'canceled' = 'succeeded') {
    return finishAssistantRun(message, content, status, now())
  }

  async function submitChatCompletion(input: any) {
    const kind = String(input?.target?.kind || '').trim() === 'group' ? 'group' : 'role'
    if (kind === 'group') throw new Error('群聊运行必须由 eucli-box 提供，客户端本地群聊路径已禁用')
    return aiGateway.submitRoleChatCompletion(input)
  }

  function pickChatModelRef(role: any, chat: any) {
    const o = normalizeChatModelOverride(chat)
    if (o) {
      const p0 = sa.getProvider(o.providerId)
      if (p0) return { providerId: o.providerId, modelId: o.modelId, overridden: true }
    }
    const providerId = String(role?.modelRef?.providerId || '').trim()
    const modelId = String(role?.modelRef?.modelId || '').trim()
    return { providerId, modelId, overridden: false }
  }

  function chatHasPendingAssistant(chat: any) {
    return hasActiveAssistantMessages(chat)
  }

  function chatHasPendingAssistantInBranch(chat: any, branchId: string, excludeMid?: any) {
    return hasActiveAssistantMessages(chat, { branchId, excludeMid })
  }

  // ============ draft image ============

  function addDraftImage(name: any, dataUrl: any) {
    const state = getState()
    if (!looksLikeImageDataUrl(dataUrl)) return false
    if (!Array.isArray(state.draft.images)) state.draft.images = []
    if (state.draft.images.length >= MAX_DRAFT_IMAGES) return false
    state.draft.images.push({ id: uid('img'), name: String(name || '图片'), dataUrl: String(dataUrl || '') })
    return true
  }

  // ============ pick images ============

  async function pickDraftImages() {
    const state = getState()
    if (state.loading) return
    if (typeof pickImageFiles !== 'function') return showToast?.('未授权：files.pickImages')

    const left = Math.max(0, MAX_DRAFT_IMAGES - (Array.isArray(state.draft.images) ? state.draft.images.length : 0))
    if (!left) return showToast?.(`最多选择 ${MAX_DRAFT_IMAGES} 张图片`)

    try {
      const items = await pickImageFiles(left)
      const list = Array.isArray(items) ? items : []
      let added = 0
      for (const it of list) {
        const name = String(it?.name || '图片')
        const dataUrl = String(it?.dataUrl || '')
        if (addDraftImage(name, dataUrl)) added++
      }
      if (!added) showToast?.('未选择图片')
    } catch (e) {
      showToast?.(String((e as any)?.message || e || '选择图片失败'))
    } finally {
      renderComposer()
    }
  }

  async function addDraftImagesFromFiles(files: File[]) {
    const state = getState()
    if (state.loading) return

    const list = Array.isArray(files)
      ? files.filter((f) => f instanceof File && String(f.type || '').startsWith('image/'))
      : []
    if (!list.length) return showToast?.('未识别到图片')

    if (!Array.isArray(state.draft.images)) state.draft.images = []
    const left = Math.max(0, MAX_DRAFT_IMAGES - state.draft.images.length)
    if (!left) return showToast?.(`最多选择 ${MAX_DRAFT_IMAGES} 张图片`)

    let added = 0
    for (const f of list.slice(0, left)) {
      try {
        const dataUrl = await readImageFileAsDataUrl(f)
        if (addDraftImage(String(f?.name || '图片'), dataUrl)) added++
      } catch (_) {}
    }

    if (!added) showToast?.('未识别到图片')
    renderComposer()
  }

  // ============ add draft files ============

  async function addDraftFilesFromFiles(files: File[]) {
    const state = getState()
    if (state.loading) return
    const list = Array.isArray(files) ? files.filter((f) => f instanceof File) : []
    if (!list.length) return
    if (!Array.isArray(state.draft.files)) state.draft.files = []

    const left = Math.max(0, MAX_DRAFT_FILES - state.draft.files.length)
    if (!left) return showToast?.(`最多选择 ${MAX_DRAFT_FILES} 个文件`)

    let added = 0
    for (const f of list.slice(0, left)) {
      const kind = detectDraftFileKind(f)
      if (!kind) {
        showToast?.(`不支持的文件：${String(f?.name || '文件')}`)
        continue
      }
      const it = addDraftFilePlaceholder(state.draft.files, f, kind)
      if (!it) break
      added++
      emit()
      ;(async () => {
        try {
          const r = await extractTextFromFile(f, kind)
          const cur = Array.isArray(state.draft.files) ? state.draft.files.find((x: any) => String(x?.id || '') === it.id) : null
          if (!cur) return
          cur.text = String(r || '')
          if (!cur.text) cur.error = '未提取到文本'
        } catch (e) {
          const cur = Array.isArray(state.draft.files) ? state.draft.files.find((x: any) => String(x?.id || '') === it.id) : null
          if (!cur) return
          cur.error = String((e as any)?.message || e || '解析失败')
        } finally {
          const cur = Array.isArray(state.draft.files) ? state.draft.files.find((x: any) => String(x?.id || '') === it.id) : null
          if (cur) cur.pending = false
          emit()
        }
      })().catch(() => {})
    }
    if (!added) showToast?.('未选择文件')
    emit()
  }

  // ============ send chat ============

  async function sendChat(opts?: { forkFromMid?: string }) {
    const state = getState()
    if (state.sending || state.loading || !state.data) return

    await ensureActiveChatLoaded?.()

    if (sa.activeTargetKind() === 'group') {
      rejectLocalBusinessPath('群聊运行必须由 eucli-box 提供，客户端本地群聊路径已禁用')
      return
    }

    const role = sa.activeRole()
    if (!role) return
    sa.ensureRoleDefaults(role)

    const input = String(state.draft.input || '').trim()
    const draftImages = Array.isArray(state.draft.images) ? state.draft.images : []
    const draftFiles: DraftFileItem[] = Array.isArray((state.draft as any).files) ? ((state.draft as any).files as any[]) : []
    const hasFiles = draftFiles.length > 0
    if (!input && !draftImages.length && !hasFiles) return showToast?.('输入不能为空')
    if (hasFiles && draftFiles.some((x: any) => !!x?.pending)) return showToast?.('文件解析中，请稍候…')

    const rid = String(role.id || '')
    const chatForModel = state.pendingChat && String(state.pendingChat.roleId || '') === rid ? null : sa.activeChatFromData()
    const picked = pickChatModelRef(role, chatForModel)

    const providerId = String(picked.providerId || '')
    const modelId = String(picked.modelId || '').trim()

    let chat = null

    let assistantMid = ''
    let boxRunAccepted = false
    let rollbackChatSnapshot: any = null
    let rollbackDraftSnapshot: any = null
    try {
      if (draftImages.length && typeof filesImages?.writeBase64 !== 'function') {
        return showToast?.('未授权：files.images.writeBase64')
      }

      state.sending = true
      renderComposer()

      const streamEnabled = !!state.data?.settings?.streamEnabled
      assistantMid = uid('m')

      chat = await ensureBoxBackedChat(rid)
      sa.clearPendingChat()
      if (!chat) throw new Error('创建会话失败')

      const meta = await loadSplitMeta?.().catch(() => null)
      const chatFolder = String(meta?.roleFolders?.[rid] || '').trim() || roleFolderName(role)
      const savedPaths: string[] = []
      for (const [index, img] of draftImages.slice(0, MAX_DRAFT_IMAGES).entries()) {
        const dataUrl = String(img?.dataUrl || '')
        if (!looksLikeImageDataUrl(dataUrl)) continue
        const relPath = roleChatImageRelPath(chatFolder, chat.id, chatImageFileName(chat.id, index, dataUrl))
        const saved = await filesImages!.writeBase64!({ scope: 'data', relPath, overwrite: false, dataUrlOrBase64: dataUrl })
        const path = String(saved || '').trim()
        if (path) savedPaths.push(path)
      }

      const messageModelRef = buildMessageModelRef(providerId, modelId)
      const branching = ensureChatBranching(chat)
      let activeBranchId = normalizeBranchId((branching as any)?.activeBranchId || CHAT_DEFAULT_BRANCH_ID)
      const activeBranch = ensureChatBranch(chat, activeBranchId)
      let parentMid = String(activeBranch?.headMid || '').trim()

      const rid2 = String(role.id || '')
      const draft0 = state.branchDraft && typeof state.branchDraft === 'object' ? (state.branchDraft as any) : null
      const draft =
        draft0 && String(draft0?.roleId || '') === rid2 && String(draft0?.chatId || '') === String(chat.id || '') ? draft0 : null

      const forkOverride = !draft ? String(opts?.forkFromMid || '').trim() : ''

      let draftForkMid = ''
      let draftNewBranchId = ''
      if (draft || forkOverride) throw new Error('分支运行必须由 eucli-box 提供，客户端本地分支路径已禁用')
      if (!parentMid) {
        const items0 = Array.isArray(chat.messages) ? chat.messages : []
        parentMid = items0.length ? String(items0[items0.length - 1]?.id || '') : ''
      }

      if (activeBranchId !== CHAT_DEFAULT_BRANCH_ID) throw new Error('分支运行必须由 eucli-box 提供，客户端本地分支路径已禁用')

      if (chatHasPendingAssistantInBranch(chat, activeBranchId)) throw new Error('该分支正在生成中，请先停止或等待完成')

      rollbackChatSnapshot = clonePlain(chat)
      rollbackDraftSnapshot = {
        input: state.draft.input,
        images: clonePlain(state.draft.images || []),
        files: clonePlain((state.draft as any).files || []),
        branchDraft: clonePlain(state.branchDraft),
      }

      const wasEmpty = !Array.isArray(chat.messages) || chat.messages.length === 0
      const userText = String(input || '').trim()
      const hasUserMain = !!userText || savedPaths.length > 0

      const groupId = hasFiles ? uid('g') : ''
      const rootMid = uid('m')

      const attachMsgs: any[] = []
      if (hasFiles) {
        for (const f of draftFiles) {
          if (!f || f.pending) continue
          if (String(f?.error || '')) continue
          const name = String(f?.name || '文件')
          const kind = String(f?.kind || 'txt') as DraftFileKind
          const lang = kind === 'md' || kind === 'ppt' ? 'markdown' : 'text'
          const raw = String(f?.text || '').trim()
          const fullLen = raw.length
          if (!raw) continue

          const pct0 = Math.round(Number(f?.sendPct ?? 100))
          const pct = clamp(pct0, 0, 100)
          const sendLen = Math.max(0, Math.ceil((fullLen * pct) / 100))
          const snippetRaw = sendLen >= fullLen ? raw : raw.slice(0, sendLen).trimEnd()
          if (!snippetRaw.trim()) continue

          const att: ChatAttachmentItem = {
            id: uid('att'),
            name,
            kind,
            lang,
            text: snippetRaw,
            fullLen,
            sendLen,
            sendPct: pct,
          }
          const mid = uid('m')
          attachMsgs.push({
            id: mid,
            role: 'user',
            content: `附件：${name}`,
            attachments: [att],
            groupId,
            groupRole: 'attachment' as ChatMsgGroupRole,
            groupParentMid: rootMid,
            branchId: activeBranchId,
            parentMid,
            createdAt: now(),
          })
          parentMid = mid
        }
      }

      if (!hasUserMain && !attachMsgs.length) throw new Error('没有可发送的内容（文件解析失败或为空）')

      const rootMsg: any = {
        id: rootMid,
        role: 'user',
        content: hasUserMain ? userText : attachMsgs.length ? '（附件）' : userText,
        images: savedPaths,
        branchId: activeBranchId,
        parentMid,
        createdAt: now(),
      }
      if (attachMsgs.length) {
        rootMsg.groupId = groupId
        rootMsg.groupRole = 'root' as ChatMsgGroupRole
        rootMsg.groupParentMid = ''
      }
      parentMid = rootMid

      chat.messages.push(...attachMsgs, rootMsg)
      chat.updatedAt = now()
      if (wasEmpty && String(chat.title || '') === '新聊天') {
        const t = userText.replace(/\s+/g, ' ').trim()
        const firstAttName = attachMsgs.length ? String((attachMsgs[0] as any)?.attachments?.[0]?.name || '').trim() : ''
        const base = t || (savedPaths.length ? '图片' : firstAttName || (hasFiles ? '文件' : '新聊天'))
        chat.title = base.length > 16 ? base.slice(0, 16) + '…' : base || '新聊天'
      }

      state.draft.input = ''
      state.draft.images = []
      ;(state.draft as any).files = []
      if (draft) state.branchDraft = null

      chat.messages.push({
        id: assistantMid,
        role: 'assistant',
        content: ASSISTANT_RUNNING_CONTENT,
        branchId: activeBranchId,
        parentMid,
        createdAt: now(),
        modelRef: messageModelRef,
      })
      const assistantMsg = chat.messages[chat.messages.length - 1]
      beginAssistantMessageRun(assistantMsg, streamEnabled, 'new')
      const generationId = assistantGenerationId(assistantMsg)
      chat.updatedAt = now()
      setChatBranchHeadMid(chat, activeBranchId, assistantMid)
      repairChatLinearBranching(chat)

      const jobStub: any = {
        kind: 'openai.chat.completions',
        roleId: String(role.id || ''),
        chatId: String(chat.id || ''),
        assistantMid,
        generationId,
        branchId: activeBranchId,
        stream: streamEnabled,
      }
      jobStub.message = buildUserTextForBox(rootMsg, attachMsgs)
      jobStub.sessionId = String((chat as any)?.boxSessionId || '')
      await submitChatCompletion({
        target: {
          kind: 'role',
          roleId: String(role.id || ''),
          chatId: String(chat.id || ''),
          branchId: activeBranchId,
          assistantMid,
          generationId,
        } as any,
        stream: streamEnabled,
        jobStub,
      })
      boxRunAccepted = true
    } catch (e) {
      const msg = String((e as any)?.message || e || '请求失败')
      if (rollbackChatSnapshot && !boxRunAccepted) {
        restoreObject(chat, rollbackChatSnapshot)
        if (rollbackDraftSnapshot) {
          state.draft.input = rollbackDraftSnapshot.input
          state.draft.images = rollbackDraftSnapshot.images
          ;(state.draft as any).files = rollbackDraftSnapshot.files
          state.branchDraft = rollbackDraftSnapshot.branchDraft
        }
      } else {
        const items = Array.isArray(chat?.messages) ? chat.messages : []
        const am = assistantMid ? items.find((m: any) => String(m?.id || '') === assistantMid) : null
        if (am) {
          finishAssistantMessage(am, `（请求失败：${msg}）`, 'failed')
        }
      }
      showToast?.(msg)
    } finally {
      state.sending = false
      render()
    }
  }

  // ============ send group chat ============

  async function sendGroupChat(_opts?: { forkFromMid?: string }) {
    void _opts
    rejectLocalBusinessPath('群聊运行必须由 eucli-box 提供，客户端本地群聊路径已禁用')
  }

  // ============ stop sending ============

  async function stopSending() {
    const state = getState()
    if (state.loading) return

    await ensureActiveChatLoaded?.()

    const kind = sa.activeTargetKind()
    const roleId = String(sa.activeRole()?.id || '')
    const groupId = String((sa.activeGroup() as any)?.id || '')
    const chatId = String(sa.activeChatFromData()?.id || '')
    if (!state.data || !chatId || (kind === 'role' && !roleId) || (kind === 'group' && !groupId)) return

    const chat = kind === 'group' ? sa.findGroupChatByIds(groupId, chatId) : sa.findChatByIds(roleId, chatId)
    if (!chat) return

    const branching = ensureChatBranching(chat)
    const activeBranchId = normalizeBranchId((branching as any)?.activeBranchId || CHAT_DEFAULT_BRANCH_ID)
    const activeRefs = listActiveAssistantMessages(chat, { branchId: activeBranchId })
    const refs = activeRefs.length ? activeRefs : listActiveAssistantMessages(chat)
    if (!refs.length) return showToast?.('当前会话没有正在生成的消息')

    try {
      await Promise.all(refs.map(({ mid }) => aiGateway.cancelAssistant(mid).catch(() => undefined)))

      for (const ref of refs) {
        const mid = ref.mid
        const m = ref.message
        let text = ''
        try {
          const s = await aiGateway.readAssistantStream(mid)
          const signal = normalizeAssistantRunSignal(s)
          if (signal && isAssistantRunSignalCurrent(m, signal)) text = String(signal.text || '')
        } catch (_) {}

        if (!text) {
          try {
            const cached = normalizeAssistantRunSignal((uiStreamCache as any)?.get?.(mid))
            if (cached && isAssistantRunSignalCurrent(m, cached) && cached.text) text = cached.text
          } catch (_) {}
        }
        if (!text) {
          const cur = String((m as any)?.content || '').trim()
          if (cur && cur !== ASSISTANT_RUNNING_CONTENT) text = cur
        }

        finishAssistantMessage(m, text || '（已停止）', 'canceled')
        uiStreamCache.delete(mid)
        await aiGateway.resetAssistantRuntime(mid).catch(() => undefined)
      }

      chat.updatedAt = now()
      repairChatLinearBranching(chat)
      await save()
    } catch (e) {
      showToast?.(String((e as any)?.message || e || '停止失败'))
    } finally {
      state.sending = false
      emit()
    }
  }

  // ============ regenerate assistant message ============

  async function regenerateAssistantMessage(assistantMid: any) {
    const state = getState()
    if (state.sending || state.loading || !state.data) return

    await ensureActiveChatLoaded?.()

    if (sa.activeTargetKind() === 'group') {
      await regenerateGroupAssistantMessage(String(assistantMid || ''))
      return
    }

    const role = sa.activeRole()
    const chat = sa.activeChatFromData()
    if (!role || !chat) return
    sa.ensureRoleDefaults(role)

    const mid = String(assistantMid || '').trim()
    if (!mid) return

    const picked = pickChatModelRef(role, chat)
    const providerId = String(picked.providerId || '')
    const modelId = String(picked.modelId || '').trim()

    try {
      state.sending = true
      renderComposer()

      const msgs = Array.isArray(chat.messages) ? chat.messages : []
      const aiIndex = msgs.findIndex((m: any) => String(m?.id || '') === mid)
      if (aiIndex < 0) throw new Error('未找到该消息')

      const target = msgs[aiIndex]
      if (!target || target.role !== 'assistant') throw new Error('只能重新生成 AI 回复')
      if (hasActiveAssistantMessages({ messages: [target] })) throw new Error('该消息正在生成中')
      const branching = ensureChatBranching(chat)
      const activeBranchId = normalizeBranchId((branching as any)?.activeBranchId || CHAT_DEFAULT_BRANCH_ID)
      const branchId = normalizeBranchId((target as any)?.branchId || activeBranchId)
      if (chatHasPendingAssistantInBranch(chat, branchId, mid)) throw new Error('该分支正在生成中，请先停止或等待完成')

      let userMid = String((target as any)?.parentMid || '').trim()
      let userMsg = userMid ? msgs.find((m: any) => String(m?.id || '') === userMid) || null : null
      if (!userMsg || userMsg.role !== 'user') {
        let userIndex = -1
        for (let i = aiIndex - 1; i >= 0; i--) {
          const m = msgs[i]
          if (m && m.role === 'user') {
            userIndex = i
            break
          }
        }
        if (userIndex < 0) throw new Error('未找到对应的用户消息')
        userMsg = msgs[userIndex]
        userMid = String(userMsg?.id || '').trim()
      }

      const streamEnabled = !!state.data?.settings?.streamEnabled
      beginAssistantMessageRun(target, streamEnabled, 'regenerate')
      const generationId = assistantGenerationId(target)
      target.modelRef = buildMessageModelRef(providerId, modelId)
      chat.updatedAt = now()
      repairChatLinearBranching(chat)

      try {
        await aiGateway.resetAssistantRuntime(mid)
      } catch (_) {}

      const jobStub: any = {
        kind: 'openai.chat.completions',
        roleId: String(role.id || ''),
        chatId: String(chat.id || ''),
        assistantMid: mid,
        generationId,
        cutoffMid: mid,
        branchId,
        stream: streamEnabled,
      }
      jobStub.message = userMessageTextForBox(userMsg)
      jobStub.sessionId = String((chat as any)?.boxSessionId || chat.id || '')
      await submitChatCompletion({
        target: {
          kind: 'role',
          roleId: String(role.id || ''),
          chatId: String(chat.id || ''),
          branchId,
          assistantMid: mid,
          generationId,
        } as any,
        stream: streamEnabled,
        jobStub,
      })
    } catch (e) {
      const msg = String((e as any)?.message || e || '请求失败')
      const items = Array.isArray(chat.messages) ? chat.messages : []
      const am = mid ? items.find((m: any) => String(m?.id || '') === mid) : null
      if (am) {
        finishAssistantMessage(am, `（请求失败：${msg}）`, 'failed')
      }
      showToast?.(msg)
    } finally {
      state.sending = false
      render()
    }
  }

  // ============ regenerate group assistant message ============

  async function regenerateGroupAssistantMessage(assistantMid: string) {
    void assistantMid
    rejectLocalBusinessPath('群聊运行必须由 eucli-box 提供，客户端本地群聊路径已禁用')
  }

  // ============ reply from user message ============

  async function replyFromUserMessage(userMid: any) {
    const state = getState()
    if (state.sending || state.loading || !state.data) return

    await ensureActiveChatLoaded?.()

    if (sa.activeTargetKind() === 'group') {
      await replyFromUserMessageInGroup(String(userMid || ''))
      return
    }

    const role = sa.activeRole()
    const chat = sa.activeChatFromData()
    if (!role || !chat) return
    sa.ensureRoleDefaults(role)

    const mid = String(userMid || '').trim()
    if (!mid) return

    const picked = pickChatModelRef(role, chat)
    const providerId = String(picked.providerId || '')
    const modelId = String(picked.modelId || '').trim()

    let assistantMid = ''
    try {
      state.sending = true
      renderComposer()

      const msgs = Array.isArray(chat.messages) ? chat.messages : []
      const userIndex = msgs.findIndex((m: any) => String(m?.id || '') === mid)
      if (userIndex < 0) throw new Error('未找到该消息')

      const target = msgs[userIndex]
      if (!target || target.role !== 'user') throw new Error('只能从用户消息发起重新回复')

      const streamEnabled = !!state.data?.settings?.streamEnabled
      const branching = ensureChatBranching(chat)
      const activeBranchId = normalizeBranchId((branching as any)?.activeBranchId || CHAT_DEFAULT_BRANCH_ID)
      if (chatHasPendingAssistantInBranch(chat, activeBranchId)) throw new Error('该分支正在生成中，请先停止或等待完成')
      const messageModelRef = buildMessageModelRef(providerId, modelId)
      assistantMid = uid('m')
      msgs.splice(userIndex + 1, 0, {
        id: assistantMid,
        role: 'assistant',
        content: ASSISTANT_RUNNING_CONTENT,
        branchId: activeBranchId,
        parentMid: mid,
        createdAt: now(),
        modelRef: messageModelRef,
      })
      beginAssistantMessageRun(msgs[userIndex + 1], streamEnabled, 'new')
      const generationId = assistantGenerationId(msgs[userIndex + 1])
      chat.messages = msgs
      chat.updatedAt = now()
      setChatBranchHeadMid(chat, activeBranchId, assistantMid)
      repairChatLinearBranching(chat)

      const jobStub: any = {
        kind: 'openai.chat.completions',
        roleId: String(role.id || ''),
        chatId: String(chat.id || ''),
        assistantMid,
        generationId,
        cutoffMid: assistantMid,
        branchId: activeBranchId,
        stream: streamEnabled,
      }
      jobStub.message = userMessageTextForBox(target)
      jobStub.sessionId = String((chat as any)?.boxSessionId || chat.id || '')
      await submitChatCompletion({
        target: {
          kind: 'role',
          roleId: String(role.id || ''),
          chatId: String(chat.id || ''),
          branchId: activeBranchId,
          assistantMid,
          generationId,
        } as any,
        stream: streamEnabled,
        jobStub,
      })
    } catch (e) {
      const msg = String((e as any)?.message || e || '请求失败')
      const items = Array.isArray(chat?.messages) ? chat.messages : []
      const am = assistantMid ? items.find((m: any) => String(m?.id || '') === assistantMid) : null
      if (am) {
        finishAssistantMessage(am, `（请求失败：${msg}）`, 'failed')
      }
      showToast?.(msg)
    } finally {
      state.sending = false
      render()
    }
  }

  // ============ reply from user message in group ============

  async function replyFromUserMessageInGroup(userMid: string) {
    rejectLocalBusinessPath('群聊运行必须由 eucli-box 提供，客户端本地群聊路径已禁用')
    return

    const state = getState()
    if (state.sending || state.loading || !state.data) return

    await ensureActiveChatLoaded?.()

    const group = sa.activeGroup()
    const chat = sa.activeChatFromData()
    if (!group || !chat) return

    const groupId = String((group as any)?.id || '').trim()
    const chatId = String((chat as any)?.id || '').trim()
    const mid = String(userMid || '').trim()
    if (!groupId || !chatId || !mid) return

    const roles = Array.isArray(state.data.roles) ? state.data.roles : []
    const roleById = new Map<string, any>()
    for (const r of roles) {
      const rid = String(r?.id || '').trim()
      if (!rid || roleById.has(rid)) continue
      roleById.set(rid, r)
    }

    const member0 = Array.isArray((group as any).memberRoleIds) ? ((group as any).memberRoleIds as any[]) : []
    const memberRoleIds = member0.map((x: any) => String(x || '').trim()).filter((x: any) => !!x && roleById.has(x)).slice(0, 50)
    if (!memberRoleIds.length) return showToast?.('该群组还没有成员角色')

    const extractAtMentionNames = (text: string) => {
      const t = String(text || '')
      if (!t) return [] as string[]
      const out: string[] = []
      const re = /@\{([^\}\r\n]{1,80})\}/g
      let m: RegExpExecArray | null = null
      while ((m = re.exec(t))) {
        const name = String(m[1] || '').trim()
        if (name) out.push(name)
      }
      return out
    }

    const buildAtMentionSpeakerRoleIds = (text: string) => {
      const names = extractAtMentionNames(text)
      if (!names.length) return [] as string[]
      const idByName = new Map<string, string>()
      for (const rid of memberRoleIds) {
        const r = roleById.get(rid) || null
        const name = String((r as any)?.name || '').trim()
        if (!name || idByName.has(name)) continue
        idByName.set(name, rid)
      }
      const out: string[] = []
      const seen = new Set<string>()
      for (const name of names) {
        const rid = idByName.get(name) || ''
        if (!rid || seen.has(rid)) continue
        seen.add(rid)
        out.push(rid)
      }
      return out
    }

    const mode = String((group as any).mode || '').trim() === 'random' ? 'random' : 'roundRobin'

    const pickRandomRolesOnce = () => {
      const randomCfg = (group as any).random && typeof (group as any).random === 'object' ? (group as any).random : {}
      const weights0 = (randomCfg as any).weightsByRoleId && typeof (randomCfg as any).weightsByRoleId === 'object' ? (randomCfg as any).weightsByRoleId : {}
      let minCount = Number((randomCfg as any).minCount ?? 1)
      let maxCount = Number((randomCfg as any).maxCount ?? 2)
      if (!isFinite(minCount)) minCount = 1
      if (!isFinite(maxCount)) maxCount = 2
      minCount = clamp(Math.round(minCount), 1, 20)
      maxCount = clamp(Math.round(maxCount), 1, 20)
      if (maxCount < minCount) maxCount = minCount

      const pool = memberRoleIds
        .map((rid) => {
          const w = Number((weights0 as any)[rid] ?? 1)
          const weight = isFinite(w) && w >= 0 ? w : 1
          return { rid, weight }
        })
        .filter((x) => x.weight > 0)

      const candidates = pool.length ? pool.slice() : memberRoleIds.map((rid) => ({ rid, weight: 1 }))
      const maxK = Math.max(1, Math.min(candidates.length, maxCount))
      const minK = Math.max(1, Math.min(maxK, minCount))
      const k = minK + Math.floor(Math.random() * (maxK - minK + 1))

      const chosen: string[] = []
      const bag = candidates.slice()
      for (let i = 0; i < k && bag.length; i++) {
        let sum = 0
        for (const it of bag) sum += it.weight
        if (!(sum > 0)) break
        let r = Math.random() * sum
        let idx = -1
        for (let j = 0; j < bag.length; j++) {
          r -= bag[j].weight
          if (r <= 0) {
            idx = j
            break
          }
        }
        if (idx < 0) idx = bag.length - 1
        const picked = bag.splice(idx, 1)[0]
        if (picked?.rid) chosen.push(String(picked.rid))
      }
      return chosen.length ? chosen : memberRoleIds.slice(0, 1)
    }

    try {
      state.sending = true
      renderComposer()

      const msgs = Array.isArray(chat.messages) ? chat.messages : []
      const userIndex = msgs.findIndex((m: any) => String(m?.id || '') === mid)
      if (userIndex < 0) throw new Error('未找到该消息')

      const target = msgs[userIndex]
      if (!target || target.role !== 'user') throw new Error('只能从用户消息发起重新回复')

      const userText = String((target as any)?.content || '').trim()
      const atMentionSpeakerRoleIds = buildAtMentionSpeakerRoleIds(userText)

      const speakerRoleIds = (() => {
        if (atMentionSpeakerRoleIds.length) return atMentionSpeakerRoleIds
        if (mode === 'random') return pickRandomRolesOnce()
        const order0 = Array.isArray((group as any).roundRobinOrder) ? (group as any).roundRobinOrder : []
        const order = order0.map((x: any) => String(x || '').trim()).filter((x: any) => !!x && memberRoleIds.includes(x))
        return order.length ? order : memberRoleIds.slice()
      })()

      // 校验每个参与发言的角色是否可用（避免插入"生成中…"但后台不接单）
      for (const rid of speakerRoleIds) {
        const r = roleById.get(String(rid || ''))
        if (!r) throw new Error('群组成员角色不存在')
        sa.ensureRoleDefaults(r)
        const picked = pickChatModelRef(r, null)
        const providerId = String(picked.providerId || '')
        const modelId = String(picked.modelId || '').trim()
        const p = sa.getProvider(providerId)
        if (!p) throw new Error(`未找到供应商：${String((r as any).name || '角色')}`)
        const baseUrl = trimSlash(p.baseUrl || '')
        const apiKey = String(p.apiKey || '').trim()
        if (!isHttpBaseUrl(baseUrl)) throw new Error(`请先为「${String((r as any).name || '角色')}」配置 Base URL（http/https）`)
        if (!apiKey) throw new Error(`请先为「${String((r as any).name || '角色')}」配置 API Key`)
        if (!modelId) throw new Error(`请先为「${String((r as any).name || '角色')}」选择模型ID`)
      }

      const streamEnabled = !!state.data?.settings?.streamEnabled
      const branching = ensureChatBranching(chat)
      const activeBranchId = normalizeBranchId((branching as any)?.activeBranchId || CHAT_DEFAULT_BRANCH_ID)
      const desiredBranchId = normalizeBranchId((target as any)?.branchId || activeBranchId)
      if (chatHasPendingAssistantInBranch(chat, desiredBranchId)) throw new Error('该分支正在生成中，请先停止或等待完成')

      const toInsert: any[] = []
      let parentMid = mid
      for (const rid of speakerRoleIds) {
        const rid0 = String(rid || '')
        const speakerRole = roleById.get(rid0)
        const picked = pickChatModelRef(speakerRole, null)
        const messageModelRef = buildMessageModelRef(picked.providerId, picked.modelId)
        const assistantMid = uid('m')
        toInsert.push({
          id: assistantMid,
          role: 'assistant',
          speakerRoleId: rid0,
          content: ASSISTANT_RUNNING_CONTENT,
          branchId: desiredBranchId,
          parentMid,
          createdAt: now(),
          modelRef: messageModelRef,
        })
        beginAssistantMessageRun(toInsert[toInsert.length - 1], streamEnabled, 'new')
        parentMid = assistantMid
      }

      if (!toInsert.length) throw new Error('未选中任何发言角色')

      msgs.splice(userIndex + 1, 0, ...toInsert)
      chat.messages = msgs
      chat.updatedAt = now()
      setChatBranchHeadMid(chat, desiredBranchId, String(toInsert[toInsert.length - 1]?.id || ''))
      repairChatLinearBranching(chat)

      await save()

      await aiGateway.submitManyChatCompletions(
        toInsert
          .map((am: any) => {
            const assistantMid = String(am?.id || '').trim()
            const roleId = String(am?.speakerRoleId || '').trim()
            const generationId = assistantGenerationId(am)
            if (!assistantMid || !roleId) return null
            const jobStub: any = {
              kind: 'openai.chat.completions',
              targetKind: 'group',
              groupId,
              roleId,
              chatId,
              assistantMid,
              generationId,
              cutoffMid: assistantMid,
              branchId: desiredBranchId,
              stream: streamEnabled,
            }
            return {
              target: {
                kind: 'group',
                groupId,
                roleId,
                chatId,
                branchId: desiredBranchId,
                assistantMid,
                generationId,
              } as any,
              stream: streamEnabled,
              jobStub,
            }
          })
          .filter((x: any): x is any => !!x)
      )
    } catch (e) {
      const msg = String((e as any)?.message || e || '请求失败')
      showToast?.(msg)
    } finally {
      state.sending = false
      render()
    }
  }

  // ============ create parallel branch from assistant message ============

  async function createParallelBranchFromAssistantMessage(assistantMid: any) {
    rejectLocalBusinessPath('分支能力必须由 eucli-box 提供，客户端本地分支路径已禁用')
    return

    const state = getState()
    if (state.sending || state.loading || !state.data) return

    await ensureActiveChatLoaded?.()

    const role = sa.activeRole()
    const chat = sa.activeChatFromData()
    if (!role || !chat) return
    sa.ensureRoleDefaults(role)

    const mid = String(assistantMid || '').trim()
    if (!mid) return

    const msgs = Array.isArray(chat.messages) ? chat.messages : []
    const target = msgs.find((m: any) => String(m?.id || '') === mid) || null
    if (!target || target.role !== 'assistant') return showToast?.('只能从 AI 消息新建分支')
    if (hasActiveAssistantMessages({ messages: [target] })) return showToast?.('该消息正在生成中')

    const userMid0 = String((target as any)?.parentMid || '').trim()
    const userMsg = userMid0 ? msgs.find((m: any) => String(m?.id || '') === userMid0) || null : null
    if (!userMsg || userMsg.role !== 'user') return showToast?.('未找到对应的用户消息')

    let prevAiMid = ''
    const p0 = String((userMsg as any)?.parentMid || '').trim()
    const pMsg = p0 ? msgs.find((m: any) => String(m?.id || '') === p0) || null : null
    if (pMsg && pMsg.role === 'assistant') prevAiMid = String(pMsg.id || '')
    else {
      const idx = msgs.findIndex((m: any) => String(m?.id || '') === userMid0)
      if (idx >= 0) {
        for (let i = idx - 1; i >= 0; i--) {
          const m = msgs[i]
          if (m && m.role === 'assistant') {
            prevAiMid = String(m.id || '')
            break
          }
        }
      }
    }

    if (!prevAiMid) return showToast?.('未找到上一条 AI 消息，无法新建分支')

    state.branchDraft = {
      roleId: String(role.id || ''),
      chatId: String(chat.id || ''),
      forkFromMid: prevAiMid,
      sourceAssistantMid: mid,
      createdAt: now(),
    }
    render()
    scrollToBottomSoon()
  }

  // ============ switch branch by assistant sibling ============

  async function switchBranchByAssistantSibling(assistantMid: any, delta: any) {
    rejectLocalBusinessPath('分支能力必须由 eucli-box 提供，客户端本地分支路径已禁用')
    return

    const state = getState()
    if (state.loading || !state.data) return

    await ensureActiveChatLoaded?.()

    const chat = sa.activeChatFromData()
    if (!chat) return

    const mid = String(assistantMid || '').trim()
    if (!mid) return

    const d = Math.sign(Number(delta || 0))
    if (!d) return

    const target = findChatMessageById(chat, mid)
    if (!target || String((target as any).role || '') !== 'assistant') return
    const prevAiMid = findPrevAssistantMidForAssistant(chat, mid)
    if (!prevAiMid) return

    const msgs = Array.isArray((chat as any)?.messages) ? (chat as any).messages : []
    const byId = new Map<string, any>()
    for (const m of msgs) {
      const id = String(m?.id || '').trim()
      if (!id || byId.has(id)) continue
      byId.set(id, m)
    }

    let sibs = msgs.filter((m: any) => {
      if (!m || m.role !== 'assistant') return false
      const userMid = String((m as any)?.parentMid || '').trim()
      if (!userMid) return false
      const u = byId.get(userMid) || null
      if (!u || u.role !== 'user') return false
      const p = String((u as any)?.parentMid || '').trim()
      if (!p) return false
      const pa = byId.get(p) || null
      if (!pa || pa.role !== 'assistant') return false
      return String(pa?.id || '').trim() === prevAiMid
    })

    if (sibs.length < 2) {
      const alt: any[] = []
      for (const m of msgs) {
        if (!m || m.role !== 'assistant') continue
        const id = String(m?.id || '').trim()
        if (!id) continue
        const p = findPrevAssistantMidForAssistant(chat, id)
        if (p && p === prevAiMid) alt.push(m)
        if (alt.length >= 80) break
      }
      sibs = alt
    }

    sibs.sort((a: any, b: any) => {
      const da = Number(a?.createdAt || 0)
      const db = Number(b?.createdAt || 0)
      if (da !== db) return da - db
      return String(a?.id || '').localeCompare(String(b?.id || ''))
    })

    if (sibs.length < 2) return

    const i0 = sibs.findIndex((m: any) => String(m?.id || '') === mid)
    if (i0 < 0) return

    const len = sibs.length
    const i = (i0 + d + len) % len
    const picked = sibs[i]
    const pickedMid = String(picked?.id || '').trim()
    const pickedBranchId = normalizeBranchId((picked as any)?.branchId || CHAT_DEFAULT_BRANCH_ID)
    if (!pickedMid || !pickedBranchId) return

    const branching = ensureChatBranching(chat)
    if (!branching) return
    ensureChatBranch(chat, pickedBranchId)
    ;(branching as any).activeBranchId = pickedBranchId
    ;(chat as any).branching = branching

    const b = findChatBranch(chat, pickedBranchId)
    if (b && !String((b as any)?.headMid || '').trim()) (b as any).headMid = pickedMid

    save().catch(() => {})
    const draft0 = state.branchDraft && typeof state.branchDraft === 'object' ? (state.branchDraft as any) : null
    if (draft0 && String(draft0?.roleId || '') === String(sa.activeRole()?.id || '') && String(draft0?.chatId || '') === String(chat.id || '')) {
      state.branchDraft = null
    }
    render()
    scrollToBottomSoon()
  }

  // ============ set active branch ============

  async function setActiveBranch(branchId: any) {
    rejectLocalBusinessPath('分支能力必须由 eucli-box 提供，客户端本地分支路径已禁用')
    return

    const state = getState()
    if (state.loading || !state.data) return

    await ensureActiveChatLoaded?.()

    const chat = sa.activeChatFromData()
    if (!chat) return

    const bid = normalizeBranchId(branchId || CHAT_DEFAULT_BRANCH_ID)
    const branching = ensureChatBranching(chat)
    if (!branching) return
    ensureChatBranch(chat, bid)
    ;(branching as any).activeBranchId = bid
    ;(chat as any).branching = branching

    const draft0 = state.branchDraft && typeof state.branchDraft === 'object' ? (state.branchDraft as any) : null
    if (draft0 && String(draft0?.roleId || '') === String(sa.activeRole()?.id || '') && String(draft0?.chatId || '') === String(chat.id || '')) {
      state.branchDraft = null
    }

    save().catch(() => {})
    render()
    scrollToBottomSoon()
  }

  // ============ delete message ============

  async function deleteMessage(messageId: any) {
    void messageId
    rejectLocalBusinessPath('消息删除必须由 eucli-box 提供，客户端本地会话历史修改已禁用')
  }

  // ============ delete message subtree ============

  async function deleteMessageSubtree(messageId: any) {
    void messageId
    rejectLocalBusinessPath('消息删除必须由 eucli-box 提供，客户端本地会话历史修改已禁用')
  }

  // ============ edit message ============

  async function editMessage(messageId: any, content: any) {
    void messageId
    void content
    rejectLocalBusinessPath('消息编辑必须由 eucli-box 提供，客户端本地会话历史修改已禁用')
  }

  return {
    pickDraftImages,
    addDraftImagesFromFiles,
    addDraftFilesFromFiles,
    sendChat,
    sendGroupChat,
    stopSending,
    regenerateAssistantMessage,
    regenerateGroupAssistantMessage,
    replyFromUserMessage,
    replyFromUserMessageInGroup,
    createParallelBranchFromAssistantMessage,
    switchBranchByAssistantSibling,
    setActiveBranch,
    deleteMessage,
    deleteMessageSubtree,
    editMessage,
  }
}
