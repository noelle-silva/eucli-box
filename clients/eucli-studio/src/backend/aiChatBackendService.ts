import type { AiChatDirectEvent } from '../protocol/aiChatProtocol'
import { AI_CHAT_DIRECT_METHOD } from '../protocol/aiChatProtocol'
import { AiChatDirectError } from '../protocol/aiChatProtocolGuards'
import type { AiChatCapabilities } from '../gateway/capabilities'

export type AiChatBackendService = {
  dispatch: (method: string, params: unknown) => Promise<unknown>
  dispose: () => Promise<void>
}

export function createAiChatBackendService(opts: {
  capabilities: AiChatCapabilities
  onEvent?: (event: AiChatDirectEvent) => void
}): AiChatBackendService {
  const cap = opts.capabilities
  void opts.onEvent

  async function dispatch(method: string, params: unknown): Promise<unknown> {
    const p = (params && typeof params === 'object' ? params : {}) as Record<string, unknown>

    switch (method) {
      case AI_CHAT_DIRECT_METHOD.healthCheck:
        return { version: 1, status: 'ok' }

      case AI_CHAT_DIRECT_METHOD.submitChatCompletion: {
        throw new AiChatDirectError('NOT_IMPLEMENTED', '请通过 eucli-box Go 后端提交聊天请求')
      }
      case AI_CHAT_DIRECT_METHOD.submitManyChatCompletions: {
        throw new AiChatDirectError('NOT_IMPLEMENTED', '请通过 eucli-box Go 后端提交聊天请求')
      }
      case AI_CHAT_DIRECT_METHOD.submitRawServiceRequest: {
        throw new AiChatDirectError('NOT_IMPLEMENTED', '请通过 eucli-box Go 后端提交服务请求')
      }

      case AI_CHAT_DIRECT_METHOD.cancelAssistant: {
        const mid = String(p?.assistantMid || '').trim()
        if (!mid) throw new AiChatDirectError('BAD_REQUEST', 'assistantMid is required')
        throw new AiChatDirectError('NOT_IMPLEMENTED', '请通过 eucli-box Go 后端取消任务')
      }
      case AI_CHAT_DIRECT_METHOD.getAssistantRuntime: {
        const mid = String(p?.assistantMid || '').trim()
        if (!mid) throw new AiChatDirectError('BAD_REQUEST', 'assistantMid is required')
        return null
      }
      case AI_CHAT_DIRECT_METHOD.readAssistantStream: {
        const mid = String(p?.assistantMid || '').trim()
        if (!mid) throw new AiChatDirectError('BAD_REQUEST', 'assistantMid is required')
        return null
      }
      case AI_CHAT_DIRECT_METHOD.consumeAssistantFinal: {
        const mid = String(p?.assistantMid || '').trim()
        if (!mid) throw new AiChatDirectError('BAD_REQUEST', 'assistantMid is required')
        return null
      }
      case AI_CHAT_DIRECT_METHOD.resetAssistantRuntime: {
        const mid = String(p?.assistantMid || '').trim()
        if (!mid) throw new AiChatDirectError('BAD_REQUEST', 'assistantMid is required')
        return {}
      }
      case AI_CHAT_DIRECT_METHOD.waitServiceFinal: {
        const mid = String(p?.assistantMid || '').trim()
        const timeoutMs = Number(p?.timeoutMs || 0)
        if (!mid) throw new AiChatDirectError('BAD_REQUEST', 'assistantMid is required')
        throw new AiChatDirectError('NOT_IMPLEMENTED', '请通过 eucli-box Go 后端等待服务结果')
      }

      case AI_CHAT_DIRECT_METHOD.storageGet: {
        const key = String(p?.key || '').trim()
        if (!key) throw new AiChatDirectError('BAD_REQUEST', 'key is required')
        return await cap.storage.get(key)
      }
      case AI_CHAT_DIRECT_METHOD.storageSet: {
        const key = String(p?.key || '').trim()
        if (!key) throw new AiChatDirectError('BAD_REQUEST', 'key is required')
        await cap.storage.set(key, p?.value)
        return {}
      }
      case AI_CHAT_DIRECT_METHOD.storageRemove: {
        const key = String(p?.key || '').trim()
        if (!key) throw new AiChatDirectError('BAD_REQUEST', 'key is required')
        await cap.storage.remove(key)
        return {}
      }

      case AI_CHAT_DIRECT_METHOD.imageRead: {
        const path = String(p?.path || '').trim()
        if (!path) throw new AiChatDirectError('BAD_REQUEST', 'path is required')
        return await cap.files.images.read!({ scope: 'data', path })
      }
      case AI_CHAT_DIRECT_METHOD.imageWrite: {
        if (typeof cap.files?.images?.writeBase64 !== 'function') throw new AiChatDirectError('NOT_IMPLEMENTED', 'imageWrite not available')
        return await cap.files.images.writeBase64(p)
      }
      case AI_CHAT_DIRECT_METHOD.imageDelete: {
        if (typeof cap.files?.images?.delete !== 'function') throw new AiChatDirectError('NOT_IMPLEMENTED', 'imageDelete not available')
        await cap.files.images.delete(p)
        return {}
      }
      case AI_CHAT_DIRECT_METHOD.imagePick:
        throw new AiChatDirectError('NOT_IMPLEMENTED', 'imagePick must be handled by UI host capability')

      case AI_CHAT_DIRECT_METHOD.getPendingConfirmation: {
        return await cap.runtimeStorage.get('toolConfirmation').catch(() => null)
      }
      case AI_CHAT_DIRECT_METHOD.submitConfirmation: {
        const decisionId = String(p?.decisionId || '').trim()
        const approved = Boolean(p?.approved)
        if (!decisionId) throw new AiChatDirectError('BAD_REQUEST', 'decisionId is required')
        throw new AiChatDirectError('NOT_IMPLEMENTED', '请通过 eucli-box Go 后端提交工具确认')
      }

      case AI_CHAT_DIRECT_METHOD.boxConnectionGet: {
        throw new AiChatDirectError('NOT_IMPLEMENTED', '请通过 eucli-box Go 后端读取连接配置')
      }
      case AI_CHAT_DIRECT_METHOD.boxConnectionSave: {
        throw new AiChatDirectError('NOT_IMPLEMENTED', '请通过 eucli-box Go 后端保存连接配置')
      }
      case AI_CHAT_DIRECT_METHOD.boxConnectionTest: {
        throw new AiChatDirectError('NOT_IMPLEMENTED', '请通过 eucli-box Go 后端测试连接')
      }

      default:
        throw new AiChatDirectError('METHOD_NOT_FOUND', `未知方法: ${method}`)
    }
  }

  return { dispatch, async dispose() {} }
}
