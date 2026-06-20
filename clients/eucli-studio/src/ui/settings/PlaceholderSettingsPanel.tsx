import * as React from 'react'
import { Box, Button, Checkbox, Divider, FormControl, InputLabel, MenuItem, Paper, Select, Stack, TextField, Typography } from '@mui/material'
import AddIcon from '@mui/icons-material/Add'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import RefreshIcon from '@mui/icons-material/Refresh'
import SaveIcon from '@mui/icons-material/Save'
import {
  createPlaceholderFolder,
  createPlaceholderItem,
  normalizePlaceholderLibrary,
  placeholderProblemLabel,
  type PlaceholderDependencyNode,
  type PlaceholderFolder,
  type PlaceholderItem,
  type PlaceholderLibrary,
} from '../../domain/placeholder'

type PlaceholderSettingsPanelProps = {
  controller: any
  loading: boolean
  placeholders: any
}

function text(value: unknown) {
  return String(value ?? '').trim()
}

function cloneLibrary(raw: unknown): PlaceholderLibrary {
  return normalizePlaceholderLibrary(raw)
}

function folderDepth(folder: PlaceholderFolder, folders: PlaceholderFolder[]) {
  let depth = 0
  let parentId = text(folder.parentId)
  const seen = new Set<string>()
  while (parentId && !seen.has(parentId)) {
    seen.add(parentId)
    const parent = folders.find((item) => item.id === parentId)
    if (!parent) break
    depth += 1
    parentId = text(parent.parentId)
  }
  return depth
}

function sortedFolders(folders: PlaceholderFolder[]) {
  return folders.slice().sort((left, right) => {
    const depth = folderDepth(left, folders) - folderDepth(right, folders)
    return depth || left.name.localeCompare(right.name)
  })
}

function formatTime(value: unknown) {
  const raw = text(value)
  if (!raw) return '未知时间'
  const time = Date.parse(raw)
  if (!isFinite(time)) return raw
  return new Date(time).toLocaleString('zh-CN')
}

