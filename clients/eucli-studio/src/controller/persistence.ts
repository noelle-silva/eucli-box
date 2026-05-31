import type { ChatSaveIntent } from '../domain/chatSaveIntent'

export function createPersistence(deps: {
  getState: () => any
  activeChatFromData: () => any
  saveMetaOnly: () => Promise<void>
  saveSplitData: (data: any) => Promise<void>
  saveRoleChat: (roleId: any, chat: any, intent?: ChatSaveIntent) => Promise<void>
  saveGroupChat: (groupId: any, chat: any, intent?: ChatSaveIntent) => Promise<void>
}) {
  const { getState, activeChatFromData, saveMetaOnly, saveSplitData, saveRoleChat, saveGroupChat } = deps

  function syncDraftUiToData() {
    const state = getState()
    if (!state?.data) return null
    if (!state.data.ui || typeof state.data.ui !== 'object') state.data.ui = {}
    state.data.ui.activeRoleId = String(state.draft?.activeRoleId || '')
    ;(state.data.ui as any).activeGroupId = String(state.draft?.activeGroupId || '')
    ;(state.data.ui as any).activeTargetKind = String(state.draft?.activeTargetKind || '') === 'group' ? 'group' : 'role'
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

    void activeChatFromData
    void intent
  }

  async function saveDataTree() {
    const state = syncDraftUiToData()
    if (!state) return
    await saveMetaOnly()
  }

  return {
    saveMeta,
    saveCurrentChat,
    saveDataTree,
  }
}
