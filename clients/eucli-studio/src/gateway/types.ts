import type { AiChatRunTarget } from '../engine/types'

export type AiChatRunSpec = {
  target: AiChatRunTarget
  stream: boolean
  jobStub: any
}

export type AiChatRawServiceRequestInput = {
  target: AiChatRunTarget
  req: any
  stream: boolean
}

export type AiChatInternalGateway = {
  startBackgroundWorker: (intervalMs?: number) => Promise<void>
  cancelAssistant: (assistantMid: string) => Promise<void>
  getAssistantRuntime: (assistantMid: string) => Promise<{ runId: string; generationId: string; status: string; active: boolean } | null>
  resetAssistantRuntime: (assistantMid: string) => Promise<void>
  readAssistantStream: (assistantMid: string) => Promise<any>
  consumeAssistantFinal: (assistantMid: string) => Promise<any>
}
