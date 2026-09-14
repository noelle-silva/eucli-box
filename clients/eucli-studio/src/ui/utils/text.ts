export function snippetText(raw: any, maxLen = 26) {
  const s0 = typeof raw === 'string' ? raw : String(raw ?? '')
  const s1 = s0.replace(/\r/g, '').trim()
  if (!s1) return '（空）'
  const firstLine = (s1.split('\n').find((x) => String(x || '').trim()) || '').trim()
  const s = firstLine || s1.split('\n')[0] || s1
  const one = s.replace(/\s+/g, ' ').trim()
  if (one.length <= maxLen) return one
  return one.slice(0, Math.max(0, maxLen - 1)).trimEnd() + '…'
}
