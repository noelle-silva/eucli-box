export interface AiServicesDeps {
  netRequest?: (req: any) => Promise<any>
}

const EB_SERVICE_UNAVAILABLE = 'AI 微服务尚未接入 e-b 真实根动作，已阻止旧 provider 直连链'

export function createAiServices(deps: AiServicesDeps) {
  async function aiFixMermaidInMessage(_messageId: any, _mermaidSrc: any, _renderErrorMsg: any) {
    throw new Error(EB_SERVICE_UNAVAILABLE)
  }

  async function aiGenerateChatTitle(_roleId: any, _chatId: any) {
    throw new Error(EB_SERVICE_UNAVAILABLE)
  }

  async function aiGenerateGroupChatTitle(_groupId: any, _chatId: any) {
    throw new Error(EB_SERVICE_UNAVAILABLE)
  }

  async function aiGenerateStickerName(categoryName: any, stickerName: any) {
    const netRequest = deps.netRequest
    if (typeof netRequest !== 'function') throw new Error(EB_SERVICE_UNAVAILABLE)
    const r = await netRequest({
      method: 'POST',
      path: '/api/assist/stickers/name',
      body: {
        categoryName: String(categoryName || '').trim(),
        stickerName: String(stickerName || '').trim(),
      },
      timeoutMs: 90000,
    })
    const status = Number(r?.status || 200)
    if (status < 200 || status >= 300) throw new Error(`HTTP ${status}`)
    return r?.body
  }

  return {
    aiFixMermaidInMessage,
    aiGenerateChatTitle,
    aiGenerateGroupChatTitle,
    aiGenerateStickerName,
  }
}