export function PlaceholderSettingsPanel(props: PlaceholderSettingsPanelProps) {
  const { controller, loading, placeholders } = props
  const sourceLibrary = placeholders?.library || { placeholders: [], folders: [] }
  const busy = loading || !!placeholders?.loading
  const [draft, setDraft] = React.useState<PlaceholderLibrary>(() => cloneLibrary(sourceLibrary))
  const [selectedIndex, setSelectedIndex] = React.useState(0)
  const [selectedFolderId, setSelectedFolderId] = React.useState('')
  const [previewText, setPreviewText] = React.useState('')
  const [saving, setSaving] = React.useState(false)
  const [saveError, setSaveError] = React.useState('')

  React.useEffect(() => {
    const next = cloneLibrary(sourceLibrary)
    setDraft(next)
    setSelectedIndex((current) => Math.min(Math.max(0, current), Math.max(0, next.placeholders.length - 1)))
  }, [sourceLibrary])

  React.useEffect(() => {
    controller.actions.refreshPlaceholderLibrary?.(false)
  }, [controller])

  React.useEffect(() => {
    const timer = window.setTimeout(() => {
      controller.actions.previewPlaceholders?.(previewText)?.catch?.(() => null)
    }, 240)
    return () => window.clearTimeout(timer)
  }, [controller, previewText])

  React.useEffect(() => {
    const selectedName = text(draft.placeholders[selectedIndex]?.name)
    if (!selectedName) return
    controller.actions.loadPlaceholderDependencies?.(selectedName)?.catch?.(() => null)
  }, [controller, draft.placeholders, selectedIndex])

  const selectedPlaceholder = draft.placeholders[selectedIndex] || null
  const selectedFolder = draft.folders.find((folder) => folder.id === selectedFolderId) || null
  const filteredPlaceholders = draft.placeholders
    .map((item, index) => ({ item, index }))
    .filter(({ item }) => selectedFolder ? selectedFolder.placeholderNames?.includes(item.name) : true)
  const nameCounts = draft.placeholders.reduce((acc, item) => {
    const name = text(item.name)
    if (name) acc[name] = (acc[name] || 0) + 1
    return acc
  }, {} as Record<string, number>)
  const hasEmptyName = draft.placeholders.some((item) => !text(item.name))
  const hasDuplicateName = Object.values(nameCounts).some((count) => count > 1)
  const canSave = !busy && !saving && !hasEmptyName && !hasDuplicateName

  const replacePlaceholder = (index: number, updater: (item: PlaceholderItem) => PlaceholderItem) => {
    setDraft((current) => ({ ...current, placeholders: current.placeholders.map((item, itemIndex) => (itemIndex === index ? updater(item) : item)) }))
  }

  const createItem = () => {
    const item = createPlaceholderItem()
    const names = new Set(draft.placeholders.map((existing) => text(existing.name)).filter(Boolean))
    let nextName = '新占位符'
    let suffix = 2
    while (names.has(nextName)) {
      nextName = `新占位符 ${suffix}`
      suffix += 1
    }
    setDraft((current) => ({ ...current, placeholders: current.placeholders.concat({ ...item, name: nextName }) }))
    setSelectedIndex(draft.placeholders.length)
  }

  const deleteItem = (index: number, name: string) => {
    setDraft((current) => ({
      placeholders: current.placeholders.filter((_item, itemIndex) => itemIndex !== index),
      folders: current.folders.map((folder) => ({ ...folder, placeholderNames: (folder.placeholderNames || []).filter((itemName) => itemName !== name) })),
    }))
    setSelectedIndex((current) => Math.max(0, Math.min(current, draft.placeholders.length - 2)))
  }

  const renameItem = (index: number, oldName: string, nextNameRaw: string) => {
    const nextName = nextNameRaw
    replacePlaceholder(index, (item) => ({ ...item, name: nextName }))
    setDraft((current) => ({
      ...current,
      folders: current.folders.map((folder) => ({ ...folder, placeholderNames: (folder.placeholderNames || []).map((itemName) => itemName === oldName ? nextName : itemName) })),
    }))
  }

  const createFolder = () => {
    const folder = createPlaceholderFolder()
    setDraft((current) => ({ ...current, folders: current.folders.concat(folder) }))
    setSelectedFolderId(folder.id)
  }

  const updateFolder = (folderId: string, patch: Partial<PlaceholderFolder>) => {
    setDraft((current) => ({ ...current, folders: current.folders.map((folder) => folder.id === folderId ? { ...folder, ...patch, updatedAt: new Date().toISOString() } : folder) }))
  }

  const deleteFolder = (folderId: string) => {
    setDraft((current) => ({
      ...current,
      folders: current.folders.filter((folder) => folder.id !== folderId).map((folder) => folder.parentId === folderId ? { ...folder, parentId: '', updatedAt: new Date().toISOString() } : folder),
    }))
    if (selectedFolderId === folderId) setSelectedFolderId('')
  }

  const toggleFolderMembership = (folderId: string, itemName: string, checked: boolean) => {
    setDraft((current) => ({
      ...current,
      folders: current.folders.map((folder) => {
        if (folder.id !== folderId) return folder
        const names = new Set(folder.placeholderNames || [])
        if (checked) names.add(itemName)
        else names.delete(itemName)
        return { ...folder, placeholderNames: Array.from(names).sort((a, b) => a.localeCompare(b)), updatedAt: new Date().toISOString() }
      }),
    }))
  }

  const saveDraft = async () => {
    if (!canSave) {
      setSaveError(hasEmptyName ? '占位符名字不能为空' : '占位符名字不能重复')
      return
    }
    setSaving(true)
    setSaveError('')
    try {
      const saved = await controller.actions.savePlaceholderLibrary?.(draft)
      setDraft(cloneLibrary(saved || draft))
    } catch (e) {
      setSaveError(String((e as any)?.message || e || '保存失败'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Paper variant="outlined" sx={{ p: 1.5 }}>
      <Stack spacing={1.5}>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Typography sx={{ fontWeight: 900 }}>占位符管理</Typography>
            <Typography variant="caption" color="text.secondary">使用 {`{{名字}}`} 在提示词里引用；替换只发生在发送给 AI 前。</Typography>
          </Box>
          <Button startIcon={<RefreshIcon />} variant="outlined" onClick={() => controller.actions.refreshPlaceholderLibrary?.(true)} disabled={busy || saving}>{placeholders?.loading ? '刷新中…' : '刷新'}</Button>
          <Button startIcon={<AddIcon />} variant="outlined" onClick={createItem} disabled={busy || saving}>新建占位符</Button>
          <Button startIcon={<SaveIcon />} variant="contained" onClick={saveDraft} disabled={!canSave}>{saving ? '保存中…' : '保存'}</Button>
        </Stack>

        <Divider />
        {placeholders?.error ? <Typography variant="body2" color="error">{String(placeholders.error || '')}</Typography> : null}
        {saveError ? <Typography variant="body2" color="error">{saveError}</Typography> : null}
        {hasEmptyName ? <Typography variant="body2" color="error">占位符名字不能为空。</Typography> : null}
        {hasDuplicateName ? <Typography variant="body2" color="error">占位符名字必须全局唯一。</Typography> : null}

        <Stack direction={{ xs: 'column', lg: 'row' }} spacing={1.5} alignItems="flex-start">
          <Paper variant="outlined" sx={{ p: 1, width: { xs: '100%', lg: 270 }, borderRadius: 2 }}>
            <Stack spacing={1}>
              <Stack direction="row" spacing={1} alignItems="center">
                <Typography variant="body2" sx={{ fontWeight: 900, flex: 1 }}>收藏夹</Typography>
                <Button size="small" startIcon={<AddIcon />} onClick={createFolder} disabled={busy || saving}>新建</Button>
              </Stack>
              <Button size="small" variant={!selectedFolderId ? 'contained' : 'outlined'} onClick={() => setSelectedFolderId('')} sx={{ justifyContent: 'flex-start' }}>全部占位符</Button>
              {sortedFolders(draft.folders).map((folder) => (
                <Button key={folder.id} size="small" variant={selectedFolderId === folder.id ? 'contained' : 'outlined'} onClick={() => setSelectedFolderId(folder.id)} sx={{ justifyContent: 'flex-start', pl: 1 + folderDepth(folder, draft.folders) * 2 }}>
                  <Box component="span" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{folder.name}</Box>
                </Button>
              ))}
              {selectedFolder ? (
                <Paper variant="outlined" sx={{ p: 1, borderRadius: 2, bgcolor: 'grey.50' }}>
                  <Stack spacing={1}>
                    <TextField size="small" label="收藏夹名称" value={selectedFolder.name} onChange={(e) => updateFolder(selectedFolder.id, { name: e.target.value })} disabled={busy || saving} />
                    <FormControl size="small">
                      <InputLabel>父级收藏夹</InputLabel>
                      <Select label="父级收藏夹" value={selectedFolder.parentId || ''} onChange={(e) => updateFolder(selectedFolder.id, { parentId: String(e.target.value || '') })} disabled={busy || saving}>
                        <MenuItem value="">无</MenuItem>
                        {draft.folders.filter((folder) => folder.id !== selectedFolder.id).map((folder) => <MenuItem key={folder.id} value={folder.id}>{folder.name}</MenuItem>)}
                      </Select>
                    </FormControl>
                    <Button color="error" size="small" startIcon={<DeleteOutlineIcon />} onClick={() => deleteFolder(selectedFolder.id)} disabled={busy || saving}>删除收藏夹</Button>
                  </Stack>
                </Paper>
              ) : null}
            </Stack>
          </Paper>

          <Paper variant="outlined" sx={{ p: 1, width: { xs: '100%', lg: 300 }, borderRadius: 2 }}>
            <Stack spacing={1}>
              <Typography variant="body2" sx={{ fontWeight: 900 }}>占位符列表</Typography>
              {filteredPlaceholders.length ? filteredPlaceholders.map(({ item, index }) => {
                const selected = index === selectedIndex
                const label = text(item.name) || `未命名占位符 ${index + 1}`
                return <Button key={`${item.name}:${index}`} variant={selected ? 'contained' : 'outlined'} color={selected ? 'primary' : 'inherit'} onClick={() => setSelectedIndex(index)} sx={{ justifyContent: 'flex-start', textTransform: 'none' }}>{label}</Button>
              }) : <Typography variant="body2" color="text.secondary">暂无占位符。</Typography>}
            </Stack>
          </Paper>

          <Box sx={{ flex: 1, minWidth: 0, width: '100%' }}>
            {selectedPlaceholder ? (
              <Stack spacing={1.25}>
                <PlaceholderEditor
                  item={selectedPlaceholder}
                  folders={draft.folders}
                  disabled={busy || saving}
                  onRename={(nextName) => renameItem(selectedIndex, selectedPlaceholder.name, nextName)}
                  onUpdate={(patch) => replacePlaceholder(selectedIndex, (item) => ({ ...item, ...patch }))}
                  onDelete={() => deleteItem(selectedIndex, selectedPlaceholder.name)}
                  onToggleFolder={(folderId, checked) => toggleFolderMembership(folderId, selectedPlaceholder.name, checked)}
                />
                <PlaceholderDependencyTreePanel tree={placeholders?.dependencyTree} />
              </Stack>
            ) : (
              <Paper variant="outlined" sx={{ p: 2, borderRadius: 2 }}><Typography variant="body2" color="text.secondary">选择一个占位符，或新建后开始编辑。</Typography></Paper>
            )}
          </Box>
        </Stack>

        <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.5} alignItems="stretch">
          <Paper variant="outlined" sx={{ p: 1.25, borderRadius: 2, flex: 1 }}>
            <Stack spacing={1}>
              <Typography variant="body2" sx={{ fontWeight: 900 }}>解析预览</Typography>
              <TextField size="small" multiline minRows={4} label="输入包含占位符的文本" value={previewText} onChange={(e) => setPreviewText(e.target.value)} fullWidth />
              <Paper variant="outlined" sx={{ p: 1, minHeight: 92, whiteSpace: 'pre-wrap', bgcolor: 'grey.50' }}>{String(placeholders?.preview?.text || '')}</Paper>
            </Stack>
          </Paper>
          <Paper variant="outlined" sx={{ p: 1.25, borderRadius: 2, width: { xs: '100%', md: 320 } }}>
            <Stack spacing={1}>
              <Typography variant="body2" sx={{ fontWeight: 900 }}>问题看板</Typography>
              {Array.isArray(placeholders?.problems) && placeholders.problems.length ? placeholders.problems.map((problem: any, index: number) => (
                <Typography key={`${problem.name}:${problem.type}:${index}`} variant="body2" color="error">{String(problem.name || '')}：{placeholderProblemLabel(String(problem.type || ''))}</Typography>
              )) : <Typography variant="body2" color="text.secondary">当前未发现问题。</Typography>}
            </Stack>
          </Paper>
        </Stack>
      </Stack>
    </Paper>
  )
}

function PlaceholderEditor(props: {
  item: PlaceholderItem
  folders: PlaceholderFolder[]
  disabled: boolean
  onRename: (name: string) => void
  onUpdate: (patch: Partial<PlaceholderItem>) => void
  onDelete: () => void
  onToggleFolder: (folderId: string, checked: boolean) => void
}) {
  const { item, folders, disabled, onRename, onUpdate, onDelete, onToggleFolder } = props
  return (
    <Paper variant="outlined" sx={{ p: 1.25, borderRadius: 2 }}>
      <Stack spacing={1}>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}>
          <TextField size="small" label="名字" value={item.name} onChange={(e) => onRename(e.target.value)} sx={{ flex: 1 }} disabled={disabled} />
          <Button color="error" startIcon={<DeleteOutlineIcon />} onClick={onDelete} disabled={disabled}>删除</Button>
        </Stack>
        <TextField size="small" label="备注" value={item.description || ''} onChange={(e) => onUpdate({ description: e.target.value })} disabled={disabled} fullWidth />
        <TextField size="small" multiline minRows={5} label="值" value={item.value} onChange={(e) => onUpdate({ value: e.target.value })} disabled={disabled} fullWidth />
        <Typography variant="caption" color="text.secondary">创建时间：{formatTime(item.createdAt)}</Typography>
        <Divider />
        <Typography variant="body2" sx={{ fontWeight: 900 }}>所属收藏夹</Typography>
        {folders.length ? folders.map((folder) => {
          const checked = !!folder.placeholderNames?.includes(item.name)
          return (
            <Stack key={folder.id} direction="row" spacing={1} alignItems="center" sx={{ pl: folderDepth(folder, folders) * 2 }}>
              <Checkbox size="small" checked={checked} onChange={(e) => onToggleFolder(folder.id, e.target.checked)} disabled={disabled || !text(item.name)} />
              <Typography variant="body2">{folder.name}</Typography>
            </Stack>
          )
        }) : <Typography variant="caption" color="text.secondary">暂无收藏夹。</Typography>}
      </Stack>
    </Paper>
  )
}

function PlaceholderDependencyTreePanel(props: { tree: PlaceholderDependencyNode }) {
  return (
    <Paper variant="outlined" sx={{ p: 1.25, borderRadius: 2, bgcolor: 'grey.50' }}>
      <Stack spacing={1}>
        <Typography variant="body2" sx={{ fontWeight: 900 }}>依赖树</Typography>
        {props.tree?.name ? <DependencyNode node={props.tree} depth={0} /> : <Typography variant="body2" color="text.secondary">选择占位符后查看依赖。</Typography>}
      </Stack>
    </Paper>
  )
}

function DependencyNode(props: { node: PlaceholderDependencyNode; depth: number }) {
  const { node, depth } = props
  const suffix = node.cycle ? '（循环）' : node.missing ? '（未注册）' : ''
  return (
    <Box sx={{ pl: depth * 2 }}>
      <Typography variant="body2" color={node.cycle || node.missing ? 'error' : 'text.primary'}>{node.name}{suffix}</Typography>
      {Array.isArray(node.children) ? node.children.map((child, index) => <DependencyNode key={`${child.name}:${index}`} node={child} depth={depth + 1} />) : null}
    </Box>
  )
}
