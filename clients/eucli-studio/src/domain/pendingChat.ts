import { now, uid } from '../core/utils'

export type PendingChatTargetKind = 'role' | 'group'

export type PendingChatEntry = {
  chat: any
  roleId?: string
  groupId?: string
}

function normalizeId(value: unknown): string {
  return String(value || '').trim()
}

export function createPendingChatEntry(kind: PendingChatTargetKind, targetIdRaw: unknown, fallbackTitle: string): PendingChatEntry | null {
  const targetId = normalizeId(targetIdRaw)
  if (!targetId) return null
  const ts = now()
  const chat = {
    id: uid('draft-chat'),
    title: String(fallbackTitle || '').trim() || (kind === 'group' ? '群聊' : '新聊天'),
    messages: [],
    createdAt: ts,
    updatedAt: ts,
    clientDraft: true,
  }
  return kind === 'group' ? { groupId: targetId, chat } : { roleId: targetId, chat }
}

export function pendingChatForTarget(state: any, kind: PendingChatTargetKind, targetIdRaw: unknown) {
  const targetId = normalizeId(targetIdRaw)
  if (!state || !targetId) return null
  const pending = kind === 'group' ? state.pendingGroupChat : state.pendingChat
  if (!pending || typeof pending !== 'object') return null
  const pendingTargetId = kind === 'group' ? normalizeId(pending.groupId) : normalizeId(pending.roleId)
  if (pendingTargetId !== targetId) return null
  return pending.chat && typeof pending.chat === 'object' ? pending.chat : null
}

export function clearPendingChatForTarget(state: any, kind: PendingChatTargetKind, targetIdRaw: unknown) {
  const targetId = normalizeId(targetIdRaw)
  if (!state || !targetId) return
  if (kind === 'group') {
    if (state.pendingGroupChat && normalizeId(state.pendingGroupChat.groupId) === targetId) state.pendingGroupChat = null
    return
  }
  if (state.pendingChat && normalizeId(state.pendingChat.roleId) === targetId) state.pendingChat = null
}

export function activateResolvedPendingChat(state: any, kind: PendingChatTargetKind, targetIdRaw: unknown, chatIdRaw: unknown) {
  const targetId = normalizeId(targetIdRaw)
  const chatId = normalizeId(chatIdRaw)
  if (!state?.data || !targetId || !chatId) return false
  if (!pendingChatForTarget(state, kind, targetId)) return false
  clearPendingChatForTarget(state, kind, targetId)

  if (kind === 'group') {
    if (!state.data.chatsByGroup || typeof state.data.chatsByGroup !== 'object') state.data.chatsByGroup = {}
    if (!state.data.chatsByGroup[targetId] || typeof state.data.chatsByGroup[targetId] !== 'object') state.data.chatsByGroup[targetId] = { activeChatId: '', chatMetas: [], chats: [] }
    state.data.chatsByGroup[targetId].activeChatId = chatId
    return true
  }

  if (!state.data.chatsByRole || typeof state.data.chatsByRole !== 'object') state.data.chatsByRole = {}
  if (!state.data.chatsByRole[targetId] || typeof state.data.chatsByRole[targetId] !== 'object') state.data.chatsByRole[targetId] = { activeChatId: '', chatMetas: [], chats: [] }
  state.data.chatsByRole[targetId].activeChatId = chatId
  return true
}
