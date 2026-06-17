function text(value: unknown) {
  return String(value || '').trim()
}

const WORKSPACE_ROLE_TARGET_SEPARATOR = '::'

export function workspaceRoleTargetId(workspaceIdRaw: unknown, roleIdRaw: unknown) {
  const workspaceId = text(workspaceIdRaw)
  const roleId = text(roleIdRaw)
  if (!workspaceId || !roleId) return ''
  return `${workspaceId}${WORKSPACE_ROLE_TARGET_SEPARATOR}${roleId}`
}

export function parseWorkspaceRoleTargetId(targetIdRaw: unknown) {
  const targetId = text(targetIdRaw)
  if (!targetId) return { workspaceId: '', roleId: '' }
  const separatorIndex = targetId.indexOf(WORKSPACE_ROLE_TARGET_SEPARATOR)
  if (separatorIndex <= 0) return { workspaceId: '', roleId: '' }
  const workspaceId = text(targetId.slice(0, separatorIndex))
  const roleId = text(targetId.slice(separatorIndex + WORKSPACE_ROLE_TARGET_SEPARATOR.length))
  if (!workspaceId || !roleId) return { workspaceId: '', roleId: '' }
  return { workspaceId, roleId }
}
