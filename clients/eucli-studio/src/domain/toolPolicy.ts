export type RoleToolRunMode = 'ask' | 'direct'

export type RoleToolPolicy = {
  tools: string[]
  runModes: Record<string, RoleToolRunMode>
}

export function emptyRoleToolPolicy(): RoleToolPolicy {
  return { tools: [], runModes: {} }
}

export function normalizeRoleToolPolicy(value: any): RoleToolPolicy {
  const source = value && typeof value === 'object' ? value : {}
  const runModesSource = source.runModes && typeof source.runModes === 'object' && !Array.isArray(source.runModes) ? source.runModes : {}
  const tools: string[] = []
  const runModes: Record<string, RoleToolRunMode> = {}
  const seen = new Set<string>()

  for (const item of Array.isArray(source.tools) ? source.tools : []) {
    const name = String(item || '').trim()
    if (!name || seen.has(name)) continue
    const mode = normalizeToolRunMode((runModesSource as any)[name])
    seen.add(name)
    tools.push(name)
    if (mode) runModes[name] = mode
  }

  return { tools, runModes }
}

export function addToolsToPolicy(policy: any, toolNames: string[]): RoleToolPolicy {
  const next = normalizeRoleToolPolicy(policy)
  const seen = new Set(next.tools)
  for (const item of toolNames) {
    const name = String(item || '').trim()
    if (!name || seen.has(name)) continue
    seen.add(name)
    next.tools.push(name)
    next.runModes[name] = 'ask'
  }
  return next
}

export function removeToolFromPolicy(policy: any, toolName: string): RoleToolPolicy {
  const name = String(toolName || '').trim()
  const next = normalizeRoleToolPolicy(policy)
  if (!name) return next
  next.tools = next.tools.filter((item) => item !== name)
  delete next.runModes[name]
  return next
}

export function setToolRunMode(policy: any, toolName: string, mode: any): RoleToolPolicy {
  const name = String(toolName || '').trim()
  const runMode = normalizeToolRunMode(mode)
  const next = normalizeRoleToolPolicy(policy)
  if (!name || !runMode || !next.tools.includes(name)) return next
  next.runModes[name] = runMode
  return next
}

export function normalizeToolRunMode(value: any): RoleToolRunMode | '' {
  const mode = String(value || '').trim()
  if (mode === 'direct') return 'direct'
  if (mode === 'ask') return 'ask'
  return ''
}

export function toolPolicyToolCount(policy: any): number {
  return normalizeRoleToolPolicy(policy).tools.length
}
