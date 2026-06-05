import { isAssistantGenerating } from './assistantRunState'
import type { EbRoleRunCard } from './activeRunCards'

export type MessageMutationOperation = 'edit' | 'delete' | 'delete-subtree'

export type MessageMutationConflict = {
  blocked: boolean
  reason: string
  messageId: string
  runId: string
}

function text(value: unknown) {
  return String(value || '').trim()
}

function chatMessages(chat: any) {
  return Array.isArray(chat?.messages) ? chat.messages : []
}

function buildMessageTree(chat: any) {
  const messages = chatMessages(chat)
  const byId = new Map<string, any>()
  const children = new Map<string, string[]>()

  for (const message of messages) {
    const id = text(message?.id)
    if (!id || byId.has(id)) continue
    byId.set(id, message)
  }

  for (const message of messages) {
    const id = text(message?.id)
    const parentId = text(message?.parentMid || message?.parentMessageId)
    if (!id || !parentId || !byId.has(parentId)) continue
    const list = children.get(parentId) || []
    list.push(id)
    children.set(parentId, list)
  }

  return { byId, children }
}

function collectSubtreeIds(children: Map<string, string[]>, rootId: string) {
  const ids = new Set<string>()
  const stack = [rootId]
  while (stack.length) {
    const id = text(stack.pop())
    if (!id || ids.has(id)) continue
    ids.add(id)
    for (const childId of children.get(id) || []) stack.push(childId)
  }
  return ids
}

function activeAssistantIds(chat: any) {
  const ids = new Set<string>()
  for (const message of chatMessages(chat)) {
    const id = text(message?.id)
    if (id && isAssistantGenerating(message)) ids.add(id)
  }
  return ids
}

function activeRunMessageIds(cards: EbRoleRunCard[]) {
  const out: Array<{ id: string; runId: string }> = []
  for (const card of cards) {
    const runId = text(card?.runId)
    const ids = [card?.inputMessageId, card?.anchorMessageId, card?.lastMessageId, ...(Array.isArray(card?.dependencyMessageIds) ? card.dependencyMessageIds : [])].map(text).filter(Boolean)
    for (const id of ids) out.push({ id, runId })
  }
  return out
}

function operationReason(operation: MessageMutationOperation) {
  if (operation === 'edit') return '这条消息正在被运行中的回复依赖，等对应任务结束后再编辑'
  if (operation === 'delete-subtree') return '这棵消息分支里还有运行中的回复，等对应任务结束后再删除'
  return '这条消息正在被运行中的回复依赖，等对应任务结束后再删除'
}

export function messageMutationConflict(
  chat: any,
  targetMessageId: unknown,
  options?: { operation?: MessageMutationOperation; activeRunCards?: EbRoleRunCard[] },
): MessageMutationConflict {
  const targetId = text(targetMessageId)
  const operation = options?.operation || 'edit'
  if (!chat || !targetId) return { blocked: false, reason: '', messageId: targetId, runId: '' }

  const tree = buildMessageTree(chat)
  if (!tree.byId.has(targetId)) return { blocked: false, reason: '', messageId: targetId, runId: '' }

  const affectedIds = collectSubtreeIds(tree.children, targetId)
  const activeAssistant = activeAssistantIds(chat)
  for (const messageId of affectedIds) {
    if (activeAssistant.has(messageId)) return { blocked: true, reason: operationReason(operation), messageId, runId: '' }
  }

  const cards = Array.isArray(options?.activeRunCards) ? options.activeRunCards : []
  for (const item of activeRunMessageIds(cards)) {
    if (affectedIds.has(item.id)) return { blocked: true, reason: operationReason(operation), messageId: item.id, runId: item.runId }
  }

  return { blocked: false, reason: '', messageId: targetId, runId: '' }
}

export function canMutateMessage(chat: any, targetMessageId: unknown, options?: { operation?: MessageMutationOperation; activeRunCards?: EbRoleRunCard[] }) {
  return !messageMutationConflict(chat, targetMessageId, options).blocked
}
