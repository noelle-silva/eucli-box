export type CustomScrollAxis = 'x' | 'y'

export type CustomScrollMetrics = {
  canY: boolean
  canX: boolean
  yTop: number
  yHeight: number
  xLeft: number
  xWidth: number
}

type DragState = {
  axis: CustomScrollAxis
  pointerId: number
  startPointer: number
  startScroll: number
  maxScroll: number
  maxThumbTravel: number
}

type DecoratedScrollArea = {
  refresh: () => void
  destroy: () => void
}

type ScrollHost = {
  el: HTMLElement
  wrapperCreated: boolean
}

export const CUSTOM_SCROLL_EDGE_INSET = 4
export const CUSTOM_SCROLL_MIN_THUMB_SIZE = 34
export const CUSTOM_SCROLL_THUMB_THICKNESS = 7
export const CUSTOM_SCROLL_THUMB_ATTR = 'data-eucli-scroll-thumb'
export const CUSTOM_SCROLL_DECORATED_ATTR = 'data-eucli-scroll-decorated'
export const CUSTOM_SCROLL_INLINE_HOST_ATTR = 'data-eucli-inline-scroll-host'

const CUSTOM_SCROLL_CONTENT_SELECTOR = [
  'pre',
  'table',
  '.math-block',
  '.mermaid-block',
  '.mermaid-error',
  '.fw-tool-pre',
].join(',')

export const customScrollbarHiddenSx = {
  scrollbarWidth: 'none',
  msOverflowStyle: 'none',
  '&::-webkit-scrollbar': { display: 'none', width: 0, height: 0 },
} as const

export function sameCustomScrollMetrics(a: CustomScrollMetrics, b: CustomScrollMetrics) {
  return (
    a.canY === b.canY &&
    a.canX === b.canX &&
    Math.round(a.yTop) === Math.round(b.yTop) &&
    Math.round(a.yHeight) === Math.round(b.yHeight) &&
    Math.round(a.xLeft) === Math.round(b.xLeft) &&
    Math.round(a.xWidth) === Math.round(b.xWidth)
  )
}

export function measureCustomScrollArea(el: HTMLElement): CustomScrollMetrics {
  const height = Math.max(0, el.clientHeight)
  const width = Math.max(0, el.clientWidth)
  const maxY = Math.max(0, el.scrollHeight - height)
  const maxX = Math.max(0, el.scrollWidth - width)
  const yRange = Math.max(0, height - CUSTOM_SCROLL_EDGE_INSET * 2)
  const xRange = Math.max(0, width - CUSTOM_SCROLL_EDGE_INSET * 2)
  const yHeight = maxY > 1 ? Math.min(yRange, Math.max(CUSTOM_SCROLL_MIN_THUMB_SIZE, (height / Math.max(el.scrollHeight, 1)) * yRange)) : 0
  const xWidth = maxX > 1 ? Math.min(xRange, Math.max(CUSTOM_SCROLL_MIN_THUMB_SIZE, (width / Math.max(el.scrollWidth, 1)) * xRange)) : 0
  const yTop = maxY > 1 && yRange > yHeight ? CUSTOM_SCROLL_EDGE_INSET + (el.scrollTop / maxY) * (yRange - yHeight) : CUSTOM_SCROLL_EDGE_INSET
  const xLeft = maxX > 1 && xRange > xWidth ? CUSTOM_SCROLL_EDGE_INSET + (el.scrollLeft / maxX) * (xRange - xWidth) : CUSTOM_SCROLL_EDGE_INSET

  return {
    canY: maxY > 1 && yRange > CUSTOM_SCROLL_MIN_THUMB_SIZE,
    canX: maxX > 1 && xRange > CUSTOM_SCROLL_MIN_THUMB_SIZE,
    yTop,
    yHeight,
    xLeft,
    xWidth,
  }
}

export function hideNativeScrollbar(el: HTMLElement) {
  ;(el.style as any).scrollbarWidth = 'none'
  ;(el.style as any).msOverflowStyle = 'none'
}

export function decorateContentScrollbars(root: HTMLElement): DecoratedScrollArea {
  const targets = collectScrollableContentTargets(root)
  const areas = targets.map((target) => attachDecoratedScrollArea(target)).filter((area): area is DecoratedScrollArea => !!area)

  return {
    refresh: () => areas.forEach((area) => area.refresh()),
    destroy: () => areas.forEach((area) => area.destroy()),
  }
}

