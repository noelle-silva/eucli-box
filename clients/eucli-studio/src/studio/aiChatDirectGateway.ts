import { AI_CHAT_DIRECT_METHOD } from '../protocol/aiChatProtocol'
import type { AiChatDirectClient } from '../direct/createAiChatDirectClient'
import type { AiChatInternalGateway } from '../gateway/types'

const ACK_TIMEOUT_MS = 15000
const POLL_TIMEOUT_MS = 10000

export function createAiChatDirectGateway(directClient: AiChatDirectClient): AiChatInternalGateway {
  return {
    async startBackgroundWorker() {
      // The App sidecar owns the background worker. The UI only sends direct requests.
    },
    cancelAssistant(assistantMid: string) {
      return directClient.invoke(AI_CHAT_DIRECT_METHOD.cancelAssistant, { assistantMid }, { timeoutMs: ACK_TIMEOUT_MS }).then(() => undefined)
    },
    getAssistantRuntime(assistantMid: string) {
      return directClient.invoke(AI_CHAT_DIRECT_METHOD.getAssistantRuntime, { assistantMid }, { timeoutMs: POLL_TIMEOUT_MS })
    },
    resetAssistantRuntime(assistantMid: string) {
      return directClient.invoke(AI_CHAT_DIRECT_METHOD.resetAssistantRuntime, { assistantMid }, { timeoutMs: ACK_TIMEOUT_MS }).then(() => undefined)
    },
    readAssistantStream(assistantMid: string) {
      return directClient.invoke(AI_CHAT_DIRECT_METHOD.readAssistantStream, { assistantMid }, { timeoutMs: POLL_TIMEOUT_MS })
    },
    consumeAssistantFinal(assistantMid: string) {
      return directClient.invoke(AI_CHAT_DIRECT_METHOD.consumeAssistantFinal, { assistantMid }, { timeoutMs: POLL_TIMEOUT_MS })
    },
  }
}
