import type { ChatSaveIntent } from '../domain/chatSaveIntent'

export function createPersistence(deps: {
  getState: () => any
  activeChatFromData: () => any
  saveMetaOnly: () => Promise<void>
  saveRoleChat: (roleId: any, chat: any, intent?: ChatSaveIntent) => Promise<void>
  saveGroupChat: (groupId: any, chat: any, intent?: ChatSaveIntent) => Promise<void>
  saveWorkspaceChat?: (workspaceId: any, chat: any, intent?: ChatSaveIntent) => Promise<void>
}) {
  const { getState, activeChatFromData, saveMetaOnly, saveRoleChat, saveGroupChat, saveWorkspaceChat } = deps

  function syncDraftUiToData() {
    const state = getState()
    if (!state?.data) return null
    if (!state.data.ui || typeof state.data.ui !== 'object') state.data.ui = {}
    state.data.ui.activeRoleId = String(state.draft?.activeRoleId || '')
    ;(state.data.ui as any).activeGroupId = String(state.draft?.activeGroupId || '')
    ;(state.data.ui as any).activeWorkspaceId = String((state.draft as any)?.activeWorkspaceId || '')
    const targetKind = String(state.draft?.activeTargetKind || '').trim()
    ;(state.data.ui as any).activeTargetKind = targetKind === 'group' ? 'group' : targetKind === 'workspace' ? 'workspace' : 'role'
    return state
  }

  async function saveMeta() {
    const state = syncDraftUiToData()
    if (!state) return
    await saveMetaOnly()
  }

  async function saveCurrentChat(intent?: ChatSaveIntent) {
    const state = syncDraftUiToData()
    if (!state) return
    await saveMetaOnly()

    const targetKind = String(state.draft?.activeTargetKind || '').trim()
    const kind = targetKind === 'group' ? 'group' : targetKind === 'workspace' ? 'workspace' : 'role'
    const targetId = kind === 'group' ? String(state.draft?.activeGroupId || '') : kind === 'workspace' ? String((state.draft as any)?.activeWorkspaceId || '') : String(state.draft?.activeRoleId || '')
    const chat = activeChatFromData()
    if (!targetId || !chat) return
    if (kind === 'group') await saveGroupChat(targetId, chat, intent)
    else if (kind === 'workspace') await saveWorkspaceChat?.(targetId, chat, intent)
    else await saveRoleChat(targetId, chat, intent)
  }

  return {
    saveMeta,
    saveCurrentChat,
  }
}