function collectScrollableContentTargets(root: HTMLElement) {
  const all = new Set<HTMLElement>()
  if (root.matches(CUSTOM_SCROLL_CONTENT_SELECTOR)) all.add(root)
  root.querySelectorAll<HTMLElement>(CUSTOM_SCROLL_CONTENT_SELECTOR).forEach((el) => all.add(el))
  return [...all].filter((el) => !el.closest(`[${CUSTOM_SCROLL_THUMB_ATTR}="1"]`))
}

function attachDecoratedScrollArea(target: HTMLElement): DecoratedScrollArea | null {
  const host = ensureScrollableHost(target)
  const scrollEl = host.el
  if (!scrollEl || scrollEl.getAttribute(CUSTOM_SCROLL_DECORATED_ATTR) === '1') return null
  scrollEl.setAttribute(CUSTOM_SCROLL_DECORATED_ATTR, '1')
  hideNativeScrollbar(scrollEl)
  ensurePositioned(scrollEl)

  let drag: DragState | null = null
  let hovering = false
  const yThumb = createThumb('y')
  const xThumb = createThumb('x')
  scrollEl.appendChild(yThumb)
  scrollEl.appendChild(xThumb)

  const refresh = () => {
    const metrics = measureCustomScrollArea(scrollEl)
    setThumbVisibility(yThumb, metrics.canY)
    setThumbVisibility(xThumb, metrics.canX)
    if (metrics.canY) {
      yThumb.style.top = `${metrics.yTop}px`
      yThumb.style.right = '3px'
      yThumb.style.width = `${CUSTOM_SCROLL_THUMB_THICKNESS}px`
      yThumb.style.height = `${metrics.yHeight}px`
    }
    if (metrics.canX) {
      xThumb.style.left = `${metrics.xLeft}px`
      xThumb.style.bottom = '3px'
      xThumb.style.width = `${metrics.xWidth}px`
      xThumb.style.height = `${CUSTOM_SCROLL_THUMB_THICKNESS}px`
    }
  }

  const updateIdleOpacity = () => {
    if (drag) return
    yThumb.style.opacity = hovering ? '0.82' : '0'
    xThumb.style.opacity = hovering ? '0.82' : '0'
  }

  const onMouseEnter = () => {
    hovering = true
    updateIdleOpacity()
  }

  const onMouseLeave = () => {
    hovering = false
    updateIdleOpacity()
  }

  const onPointerMove = (event: PointerEvent) => {
    if (!drag || event.pointerId !== drag.pointerId) return
    event.preventDefault()
    event.stopPropagation()
    const pointer = drag.axis === 'y' ? event.clientY : event.clientX
    const delta = pointer - drag.startPointer
    const next = drag.maxThumbTravel > 0 ? drag.startScroll + delta * (drag.maxScroll / drag.maxThumbTravel) : drag.startScroll
    if (drag.axis === 'y') scrollEl.scrollTop = next
    else scrollEl.scrollLeft = next
    refresh()
  }

  const endDrag = (event?: PointerEvent) => {
    if (!drag) return
    drag = null
    setActiveThumb(yThumb, false)
    setActiveThumb(xThumb, false)
    updateIdleOpacity()
    if (event) {
      event.preventDefault()
      event.stopPropagation()
    }
    window.removeEventListener('pointermove', onPointerMove)
    window.removeEventListener('pointerup', endDrag)
    window.removeEventListener('pointercancel', endDrag)
  }

  const beginDrag = (axis: CustomScrollAxis, event: PointerEvent) => {
    event.preventDefault()
    event.stopPropagation()
    const metrics = measureCustomScrollArea(scrollEl)
    const thumbSize = axis === 'y' ? metrics.yHeight : metrics.xWidth
    const maxScroll = axis === 'y' ? scrollEl.scrollHeight - scrollEl.clientHeight : scrollEl.scrollWidth - scrollEl.clientWidth
    const maxThumbTravel = axis === 'y'
      ? scrollEl.clientHeight - CUSTOM_SCROLL_EDGE_INSET * 2 - thumbSize
      : scrollEl.clientWidth - CUSTOM_SCROLL_EDGE_INSET * 2 - thumbSize
    drag = {
      axis,
      pointerId: event.pointerId,
      startPointer: axis === 'y' ? event.clientY : event.clientX,
      startScroll: axis === 'y' ? scrollEl.scrollTop : scrollEl.scrollLeft,
      maxScroll: Math.max(0, maxScroll),
      maxThumbTravel: Math.max(1, maxThumbTravel),
    }
    setActiveThumb(axis === 'y' ? yThumb : xThumb, true)
    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', endDrag)
    window.addEventListener('pointercancel', endDrag)
  }

  yThumb.addEventListener('pointerdown', (event) => beginDrag('y', event))
  xThumb.addEventListener('pointerdown', (event) => beginDrag('x', event))
  scrollEl.addEventListener('mouseenter', onMouseEnter)
  scrollEl.addEventListener('mouseleave', onMouseLeave)
  scrollEl.addEventListener('scroll', refresh, { passive: true })
  const observer = new ResizeObserver(refresh)
  observer.observe(scrollEl)
  requestAnimationFrame(refresh)

  return {
    refresh,
    destroy: () => {
      endDrag()
      observer.disconnect()
      scrollEl.removeEventListener('mouseenter', onMouseEnter)
      scrollEl.removeEventListener('mouseleave', onMouseLeave)
      scrollEl.removeEventListener('scroll', refresh)
      yThumb.remove()
      xThumb.remove()
      scrollEl.removeAttribute(CUSTOM_SCROLL_DECORATED_ATTR)
      if (host.wrapperCreated) unwrapScrollableHost(scrollEl)
    },
  }
}

