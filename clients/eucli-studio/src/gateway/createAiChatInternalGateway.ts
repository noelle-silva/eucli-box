import type { AiChatNetAdapter, AiChatRun, AiChatRuntimeStore } from '../engine'
import type { AiChatRunSpec } from '../requestPipeline'
import { assistantFinalKey, assistantStreamKey } from '../runtime/runtimeKeys'
import type { AiChatInternalGateway, AiChatRawServiceRequestInput } from './types'

const errBoxOnlyRuntime = () => new Error('AI 运行必须通过 eucli-box Go 后端；客户端本地运行路径已禁用')

export function createAiChatInternalGateway(opts: {
  runtime: 'ui' | 'background'
  store: AiChatRuntimeStore
  net: AiChatNetAdapter
  onRunFinal: (run: AiChatRun, finalText: string) => Promise<void> | void
  buildRoleReqFromStorage: (jobStub: any) => Promise<any>
  buildGroupReqFromStorage: (jobStub: any) => Promise<any>
  onProgressEvent?: (run: AiChatRun, text: string) => Promise<void> | void
  onFinalEvent?: (run: AiChatRun, finalText: string) => Promise<void> | void
}): AiChatInternalGateway {
  const store = opts.store

  async function consumeAssistantFinal(assistantMid: string) {
    const mid = String(assistantMid || '').trim()
    if (!mid) return null
    let finalValue: any = null
    try {
      finalValue = await store.get(assistantFinalKey(mid))
    } catch (_) {
      finalValue = null
    }
    if (finalValue) {
      try {
        await store.remove(assistantFinalKey(mid))
      } catch (_) {}
    }
    return finalValue
  }

  return {
    startBackgroundWorker: async () => undefined,
    submitRoleChatCompletion: async (_input: AiChatRunSpec) => { throw errBoxOnlyRuntime() },
    submitGroupChatCompletion: async (_input: AiChatRunSpec) => { throw errBoxOnlyRuntime() },
    submitManyChatCompletions: async (_inputs: AiChatRunSpec[]) => { throw errBoxOnlyRuntime() },
    submitRawServiceRequest: async (_input: AiChatRawServiceRequestInput) => { throw errBoxOnlyRuntime() },
    waitServiceFinal: async (assistantMid: string, timeoutMs: number) => {
      void assistantMid
      void timeoutMs
      throw errBoxOnlyRuntime()
    },
    cancelAssistant: async (_assistantMid: string) => undefined,
    getAssistantRuntime: async (_assistantMid: string) => null,
    resetAssistantRuntime: async (assistantMid: string) => {
      const mid = String(assistantMid || '').trim()
      if (!mid) return
      await store.remove(assistantStreamKey(mid)).catch(() => undefined)
      await store.remove(assistantFinalKey(mid)).catch(() => undefined)
    },
    readAssistantStream: async (assistantMid: string) => {
      const mid = String(assistantMid || '').trim()
      if (!mid) return null
      try {
        return await store.get(assistantStreamKey(mid))
      } catch (_) {
        return null
      }
    },
    consumeAssistantFinal,
    getPendingConfirmation: async () => null,
    submitConfirmation: async () => { throw errBoxOnlyRuntime() },
  }
}
