import { now, uid } from '../core/utils'
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
  repairChatLinearBranching,
  findChatMessageById,
  findPrevAssistantMidForAssistant,
  findChatBranch,
} from '../domain/branching'
import { looksLikeImageDataUrl } from '../domain/textProcessing'
import { detectDraftFileKind, addDraftFilePlaceholder } from '../domain/draftFileUtils'
import type { DraftFileItem } from '../domain/draftFileUtils'
import { normalizeChatModelOverride } from '../domain/modelRefUtils'
import { createStateAccessors } from '../state/stateAccessors'
import type { AiChatInternalGateway } from '../gateway/types'
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
import { cancelRoleRun, getRunState, isTerminalRunStatus, sleepMs, startRoleRun, type EbRunState } from './ebRoleRun'

export function createChatOperations(deps: {
  getState: () => any
  aiGateway: AiChatInternalGateway
  pickImageFiles?: (maxCount: number) => Promise<any[]>
  netRequest?: (req: any) => Promise<any>
  showToast?: (msg: any) => void
  save: (intent?: ChatSaveIntent) => Promise<void>
  ensureActiveChatLoaded?: () => Promise<any>
  ensureChatLoaded?: (kind: 'role' | 'group', targetId: string, chatId: string) => Promise<any>
  emit: () => void
  render: () => void
  renderComposer: () => void
  scrollToBottomSoon: () => void
  readImageFileAsDataUrl: (file: File) => Promise<string>
  extractTextFromFile: (file: File, kind: string) => Promise<string>
  uiStreamCache: Map<string, any>
}) {
  const { getState, aiGateway, pickImageFiles, netRequest, showToast, save, ensureActiveChatLoaded, ensureChatLoaded, emit, render, renderComposer, scrollToBottomSoon, readImageFileAsDataUrl, extractTextFromFile, uiStreamCache } = deps

  const sa = createStateAccessors({ getState })
  const assistantArtifactCleanup = createAssistantArtifactCleanup({
    uiStreamCache,
    resetAssistantRuntime: (messageId) => aiGateway.resetAssistantRuntime(messageId),
  })

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

  function chatHasPendingAssistant(chat: any) {
    return hasActiveAssistantMessages(chat)
  }

  function chatHasPendingAssistantInBranch(chat: any, branchId: string, excludeMid?: any) {
    return hasActiveAssistantMessages(chat, { branchId, excludeMid })
  }

  async function refreshRoleSession(roleId: string, sessionId: string) {
    const state = getState()
    const rid = String(roleId || '').trim()
    const sid = String(sessionId || '').trim()
    if (!state.data || !rid || !sid || typeof ensureChatLoaded !== 'function') return null
    if (!state.data.chatsByRole || typeof state.data.chatsByRole !== 'object') state.data.chatsByRole = {}
    if (!state.data.chatsByRole[rid] || typeof state.data.chatsByRole[rid] !== 'object') state.data.chatsByRole[rid] = { activeChatId: '', chatMetas: [], chats: [] }
    state.data.chatsByRole[rid].activeChatId = sid
    const chat = await ensureChatLoaded('role', rid, sid)
    if (chat) {
      state.data.chatsByRole[rid].activeChatId = sid
      emit()
    }
    return chat
  }

  async function runRoleMessageViaEb(input: { roleId: string; sessionId: string; message: string }, onAccepted?: (run: EbRunState) => void) {
    if (typeof netRequest !== 'function') throw new Error('e-b 请求通道不可用')
    let state = await startRoleRun(netRequest, input)
    if (!state.id) throw new Error('e-b 未返回 run id')
    onAccepted?.(state)
    let sessionId = String(state.sessionId || input.sessionId || '').trim()
    if (sessionId) await refreshRoleSession(input.roleId, sessionId)

    const deadline = Date.now() + 120_000
    while (!isTerminalRunStatus(state.status)) {
      if (Date.now() > deadline) throw new Error('e-b 运行超时')
      await sleepMs(450)
      state = await getRunState(netRequest, state.id)
      const nextSessionId = String(state.sessionId || sessionId || '').trim()
      if (nextSessionId) {
        sessionId = nextSessionId
        await refreshRoleSession(input.roleId, sessionId)
      }
    }

    if (sessionId) await refreshRoleSession(input.roleId, sessionId)
    if (state.status === 'failed' || state.status === 'cancelled') throw new Error(state.reason || `e-b run ${state.status}`)
    if (!sessionId) throw new Error('e-b 未返回会话ID')
    return sessionId
  }

  function activeChatOperationTarget() {
    const state = getState()
    const kind = sa.activeTargetKind() === 'group' ? 'group' : 'role'
    const target = kind === 'group' ? sa.activeGroup() : sa.activeRole()
    const targetId = String((target as any)?.id || '').trim()
    if (!targetId) return null
    const pendingChat =
      kind === 'group'
        ? state.pendingGroupChat && String(state.pendingGroupChat.groupId || '') === targetId
          ? state.pendingGroupChat.chat
          : null
        : state.pendingChat && String(state.pendingChat.roleId || '') === targetId
          ? state.pendingChat.chat
          : null
    const chat = pendingChat || sa.activeChatFromData()
    if (!chat) return null
    return { kind, target, targetId, chat, pendingChat }
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

    if (sa.activeTargetKind() === 'group') {
      await sendGroupChat(opts)
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
    if (draftImages.length || hasFiles) return showToast?.('当前 e-b 发送只支持文本消息，图片/文件发送需要等 e-b 附件消息根动作接入')

    const rid = String(role.id || '')
    const loadedChat = await ensureActiveChatLoaded?.().catch(() => null)
    const chatForModel = loadedChat || sa.activeChatFromData()
    if (normalizeChatModelOverride(chatForModel)) {
      return showToast?.('当前会话临时模型尚未接入 e-b 真实根动作，请先清除当前会话临时模型')
    }

    const sessionId = String((loadedChat || sa.activeChatFromData())?.id || '').trim()
    try {
      state.sending = true
      state.sendingCtx = { kind: 'eb-role-run', runId: '', roleId: rid, sessionId, cancelledByUser: false }
      renderComposer()
      await runRoleMessageViaEb({ roleId: rid, sessionId, message: input }, (run) => {
        state.sendingCtx = {
          kind: 'eb-role-run',
          runId: String(run.id || '').trim(),
          roleId: rid,
          sessionId: String(run.sessionId || sessionId || '').trim(),
          cancelledByUser: false,
        }
        state.draft.input = ''
        state.draft.images = []
        ;(state.draft as any).files = []
        renderComposer()
      })
    } catch (e) {
      const msg = String((e as any)?.message || e || '请求失败')
      if ((state.sendingCtx as any)?.cancelledByUser) showToast?.('已停止')
      else showToast?.(msg)
    } finally {
      state.sendingCtx = null
      state.sending = false
      render()
    }
  }

  // ============ send group chat ============

  async function sendGroupChat(_opts?: { forkFromMid?: string }) {
    const state = getState()
    if (state.sending || state.loading || !state.data) return

    return showToast?.('群组发送尚未接入 e-b 真实会话根动作，已阻止本地假会话发送')
  }

  // ============ stop sending ============

  async function stopSending() {
    const state = getState()
    if (state.loading) return

    const sendingCtx = state.sendingCtx && typeof state.sendingCtx === 'object' ? state.sendingCtx : null
    if (sendingCtx && String(sendingCtx.kind || '') === 'eb-role-run') {
      const runId = String(sendingCtx.runId || '').trim()
      if (!runId) return showToast?.('当前运行尚未拿到 e-b run id，请稍候再试')
      if (typeof netRequest !== 'function') return showToast?.('e-b 请求通道不可用')
      try {
        sendingCtx.cancelledByUser = true
        await cancelRoleRun(netRequest, runId)
        showToast?.('已请求停止')
        renderComposer()
      } catch (e) {
        showToast?.(String((e as any)?.message || e || '停止失败'))
      }
      return
    }

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

    if (sa.activeTargetKind() === 'group') {
      await regenerateGroupAssistantMessage(String(assistantMid || ''))
      return
    }
    showToast?.('角色重生成尚未接入 e-b 真实会话根动作，已阻止旧本地运行链')
  }

  // ============ regenerate group assistant message ============

  async function regenerateGroupAssistantMessage(assistantMid: string) {
    const state = getState()
    if (state.sending || state.loading || !state.data) return
    showToast?.('群组重生成尚未接入 e-b 真实会话根动作，已阻止本地假运行')
  }

  // ============ reply from user message ============

  async function replyFromUserMessage(userMid: any) {
    const state = getState()
    if (state.sending || state.loading || !state.data) return

    if (sa.activeTargetKind() === 'group') {
      await replyFromUserMessageInGroup(String(userMid || ''))
      return
    }
    showToast?.('角色从用户消息继续回复尚未接入 e-b 真实会话根动作，已阻止旧本地运行链')
  }

  // ============ reply from user message in group ============

  async function replyFromUserMessageInGroup(userMid: string) {
    const state = getState()
    if (state.sending || state.loading || !state.data) return
    showToast?.('群组从用户消息继续回复尚未接入 e-b 真实会话根动作，已阻止本地假运行')
  }

  // ============ create parallel branch from assistant message ============

  async function createParallelBranchFromAssistantMessage(assistantMid: any) {
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
    const state = getState()
    if (state.loading || !state.data) return
    if (state.sending) return showToast?.('操作中，请稍后重试')
    void messageId
    showToast?.('消息删除尚未接入 e-b 真实会话消息根动作，已阻止本地假修改')
  }

  // ============ delete message subtree ============

  async function deleteMessageSubtree(messageId: any) {
    const state = getState()
    if (state.loading || !state.data) return
    if (state.sending) return showToast?.('操作中，请稍后重试')
    void messageId
    showToast?.('消息删除尚未接入 e-b 真实会话消息根动作，已阻止本地假修改')
  }

  // ============ edit message ============

  async function editMessage(messageId: any, content: any) {
    const state = getState()
    if (state.loading || !state.data) return
    if (state.sending) return showToast?.('操作中，请稍后重试')
    void messageId
    void content
    showToast?.('消息编辑尚未接入 e-b 真实会话消息根动作，已阻止本地假修改')
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
