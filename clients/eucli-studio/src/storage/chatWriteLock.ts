import { now, uid } from '../core/utils'
import { UI_CHAT_UPDATED_NOTICE_KEY } from '../runtime/runtimeKeys'

export function createChatWriteLock(deps: {
  rtStorage: { get: (k: string) => Promise<any>; set: (k: string, v: any) => Promise<void>; remove: (k: string) => Promise<void> }
}) {
  const { rtStorage } = deps
  const lockWaitMs = 8000
  const lockTtlMs = 12000

  const sleepMs = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, Math.max(0, Math.floor(ms || 0))))

  function chatWriteLockKey(kind: any, targetId: any, chatId: any) {
    const kindText = String(kind || '').trim()
    const k = kindText === 'group' || kindText === 'g' ? 'g' : 'r'
    const tid = String(targetId || '').trim()
    const cid = String(chatId || '').trim()
    return `lock.chat.${k}.${tid}.${cid}`
  }

  async function withChatWriteLock(kind: any, targetId: any, chatId: any, fn: any) {
    const k = String(kind || '').trim() === 'group' ? 'group' : 'role'
    const tid = String(targetId || '').trim()
    const cid = String(chatId || '').trim()
    if (!tid || !cid) return fn()

    const key = chatWriteLockKey(k, tid, cid)
    const owner = uid('lock')
    const deadline = now() + lockWaitMs
    let acquired = false

    while (now() < deadline) {
      let cur: any = null
      try {
        cur = await rtStorage.get(key)
      } catch (_) {}

      const exp = Number(cur?.expiresAt || 0)
      const curOwner = String(cur?.owner || '').trim()
      if (!cur || exp <= now() || curOwner === owner) {
        const nextExp = now() + lockTtlMs
        try {
          await rtStorage.set(key, { owner, expiresAt: nextExp })
        } catch (_) {}
        try {
          const v = await rtStorage.get(key)
          if (String(v?.owner || '').trim() === owner) {
            acquired = true
            break
          }
        } catch (_) {}
      }

      await sleepMs(40 + Math.floor(Math.random() * 60))
    }

    if (!acquired) throw new Error('聊天保存繁忙，请稍后重试')

    let renewTimer: any = 0
    try {
      // The lock is a runtime coordination fact, not a suggestion. Before
      // writing durable chat data we re-check ownership and extend the lease;
      // if ownership is gone, fail fast instead of doing an unlocked write.
      const v = await rtStorage.get(key)
      if (String(v?.owner || '').trim() !== owner) throw new Error('聊天保存协调权已失效，请重试')
      await rtStorage.set(key, { owner, expiresAt: now() + lockTtlMs })
      renewTimer = setInterval(() => {
        // Long saves may cross the lease window. Renewal keeps the same
        // ownership alive while the write is running, so another writer cannot
        // treat the in-flight save as stale and create a second root fact.
        rtStorage
          .get(key)
          .then((cur: any) => {
            if (String(cur?.owner || '').trim() !== owner) return
            return rtStorage.set(key, { owner, expiresAt: now() + lockTtlMs })
          })
          .catch(() => {})
      }, Math.max(1000, Math.floor(lockTtlMs / 3)))
      return await fn()
    } finally {
      if (renewTimer) clearInterval(renewTimer)
      try {
        const v = await rtStorage.get(key)
        if (String(v?.owner || '').trim() === owner) await rtStorage.remove(key)
      } catch (_) {}
    }
  }

  async function writeChatUpdatedNotice(targetKind: any, targetId: any, chatId: any, updatedAt: any) {
    const kind = String(targetKind || '').trim() === 'group' ? 'group' : 'role'
    const tid = String(targetId || '').trim()
    const cid = String(chatId || '').trim()
    if (!tid || !cid) return
    const t = now()
    try {
      await rtStorage.set(UI_CHAT_UPDATED_NOTICE_KEY, {
        id: uid('n'),
        targetKind: kind,
        targetId: tid,
        chatId: cid,
        updatedAt: Number(updatedAt || 0),
        at: t,
      })
    } catch (_) {}
  }

  return { chatWriteLockKey, withChatWriteLock, writeChatUpdatedNotice }
}
