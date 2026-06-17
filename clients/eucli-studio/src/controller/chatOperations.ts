import { clamp, now, uid } from '../core/utils'
import {
  MAX_DRAFT_IMAGES,
  MAX_DRAFT_FILES,
  CHAT_DEFAULT_BRANCH_ID,
  DEFAULT_ATTACH_SEND_LIMIT_CHARS,
} from '../domain/constants'
import {
  normalizeBranchId,
  ensureChatBranching,
  ensureChatBranch,
  findChatMessageById,
  findPrevAssistantMidForAssistant,
  findChatBranch,
  collectChatMessageIds,
  findNewestNewLeafMessageId,
  activateChatBranchByMessage,
} from '../domain/branching'
import { looksLikeImageDataUrl } from '../domain/textProcessing'
import { detectDraftFileKind, addDraftFilePlaceholder } from '../domain/draftFileUtils'
import type { DraftFileItem } from '../domain/draftFileUtils'
import { normalizeChatModelOverride, normalizeModelRef, type ModelRef } from '../domain/modelRefUtils'
import { chatReasoningEffort } from '../domain/reasoning'
import { createStateAccessors } from '../state/stateAccessors'
import {
  activeEbRoleRunCards,
  activeEbRunCardsForTarget,
  ebRoleRunCardIsOnMessagePath,
  findEbRoleRunCard,
  latestEbRunCardForTarget,
  markEbRoleRunCardCancelled,
  removeEbRoleRunCard,
  upsertEbRoleRunCard,
} from '../domain/activeRunCards'
import { messageMutationConflict, type MessageMutationOperation } from '../domain/messageMutationConflicts'
import { activateResolvedPendingChat, pendingChatForTarget } from '../domain/pendingChat'
import {
  activeComposerDraftKey,
  activateComposerDraftForCurrentSession,
  clearComposerDraftByKey,
  readActiveComposerDraft,
  readComposerDraftByKey,
  setComposerDraftFilesByKey,
  setComposerDraftImagesByKey,
} from '../domain/sessionComposerDrafts'
import type { ChatSaveIntent } from '../domain/chatSaveIntent'
import { deleteAssistantMessageBlock, editAssistantMessageBlock, replaceMessageText } from '../domain/assistantMessageBlockMutations'
import type { AiChatShowToast } from '../gateway/capabilities'
import { cancelRoleRun, pollRunUntilTerminal, runStateFailureError, startRoleRun, submitToolConfirmation as submitToolConfirmationRequest, type EbRunState } from './ebRoleRun'
import { deleteGroupSessionMessage, deleteGroupSessionMessageSubtree, deleteRoleSessionMessage, deleteRoleSessionMessageSubtree, deleteWorkspaceSessionMessage, deleteWorkspaceSessionMessageSubtree, updateGroupSessionMessage, updateRoleSessionMessage, updateWorkspaceSessionMessage } from './ebRoleSession'
import { buildGroupSpeakerPlan } from '../domain/groupSpeakerPlan'

type ChatTargetKind = 'role' | 'group' | 'workspace'

type SendChatOptions = {
  forkFromMid?: string
  onRunState?: (run: EbRunState) => void
}

type ExistingMessageRunOptions = {
  onRunState?: (run: EbRunState) => void
}

type RoleRunInput = {
  roleId: string
  groupId?: string
  workspaceId?: string
  sessionId: string
  message?: string
  attachments?: any[]
  parentMessageId?: string
  userMessageId?: string
  contextMessageId?: string
  stream?: boolean
  reasoningEffort?: string
  modelOverride?: ModelRef | null
}