function ensureScrollableHost(target: HTMLElement): ScrollHost {
  if (target.tagName.toLowerCase() !== 'table') return { el: target, wrapperCreated: false }
  const parent = target.parentElement
  if (parent?.getAttribute(CUSTOM_SCROLL_INLINE_HOST_ATTR) === '1') return { el: parent, wrapperCreated: false }
  if (!parent) return { el: target, wrapperCreated: false }
  const wrapper = document.createElement('div')
  wrapper.setAttribute(CUSTOM_SCROLL_INLINE_HOST_ATTR, '1')
  parent.insertBefore(wrapper, target)
  wrapper.appendChild(target)
  return { el: wrapper, wrapperCreated: true }
}

function unwrapScrollableHost(wrapper: HTMLElement) {
  const parent = wrapper.parentElement
  if (!parent) return
  while (wrapper.firstChild) parent.insertBefore(wrapper.firstChild, wrapper)
  wrapper.remove()
}

function ensurePositioned(el: HTMLElement) {
  const style = window.getComputedStyle(el)
  if (style.position === 'static') el.style.position = 'relative'
  if (!el.style.maxWidth) el.style.maxWidth = '100%'
}

function createThumb(axis: CustomScrollAxis) {
  const thumb = document.createElement('span')
  thumb.setAttribute(CUSTOM_SCROLL_THUMB_ATTR, '1')
  thumb.setAttribute('aria-hidden', 'true')
  thumb.dataset.axis = axis
  thumb.style.position = 'absolute'
  thumb.style.zIndex = '4'
  thumb.style.borderRadius = '999px'
  thumb.style.background = 'rgba(71,85,105,.48)'
  thumb.style.boxShadow = '0 8px 18px rgba(15,23,42,.16), inset 0 0 0 1px rgba(255,255,255,.42)'
  thumb.style.cursor = 'grab'
  thumb.style.touchAction = 'none'
  thumb.style.opacity = '0'
  thumb.style.pointerEvents = 'auto'
  thumb.style.transition = 'background-color 120ms ease, opacity 120ms ease'
  return thumb
}

function setThumbVisibility(thumb: HTMLElement, visible: boolean) {
  thumb.style.display = visible ? 'block' : 'none'
  if (visible && thumb.parentElement?.matches(':hover')) thumb.style.opacity = '0.82'
}

function setActiveThumb(thumb: HTMLElement, active: boolean) {
  thumb.style.opacity = active ? '1' : '0.82'
  thumb.style.background = active ? 'rgba(51,65,85,.72)' : 'rgba(71,85,105,.48)'
  thumb.style.cursor = active ? 'grabbing' : 'grab'
}
