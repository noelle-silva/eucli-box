export interface AiServicesDeps {
  // AI microservices are currently blocked until e-b exposes real roots.
}

const EB_SERVICE_UNAVAILABLE = 'AI 微服务尚未接入 e-b 真实根动作，已阻止旧 provider 直连链'

export function createAiServices(_deps: AiServicesDeps) {
  async function aiFixMermaidInMessage(_messageId: any, _mermaidSrc: any, _renderErrorMsg: any) {
    throw new Error(EB_SERVICE_UNAVAILABLE)
  }

  async function aiGenerateChatTitle(_roleId: any, _chatId: any) {
    throw new Error(EB_SERVICE_UNAVAILABLE)
  }

  async function aiGenerateGroupChatTitle(_groupId: any, _chatId: any) {
    throw new Error(EB_SERVICE_UNAVAILABLE)
  }

  async function aiGenerateStickerName(_categoryName: any, _stickerName: any) {
    throw new Error(EB_SERVICE_UNAVAILABLE)
  }

  return {
    aiFixMermaidInMessage,
    aiGenerateChatTitle,
    aiGenerateGroupChatTitle,
    aiGenerateStickerName,
  }
}
