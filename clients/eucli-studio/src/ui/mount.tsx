import * as React from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { EUCLI_STUDIO_CHAT_ROOT_ID, EUCLI_STUDIO_MOUNT_FLAG_KEY } from '../runtime/eucliStudioGlobals'
import { AiChatApp } from './App'

export function mountAiChatUi(controller: any) {
  const w = window as any
  if (w[EUCLI_STUDIO_MOUNT_FLAG_KEY]) return

  const rootStyle = document.documentElement.style
  const bodyStyle = document.body.style
  rootStyle.height = '100%'
  rootStyle.width = '100%'
  rootStyle.overflow = 'hidden'
  ;(rootStyle as any).overscrollBehavior = 'none'
  bodyStyle.height = '100%'
  bodyStyle.width = '100%'
  bodyStyle.margin = '0'
  bodyStyle.overflow = 'hidden'
  ;(bodyStyle as any).overscrollBehavior = 'none'

  let el = document.getElementById(EUCLI_STUDIO_CHAT_ROOT_ID)
  if (!el) {
    document.body.innerHTML = `<div id="${EUCLI_STUDIO_CHAT_ROOT_ID}"></div>`
    el = document.getElementById(EUCLI_STUDIO_CHAT_ROOT_ID)
  }
  if (!el) return
  el.style.height = '100%'

  const root: Root = createRoot(el)
  w[EUCLI_STUDIO_MOUNT_FLAG_KEY] = root
  root.render(<AiChatApp controller={controller} />)
}
