import { invoke } from '@tauri-apps/api/core'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { createAiChatControllerV2 } from '../controller/createControllerV2'
import type { AiChatController } from '../controller/types'
import { createDirectCapabilitiesAdapter } from '../direct/createDirectCapabilitiesAdapter'
import { createAiChatCapabilitiesFromHostApi } from '../gateway/capabilities'
import { AI_CHAT_DIRECT_PROTOCOL_VERSION } from '../protocol/aiChatProtocol'
import { AI_STUDIO_APP_ID, AI_STUDIO_CONTROLLER_KEY } from '../runtime/aiStudioGlobals'
import { createAiChatDirectGateway } from './aiChatDirectGateway'

type BackendEndpoint = {
  url: string
  token: string
}

export type AiChatAppRuntime = {
  controller: AiChatController
  bootstrap: unknown
  dispose: () => void
}

export type AiChatAppHostOptions = {
  showToast: (message: unknown) => void
  onBack: () => Promise<void> | void
}

export async function createAiChatAppRuntime(options: AiChatAppHostOptions): Promise<AiChatAppRuntime> {
  const baseApi = createAiStudioHostApi(options)
  try {
    const { api, directClient } = await createDirectCapabilitiesAdapter(baseApi)
    let controllerToDispose: AiChatController | null = null
    const capabilities = createAiChatCapabilitiesFromHostApi(api, AI_STUDIO_APP_ID)
    const aiGateway = createAiChatDirectGateway(directClient)
    const created = createAiChatControllerV2({ capabilities, aiGateway })
    const controller = created.controller
    controllerToDispose = controller
    const bootstrap = await directClient.invoke('studio.bootstrap').catch(() => null)

    await created.init()
    ;(window as any)[AI_STUDIO_CONTROLLER_KEY] = controller

    return {
      controller,
      bootstrap,
      dispose() {
        try {
          if ((window as any)[AI_STUDIO_CONTROLLER_KEY] === controller) {
            delete (window as any)[AI_STUDIO_CONTROLLER_KEY]
          }
          controller.dispose()
        } finally {
          directClient.close()
        }
      },
    }
  } catch (error) {
    console.warn('[eucli-studio] direct backend unavailable, fallback to local controller:', error)
    return createFallbackRuntime(baseApi)
  }
}

function createMemoryStore() {
  const map = new Map<string, any>()
  return {
    async get(key: string) {
      return map.has(key) ? map.get(key) : null
    },
    async set(key: string, value: unknown) {
      map.set(key, value)
    },
    async remove(key: string) {
      map.delete(key)
    },
    async getAll() {
      return Object.fromEntries(map.entries())
    },
    async listDir() {
      return []
    },
    async flush() {},
  }
}

export async function createFallbackRuntime(baseApi: any): Promise<AiChatAppRuntime> {
  const storage = createMemoryStore()
  const runtimeStorage = createMemoryStore()
  const hostApi = {
    __meta: { runtime: 'ui', appId: AI_STUDIO_APP_ID },
    storage,
    runtimeStorage,
    net: {
      request: async () => ({ status: 503, body: 'eucli-box unavailable' }),
      requestStream: undefined,
    },
    files: {
      pickImages: baseApi.files?.pickImages,
      images: {
        read: async () => '',
        writeBase64: async () => '',
        delete: async () => {},
      },
    },
    ui: baseApi.ui,
    clipboard: baseApi.clipboard,
    host: baseApi.host,
  }

  const capabilities = createAiChatCapabilitiesFromHostApi(hostApi, AI_STUDIO_APP_ID)
  const aiGateway: any = new Proxy(
    {},
    {
      get(_target, key) {
        const name = String(key || '')
        if (name === 'startBackgroundWorker') return async () => {}
        if (name === 'getPendingConfirmation') return async () => null
        if (name === 'readAssistantStream') return async () => null
        if (name === 'consumeAssistantFinal') return async () => null
        if (name === 'getAssistantRuntime') return async () => null
        return async (..._args: any[]) => {
          throw new Error('eucli-box 尚未接入')
        }
      },
    },
  )

  const created = createAiChatControllerV2({ capabilities, aiGateway })
  const controller = created.controller
  await created.init()
  ;(window as any)[AI_STUDIO_CONTROLLER_KEY] = controller
  return {
    controller,
    bootstrap: null,
    dispose() {
      if ((window as any)[AI_STUDIO_CONTROLLER_KEY] === controller) {
        delete (window as any)[AI_STUDIO_CONTROLLER_KEY]
      }
      controller.dispose()
    },
  }
}

