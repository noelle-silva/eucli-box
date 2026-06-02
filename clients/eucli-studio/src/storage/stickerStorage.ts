import { STICKERS_KEY } from '../domain/constants'
import { looksLikeImageDataUrl } from '../domain/textProcessing'
import type { AiChatImageStorageAdapter, AiChatPersistentStorageAdapter } from './types'

type NetRequest = (req: any) => Promise<any>

export function createStickerStorage(deps: {
  filesImages: AiChatImageStorageAdapter
  storage: AiChatPersistentStorageAdapter
  netRequest: NetRequest
  getState: () => any
}) {
  const { filesImages, storage, netRequest, getState } = deps

  async function loadStickersFromSource() {
    const stickers = await storage.get(STICKERS_KEY)
    const data = getState()
    if (data && typeof data === 'object') {
      if (!data.settings || typeof data.settings !== 'object') data.settings = {}
      data.settings.stickers = stickers && typeof stickers === 'object' ? stickers : {}
    }
    return stickers && typeof stickers === 'object' ? stickers : {}
  }

  async function setStickersEnabled(enabled: any) {
    await storage.set(STICKERS_KEY, { enabled: !!enabled })
    return loadStickersFromSource()
  }

  async function ebRequest(req: any) {
    if (typeof netRequest !== 'function') throw new Error('e-b request 不可用')
    const res = await netRequest(req)
    const status = Number(res?.status || 200)
    if (status < 200 || status >= 300) throw new Error(`HTTP ${status}`)
    return res?.body
  }

  async function addStickerInternal(cat: any, name: any, dataUrl: any) {
    const data = getState()
    if (!data) return { ok: false, kind: 'no-data' as const }

    const st = data.settings?.stickers && typeof data.settings.stickers === 'object' ? data.settings.stickers : {}
    const box = st.map && typeof st.map === 'object' ? st.map[cat] : null
    if (box && typeof box === 'object' && box[name]) return { ok: false, kind: 'dup' as const }

    const u = String(dataUrl || '').trim()
    if (!looksLikeImageDataUrl(u)) return { ok: false, kind: 'bad-image' as const }
    const item = await ebRequest({ method: 'POST', path: '/api/stickers/items', body: { categoryName: cat, stickerName: name, dataUrl: u }, timeoutMs: 30000 })
    return { ok: true, kind: 'ok' as const, relPath: String(item?.relPath || ''), item }
  }

  async function createStickerCategoryInternal(categoryName: any) {
    await ebRequest({ method: 'POST', path: '/api/stickers/categories', body: { categoryName } })
    return loadStickersFromSource()
  }

  async function deleteStickerCategoryInternal(categoryName: any) {
    await ebRequest({ method: 'DELETE', path: `/api/stickers/categories/${encodeURIComponent(String(categoryName || ''))}` })
    return loadStickersFromSource()
  }

  async function deleteStickerInternal(categoryName: any, stickerName: any) {
    await ebRequest({ method: 'DELETE', path: '/api/stickers/items', body: { categoryName, stickerName } })
    return loadStickersFromSource()
  }

  async function renameStickerInternal(categoryName: any, oldStickerName: any, newStickerName: any) {
    await ebRequest({ method: 'PATCH', path: '/api/stickers/items/name', body: { categoryName, oldStickerName, newStickerName } })
    return loadStickersFromSource()
  }

  async function syncRoleAvatarFile(folder: any, role: any) {
    const f = String(folder || '').trim()
    if (!f) return

    const relPath = `roles/${f}/avatar.png`
    const avatarImage = String(role?.avatarImage || '').trim()

    if (looksLikeImageDataUrl(avatarImage)) {
      if (typeof filesImages?.writeBase64 !== 'function') throw new Error('未授权：files.images.writeBase64')
      await filesImages.writeBase64({ scope: 'data', relPath, overwrite: true, dataUrlOrBase64: avatarImage })
      return
    }

    if (typeof filesImages?.delete !== 'function') throw new Error('未授权：files.images.delete')
    await filesImages.delete({ scope: 'data', path: relPath })
  }

  async function syncGroupAvatarFile(folder: any, group: any) {
    const f = String(folder || '').trim()
    if (!f) return

    const relPath = `groups/${f}/avatar.png`
    const avatarImage = String(group?.avatarImage || '').trim()

    if (looksLikeImageDataUrl(avatarImage)) {
      if (typeof filesImages?.writeBase64 !== 'function') throw new Error('未授权：files.images.writeBase64')
      await filesImages.writeBase64({ scope: 'data', relPath, overwrite: true, dataUrlOrBase64: avatarImage })
      return
    }

    if (typeof filesImages?.delete !== 'function') throw new Error('未授权：files.images.delete')
    await filesImages.delete({ scope: 'data', path: relPath })
  }

  function getStickerRelPath(category: any, name: any) {
    const cat = typeof category === 'string' ? category.trim() : ''
    const nm = typeof name === 'string' ? name.trim() : ''
    if (!cat || !nm) return ''
    const data = getState()
    const st = data?.settings?.stickers
    const box = st && typeof st === 'object' ? st.map?.[cat] : null
    const it = box && typeof box === 'object' ? box[nm] : null
    const relPath = it && typeof it === 'object' ? String(it.relPath || '').trim() : ''
    return relPath
  }

  return {
    addStickerInternal,
    createStickerCategoryInternal,
    deleteStickerCategoryInternal,
    deleteStickerInternal,
    renameStickerInternal,
    loadStickersFromSource,
    setStickersEnabled,
    syncRoleAvatarFile,
    syncGroupAvatarFile,
    getStickerRelPath,
  }
}
