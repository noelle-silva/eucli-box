import { normalizeTimeMs } from '../core/utils'
import { ensureChatBranch, normalizeBranchId } from '../domain/branching'
import { beginAssistantRun, checkpointAssistantRun, finishAssistantRun, hasSettledAssistantToolParts, type AssistantRunStatus } from '../domain/assistantRunState'
import { chatMetaFromChat, upsertChatMeta } from '../domain/chatMeta'
import { normalizeChatMessage, normalizeMessageParentMid } from '../domain/message'
import { CHAT_DEFAULT_BRANCH_ID } from '../domain/constants'
import { activeEbRoleRunCardsForSession, isTerminalEbRunStatus, removeEbRoleRunCard, upsertEbRoleRunCard } from '../domain/activeRunCards'

type DirectEventSubscription = (listener: (event: any) => void) => () => void

export function createEbRunEventConsumer(deps: {
  getState: () => any
  emit: () => void
  subscribeDirectEvents?: DirectEventSubscription
}) {
  let unsubscribe: (() => void) | null = null
  let renderRaf = 0
  const pendingBySession = new Map<string, Map<string, any>>()

  function scheduleRender() {
    if (renderRaf) return
    renderRaf = window.requestAnimationFrame(() => {
      renderRaf = 0
      deps.emit()
    })
  }

  function sessionKey(roleId: string, sessionId: string) {
    return `${roleId}\n${sessionId}`
  }

  function bufferAssistantMessageUpdate(payload: any, roleId: string, sessionId: string, messageId: string) {
    const key = sessionKey(roleId, sessionId)
    const messages = pendingBySession.get(key) || new Map<string, any>()
    messages.set(messageId, payload)
    pendingBySession.set(key, messages)
  }

  function clearBufferedAssistantMessageUpdate(roleId: string, sessionId: string, messageId: string) {
    const key = sessionKey(roleId, sessionId)
    const messages = pendingBySession.get(key)
    if (!messages) return
    messages.delete(messageId)
    if (!messages.size) pendingBySession.delete(key)
  }

  function ensureRoleChatBox(state: any, roleId: string) {
    if (!state?.data || !roleId) return null
    if (!state.data.chatsByRole || typeof state.data.chatsByRole !== 'object') state.data.chatsByRole = {}
    if (!state.data.chatsByRole[roleId] || typeof state.data.chatsByRole[roleId] !== 'object') state.data.chatsByRole[roleId] = { activeChatId: '', chatMetas: [], chats: [] }
    const box = state.data.chatsByRole[roleId]
    if (!Array.isArray(box.chats)) box.chats = []
    if (!Array.isArray(box.chatMetas)) box.chatMetas = []
    return box
  }

  function ensureRuntimeRoleChat(state: any, roleId: string, sessionId: string, eventTime: number) {
    const box = ensureRoleChatBox(state, roleId)
    if (!box || !sessionId) return null
    const chat = box.chats.find((item: any) => String(item?.id || '').trim() === sessionId) || null
    if (chat) return chat

    const meta = box.chatMetas.find((item: any) => String(item?.id || '').trim() === sessionId) || null
    const t = Number(eventTime || 0) || Date.now()
    const runtimeChat = {
      id: sessionId,
      title: String(meta?.title || '').trim() || '新聊天',
      messages: [],
      createdAt: Number(meta?.createdAt || 0) || t,
      updatedAt: Math.max(Number(meta?.updatedAt || 0), t),
      runtimePartial: true,
    }
    box.chats.unshift(runtimeChat)
    return runtimeChat
  }

  function normalizeRuntimeAssistantMessage(raw: any, fallback: { parentMessageId?: string; branchId?: string; eventTime: number }) {
    const message = raw && typeof raw === 'object' ? raw : null
    if (!message) return null
    const id = String(message.id || '').trim()
    const type = String(message.type || message.role || '').trim()
    if (!id || type !== 'assistant') return null

    const out = normalizeChatMessage(message, { activeBranchId: fallback.branchId || CHAT_DEFAULT_BRANCH_ID, toolMessagesAsAssistant: true })
    out.id = id
    out.type = 'assistant'
    out.role = 'assistant'
    out.parentMid = normalizeMessageParentMid(message) || String(fallback.parentMessageId || '').trim()
    out.branchId = String(message.branchId || '').trim() ? normalizeBranchId(message.branchId) : ''
    out.createdAt = normalizeTimeMs(message.createdAt, fallback.eventTime)
    out.updatedAt = normalizeTimeMs(message.updatedAt, out.createdAt)
    return out
  }

  function assistantStatusFromRunStatus(value: unknown): AssistantRunStatus | null {
    const status = String(value || '').trim()
    if (status === 'completed') return 'succeeded'
    if (status === 'failed') return 'failed'
    if (status === 'cancelled' || status === 'canceled') return 'canceled'
    return null
  }

  function applyRunStateEvent(raw: unknown, fallbackRunIdRaw?: unknown) {
    const payload = raw && typeof raw === 'object' ? (raw as any) : null
    const runId = String(payload?.id || payload?.runId || fallbackRunIdRaw || '').trim()
    if (!runId) return false
    const state = deps.getState()
    if (!state?.data) return false
    const status = String(payload?.status || '').trim()
    if (isTerminalEbRunStatus(status)) {
      const removed = removeEbRoleRunCard(state, runId)
      if (!removed) return false
      scheduleRender()
      return true
    }
    const roleId = String(payload?.roleId || '').trim()
    const sessionId = String(payload?.sessionId || '').trim()
    if (!roleId || !sessionId) return false
    upsertEbRoleRunCard(state, {
      runId,
      roleId,
      sessionId,
      inputMessageId: String(payload?.inputMessageId || '').trim(),
      lastMessageId: String(payload?.lastMessageId || payload?.inputMessageId || '').trim(),
      anchorMessageId: String(payload?.inputMessageId || '').trim(),
      dependencyMessageIds: Array.isArray(payload?.dependencyMessageIds) ? payload.dependencyMessageIds : [],
      status: status || 'running',
      stream: !!payload?.stream,
    })
    scheduleRender()
    return true
  }

  function applyAssistantMessageUpdate(raw: unknown) {
    const payload = raw && typeof raw === 'object' ? (raw as any) : null
    const state = deps.getState()
    if (!payload || !state.data) return false

    const runId = String(payload.runId || '').trim()
    const roleId = String(payload.roleId || '').trim()
    const sessionId = String(payload.sessionId || '').trim()
    const eventTime = normalizeTimeMs(payload.createdAt)
    const incomingMessage = normalizeRuntimeAssistantMessage(payload.message, { eventTime })
    const messageId = String(incomingMessage?.id || '').trim()
    if (!runId || !roleId || !sessionId || !messageId) return false

    const box = ensureRoleChatBox(state, roleId)
    const chat = ensureRuntimeRoleChat(state, roleId, sessionId, eventTime)
    if (!chat) {
      bufferAssistantMessageUpdate(payload, roleId, sessionId, messageId)
      return false
    }
    clearBufferedAssistantMessageUpdate(roleId, sessionId, messageId)
    if (!Array.isArray(chat.messages)) chat.messages = []

    const parentMid = String(incomingMessage.parentMid || '').trim()
    const parent = parentMid ? chat.messages.find((item: any) => String(item?.id || '').trim() === parentMid) || null : null
    const branchId = normalizeBranchId(incomingMessage.branchId || parent?.branchId || CHAT_DEFAULT_BRANCH_ID)

    let message = chat.messages.find((item: any) => String(item?.id || '').trim() === messageId) || null
    if (!message) {
      message = incomingMessage
      chat.messages.push(message)
    } else {
      Object.assign(message, incomingMessage)
    }

    message.role = 'assistant'
    message.type = 'assistant'
    if (parentMid) message.parentMid = parentMid
    message.branchId = branchId
    message.updatedAt = Math.max(Number(message.updatedAt || 0), eventTime)
    const terminalStatus = assistantStatusFromRunStatus(payload.status)
    if (terminalStatus) {
      finishAssistantRun(message, message.content, terminalStatus, message.updatedAt)
    } else if (hasSettledAssistantToolParts(message)) {
      finishAssistantRun(message, message.content, 'succeeded', message.updatedAt)
    } else {
      beginAssistantRun(message, { generationId: runId, stream: !!payload.stream, resetContent: false, startedAt: Number(message.createdAt || eventTime) || eventTime })
      checkpointAssistantRun(message, message.content, message.updatedAt)
    }

    if (isTerminalEbRunStatus(payload.status)) {
      removeEbRoleRunCard(state, runId)
    } else {
      upsertEbRoleRunCard(state, {
        runId,
        roleId,
        sessionId,
        inputMessageId: parentMid,
        lastMessageId: messageId,
        anchorMessageId: parentMid,
        dependencyMessageIds: Array.isArray(payload?.dependencyMessageIds) ? payload.dependencyMessageIds : [],
        status: String(payload.status || 'running').trim() || 'running',
        stream: !!payload.stream,
      })
    }
    chat.updatedAt = Math.max(Number(chat.updatedAt || 0), eventTime)
    const branch = ensureChatBranch(chat, branchId)
    if (branch) {
      branch.headMid = messageId
      branch.updatedAt = chat.updatedAt
      if (!String(branch.forkFromMid || '').trim() && parentMid) branch.forkFromMid = parentMid
    }

    const runStatus = String(payload.status || '').trim()
    const activeCards = activeEbRoleRunCardsForSession(state, roleId, sessionId)
    const chatStatus = activeCards.length ? String(activeCards[activeCards.length - 1]?.status || 'running').trim() || 'running' : runStatus
    if (chatStatus) chat.status = chatStatus

    if (box) box.chatMetas = upsertChatMeta(box.chatMetas, chatMetaFromChat(chat, '新聊天'), '新聊天')

    scheduleRender()
    return true
  }

  function flushSession(roleIdRaw: unknown, sessionIdRaw: unknown) {
    const roleId = String(roleIdRaw || '').trim()
    const sessionId = String(sessionIdRaw || '').trim()
    if (!roleId || !sessionId) return false
    const key = sessionKey(roleId, sessionId)
    const pending = pendingBySession.get(key)
    if (!pending || !pending.size) return false
    const items = Array.from(pending.values())
    pendingBySession.delete(key)
    let changed = false
    for (const payload of items) {
      changed = applyAssistantMessageUpdate(payload) || changed
    }
    return changed
  }

  function applyRunEvent(raw: unknown) {
    const event = raw && typeof raw === 'object' ? (raw as any) : null
    if (!event) return false
    const type = String(event.type || '').trim()
    if (type === 'assistant_message_update') return applyAssistantMessageUpdate(event.payload)
    if (type === 'run_started' || type === 'run_completed' || type === 'run_cancelled' || type === 'run_failed') return applyRunStateEvent(event.payload, event.runId)
    return false
  }

  function start() {
    if (unsubscribe) return
    if (typeof deps.subscribeDirectEvents !== 'function') return
    unsubscribe = deps.subscribeDirectEvents((event: any) => {
      if (String(event?.name || '').trim() !== 'eucliBox.run.event') return
      applyRunEvent(event?.payload)
    })
  }

  function stop() {
    if (unsubscribe) {
      unsubscribe()
      unsubscribe = null
    }
    pendingBySession.clear()
    if (renderRaf) {
      window.cancelAnimationFrame(renderRaf)
      renderRaf = 0
    }
  }

  return { start, stop, applyRunEvent, flushSession }
}
