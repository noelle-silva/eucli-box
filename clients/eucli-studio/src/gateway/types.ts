import type { AiChatRunSpec, AiChatRunTarget } from '../requestPipeline'

export type AiChatRawServiceRequestInput = {
  target: AiChatRunTarget
  req: any
  stream: boolean
}

export type AiChatInternalGateway = {
  startBackgroundWorker: (intervalMs?: number) => Promise<void>
  submitRoleChatCompletion: (input: AiChatRunSpec) => Promise<void>
  submitGroupChatCompletion: (input: AiChatRunSpec) => Promise<void>
  submitManyChatCompletions: (inputs: AiChatRunSpec[]) => Promise<void>
  submitRawServiceRequest: (input: AiChatRawServiceRequestInput) => Promise<void>
  waitServiceFinal: (assistantMid: string, timeoutMs: number) => Promise<string>
  cancelAssistant: (assistantMid: string) => Promise<void>
  getAssistantRuntime: (assistantMid: string) => Promise<{ runId: string; generationId: string; status: string; active: boolean } | null>
  resetAssistantRuntime: (assistantMid: string) => Promise<void>
  readAssistantStream: (assistantMid: string) => Promise<any>
  consumeAssistantFinal: (assistantMid: string) => Promise<any>
  getPendingConfirmation?: () => Promise<any>
  submitConfirmation?: (decisionId: string, approved: boolean) => Promise<void>
  confirmTool?: (decisionId: string, approved: boolean) => Promise<void>
  getBoxConnection?: () => Promise<{ url: string; key: string }>
  saveBoxConnection?: (connection: { url: string; key: string }) => Promise<void>
  testBoxConnection?: () => Promise<{ status: string; url: string; roles?: number }>
  pushBoxRole?: (role: Record<string, unknown>) => Promise<void>
  pushBoxProvider?: (provider: Record<string, unknown>) => Promise<void>
  deleteBoxRole?: (id: string) => Promise<void>
  deleteBoxProvider?: (id: string) => Promise<void>
  syncBoxCatalog?: () => Promise<void>
  listBoxSessions?: (roleId: string) => Promise<any[]>
  createBoxSession?: (roleId: string) => Promise<any>
  getBoxSession?: (id: string) => Promise<any>
  deleteBoxSession?: (id: string) => Promise<void>
}
