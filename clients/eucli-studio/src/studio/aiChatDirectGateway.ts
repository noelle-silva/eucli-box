import { AI_CHAT_DIRECT_METHOD } from '../protocol/aiChatProtocol'
import type { AiChatDirectClient } from '../direct/createAiChatDirectClient'
import type { AiChatInternalGateway, AiChatRawServiceRequestInput } from '../gateway/types'
import type { AiChatRunSpec } from '../requestPipeline'

const ACK_TIMEOUT_MS = 15000
const POLL_TIMEOUT_MS = 10000

export function createAiChatDirectGateway(directClient: AiChatDirectClient): AiChatInternalGateway {
  return {
    async startBackgroundWorker() {
      // The App sidecar owns the background worker. The UI only sends direct requests.
    },
    submitRoleChatCompletion(input: AiChatRunSpec) {
      return directClient.invoke(AI_CHAT_DIRECT_METHOD.submitChatCompletion, input, { timeoutMs: ACK_TIMEOUT_MS }).then(() => undefined)
    },
    submitGroupChatCompletion(input: AiChatRunSpec) {
      return directClient.invoke(AI_CHAT_DIRECT_METHOD.submitChatCompletion, input, { timeoutMs: ACK_TIMEOUT_MS }).then(() => undefined)
    },
    submitManyChatCompletions(inputs: AiChatRunSpec[]) {
      return directClient.invoke(AI_CHAT_DIRECT_METHOD.submitManyChatCompletions, { inputs }, { timeoutMs: ACK_TIMEOUT_MS }).then(() => undefined)
    },
    submitRawServiceRequest(input: AiChatRawServiceRequestInput) {
      return directClient.invoke(AI_CHAT_DIRECT_METHOD.submitRawServiceRequest, input, { timeoutMs: ACK_TIMEOUT_MS }).then(() => undefined)
    },
    waitServiceFinal(assistantMid: string, timeoutMs: number) {
      const requestTimeout = Math.max(ACK_TIMEOUT_MS, Math.floor(Number(timeoutMs || 0)) + 5000)
      return directClient.invoke<string>(AI_CHAT_DIRECT_METHOD.waitServiceFinal, { assistantMid, timeoutMs }, { timeoutMs: requestTimeout })
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
    getPendingConfirmation() {
      return directClient.invoke(AI_CHAT_DIRECT_METHOD.getPendingConfirmation, {}, { timeoutMs: POLL_TIMEOUT_MS })
    },
    submitConfirmation(decisionId: string, approved: boolean) {
      return directClient.invoke(AI_CHAT_DIRECT_METHOD.submitConfirmation, { decisionId, approved }, { timeoutMs: ACK_TIMEOUT_MS }).then(() => undefined)
    },
    confirmTool(decisionId: string, approved: boolean) {
      return directClient.invoke(AI_CHAT_DIRECT_METHOD.boxToolConfirm, { decisionId, approved }, { timeoutMs: ACK_TIMEOUT_MS }).then(() => undefined)
    },
    getBoxConnection() {
      return directClient.invoke<{ url: string; key: string }>(AI_CHAT_DIRECT_METHOD.boxConnectionGet, undefined, { timeoutMs: POLL_TIMEOUT_MS })
    },
    saveBoxConnection(connection: { url: string; key: string }) {
      return directClient.invoke<void>(AI_CHAT_DIRECT_METHOD.boxConnectionSave, connection, { timeoutMs: ACK_TIMEOUT_MS })
    },
    testBoxConnection() {
      return directClient.invoke<{ status: string; url: string; roles?: number }>(AI_CHAT_DIRECT_METHOD.boxConnectionTest, undefined, { timeoutMs: 10000 })
    },
    pushBoxRole(role: Record<string, unknown>) {
      return directClient.invoke<void>(AI_CHAT_DIRECT_METHOD.boxRolePush, role, { timeoutMs: ACK_TIMEOUT_MS })
    },
    pushBoxProvider(provider: Record<string, unknown>) {
      return directClient.invoke<void>(AI_CHAT_DIRECT_METHOD.boxProviderPush, provider, { timeoutMs: ACK_TIMEOUT_MS })
    },
    deleteBoxRole(id: string) {
      return directClient.invoke<void>(AI_CHAT_DIRECT_METHOD.boxRoleDelete, { id }, { timeoutMs: ACK_TIMEOUT_MS })
    },
    deleteBoxProvider(id: string) {
      return directClient.invoke<void>(AI_CHAT_DIRECT_METHOD.boxProviderDelete, { id }, { timeoutMs: ACK_TIMEOUT_MS })
    },
    syncBoxCatalog() {
      return directClient.invoke<void>(AI_CHAT_DIRECT_METHOD.boxCatalogSync, undefined, { timeoutMs: ACK_TIMEOUT_MS })
    },
    listBoxSessions(roleId: string) {
      return directClient.invoke<any[]>(AI_CHAT_DIRECT_METHOD.boxSessionList, { roleId }, { timeoutMs: POLL_TIMEOUT_MS })
    },
    createBoxSession(roleId: string) {
      return directClient.invoke<any>(AI_CHAT_DIRECT_METHOD.boxSessionCreate, { roleId }, { timeoutMs: ACK_TIMEOUT_MS })
    },
    getBoxSession(id: string) {
      return directClient.invoke<any>(AI_CHAT_DIRECT_METHOD.boxSessionGet, { id }, { timeoutMs: POLL_TIMEOUT_MS })
    },
    deleteBoxSession(id: string) {
      return directClient.invoke<void>(AI_CHAT_DIRECT_METHOD.boxSessionDelete, { id }, { timeoutMs: ACK_TIMEOUT_MS })
    },
  }
}