function createAiStudioHostApi(options: AiChatAppHostOptions) {
  return {
    __meta: { runtime: 'ui', appId: AI_STUDIO_APP_ID },
    background: {
      endpoint: createBackendEndpoint,
    },
    files: {
      pickImages: pickImageFiles,
    },
    ui: {
      showToast: options.showToast,
      startDragging: () => getCurrentWindow().startDragging(),
    },
    clipboard: {
      writeText,
      readText,
      writeImage,
    },
    host: {
      back: options.onBack,
      background: {
        endpoint: createBackendEndpoint,
      },
    },
  }
}

async function createBackendEndpoint() {
  const endpoint = await invoke<BackendEndpoint>('backend_endpoint')
  return {
    mode: 'direct',
    transport: 'local-websocket',
    protocolVersion: AI_CHAT_DIRECT_PROTOCOL_VERSION,
    url: endpoint.url,
    token: endpoint.token,
  }
}

async function pickImageFiles(maxCount?: number): Promise<Array<{ name: string; dataUrl: string }>> {
  const limit = Math.max(1, Math.min(20, Math.floor(Number(maxCount || 1))))
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = 'image/*'
  input.multiple = limit > 1
  input.tabIndex = -1
  input.style.position = 'fixed'
  input.style.left = '-10000px'
  input.style.top = '-10000px'

  document.body.appendChild(input)
  try {
    const files = await new Promise<File[]>((resolve) => {
      input.addEventListener(
        'change',
        () => {
          const selected = Array.from(input.files || [])
            .filter((file) => file instanceof File && String(file.type || '').startsWith('image/'))
            .slice(0, limit)
          resolve(selected)
        },
        { once: true },
      )
      input.click()
    })

    const items: Array<{ name: string; dataUrl: string }> = []
    for (const file of files) {
      const dataUrl = await readFileAsDataUrl(file)
      if (dataUrl) items.push({ name: file.name || '图片', dataUrl })
    }
    return items
  } finally {
    input.remove()
  }
}

function readFileAsDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '')
    reader.onerror = () => reject(reader.error || new Error('读取图片失败'))
    reader.readAsDataURL(file)
  })
}

async function writeText(text: unknown): Promise<void> {
  const value = String(text ?? '')
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value)
    return
  }

  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.setAttribute('readonly', 'true')
  textarea.style.position = 'fixed'
  textarea.style.left = '-10000px'
  document.body.appendChild(textarea)
  try {
    textarea.select()
    document.execCommand('copy')
  } finally {
    textarea.remove()
  }
}

async function readText(): Promise<string> {
  if (!navigator.clipboard?.readText) return ''
  return navigator.clipboard.readText()
}

async function writeImage(dataUrl: unknown): Promise<void> {
  const value = String(dataUrl || '').trim()
  if (!value.startsWith('data:image/')) throw new Error('图片剪贴板只支持 data URL')
  const clipboard = navigator.clipboard as Clipboard & { write?: (items: ClipboardItem[]) => Promise<void> }
  if (typeof clipboard?.write !== 'function' || typeof ClipboardItem === 'undefined') {
    throw new Error('当前系统不支持写入图片剪贴板')
  }

  const response = await fetch(value)
  const blob = await response.blob()
  await clipboard.write([new ClipboardItem({ [blob.type || 'image/png']: blob })])
}