export function createChatOperations(deps: {
  getState: () => any
  pickImageFiles?: (maxCount: number) => Promise<any[]>
  netRequest?: (req: any) => Promise<any>
  showToast?: AiChatShowToast
  save: (intent?: ChatSaveIntent) => Promise<void>
  ensureActiveChatLoaded?: () => Promise<any>
  ensureChatLoaded?: (kind: 'role' | 'group' | 'workspace', targetId: string, chatId: string) => Promise<any>
  reloadRoleSession?: (roleId: string, sessionId: string) => Promise<any>
  reloadGroupSession?: (groupId: string, sessionId: string) => Promise<any>
  reloadWorkspaceSession?: (workspaceId: string, sessionId: string) => Promise<any>
  waitForChatSettingsSave?: () => Promise<void>
  emit: () => void
  render: () => void
  renderComposer: () => void
  scrollToBottomSoon: () => void
  readImageFileAsDataUrl: (file: File) => Promise<string>
  extractTextFromFile: (file: File, kind: string) => Promise<string>
}) {
  const { getState, pickImageFiles, netRequest, showToast, save, ensureActiveChatLoaded, ensureChatLoaded, reloadRoleSession, reloadGroupSession, reloadWorkspaceSession, waitForChatSettingsSave, emit, render, renderComposer, scrollToBottomSoon, readImageFileAsDataUrl, extractTextFromFile } = deps

  const sa = createStateAccessors({ getState })
  const cancelledRunIds = new Set<string>()
  const startingRoleRunKeys = new Set<string>()

  async function waitForCurrentChatSettingsSave() {
    try {
      await waitForChatSettingsSave?.()
      return true
    } catch (e) {
      showToast?.(String((e as any)?.message || e || '当前会话设置保存失败'), { kind: 'error' })
      return false
    }
  }

  function roleRunStartKey(input: RoleRunInput) {
    const roleId = String(input.roleId || '').trim()
    const groupId = String(input.groupId || '').trim()
    const workspaceId = String(input.workspaceId || '').trim()
    const sessionId = String(input.sessionId || '').trim()
    const parentMessageId = String(input.parentMessageId || '').trim()
    const userMessageId = String(input.userMessageId || '').trim()
    const contextMessageId = String(input.contextMessageId || '').trim()
    const message = String(input.message || '').trim()
    const reasoningEffort = String(input.reasoningEffort || '').trim()
    return [workspaceId ? `workspace:${workspaceId}` : groupId ? `group:${groupId}` : 'role', roleId, sessionId, contextMessageId ? `context:${contextMessageId}` : userMessageId ? `user:${userMessageId}` : `parent:${parentMessageId}`, message, reasoningEffort, roleRunModelOverrideKey(input.modelOverride)].join('\n')
  }

  function roleRunModelOverrideKey(value: unknown) {
    const ref = normalizeModelRef(value)
    return ref ? JSON.stringify({ kind: ref.kind, providerId: ref.providerId, groupId: ref.groupId, modelId: ref.modelId }) : ''
  }

  function currentRoleChatModelOverride() {
    return normalizeChatModelOverride(sa.activeChatFromData())
  }

  function findActiveRunAtMessage(roleId: string, sessionId: string, messageId: string) {
    return findActiveRunAtMessageForTarget('role', roleId, sessionId, messageId)
  }

  function findActiveRunAtMessageForTarget(targetKind: ChatTargetKind, targetId: string, sessionId: string, messageId: string) {
    const mid = String(messageId || '').trim()
    const tid = String(targetId || '').trim()
    const sid = String(sessionId || '').trim()
    if (!mid || !tid || !sid) return null
    return activeEbRunCardsForTarget(getState(), targetKind, tid, sid).find((card) => card.inputMessageId === mid || card.anchorMessageId === mid || card.lastMessageId === mid) || null
  }

  function activeBranchMessagePath(chat: any) {
    const ids = new Set<string>()
    const headMid = activeChatHeadMid(chat)
    const messages = Array.isArray(chat?.messages) ? chat.messages : []
    if (!headMid || !messages.length) return { ids, headMid }
    const byId = new Map<string, any>()
    for (const message of messages) {
      const id = String(message?.id || '').trim()
      if (id && !byId.has(id)) byId.set(id, message)
    }
    let current = headMid
    const seen = new Set<string>()
    while (current && !seen.has(current)) {
      seen.add(current)
      const message = byId.get(current) || null
      if (!message) break
      ids.add(current)
      current = String(message?.parentMid || '').trim()
    }
    return { ids, headMid }
  }

  function activeBranchHasActiveRun(roleId: string, sessionId: string, chat: any) {
    return activeBranchHasActiveRunForTarget('role', roleId, sessionId, chat)
  }

  function activeBranchHasActiveRunForTarget(targetKind: ChatTargetKind, targetId: string, sessionId: string, chat: any) {
    const cards = activeEbRunCardsForTarget(getState(), targetKind, targetId, sessionId)
    const path = activeBranchMessagePath(chat)
    return cards.some((card) => ebRoleRunCardIsOnMessagePath(card, path.ids, path.headMid))
  }

  function ensureStableComposerParent(roleId: string, sessionId: string, chat: any, parentMid: string, explicitParent: boolean) {
    return ensureStableComposerParentForTarget('role', roleId, sessionId, chat, parentMid, explicitParent)
  }

  function ensureStableComposerParentForTarget(targetKind: ChatTargetKind, targetId: string, sessionId: string, chat: any, parentMid: string, explicitParent: boolean) {
    const mid = String(parentMid || '').trim()
    if (!chat || !mid) return true
    const parent = findChatMessageById(chat, mid)
    if (!explicitParent && parent && String((parent as any).role || '') === 'user' && findActiveRunAtMessageForTarget(targetKind, targetId, sessionId, mid)) {
      showToast?.('这个问题已有运行中的回答，请从用户消息菜单并排生成，或选择一条稳定回复后继续', { kind: 'error' })
      return false
    }
    if (!explicitParent && activeBranchHasActiveRunForTarget(targetKind, targetId, sessionId, chat)) {
      showToast?.('当前路线仍有运行中的任务，请停止或等待完成后再发送', { kind: 'error' })
      return false
    }
    return true
  }

  function dependencyMessageIdsForRunStart(chat: any, anchorMessageId: string) {
    const anchor = String(anchorMessageId || '').trim()
    if (!chat || !anchor) return []
    const messages = Array.isArray(chat?.messages) ? chat.messages : []
    const byId = new Map<string, any>()
    for (const message of messages) {
      const id = String(message?.id || '').trim()
      if (id && !byId.has(id)) byId.set(id, message)
    }
    const ids: string[] = []
    const seen = new Set<string>()
    let current = anchor
    while (current && !seen.has(current)) {
      seen.add(current)
      const message = byId.get(current) || null
      if (!message) break
      ids.push(current)
      current = String((message as any)?.parentMid || '').trim()
    }
    return ids
  }

  function syncEbRoleRunCard(run: EbRunState, fallback: { roleId: string; groupId?: string; workspaceId?: string; sessionId?: string; lastMessageId?: string; anchorMessageId?: string; dependencyMessageIds?: string[]; startedFromPending?: boolean; pendingChatId?: string }) {
    const runId = String(run?.id || '').trim()
    const state = getState()
    if (!runId) return null

    const current = findEbRoleRunCard(state, runId)
    const roleId = String(run?.roleId || fallback.roleId || current?.roleId || '').trim()
    const groupId = String((run as any)?.groupId || fallback.groupId || current?.groupId || '').trim()
    const workspaceId = String((run as any)?.workspaceId || fallback.workspaceId || current?.workspaceId || '').trim()
    const sessionId = String(run?.sessionId || fallback.sessionId || current?.sessionId || '').trim()
    const targetKind: ChatTargetKind = workspaceId ? 'workspace' : groupId ? 'group' : 'role'
    const targetId = workspaceId || groupId || roleId
    if (fallback.startedFromPending && targetId && sessionId) activateResolvedPendingChat(state, targetKind, targetId, sessionId, fallback.pendingChatId)
    const inputMessageId = String(run?.inputMessageId || current?.inputMessageId || '').trim()
    const lastMessageId = String(run?.lastMessageId || fallback.lastMessageId || current?.lastMessageId || inputMessageId || '').trim()
    return upsertEbRoleRunCard(state, {
      runId: runId || String(current?.runId || '').trim(),
      roleId,
      groupId,
      workspaceId,
      sessionId,
      inputMessageId,
      lastMessageId,
      anchorMessageId: String(fallback.anchorMessageId || current?.anchorMessageId || inputMessageId || '').trim(),
      dependencyMessageIds: run?.dependencyMessageIds?.length ? run.dependencyMessageIds : Array.isArray(fallback.dependencyMessageIds) ? fallback.dependencyMessageIds : current?.dependencyMessageIds || [],
      status: String(run?.status || current?.status || 'running').trim(),
      stream: !!run?.stream,
      retry: run?.retry,
      cancelledByUser: !!current?.cancelledByUser,
    })
  }

  function isCurrentRoleSession(roleId: string, sessionId: string) {
    return isCurrentTargetSession('role', roleId, sessionId)
  }

  function isCurrentTargetSession(targetKind: ChatTargetKind, targetId: string, sessionId: string) {
    const state = getState()
    const tid = String(targetId || '').trim()
    const sid = String(sessionId || '').trim()
    if (!state?.data || !tid || !sid) return false
    if (sa.activeTargetKind() !== targetKind) return false
    const currentTargetId = targetKind === 'group'
      ? String(sa.activeGroup()?.id || state.draft?.activeGroupId || state.data?.ui?.activeGroupId || '').trim()
      : targetKind === 'workspace'
        ? String(sa.activeWorkspace?.()?.id || state.draft?.activeWorkspaceId || state.data?.ui?.activeWorkspaceId || '').trim()
        : String(state.draft?.activeRoleId || state.data?.ui?.activeRoleId || '').trim()
    if (currentTargetId !== tid) return false
    const box = targetKind === 'group'
      ? state.data.chatsByGroup && typeof state.data.chatsByGroup === 'object'
        ? state.data.chatsByGroup[tid]
        : null
      : targetKind === 'workspace'
        ? state.data.chatsByWorkspace && typeof state.data.chatsByWorkspace === 'object'
          ? state.data.chatsByWorkspace[tid]
          : null
      : state.data.chatsByRole && typeof state.data.chatsByRole === 'object'
        ? state.data.chatsByRole[tid]
        : null
    return String(box?.activeChatId || '').trim() === sid
  }

  async function refreshRoleSession(roleId: string, sessionId: string, onLoaded?: (chat: any) => void, options?: { activate?: boolean }) {
    return refreshTargetSession('role', roleId, sessionId, onLoaded, options)
  }

  async function refreshTargetSession(targetKind: ChatTargetKind, targetId: string, sessionId: string, onLoaded?: (chat: any) => void, options?: { activate?: boolean }) {
    const state = getState()
    const tid = String(targetId || '').trim()
    const sid = String(sessionId || '').trim()
    if (!state.data || !tid || !sid || typeof ensureChatLoaded !== 'function') return null
    if (targetKind === 'group') {
      if (!state.data.chatsByGroup || typeof state.data.chatsByGroup !== 'object') state.data.chatsByGroup = {}
      if (!state.data.chatsByGroup[tid] || typeof state.data.chatsByGroup[tid] !== 'object') state.data.chatsByGroup[tid] = { activeChatId: '', chatMetas: [], chats: [] }
    } else if (targetKind === 'workspace') {
      if (!state.data.chatsByWorkspace || typeof state.data.chatsByWorkspace !== 'object') state.data.chatsByWorkspace = {}
      if (!state.data.chatsByWorkspace[tid] || typeof state.data.chatsByWorkspace[tid] !== 'object') state.data.chatsByWorkspace[tid] = { activeChatId: '', chatMetas: [], chats: [] }
    } else {
      if (!state.data.chatsByRole || typeof state.data.chatsByRole !== 'object') state.data.chatsByRole = {}
      if (!state.data.chatsByRole[tid] || typeof state.data.chatsByRole[tid] !== 'object') state.data.chatsByRole[tid] = { activeChatId: '', chatMetas: [], chats: [] }
    }
    const chat = targetKind === 'group'
      ? typeof reloadGroupSession === 'function'
        ? await reloadGroupSession(tid, sid)
        : await ensureChatLoaded('group', tid, sid)
      : targetKind === 'workspace'
        ? typeof reloadWorkspaceSession === 'function'
          ? await reloadWorkspaceSession(tid, sid)
          : await ensureChatLoaded('workspace', tid, sid)
      : typeof reloadRoleSession === 'function'
        ? await reloadRoleSession(tid, sid)
        : await ensureChatLoaded('role', tid, sid)
    if (chat) {
      if (options?.activate !== false) {
        if (targetKind === 'group') state.data.chatsByGroup[tid].activeChatId = sid
        else if (targetKind === 'workspace') state.data.chatsByWorkspace[tid].activeChatId = sid
        else state.data.chatsByRole[tid].activeChatId = sid
      }
      onLoaded?.(chat)
      emit()
    }
    return chat
  }

  function followRunResultBranch(chat: any, follow: { previousMessageIds: Set<string>; ancestorMessageId?: string; messageId?: string } | null | undefined) {
    if (!follow || !chat) return false
    const explicitMessageId = String(follow.messageId || '').trim()
    const targetMessageId = findNewestNewLeafMessageId(chat, follow.previousMessageIds, follow.ancestorMessageId, explicitMessageId)
    if (!targetMessageId) return false
    return activateChatBranchByMessage(chat, targetMessageId)
  }

  function activeChatHeadMid(chat: any) {
    const branching = chat && typeof chat === 'object' ? (chat as any).branching : null
    const activeBranchId = String(branching?.activeBranchId || 'main').trim() || 'main'
    const branches = Array.isArray(branching?.branches) ? branching.branches : []
    const branch = branches.find((item: any) => String(item?.id || '').trim() === activeBranchId) || null
    const headMid = String(branch?.headMid || '').trim()
    if (headMid) return headMid
    const messages = Array.isArray(chat?.messages) ? chat.messages : []
    return messages.length ? String(messages[messages.length - 1]?.id || '').trim() : ''
  }

  function targetSessionViewAnchor(targetKind: ChatTargetKind, targetId: string, sessionId: string) {
    if (!isCurrentTargetSession(targetKind, targetId, sessionId)) return null
    const chat = sa.activeChatFromData()
    const branchId = String((chat as any)?.branching?.activeBranchId || '').trim()
    return { branchId, headMid: activeChatHeadMid(chat) }
  }

  function targetSessionViewUnchanged(targetKind: ChatTargetKind, targetId: string, sessionId: string, anchor: { branchId: string; headMid: string } | null | undefined) {
    if (!anchor || !isCurrentTargetSession(targetKind, targetId, sessionId)) return false
    const chat = sa.activeChatFromData()
    const branchId = String((chat as any)?.branching?.activeBranchId || '').trim()
    return branchId === anchor.branchId && activeChatHeadMid(chat) === anchor.headMid
  }

  function targetPendingViewStillCurrent(targetKind: ChatTargetKind, targetId: string, sessionId: string, pendingChatId: string) {
    const state = getState()
    const pending = pendingChatForTarget(state, targetKind, targetId)
    if (pending) return String(pending?.id || '').trim() === String(pendingChatId || '').trim()
    const box = targetKind === 'group'
      ? state?.data?.chatsByGroup && typeof state.data.chatsByGroup === 'object'
        ? state.data.chatsByGroup[targetId]
        : null
      : targetKind === 'workspace'
        ? state?.data?.chatsByWorkspace && typeof state.data.chatsByWorkspace === 'object'
          ? state.data.chatsByWorkspace[targetId]
          : null
      : state?.data?.chatsByRole && typeof state.data.chatsByRole === 'object'
        ? state.data.chatsByRole[targetId]
        : null
    return !!sessionId && String(box?.activeChatId || '').trim() === sessionId
  }

  function roleMessageParentForSend(chat: any, explicitParentMid?: string) {
    const explicit = String(explicitParentMid || '').trim()
    if (explicit) return explicit
    const state = getState()
    const draft = state.branchDraft && typeof state.branchDraft === 'object' ? state.branchDraft : null
    const role = sa.activeRole()
    const chatId = String(chat?.id || '').trim()
    if (draft && String(draft?.roleId || '') === String(role?.id || '') && String(draft?.chatId || '') === chatId) {
      const forkFromMid = String(draft?.forkFromMid || '').trim()
      if (forkFromMid) return forkFromMid
    }
    return activeChatHeadMid(chat)
  }

  function stableExplicitParentForSend(roleId: string, sessionId: string, chat: any, explicitParentMid: string) {
    const mid = String(explicitParentMid || '').trim()
    if (!mid || !chat) return mid
    const parent = findChatMessageById(chat, mid)
    if (!parent || String((parent as any).role || '') !== 'user') return mid
    const hasRunningReply = !!findActiveRunAtMessage(roleId, sessionId, mid)
    if (!hasRunningReply) return mid
    return String((parent as any).parentMid || '').trim() || mid
  }

  async function runRoleMessageViaEb(
    input: RoleRunInput,
    onAccepted?: (run: EbRunState) => void,
    follow?: { previousMessageIds: Set<string>; ancestorMessageId?: string },
    onState?: (run: EbRunState) => void,
  ) {
    if (typeof netRequest !== 'function') throw new Error('e-b 请求通道不可用')
    const startKey = roleRunStartKey(input)
    if (startingRoleRunKeys.has(startKey)) throw new Error('该位置已有启动中的请求，请稍候')
    const stateBeforeRun = getState()
      const groupId = String(input.groupId || '').trim()
      const workspaceId = String(input.workspaceId || '').trim()
      const targetKind: ChatTargetKind = workspaceId ? 'workspace' : groupId ? 'group' : 'role'
      const targetId = workspaceId || groupId || String(input.roleId || '').trim()
    const pendingAtStart = !String(input.sessionId || '').trim() ? pendingChatForTarget(stateBeforeRun, targetKind, targetId) : null
    const startedFromPending = !!pendingAtStart
    const startedFromPendingChatId = String(pendingAtStart?.id || '').trim()
    let followPendingOnce = startedFromPending
    let followViewAnchor = targetSessionViewAnchor(targetKind, targetId, String(input.sessionId || '').trim())
    startingRoleRunKeys.add(startKey)
    let state: EbRunState
    try {
      state = await startRoleRun(netRequest, input)
    } finally {
      startingRoleRunKeys.delete(startKey)
    }
    if (!state.id) throw new Error('e-b 未返回 run id')
    onAccepted?.(state)
    let sessionId = String(state.sessionId || input.sessionId || '').trim()
    let followMessageId = String(state.lastMessageId || '').trim()
    const runAnchorMessageId = String(follow?.ancestorMessageId || input.contextMessageId || input.userMessageId || input.parentMessageId || '').trim()
    const dependencyMessageIds = dependencyMessageIdsForRunStart(sa.activeChatFromData(), runAnchorMessageId)
    syncEbRoleRunCard(state, { roleId: input.roleId, groupId, workspaceId, sessionId, lastMessageId: followMessageId, anchorMessageId: runAnchorMessageId, dependencyMessageIds, startedFromPending, pendingChatId: startedFromPendingChatId })
    onState?.(state)
    let runSessionLoadedOnce = false

    const refreshRunSession = async () => {
      if (!sessionId) return
      const shouldFollowNow = (followPendingOnce && targetPendingViewStillCurrent(targetKind, targetId, sessionId, startedFromPendingChatId)) || targetSessionViewUnchanged(targetKind, targetId, sessionId, followViewAnchor)
      let followed = false
      await refreshTargetSession(targetKind, targetId, sessionId, (chat) => {
        if (shouldFollowNow) followed = followRunResultBranch(chat, follow ? { ...follow, messageId: followMessageId } : null)
      }, { activate: shouldFollowNow })
      if (followed) followViewAnchor = targetSessionViewAnchor(targetKind, targetId, sessionId)
      followPendingOnce = false
      runSessionLoadedOnce = true
    }

    const refreshTerminalRunSession = async () => {
      if (!sessionId) return
      const shouldFollowNow = targetSessionViewUnchanged(targetKind, targetId, sessionId, followViewAnchor)
      let followed = false
      await refreshTargetSession(targetKind, targetId, sessionId, (chat) => {
        if (shouldFollowNow) followed = followRunResultBranch(chat, follow ? { ...follow, messageId: followMessageId } : null)
      }, { activate: shouldFollowNow })
      if (followed) followViewAnchor = targetSessionViewAnchor(targetKind, targetId, sessionId)
      runSessionLoadedOnce = true
    }

    if (sessionId) await refreshRunSession()

    state = await pollRunUntilTerminal(netRequest, state, async (nextState) => {
      state = nextState
      followMessageId = String(state.lastMessageId || followMessageId || '').trim()
      syncEbRoleRunCard(state, { roleId: input.roleId, groupId, workspaceId, sessionId, lastMessageId: followMessageId, anchorMessageId: runAnchorMessageId, dependencyMessageIds, startedFromPending, pendingChatId: startedFromPendingChatId })
      onState?.(state)
      const nextSessionId = String(state.sessionId || sessionId || '').trim()
      if (nextSessionId) {
        sessionId = nextSessionId
        if (!runSessionLoadedOnce && (startedFromPending || !input.sessionId)) await refreshRunSession()
      }
    })

    followMessageId = String(state.lastMessageId || followMessageId || '').trim()
    await refreshTerminalRunSession()
    if (state.status === 'failed' || state.status === 'cancelled') throw runStateFailureError(state)
    if (!sessionId) throw new Error('e-b 未返回会话ID')
    return sessionId
  }

  function finishRoleRun(runId: string) {
    const id = String(runId || '').trim()
    if (!id) return null
    const state = getState()
    const removed = removeEbRoleRunCard(state, id)
    cancelledRunIds.delete(id)
    return removed
  }

  async function activeSingleRoleTarget() {
    const target = await activeTargetSessionMutationTarget()
    if (!target || target.targetKind === 'group' || !target.roleId) return null
    return target
  }

  function latestTargetRunCard(targetKind: ChatTargetKind, targetId: string, sessionId: string) {
    const tid = String(targetId || '').trim()
    const sid = String(sessionId || '').trim()
    if (!tid) return null
    if (sid) return latestEbRunCardForTarget(getState(), targetKind, tid, sid)
    const cards = activeEbRoleRunCards(getState()).filter((card) => card.sessionId && card.sessionId.trim() && ((targetKind === 'group' && card.groupId === tid) || (targetKind === 'workspace' && card.workspaceId === tid) || (targetKind === 'role' && !card.groupId && !card.workspaceId && card.roleId === tid)))
    return cards.length ? cards[cards.length - 1] : null
  }

  async function activeTargetSessionMutationTarget() {
    const activeKind = sa.activeTargetKind()
    const targetKind: ChatTargetKind = activeKind === 'group' ? 'group' : activeKind === 'workspace' ? 'workspace' : 'role'
    const target = targetKind === 'group' ? sa.activeGroup() : targetKind === 'workspace' ? sa.activeWorkspace?.() : sa.activeRole()
    const chat = await ensureActiveChatLoaded?.().catch(() => null) || sa.activeChatFromData()
    const targetId = String(target?.id || '').trim()
    const sessionId = String(chat?.id || '').trim()
    const roleId = String(sa.activeRole()?.id || '').trim()
    if (!targetId || !sessionId || !chat) return null
    return {
      targetKind,
      targetId,
      roleId: targetKind === 'group' ? '' : targetKind === 'workspace' ? roleId : targetId,
      groupId: targetKind === 'group' ? targetId : '',
      workspaceId: targetKind === 'workspace' ? targetId : '',
      sessionId,
      chat,
    }
  }

  async function runRoleFromUserMessage(input: { roleId: string; workspaceId?: string; sessionId: string; userMessageId: string }, operationText: string, opts?: ExistingMessageRunOptions) {
    const state = getState()
    const userMessageId = String(input.userMessageId || '').trim()
    if (!userMessageId) return false
    if (!(await waitForCurrentChatSettingsSave())) return false
    const chatBeforeRun = sa.activeChatFromData()
    const previousMessageIds = collectChatMessageIds(chatBeforeRun)
    let acceptedRunId = ''
    try {
      renderComposer()
      const reasoningEffort = chatReasoningEffort(sa.activeChatFromData())
      const modelOverride = currentRoleChatModelOverride()
      await runRoleMessageViaEb({ roleId: input.roleId, workspaceId: input.workspaceId, sessionId: input.sessionId, userMessageId, reasoningEffort, modelOverride, stream: !!state.data?.settings?.streamEnabled }, (run) => {
        acceptedRunId = String(run?.id || '').trim()
        syncEbRoleRunCard(run, { roleId: input.roleId, workspaceId: input.workspaceId, sessionId: input.sessionId, anchorMessageId: userMessageId })
        renderComposer()
      }, { previousMessageIds, ancestorMessageId: userMessageId }, (run) => opts?.onRunState?.(run))
      return true
    } catch (e) {
      const msg = String((e as any)?.message || e || `${operationText}失败`)
      if (acceptedRunId && cancelledRunIds.has(acceptedRunId)) showToast?.('已停止', { kind: 'success' })
      else showToast?.(msg, { kind: 'error' })
      return false
    } finally {
      if (acceptedRunId) finishRoleRun(acceptedRunId)
      render()
      scrollToBottomSoon()
    }
  }

  async function runRoleFromContextMessage(input: { roleId: string; workspaceId?: string; sessionId: string; contextMessageId: string }, operationText: string, opts?: ExistingMessageRunOptions) {
    const state = getState()
    const contextMessageId = String(input.contextMessageId || '').trim()
    if (!contextMessageId) return false
    if (!(await waitForCurrentChatSettingsSave())) return false
    const chatBeforeRun = sa.activeChatFromData()
    const previousMessageIds = collectChatMessageIds(chatBeforeRun)
    let acceptedRunId = ''
    try {
      renderComposer()
      const reasoningEffort = chatReasoningEffort(sa.activeChatFromData())
      const modelOverride = currentRoleChatModelOverride()
      await runRoleMessageViaEb({ roleId: input.roleId, workspaceId: input.workspaceId, sessionId: input.sessionId, contextMessageId, reasoningEffort, modelOverride, stream: !!state.data?.settings?.streamEnabled }, (run) => {
        acceptedRunId = String(run?.id || '').trim()
        syncEbRoleRunCard(run, { roleId: input.roleId, workspaceId: input.workspaceId, sessionId: input.sessionId, anchorMessageId: contextMessageId })
        renderComposer()
      }, { previousMessageIds, ancestorMessageId: contextMessageId }, (run) => opts?.onRunState?.(run))
      return true
    } catch (e) {
      const msg = String((e as any)?.message || e || `${operationText}失败`)
      if (acceptedRunId && cancelledRunIds.has(acceptedRunId)) showToast?.('已停止', { kind: 'success' })
      else showToast?.(msg, { kind: 'error' })
      return false
    } finally {
      if (acceptedRunId) finishRoleRun(acceptedRunId)
      render()
      scrollToBottomSoon()
    }
  }

  async function runGroupSpeakerSequence(input: { groupId: string; sessionId: string; roleIds: string[]; operationText: string; contextMessageId?: string; message?: string; attachments?: any[]; parentMessageId?: string; clearComposerDraftKey?: string }, opts?: ExistingMessageRunOptions) {
    const state = getState()
    const groupId = String(input.groupId || '').trim()
    let sessionId = String(input.sessionId || '').trim()
    let contextMessageId = String(input.contextMessageId || '').trim()
    const roleIds = Array.isArray(input.roleIds) ? input.roleIds.map((roleId) => String(roleId || '').trim()).filter(Boolean) : []
    if (!groupId || !roleIds.length) return false
    if (!(await waitForCurrentChatSettingsSave())) return false

    let clearDraftOnce = !!String(input.clearComposerDraftKey || '').trim()
    try {
      renderComposer()
      for (let index = 0; index < roleIds.length; index++) {
        const roleId = roleIds[index]
        const role = sa.getRoleById(roleId)
        if (!role) throw new Error('群组成员角色不存在')
        sa.ensureRoleDefaults(role)

        const isFirstMessageRun = index === 0 && (!!String(input.message || '').trim() || (Array.isArray(input.attachments) && input.attachments.length > 0))
        if (!isFirstMessageRun && !contextMessageId) throw new Error('未找到可用于继续发言的上文')

        const chatBeforeRun = sa.activeChatFromData()
        const previousMessageIds = collectChatMessageIds(chatBeforeRun)
        const anchorMessageId = isFirstMessageRun ? String(input.parentMessageId || '').trim() : contextMessageId
        const runInput: RoleRunInput = {
          roleId,
          groupId,
          sessionId,
          stream: !!state.data?.settings?.streamEnabled,
        }
        if (isFirstMessageRun) {
          runInput.message = String(input.message || '').trim()
          runInput.attachments = Array.isArray(input.attachments) ? input.attachments : []
          if (anchorMessageId) runInput.parentMessageId = anchorMessageId
        } else {
          runInput.contextMessageId = contextMessageId
        }

        let acceptedRunId = ''
        try {
          const nextSessionId = await runRoleMessageViaEb(runInput, (run) => {
            acceptedRunId = String(run?.id || '').trim()
            syncEbRoleRunCard(run, { roleId, groupId, sessionId, anchorMessageId })
            if (clearDraftOnce) {
              clearComposerDraftByKey(state, input.clearComposerDraftKey)
              clearDraftOnce = false
            }
            renderComposer()
          }, { previousMessageIds, ancestorMessageId: anchorMessageId }, (run) => opts?.onRunState?.(run))
          sessionId = String(nextSessionId || sessionId || '').trim()
        } catch (e) {
          const msg = String((e as any)?.message || e || `${input.operationText || '群组发言'}失败`)
          if (acceptedRunId && cancelledRunIds.has(acceptedRunId)) showToast?.('已停止', { kind: 'success' })
          else showToast?.(msg, { kind: 'error' })
          return false
        } finally {
          if (acceptedRunId) finishRoleRun(acceptedRunId)
        }

        const activeChat = sa.activeChatFromData()
        contextMessageId = activeChatHeadMid(activeChat) || contextMessageId
      }
      return true
    } finally {
      render()
      scrollToBottomSoon()
    }
  }

  async function runGroupSingleSpeaker(input: { groupId: string; sessionId: string; roleId: string; contextMessageId: string; operationText: string }, opts?: ExistingMessageRunOptions) {
    return runGroupSpeakerSequence({ groupId: input.groupId, sessionId: input.sessionId, roleIds: [input.roleId], contextMessageId: input.contextMessageId, operationText: input.operationText }, opts)
  }

  function ensureTargetSessionMessageMutationAllowed(target: { targetKind: ChatTargetKind; targetId: string; sessionId: string; chat: any }, messageId: string, operation: MessageMutationOperation) {
    const conflict = messageMutationConflict(target.chat, messageId, { operation, activeRunCards: activeEbRunCardsForTarget(getState(), target.targetKind, target.targetId, target.sessionId) })
    if (!conflict.blocked) return true
    showToast?.(conflict.reason || '这条消息正在被运行中的回复使用，稍后再操作', { kind: 'error' })
    return false
  }

  function chatHasPendingToolConfirmation(chat: any, messageId: string, decisionId: string) {
    const message = findChatMessageById(chat, messageId)
    const parts = Array.isArray((message as any)?.parts) ? (message as any).parts : []
    return parts.some((part: any) => {
      const decision = part?.decision && typeof part.decision === 'object' ? part.decision : null
      return String(part?.type || '') === 'tool'
        && String(part?.state || '').trim() === 'needs_confirmation'
        && String(decision?.status || '').trim() === 'needs_confirmation'
        && String(decision?.id || '').trim() === decisionId
    })
  }

  async function submitToolConfirmationDecision(input: { messageId?: string; decisionId?: string; approved?: boolean }) {
    const state = getState()
    if (state.loading || !state.data) return false
    const target = await activeTargetSessionMutationTarget()
    const messageId = String(input?.messageId || '').trim()
    const decisionId = String(input?.decisionId || '').trim()
    if (!target || !messageId || !decisionId) return false
    if (!chatHasPendingToolConfirmation(target.chat, messageId, decisionId)) {
      showToast?.('确认项已更新，请刷新会话后再试', { kind: 'error' })
      return false
    }
    if (typeof netRequest !== 'function') {
      showToast?.('e-b 请求通道不可用', { kind: 'error' })
      return false
    }
    try {
      const approved = !!input?.approved
      await submitToolConfirmationRequest(netRequest, { decisionId, approved, reason: approved ? '' : '用户拒绝工具调用' })
      showToast?.(approved ? '已同意工具执行' : '已拒绝工具执行', { kind: 'success' })
      await refreshTargetSession(target.targetKind, target.targetId, target.sessionId, undefined, { activate: false })
      render()
      return true
    } catch (e: any) {
      showToast?.(String(e?.message || e || '工具确认提交失败'), { kind: 'error' })
      render()
      return false
    }
  }

  async function applyTargetSessionMessageMutation(messageId: any, operationText: string, operation: MessageMutationOperation, mutate: (message: any) => { ok: true } | { ok: false; error: string }) {
    const state = getState()
    if (state.loading || !state.data) return false
    const target = await activeTargetSessionMutationTarget()
    const mid = String(messageId || '').trim()
    if (!target || !mid) return false
    const messages = Array.isArray(target.chat?.messages) ? target.chat.messages : []
    const message = messages.find((item: any) => String(item?.id || '').trim() === mid) || null
    if (!message) {
      showToast?.('消息不存在', { kind: 'error' })
      return false
    }
    if (!ensureTargetSessionMessageMutationAllowed(target, mid, operation)) return false
    const messageIndex = messages.indexOf(message)
    const beforeMessage = clonePlain(message)
    const beforeChatUpdatedAt = target.chat.updatedAt
    const result = mutate(message)
    if (!result.ok) {
      showToast?.(result.error, { kind: 'error' })
      return false
    }
    message.updatedAt = now()
    target.chat.updatedAt = message.updatedAt
    if (typeof netRequest !== 'function') {
      showToast?.('e-b 请求通道不可用', { kind: 'error' })
      if (messageIndex >= 0) messages[messageIndex] = beforeMessage
      target.chat.updatedAt = beforeChatUpdatedAt
      render()
      return false
    }
    try {
      if (target.targetKind === 'group') await updateGroupSessionMessage(netRequest, { groupId: target.groupId, sessionId: target.sessionId, messageId: mid, content: String(message.content ?? ''), parts: Array.isArray(message.parts) ? message.parts : [] })
      else if (target.targetKind === 'workspace') await updateWorkspaceSessionMessage(netRequest, { workspaceId: target.workspaceId, sessionId: target.sessionId, messageId: mid, content: String(message.content ?? ''), parts: Array.isArray(message.parts) ? message.parts : [] })
      else await updateRoleSessionMessage(netRequest, { roleId: target.roleId, sessionId: target.sessionId, messageId: mid, content: String(message.content ?? ''), parts: Array.isArray(message.parts) ? message.parts : [] })
    } catch (e: any) {
      if (messageIndex >= 0) messages[messageIndex] = beforeMessage
      target.chat.updatedAt = beforeChatUpdatedAt
      showToast?.(String(e?.message || e || `消息块${operationText}失败`), { kind: 'error' })
      render()
      return false
    }
    render()
    return true
  }

  // ============ draft image ============

  function addDraftImage(name: any, dataUrl: any, draftKeyRaw?: any) {
    const state = getState()
    if (!looksLikeImageDataUrl(dataUrl)) return false
    const draftKey = String(draftKeyRaw || activeComposerDraftKey(state)).trim()
    if (!draftKey) return false
    if (!draftKeyRaw) activateComposerDraftForCurrentSession(state)
    const draft = readComposerDraftByKey(state, draftKey)
    if (draft.images.length >= MAX_DRAFT_IMAGES) return false
    setComposerDraftImagesByKey(state, draftKey, draft.images.concat({ id: uid('img'), name: String(name || '图片'), dataUrl: String(dataUrl || '') }))
    return true
  }

  // ============ pick images ============

  async function pickDraftImages() {
    const state = getState()
    if (state.loading) return
    if (typeof pickImageFiles !== 'function') return showToast?.('未授权：files.pickImages', { kind: 'error' })

    activateComposerDraftForCurrentSession(state)
    const draftKey = activeComposerDraftKey(state)
    if (!draftKey) return showToast?.('请先选择会话', { kind: 'error' })
    const draft = readComposerDraftByKey(state, draftKey)
    const left = Math.max(0, MAX_DRAFT_IMAGES - draft.images.length)
    if (!left) return showToast?.(`最多选择 ${MAX_DRAFT_IMAGES} 张图片`, { kind: 'error' })

    try {
      const items = await pickImageFiles(left)
      const list = Array.isArray(items) ? items : []
      let added = 0
      for (const it of list) {
        const name = String(it?.name || '图片')
        const dataUrl = String(it?.dataUrl || '')
        if (addDraftImage(name, dataUrl, draftKey)) added++
      }
      if (!added) showToast?.('未选择图片', { kind: 'error' })
    } catch (e) {
      showToast?.(String((e as any)?.message || e || '选择图片失败'), { kind: 'error' })
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
    if (!list.length) return showToast?.('未识别到图片', { kind: 'error' })

    const draftKey = activeComposerDraftKey(state)
    if (!draftKey) return showToast?.('请先选择会话', { kind: 'error' })
    activateComposerDraftForCurrentSession(state)
    const draft = readComposerDraftByKey(state, draftKey)
    const left = Math.max(0, MAX_DRAFT_IMAGES - draft.images.length)
    if (!left) return showToast?.(`最多选择 ${MAX_DRAFT_IMAGES} 张图片`, { kind: 'error' })

    let added = 0
    for (const f of list.slice(0, left)) {
      try {
        const dataUrl = await readImageFileAsDataUrl(f)
        if (addDraftImage(String(f?.name || '图片'), dataUrl, draftKey)) added++
      } catch (_) {}
    }

    if (!added) showToast?.('未识别到图片', { kind: 'error' })
    renderComposer()
  }

  // ============ add draft files ============

  async function addDraftFilesFromFiles(files: File[]) {
    const state = getState()
    if (state.loading) return
    const list = Array.isArray(files) ? files.filter((f) => f instanceof File) : []
    if (!list.length) return
    const draftKey = activeComposerDraftKey(state)
    if (!draftKey) return showToast?.('请先选择会话', { kind: 'error' })
    activateComposerDraftForCurrentSession(state)
    const draft = readComposerDraftByKey(state, draftKey)
    const left = Math.max(0, MAX_DRAFT_FILES - draft.files.length)
    if (!left) return showToast?.(`最多选择 ${MAX_DRAFT_FILES} 个文件`, { kind: 'error' })

    let added = 0
    for (const f of list.slice(0, left)) {
      const kind = detectDraftFileKind(f)
      if (!kind) {
        showToast?.(`不支持的文件：${String(f?.name || '文件')}`, { kind: 'error' })
        continue
      }
      const nextFiles = readComposerDraftByKey(state, draftKey).files.slice()
      const it = addDraftFilePlaceholder(nextFiles, f, kind)
      if (!it) break
      setComposerDraftFilesByKey(state, draftKey, nextFiles)
      added++
      emit()
      ;(async () => {
        try {
          const r = await extractTextFromFile(f, kind)
          const currentState = getState()
          const currentDraft = readComposerDraftByKey(currentState, draftKey)
          const cur = currentDraft.files.find((x: any) => String(x?.id || '') === it.id) || null
          if (!cur) return
          cur.text = String(r || '')
          if (!cur.text) cur.error = '未提取到文本'
          setComposerDraftFilesByKey(currentState, draftKey, currentDraft.files)
        } catch (e) {
          const currentState = getState()
          const currentDraft = readComposerDraftByKey(currentState, draftKey)
          const cur = currentDraft.files.find((x: any) => String(x?.id || '') === it.id) || null
          if (!cur) return
          cur.error = String((e as any)?.message || e || '解析失败')
          setComposerDraftFilesByKey(currentState, draftKey, currentDraft.files)
        } finally {
          const currentState = getState()
          const currentDraft = readComposerDraftByKey(currentState, draftKey)
          const cur = currentDraft.files.find((x: any) => String(x?.id || '') === it.id) || null
          if (cur) cur.pending = false
          if (cur) setComposerDraftFilesByKey(currentState, draftKey, currentDraft.files)
          emit()
        }
      })().catch(() => {})
    }
    if (!added) showToast?.('未选择文件', { kind: 'error' })
    emit()
  }

  function currentAttachSendLimitChars() {
    const value = Number(stateDataSettings()?.attachments?.sendLimitChars ?? DEFAULT_ATTACH_SEND_LIMIT_CHARS)
    return clamp(Math.round(value), 1000, 2_000_000)
  }

  function stateDataSettings() {
    const state = getState()
    return state?.data?.settings && typeof state.data.settings === 'object' ? state.data.settings : {}
  }

  function buildRunAttachments(draftImages: any[], draftFiles: DraftFileItem[]) {
    const attachments: any[] = []
    for (const image of draftImages) {
      const dataUrl = String(image?.dataUrl || '').trim()
      if (!looksLikeImageDataUrl(dataUrl)) throw new Error(`图片无效：${String(image?.name || '图片')}`)
      attachments.push({ kind: 'image', name: String(image?.name || '图片'), dataUrl })
    }

    const sendLimit = currentAttachSendLimitChars()
    for (const file of draftFiles) {
      const name = String(file?.name || '文件')
      if (file?.pending) throw new Error('文件解析中，请稍候…')
      const error = String(file?.error || '').trim()
      if (error) throw new Error(`${name} 解析失败：${error}`)
      const raw = String(file?.text || '').trim()
      if (!raw) throw new Error(`${name} 未提取到可发送文本`)
      const sendPct = clamp(Math.round(Number(file?.sendPct ?? 100)), 0, 100)
      const fullLen = raw.length
      const sendLen = Math.max(0, Math.ceil((fullLen * sendPct) / 100))
      if (sendLen <= 0) throw new Error(`${name} 的发送内容为空`)
      if (sendLen > sendLimit) throw new Error(`${name} 发送内容超过限制，请在附件设置里调低发送比例`)
      attachments.push({ kind: String(file?.kind || 'txt'), name, lang: String(file?.kind || '') === 'md' ? 'markdown' : 'text', text: raw.slice(0, sendLen), fullLen, sendLen, sendPct })
    }
    return attachments
  }

  // ============ send chat ============

  async function sendChat(opts?: SendChatOptions) {
    const state = getState()
    if (state.loading || !state.data) return

    if (sa.activeTargetKind() === 'group') {
      await sendGroupChat(opts)
      return
    }

    if (!(await waitForCurrentChatSettingsSave())) return

    const role = sa.activeRole()
    if (!role) return
    sa.ensureRoleDefaults(role)

    activateComposerDraftForCurrentSession(state)
    const draftKey = activeComposerDraftKey(state)
    const composerDraft = draftKey ? readComposerDraftByKey(state, draftKey) : readActiveComposerDraft(state)
    const input = String(composerDraft.input || '').trim()
    const draftImages = Array.isArray(composerDraft.images) ? composerDraft.images : []
    const draftFiles: DraftFileItem[] = Array.isArray(composerDraft.files) ? (composerDraft.files as any[]) : []
    const hasFiles = draftFiles.length > 0
    if (!input && !draftImages.length && !hasFiles) return showToast?.('输入不能为空', { kind: 'error' })
    let attachments: any[] = []
    try {
      attachments = buildRunAttachments(draftImages, draftFiles)
    } catch (e) {
      return showToast?.(String((e as any)?.message || e || '附件无效'), { kind: 'error' })
    }

    const rid = String(role.id || '')
    const workspaceId = sa.activeTargetKind() === 'workspace' ? String(sa.activeWorkspace?.()?.id || '').trim() : ''
    const pendingKind: ChatTargetKind = workspaceId ? 'workspace' : 'role'
    const pendingTargetId = workspaceId || rid
    const pendingChat = pendingChatForTarget(state, pendingKind, pendingTargetId)
    const loadedChat = pendingChat ? null : await ensureActiveChatLoaded?.().catch(() => null)
    const currentChat = pendingChat ? null : loadedChat || sa.activeChatFromData()
    const modelOverride = normalizeChatModelOverride(pendingChat || currentChat)

    let chat = pendingChat ? null : currentChat
    let sessionId = String(chat?.id || '').trim()
    if (sessionId) sessionId = String(chat?.id || sessionId).trim()
    const forkFromMid = String(opts?.forkFromMid || '').trim()
    const branchDraft = state.branchDraft && typeof state.branchDraft === 'object' ? state.branchDraft : null
    const branchDraftParentId =
      branchDraft && String(branchDraft?.roleId || '') === rid && String(branchDraft?.chatId || '') === sessionId
        ? String(branchDraft?.forkFromMid || '').trim()
        : ''
    const explicitParentMid = forkFromMid || branchDraftParentId
    const parentMessageId = sessionId ? stableExplicitParentForSend(rid, sessionId, chat, explicitParentMid) || roleMessageParentForSend(chat) : ''
    const hasExplicitParent = !!forkFromMid || !!branchDraftParentId
    if (sessionId && !ensureStableComposerParent(rid, sessionId, chat, parentMessageId, hasExplicitParent)) return
    const previousMessageIds = collectChatMessageIds(chat)
    let acceptedRunId = ''
    try {
      renderComposer()
      const reasoningEffort = chatReasoningEffort(pendingChat || currentChat)
      await runRoleMessageViaEb({ roleId: rid, workspaceId, sessionId, message: input, attachments, parentMessageId, reasoningEffort, modelOverride, stream: !!state.data?.settings?.streamEnabled }, (run) => {
        acceptedRunId = String(run?.id || '').trim()
        syncEbRoleRunCard(run, { roleId: rid, workspaceId, sessionId, anchorMessageId: parentMessageId })
        clearComposerDraftByKey(state, draftKey)
        state.branchDraft = null
        renderComposer()
        render()
      }, { previousMessageIds, ancestorMessageId: parentMessageId }, (run) => opts?.onRunState?.(run))
    } catch (e) {
      const msg = String((e as any)?.message || e || '请求失败')
      if (acceptedRunId && cancelledRunIds.has(acceptedRunId)) showToast?.('已停止', { kind: 'success' })
      else showToast?.(msg, { kind: 'error' })
    } finally {
      if (acceptedRunId) finishRoleRun(acceptedRunId)
      render()
    }
  }

  // ============ send group chat ============

  async function sendGroupChat(opts?: SendChatOptions) {
    const state = getState()
    if (state.loading || !state.data) return

    const group = sa.activeGroup()
    if (!group) return

    activateComposerDraftForCurrentSession(state)
    const draftKey = activeComposerDraftKey(state)
    const composerDraft = draftKey ? readComposerDraftByKey(state, draftKey) : readActiveComposerDraft(state)
    const input = String(composerDraft.input || '').trim()
    const draftImages = Array.isArray(composerDraft.images) ? composerDraft.images : []
    const draftFiles: DraftFileItem[] = Array.isArray(composerDraft.files) ? (composerDraft.files as any[]) : []
    const hasFiles = draftFiles.length > 0
    if (!input && !draftImages.length && !hasFiles) return showToast?.('输入不能为空', { kind: 'error' })

    let attachments: any[] = []
    try {
      attachments = buildRunAttachments(draftImages, draftFiles)
    } catch (e) {
      return showToast?.(String((e as any)?.message || e || '附件无效'), { kind: 'error' })
    }

    const groupId = String(group.id || '').trim()
    const currentChat = (await ensureActiveChatLoaded?.().catch(() => null)) || sa.activeChatFromData()
    const pendingChat = pendingChatForTarget(state, 'group', groupId)
    const sessionChat = pendingChat ? null : currentChat
    const speakerPlan = buildGroupSpeakerPlan(group, (roleId) => !!sa.getRoleById(roleId))
    if (speakerPlan.error) return showToast?.(speakerPlan.error, { kind: 'error' })

    const currentSessionId = String(sessionChat?.id || '').trim()
    const parentMessageId = currentSessionId ? roleMessageParentForSend(sessionChat) : ''
    if (currentSessionId && !ensureStableComposerParentForTarget('group', groupId, currentSessionId, sessionChat, parentMessageId, false)) return

    return runGroupSpeakerSequence({
      groupId,
      sessionId: currentSessionId,
      roleIds: speakerPlan.roleIds,
      message: input,
      attachments,
      parentMessageId,
      clearComposerDraftKey: draftKey,
      operationText: '群组发送',
    }, opts)
  }

  // ============ stop sending ============

  async function stopSending(runIdRaw?: any) {
    const state = getState()
    if (state.loading) return

    const explicitRunId = String(runIdRaw || '').trim()
    const targetKind = sa.activeTargetKind()
    const targetId = targetKind === 'group' ? String(sa.activeGroup()?.id || '').trim() : targetKind === 'workspace' ? String(sa.activeWorkspace?.()?.id || '').trim() : String(sa.activeRole()?.id || '').trim()
    const sessionId = String(sa.activeChatFromData()?.id || '').trim()
    const activeRun = explicitRunId
      ? findEbRoleRunCard(state, explicitRunId)
      : latestTargetRunCard(targetKind, targetId, sessionId) || activeEbRoleRunCards(state).slice(-1)[0] || null
    if (activeRun) {
      const runId = String(activeRun.runId || '').trim()
      if (!runId) return showToast?.('当前运行尚未拿到 e-b run id，请稍候再试', { kind: 'error' })
      if (typeof netRequest !== 'function') return showToast?.('e-b 请求通道不可用', { kind: 'error' })
      try {
        cancelledRunIds.add(runId)
        if (activeRun) markEbRoleRunCardCancelled(state, runId)
        await cancelRoleRun(netRequest, runId)
        showToast?.('已请求停止', { kind: 'success' })
        const roleId = String(activeRun?.roleId || sa.activeRole()?.id || '').trim()
        const groupId = String((activeRun as any)?.groupId || sa.activeGroup()?.id || '').trim()
        const workspaceId = String((activeRun as any)?.workspaceId || sa.activeWorkspace?.()?.id || '').trim()
        const nextSessionId = String(activeRun?.sessionId || sa.activeChatFromData()?.id || '').trim()
        if (groupId && nextSessionId) refreshTargetSession('group', groupId, nextSessionId, undefined, { activate: false }).catch(() => {})
        else if (workspaceId && nextSessionId) refreshTargetSession('workspace', workspaceId, nextSessionId, undefined, { activate: false }).catch(() => {})
        else if (roleId && nextSessionId) refreshRoleSession(roleId, nextSessionId, undefined, { activate: false }).catch(() => {})
        renderComposer()
      } catch (e) {
        showToast?.(String((e as any)?.message || e || '停止失败'), { kind: 'error' })
      }
      return
    }
    showToast?.('当前没有可停止的 e-b 真实运行', { kind: 'error' })
  }

  // ============ regenerate assistant message ============

  async function regenerateAssistantMessage(assistantMid: any, opts?: ExistingMessageRunOptions) {
    const state = getState()
    if (state.loading || !state.data) return

    if (sa.activeTargetKind() === 'group') {
      await regenerateGroupAssistantMessage(String(assistantMid || ''), opts)
      return
    }
    const target = await activeSingleRoleTarget()
    const mid = String(assistantMid || '').trim()
    if (!target || !mid) return false
    const assistant = findChatMessageById(target.chat, mid)
    if (!assistant || String((assistant as any).role || '') !== 'assistant') return showToast?.('只能重新生成 AI 消息', { kind: 'error' })
    if (!ensureTargetSessionMessageMutationAllowed(target, mid, 'edit')) return false
    const contextMessageId = String((assistant as any).parentMid || '').trim()
    if (!contextMessageId) return showToast?.('未找到可用于重新回复的上文', { kind: 'error' })
    if (!findChatMessageById(target.chat, contextMessageId)) return showToast?.('未找到可用于重新回复的上文', { kind: 'error' })
    return runRoleFromContextMessage({ roleId: target.roleId, workspaceId: (target as any).workspaceId, sessionId: target.sessionId, contextMessageId }, '重新回复', opts)
  }

  // ============ regenerate group assistant message ============

  async function regenerateGroupAssistantMessage(assistantMid: string, opts?: ExistingMessageRunOptions) {
    const state = getState()
    if (state.loading || !state.data) return

    const target = await activeTargetSessionMutationTarget()
    const mid = String(assistantMid || '').trim()
    if (!target || !mid) return false
    const assistant = findChatMessageById(target.chat, mid)
    if (!assistant || String((assistant as any).role || '') !== 'assistant') return showToast?.('只能重新生成 AI 消息', { kind: 'error' })
    const speakerRoleId = String((assistant as any).speakerRoleId || '').trim()
    if (!speakerRoleId) return showToast?.('群组消息缺少发言人标记', { kind: 'error' })
    if (!ensureTargetSessionMessageMutationAllowed(target, mid, 'edit')) return false
    const contextMessageId = String((assistant as any).parentMid || '').trim()
    if (!contextMessageId) return showToast?.('未找到可用于重新回复的上文', { kind: 'error' })
    return runGroupSingleSpeaker({
      groupId: target.groupId,
      sessionId: target.sessionId,
      roleId: speakerRoleId,
      contextMessageId,
      operationText: '重新回复',
    }, opts)
  }

  // ============ reply from user message ============

  async function replyFromUserMessage(userMid: any, opts?: ExistingMessageRunOptions) {
    const state = getState()
    if (state.loading || !state.data) return

    if (sa.activeTargetKind() === 'group') {
      await replyFromUserMessageInGroup(String(userMid || ''), opts)
      return
    }
    const target = await activeSingleRoleTarget()
    const userMessageId = String(userMid || '').trim()
    if (!target || !userMessageId) return false
    const userMessage = findChatMessageById(target.chat, userMessageId)
    if (!userMessage || String((userMessage as any).role || '') !== 'user') return showToast?.('只能从用户消息继续回复', { kind: 'error' })
    return runRoleFromUserMessage({ roleId: target.roleId, workspaceId: (target as any).workspaceId, sessionId: target.sessionId, userMessageId }, '继续回复', opts)
  }

  // ============ reply from user message in group ============

  async function replyFromUserMessageInGroup(userMid: string, opts?: ExistingMessageRunOptions) {
    const state = getState()
    if (state.loading || !state.data) return

    const target = await activeTargetSessionMutationTarget()
    const userMessageId = String(userMid || '').trim()
    if (!target || !userMessageId) return false
    const userMessage = findChatMessageById(target.chat, userMessageId)
    if (!userMessage || String((userMessage as any).role || '') !== 'user') return showToast?.('只能从用户消息继续回复', { kind: 'error' })
    const speakerPlan = buildGroupSpeakerPlan(sa.activeGroup(), (roleId) => !!sa.getRoleById(roleId))
    if (speakerPlan.error) return showToast?.(speakerPlan.error, { kind: 'error' })
    return runGroupSpeakerSequence({
      groupId: target.groupId,
      sessionId: target.sessionId,
      roleIds: speakerPlan.roleIds,
      contextMessageId: userMessageId,
      operationText: '继续回复',
    }, opts)
  }

  // ============ create parallel branch from assistant message ============

  async function createParallelBranchFromAssistantMessage(assistantMid: any) {
    const state = getState()
    if (state.loading || !state.data) return

    await ensureActiveChatLoaded?.()

    const targetMeta = await activeSingleRoleTarget()
    const role = sa.activeRole()
    const chat = sa.activeChatFromData()
    if (!targetMeta || !role || !chat) return
    sa.ensureRoleDefaults(role)

    const mid = String(assistantMid || '').trim()
    if (!mid) return

    const msgs = Array.isArray(chat.messages) ? chat.messages : []
    const target = msgs.find((m: any) => String(m?.id || '') === mid) || null
    if (!target || target.role !== 'assistant') return showToast?.('只能从 AI 消息新建分支', { kind: 'error' })
    if (!ensureTargetSessionMessageMutationAllowed(targetMeta, mid, 'edit')) return false

    const userMid0 = String((target as any)?.parentMid || '').trim()
    const userMsg = userMid0 ? msgs.find((m: any) => String(m?.id || '') === userMid0) || null : null
    if (!userMsg || userMsg.role !== 'user') return showToast?.('未找到对应的用户消息', { kind: 'error' })

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

    if (!prevAiMid) return showToast?.('未找到上一条 AI 消息，无法新建分支', { kind: 'error' })

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
    const target = await activeTargetSessionMutationTarget()
    const mid = String(messageId || '').trim()
    if (!target || !mid) return false
    if (!ensureTargetSessionMessageMutationAllowed(target, mid, 'delete')) return false
    if (typeof netRequest !== 'function') { showToast?.('e-b 请求通道不可用', { kind: 'error' }); return false }
    try {
      if (target.targetKind === 'group') await deleteGroupSessionMessage(netRequest, { groupId: target.groupId, sessionId: target.sessionId, messageId: mid })
      else if (target.targetKind === 'workspace') await deleteWorkspaceSessionMessage(netRequest, { workspaceId: target.workspaceId, sessionId: target.sessionId, messageId: mid })
      else await deleteRoleSessionMessage(netRequest, { roleId: target.roleId, sessionId: target.sessionId, messageId: mid })
      await refreshTargetSession(target.targetKind, target.targetId, target.sessionId)
      render()
      return true
    } catch (e: any) {
      showToast?.(String(e?.message || e || '消息删除失败'), { kind: 'error' })
      render()
      return false
    }
  }

  // ============ delete message subtree ============

  async function deleteMessageSubtree(messageId: any) {
    const state = getState()
    if (state.loading || !state.data) return
    const target = await activeTargetSessionMutationTarget()
    const mid = String(messageId || '').trim()
    if (!target || !mid) return false
    if (!ensureTargetSessionMessageMutationAllowed(target, mid, 'delete-subtree')) return false
    if (typeof netRequest !== 'function') { showToast?.('e-b 请求通道不可用', { kind: 'error' }); return false }
    try {
      if (target.targetKind === 'group') await deleteGroupSessionMessageSubtree(netRequest, { groupId: target.groupId, sessionId: target.sessionId, messageId: mid })
      else if (target.targetKind === 'workspace') await deleteWorkspaceSessionMessageSubtree(netRequest, { workspaceId: target.workspaceId, sessionId: target.sessionId, messageId: mid })
      else await deleteRoleSessionMessageSubtree(netRequest, { roleId: target.roleId, sessionId: target.sessionId, messageId: mid })
      await refreshTargetSession(target.targetKind, target.targetId, target.sessionId)
      render()
      return true
    } catch (e: any) {
      showToast?.(String(e?.message || e || '消息删除失败'), { kind: 'error' })
      render()
      return false
    }
  }

  // ============ edit message ============

  async function editMessage(messageId: any, content: any) {
    return applyTargetSessionMessageMutation(messageId, '编辑', 'edit', (message) => replaceMessageText(message, content))
  }

  async function editMessageBlock(messageId: any, blockRef: any, text: any) {
    return applyTargetSessionMessageMutation(messageId, '编辑', 'edit', (message) => editAssistantMessageBlock(message, blockRef, text))
  }

  async function deleteMessageBlock(messageId: any, blockRef: any) {
    return applyTargetSessionMessageMutation(messageId, '删除', 'delete', (message) => deleteAssistantMessageBlock(message, blockRef))
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
    submitToolConfirmationDecision,
    deleteMessage,
    deleteMessageSubtree,
    editMessage,
    editMessageBlock,
    deleteMessageBlock,
  }
}

function clonePlain<T>(value: T): T {
  try {
    return JSON.parse(JSON.stringify(value))
  } catch (_) {
    return value
  }
}
