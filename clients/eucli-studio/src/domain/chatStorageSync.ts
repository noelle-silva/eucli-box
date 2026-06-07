import { preserveLocalBranchSelection } from './branching'
import { hasAssistantVisibleOutput, isAssistantGenerating, resolveAssistantMessageForMerge } from './assistantRunState'

function messageId(value: any) {
  return String(value?.id || '').trim()
}

function isAssistantTextMessage(value: any) {
  if (!value || typeof value !== 'object') return false
  const role = String(value?.role || value?.type || '').trim()
  const type = String(value?.type || role || '').trim()
  return role === 'assistant' && type === 'assistant'
}

function mergeMessagesFromStorage(nextChat: any, currentChat: any) {
  const nextMessages = Array.isArray(nextChat?.messages) ? nextChat.messages : null
  const currentMessages = Array.isArray(currentChat?.messages) ? currentChat.messages : null
  if (!nextMessages || !currentMessages || !currentMessages.length) return

  const currentById = new Map<string, any>()
  for (const message of currentMessages) {
    const id = messageId(message)
    if (id && !currentById.has(id)) currentById.set(id, message)
  }

  nextChat.messages = nextMessages.map((storedMessage: any) => {
    const currentMessage = currentById.get(messageId(storedMessage)) || null
    if (!currentMessage) return storedMessage
    if (!isAssistantTextMessage(currentMessage) || !isAssistantTextMessage(storedMessage)) return storedMessage
    if (!isAssistantGenerating(currentMessage) && !isAssistantGenerating(storedMessage)) return storedMessage
    return resolveAssistantMessageForMerge(currentMessage, storedMessage, { storedChatStatus: nextChat?.status })
  })

  const nextIds = new Set(nextChat.messages.map((message: any) => messageId(message)).filter(Boolean))
  for (const currentMessage of currentMessages) {
    const id = messageId(currentMessage)
    if (!id || nextIds.has(id)) continue
    if (!isAssistantTextMessage(currentMessage) || !isAssistantGenerating(currentMessage)) continue
    if (!hasAssistantVisibleOutput(currentMessage)) continue
    nextChat.messages.push(currentMessage)
    nextIds.add(id)
  }
}

export function mergeChatFromStorage(nextChat: any, currentChat: any) {
  const next = preserveLocalBranchSelection(nextChat, currentChat)
  if (!next || !currentChat || typeof next !== 'object' || typeof currentChat !== 'object') return next
  mergeMessagesFromStorage(next, currentChat)
  if ((next as any).runtimePartial) delete (next as any).runtimePartial
  return next
}

export function isStoredChatNewerThanCurrent(storedUpdatedAtRaw: any, currentUpdatedAtRaw: any) {
  const storedUpdatedAt = Number(storedUpdatedAtRaw || 0)
  const currentUpdatedAt = Number(currentUpdatedAtRaw || 0)
  if (!Number.isFinite(storedUpdatedAt) || storedUpdatedAt <= 0) return false
  if (!Number.isFinite(currentUpdatedAt) || currentUpdatedAt <= 0) return true
  return storedUpdatedAt > currentUpdatedAt
}
