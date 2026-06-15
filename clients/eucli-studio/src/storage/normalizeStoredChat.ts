import { normalizeData } from '../domain/dataNormalizers'
import { VERSION } from '../domain/constants'

export type StoredChatKind = 'role' | 'group'

export function normalizeStoredChat(chat: any, kind: StoredChatKind) {
  const fallbackTitle = kind === 'group' ? '群聊' : '新聊天'
  const id = String(chat?.id || '').trim()
  if (!id) return null
  const data: any = {
    version: VERSION,
    settings: { providers: [{ id: '__lazy__', name: '__lazy__', baseUrl: 'http://', apiKey: '' }] },
    favorites: { folders: [], chatRefsByFolderId: {} },
    roles: [{ id: '__lazy_role__', name: '__lazy__', createdAt: 1, updatedAt: 1, modelRef: { providerId: '__lazy__', modelId: '' } }],
    chatsByRole: {
      __lazy_role__: {
        activeChatId: id,
        chats: [{ ...chat, title: String(chat?.title || '').trim() || fallbackTitle }],
      },
    },
    groups: kind === 'group' ? [{ id: '__lazy_group__', name: '__lazy__', createdAt: 1, updatedAt: 1, memberRoleIds: [], roundRobinOrder: [], random: { weightsByRoleId: {}, minCount: 1, maxCount: 1 } }] : [],
    chatsByGroup: kind === 'group'
      ? { __lazy_group__: { activeChatId: id, chats: [{ ...chat, title: String(chat?.title || '').trim() || fallbackTitle }] } }
      : {},
    ui: { activeTargetKind: kind, activeRoleId: '__lazy_role__', activeGroupId: kind === 'group' ? '__lazy_group__' : '' },
  }
  const normalized = normalizeData(data) as any
  return kind === 'group'
    ? normalized.chatsByGroup.__lazy_group__.chats[0] || null
    : normalized.chatsByRole.__lazy_role__.chats[0] || null
}
