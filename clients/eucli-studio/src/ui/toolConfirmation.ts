// AI Studio 工具确认轮询模块
// 职责：轮询 runtime store 中的待确认工具请求，供 V2 controller 使用

export function createToolConfirmationPoller(deps: {
  getPendingConfirmation: () => Promise<any>
  onSubmit: (approved: boolean) => Promise<void>
  onFound: (confirmation: any) => void
  onDisconnected?: () => void
  onError?: (message: string) => void
}) {
  let pollingActive = false
  let pollTimer = 0
  let lastConfirmationId = ''
  let wasDisconnected = false

  function startPolling(intervalMs = 800) {
    if (pollingActive) return
    pollingActive = true
    const tick = async () => {
      if (!pollingActive) return
      try {
        const data = await deps.getPendingConfirmation()
        if (data && typeof data === 'object') {
          const disconnected = !!(data as any)?.disconnected
          if (disconnected) {
            if (!wasDisconnected) {
              wasDisconnected = true
              lastConfirmationId = ''
              deps.onDisconnected?.()
            }
            return
          }
          if (wasDisconnected) wasDisconnected = false
          const id = String((data as any)?.event?.payload?.decisionId || (data as any)?.decisionId || '')
          if (id && id !== lastConfirmationId) {
            lastConfirmationId = id
            deps.onFound(data)
          }
        }
      } catch (e) {
        deps.onError?.(String((e as any)?.message || e || '工具确认通道异常'))
      }
    }
    tick()
    pollTimer = window.setInterval(tick, Math.max(200, Math.floor(intervalMs || 0)))
  }

  function stopPolling() {
    pollingActive = false
    if (pollTimer) {
      window.clearInterval(pollTimer)
      pollTimer = 0
    }
    lastConfirmationId = ''
    wasDisconnected = false
  }

  async function submit(approved: boolean) {
    await deps.onSubmit(approved)
    lastConfirmationId = ''
  }

  return { startPolling, stopPolling, submit }
}
