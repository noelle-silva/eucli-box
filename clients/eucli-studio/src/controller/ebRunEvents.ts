import { normalizeTimeMs } from '../core/utils'
import { activateChatBranchByMessage, ensureChatBranch, normalizeBranchId } from '../domain/branching'
import { beginAssistantRun, checkpointAssistantRun, finishAssistantRun, type AssistantRunStatus } from '../domain/assistantRunState'
import { chatMetaFromChat, upsertChatMeta } from '../domain/chatMeta'
import { normalizeChatMessage, normalizeMessageParentMid } from '../domain/message'
import { CHAT_DEFAULT_BRANCH_ID } from '../domain/constants'

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

    const box = state.data.chatsByRole && typeof state.data.chatsByRole === 'object' ? state.data.chatsByRole[roleId] : null
    const chats = Array.isArray(box?.chats) ? box.chats : []
    const chat = chats.find((item: any) => String(item?.id || '').trim() === sessionId) || null
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
    } else {
      beginAssistantRun(message, { generationId: runId, stream: !!payload.stream, resetContent: false, startedAt: Number(message.createdAt || eventTime) || eventTime })
      checkpointAssistantRun(message, message.content, message.updatedAt)
    }

    chat.updatedAt = Math.max(Number(chat.updatedAt || 0), eventTime)
    const branch = ensureChatBranch(chat, branchId)
    if (branch) {
      branch.headMid = messageId
      branch.updatedAt = chat.updatedAt
      if (!String(branch.forkFromMid || '').trim() && parentMid) branch.forkFromMid = parentMid
      if (chat.branching && typeof chat.branching === 'object') chat.branching.activeBranchId = branch.id
    }
    activateChatBranchByMessage(chat, messageId)

    if (box) box.chatMetas = upsertChatMeta(box.chatMetas, chatMetaFromChat(chat, '新聊天'), '新聊天')
    const sendingCtx = state.sendingCtx && typeof state.sendingCtx === 'object' ? state.sendingCtx : null
    if (sendingCtx && String(sendingCtx.kind || '') === 'eb-role-run' && String(sendingCtx.runId || '').trim() === runId) {
      sendingCtx.lastMessageId = messageId
      sendingCtx.sessionId = sessionId
    }

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
    if (String(event.type || '').trim() === 'assistant_message_update') return applyAssistantMessageUpdate(event.payload)
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
