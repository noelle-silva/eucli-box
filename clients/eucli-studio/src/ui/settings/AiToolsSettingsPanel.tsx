import * as React from 'react'
import {
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControl,
  FormControlLabel,
  IconButton,
  InputLabel,
  MenuItem,
  Paper,
  Select,
  Stack,
  Switch,
  TextField,
  Typography,
} from '@mui/material'
import BuildIcon from '@mui/icons-material/Build'
import CloseIcon from '@mui/icons-material/Close'
import RefreshIcon from '@mui/icons-material/Refresh'
import SettingsIcon from '@mui/icons-material/Settings'
import { useEvent } from '../hooks/useEvent'

type AiToolsSettingsPanelProps = {
  controller: any
  loading: boolean
  tools: any
  topbarHeight: number
}

type ToolSummary = {
  id?: unknown
  name?: unknown
  description?: unknown
  type?: unknown
  updatedAt?: unknown
}

type ConfigField = {
  path: string[]
  key: string
  label: string
  description: string
  type: 'string' | 'number' | 'boolean' | 'object' | 'array'
  enumValues: string[]
  required: boolean
  currentValue: any
  defaultValue: any
  schema: Record<string, any>
}

export function AiToolsSettingsPanel(props: AiToolsSettingsPanelProps) {
  const { controller, loading, tools, topbarHeight } = props
  const [filter, setFilter] = React.useState('')

  React.useEffect(() => {
    controller.actions.refreshTools?.(false)
  }, [controller])

  const items = toolItems(tools)
  const query = filter.trim().toLowerCase()
  const filtered = query
    ? items.filter((tool) => [toolName(tool), toolDescription(tool), String(tool.type || '')].some((value) => value.toLowerCase().includes(query)))
    : items

  return (
    <Box sx={{ flex: 1, minWidth: 0, minHeight: 0, overflow: 'auto', px: 2, pt: `calc(${topbarHeight}px + 16px)`, pb: 2, bgcolor: 'grey.50' }}>
      <Paper variant="outlined" sx={{ p: 1.5 }}>
        <Stack spacing={1.5}>
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}>
            <Stack direction="row" spacing={1.25} alignItems="center" sx={{ minWidth: 0, flex: 1 }}>
              <Box sx={{ width: 42, height: 42, borderRadius: 2, bgcolor: 'rgba(25,118,210,.10)', color: 'primary.main', display: 'grid', placeItems: 'center' }}>
                <BuildIcon fontSize="small" />
              </Box>
              <Box sx={{ minWidth: 0 }}>
                <Typography sx={{ fontWeight: 900 }}>AI 工具</Typography>
                <Typography variant="body2" color="text.secondary">
                  从 e-b 工具目录加载工具，并编辑工具的用户配置。
                </Typography>
              </Box>
            </Stack>
            <Button startIcon={<RefreshIcon />} variant="outlined" onClick={() => controller.actions.refreshTools?.(true)} disabled={loading || !!tools?.loading}>
              {tools?.loading ? '刷新中…' : '刷新工具'}
            </Button>
          </Stack>

          <Divider />

          <TextField
            size="small"
            label="搜索工具"
            placeholder="搜索工具名、描述或类型"
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
            fullWidth
          />

          {tools?.error ? (
            <Typography variant="body2" color="error">
              {String(tools.error || '')}
            </Typography>
          ) : null}

          <Stack spacing={1.25}>
            {filtered.length ? (
              filtered.map((tool) => <ToolCard key={toolId(tool)} controller={controller} tool={tool} loading={loading} />)
            ) : (
              <Paper variant="outlined" sx={{ p: 3, borderRadius: 2.5, textAlign: 'center', bgcolor: 'grey.50' }}>
                <Typography sx={{ fontWeight: 900 }}>{tools?.loading ? '工具列表加载中…' : '暂无可显示工具'}</Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                  {query ? '当前搜索没有匹配结果。' : '请确认 e-b 已加载工具目录。'}
                </Typography>
              </Paper>
            )}
          </Stack>
        </Stack>
      </Paper>

      <ToolConfigDialog controller={controller} tools={tools} />
    </Box>
  )
}

function ToolCard(props: { controller: any; tool: ToolSummary; loading: boolean }) {
  const { controller, tool, loading } = props
  const id = toolId(tool)
  const name = toolName(tool)
  const description = toolDescription(tool)
  return (
    <Paper variant="outlined" sx={{ p: 1.25, borderRadius: 2 }}>
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.25} alignItems={{ xs: 'stretch', sm: 'center' }}>
        <Stack direction="row" spacing={1.25} alignItems="flex-start" sx={{ minWidth: 0, flex: 1 }}>
          <Box sx={{ width: 38, height: 38, borderRadius: 2, bgcolor: 'grey.100', display: 'grid', placeItems: 'center', color: 'text.secondary', flexShrink: 0 }}>
            <BuildIcon fontSize="small" />
          </Box>
          <Box sx={{ minWidth: 0 }}>
            <Stack direction="row" spacing={0.75} alignItems="center" sx={{ flexWrap: 'wrap' }}>
              <Typography sx={{ fontWeight: 900 }}>{name}</Typography>
              {String(tool.type || '').trim() ? <Chip size="small" variant="outlined" label={String(tool.type)} /> : null}
            </Stack>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
              {description || '暂无描述'}
            </Typography>
          </Box>
        </Stack>
        <Button variant="outlined" onClick={() => controller.actions.openToolConfig?.(id)} disabled={loading || !id} sx={{ alignSelf: { xs: 'flex-end', sm: 'center' } }}>
          查看/配置
        </Button>
      </Stack>
    </Paper>
  )
}

function ToolConfigDialog(props: { controller: any; tools: any }) {
  const { controller, tools } = props
  const selectedTool = tools?.selectedTool && typeof tools.selectedTool === 'object' ? tools.selectedTool : null
  const open = !!tools?.selectedToolId || !!selectedTool || !!tools?.detailLoading
  const name = toolName(selectedTool || { id: tools?.selectedToolId })
  const configFields = selectedTool ? buildConfigFields(selectedTool, tools?.configDraft) : []
  const save = useEvent(() => controller.actions.saveSelectedToolConfig?.())

  return (
    <Dialog open={open} onClose={() => controller.actions.closeToolConfig?.()} fullWidth maxWidth="md">
      <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        <SettingsIcon fontSize="small" />
        AI 工具配置
        {name ? (
          <Typography variant="body2" color="text.secondary" sx={{ ml: 0.5 }}>
            {name}
          </Typography>
        ) : null}
        <Box sx={{ flex: 1 }} />
        <IconButton onClick={() => controller.actions.closeToolConfig?.()} size="small" aria-label="关闭工具配置">
          <CloseIcon fontSize="small" />
        </IconButton>
      </DialogTitle>
      <DialogContent dividers>
        <Stack spacing={1.5}>
          {tools?.detailLoading ? (
            <Typography variant="body2" color="text.secondary">
              工具详情加载中…
            </Typography>
          ) : null}
          {tools?.detailError ? (
            <Typography variant="body2" color="error">
              {String(tools.detailError || '')}
            </Typography>
          ) : null}

          {selectedTool ? (
            <>
              <Paper variant="outlined" sx={{ p: 1.25, borderRadius: 2, bgcolor: 'grey.50' }}>
                <Stack spacing={0.75}>
                  <Stack direction="row" spacing={0.75} alignItems="center" sx={{ flexWrap: 'wrap' }}>
                    <Typography sx={{ fontWeight: 900 }}>{toolName(selectedTool)}</Typography>
                    {String(selectedTool.type || '').trim() ? <Chip size="small" variant="outlined" label={String(selectedTool.type)} /> : null}
                  </Stack>
                  <Typography variant="body2" color="text.secondary">
                    {toolDescription(selectedTool) || '暂无描述'}
                  </Typography>
                </Stack>
              </Paper>

              <Paper variant="outlined" sx={{ p: 1.25, borderRadius: 2 }}>
                <Stack spacing={1.25}>
                  <Typography sx={{ fontWeight: 900 }}>用户配置</Typography>
                  {configFields.length ? (
                    configFields.map((field) => <ConfigFieldControl key={field.path.join('.')} controller={controller} field={field} />)
                  ) : (
                    <Typography variant="body2" color="text.secondary">
                      该工具当前没有可编辑的用户配置字段。
                    </Typography>
                  )}
                </Stack>
              </Paper>

              {tools?.saveError ? (
                <Typography variant="body2" color="error">
                  {String(tools.saveError || '')}
                </Typography>
              ) : null}

              <ToolInputSchemaSummary schema={selectedTool?.inputSchema} />
            </>
          ) : null}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={() => controller.actions.closeToolConfig?.()}>取消</Button>
        <Button variant="contained" onClick={save} disabled={!selectedTool || !!tools?.saving || !!tools?.detailLoading}>
          {tools?.saving ? '保存中…' : '保存配置'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

function ToolInputSchemaSummary(props: { schema: any }) {
  const properties = plainObject(plainObject(props.schema).properties)
  const fields = Object.keys(properties).map((key) => ({ key, schema: plainObject(properties[key]) }))
  if (!fields.length) return null
  return (
    <Paper variant="outlined" sx={{ p: 1.25, borderRadius: 2, bgcolor: 'grey.50' }}>
      <Stack spacing={1}>
        <Typography sx={{ fontWeight: 900 }}>工具调用参数</Typography>
        <Stack spacing={0.75}>
          {fields.map((field) => {
            const type = String(field.schema.type || 'string')
            const description = stringField(field.schema.description)
            return (
              <Paper key={field.key} variant="outlined" sx={{ p: 1, borderRadius: 1.5, bgcolor: 'background.paper' }}>
                <Stack direction="row" spacing={0.75} alignItems="center" sx={{ flexWrap: 'wrap' }}>
                  <Typography variant="body2" sx={{ fontWeight: 900 }}>{field.key}</Typography>
                  <Chip size="small" variant="outlined" label={type} />
                </Stack>
                {description ? <Typography variant="caption" color="text.secondary">{description}</Typography> : null}
              </Paper>
            )
          })}
        </Stack>
      </Stack>
    </Paper>
  )
}

function ConfigFieldControl(props: { controller: any; field: ConfigField }) {
  const { controller, field } = props
  const hasValue = !isBlankConfigValue(field.currentValue)
  const displayValue = hasValue ? field.currentValue : field.defaultValue
  const helper = configFieldHelper(field, hasValue)
  const setValue = (value: any) => controller.actions.setToolConfigValue?.(field.path, value)
  const removeValue = () => controller.actions.removeToolConfigValue?.(field.path)

  if (field.type === 'boolean') {
    return (
      <Paper variant="outlined" sx={{ p: 1.25, borderRadius: 2, bgcolor: hasValue ? 'background.paper' : 'grey.50' }}>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}>
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Typography sx={{ fontWeight: 900 }}>{field.label}{field.required ? ' *' : ''}</Typography>
            {helper ? <Typography variant="caption" color="text.secondary">{helper}</Typography> : null}
          </Box>
          <Stack direction="row" spacing={1} alignItems="center" justifyContent="flex-end">
            <FormControlLabel control={<Switch checked={!!displayValue} onChange={(event) => setValue(event.target.checked)} />} label={displayValue ? '开启' : '关闭'} />
            <Button size="small" onClick={removeValue} disabled={!hasValue}>恢复默认</Button>
          </Stack>
        </Stack>
      </Paper>
    )
  }

  if (field.enumValues.length) {
    const selectValues = orderedUniqueStrings([...(displayValue == null || displayValue === '' ? [] : [String(displayValue)]), ...field.enumValues])
    return (
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'flex-start' }}>
        <FormControl size="small" fullWidth>
          <InputLabel>{field.label}{field.required ? ' *' : ''}</InputLabel>
          <Select label={`${field.label}${field.required ? ' *' : ''}`} value={String(displayValue ?? '')} onChange={(event) => event.target.value === '' ? removeValue() : setValue(event.target.value)}>
            <MenuItem value=""><em>使用默认值</em></MenuItem>
            {selectValues.map((value) => <MenuItem key={value} value={value}>{value}</MenuItem>)}
          </Select>
          {helper ? <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>{helper}</Typography> : null}
        </FormControl>
        <Button size="small" onClick={removeValue} disabled={!hasValue} sx={{ mt: { sm: 0.5 } }}>恢复默认</Button>
      </Stack>
    )
  }

  if (field.type === 'object' || field.type === 'array') {
    return field.type === 'array'
      ? <ConfigArrayField controller={controller} field={field} hasValue={hasValue} value={displayValue} helper={helper} />
      : <ConfigObjectField controller={controller} field={field} hasValue={hasValue} helper={helper} />
  }

  return (
    <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'flex-start' }}>
      <TextField
        size="small"
        label={`${field.label}${field.required ? ' *' : ''}`}
        type={field.type === 'number' ? 'number' : 'text'}
        value={displayValue ?? ''}
        onChange={(event) => {
          if (field.type === 'number' && event.target.value.trim() === '') removeValue()
          else if (field.type === 'number') {
            const nextValue = numberFromInput(event.target.value)
            if (nextValue === '') removeValue()
            else setValue(nextValue)
          } else setValue(event.target.value)
        }}
        placeholder={field.defaultValue == null ? '' : String(field.defaultValue)}
        helperText={helper}
        fullWidth
      />
      <Button size="small" onClick={removeValue} disabled={!hasValue} sx={{ mt: { sm: 0.5 } }}>恢复默认</Button>
    </Stack>
  )
}

function ConfigObjectField(props: { controller: any; field: ConfigField; hasValue: boolean; helper: string }) {
  const { controller, field, hasValue, helper } = props
  const childFields = buildNestedConfigFields(field)
  return (
    <Paper variant="outlined" sx={{ p: 1.25, borderRadius: 2, bgcolor: hasValue ? 'background.paper' : 'grey.50' }}>
      <Stack spacing={1.25}>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}>
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Typography sx={{ fontWeight: 900 }}>{field.label}{field.required ? ' *' : ''}</Typography>
            {helper ? <Typography variant="caption" color="text.secondary">{helper}</Typography> : null}
          </Box>
          <Button size="small" onClick={() => controller.actions.removeToolConfigValue?.(field.path)} disabled={!hasValue}>恢复默认</Button>
        </Stack>
        {childFields.length ? (
          <Stack spacing={1.25} sx={{ pl: { xs: 0, sm: 1.5 }, borderLeft: { sm: '2px solid' }, borderColor: { sm: 'divider' } }}>
            {childFields.map((child) => <ConfigFieldControl key={child.path.join('.')} controller={controller} field={child} />)}
          </Stack>
        ) : (
          <Typography variant="body2" color="text.secondary">该对象当前没有可编辑子字段。</Typography>
        )}
      </Stack>
    </Paper>
  )
}

function ConfigArrayField(props: { controller: any; field: ConfigField; hasValue: boolean; value: any; helper: string }) {
  const { controller, field, hasValue, value, helper } = props
  const lines = Array.isArray(value) ? value.map((item) => String(item ?? '')).join('\n') : ''
  return (
    <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'flex-start' }}>
      <TextField
        size="small"
        label={`${field.label}${field.required ? ' *' : ''}`}
        value={lines}
        onChange={(event) => controller.actions.setToolConfigValue?.(field.path, arrayFromLines(event.target.value))}
        helperText={helper ? `${helper} 一行一个值。` : '一行一个值。'}
        fullWidth
        multiline
        minRows={3}
      />
      <Button size="small" onClick={() => controller.actions.removeToolConfigValue?.(field.path)} disabled={!hasValue} sx={{ mt: { sm: 0.5 } }}>恢复默认</Button>
    </Stack>
  )
}

function toolItems(tools: any): ToolSummary[] {
  return Array.isArray(tools?.items) ? tools.items.filter((tool: any) => tool && typeof tool === 'object') : []
}

function toolId(tool: any): string {
  return String(tool?.id || tool?.name || '').trim()
}

function toolName(tool: any): string {
  return String(tool?.name || tool?.id || '').trim()
}

function toolDescription(tool: any): string {
  return String(tool?.description || '').trim()
}

function buildConfigFields(tool: any, draft: any): ConfigField[] {
  const schema = plainObject(tool?.userConfigSchema)
  const defaultConfig = plainObject(tool?.defaultConfig)
  const userConfig = plainObject(tool?.userConfig)
  const draftConfig = plainObject(draft)
  return buildConfigFieldsFromSources([], schema, defaultConfig, userConfig, draftConfig)
}

function buildNestedConfigFields(parent: ConfigField): ConfigField[] {
  const schema = parent.schema
  const defaultConfig = plainObject(parent.defaultValue)
  const currentConfig = plainObject(parent.currentValue)
  return buildConfigFieldsFromSources(parent.path, schema, defaultConfig, currentConfig, currentConfig)
}

function buildConfigFieldsFromSources(pathPrefix: string[], schema: Record<string, any>, defaultConfig: Record<string, any>, userConfig: Record<string, any>, draftConfig: Record<string, any>): ConfigField[] {
  const required = new Set(stringArray(schema.required))
  const properties = plainObject(schema.properties)
  const keys = orderedUniqueStrings([...Object.keys(properties), ...Object.keys(defaultConfig), ...Object.keys(userConfig), ...Object.keys(draftConfig)])

  return keys.map((key) => {
    const fieldSchema = plainObject(properties[key])
    const currentValue = draftConfig[key]
    const defaultValue = hasOwn(defaultConfig, key) ? defaultConfig[key] : undefined
    return {
      path: [...pathPrefix, key],
      key,
      label: stringField(fieldSchema.title) || key,
      description: stringField(fieldSchema.description),
      type: inferConfigFieldType(fieldSchema, currentValue, defaultValue),
      enumValues: stringArray(fieldSchema.enum),
      required: required.has(key),
      currentValue,
      defaultValue,
      schema: fieldSchema,
    }
  })
}

function inferConfigFieldType(schema: Record<string, any>, currentValue: any, defaultValue: any): ConfigField['type'] {
  const rawType = String(schema.type || '').trim()
  if (rawType === 'boolean') return 'boolean'
  if (rawType === 'number' || rawType === 'integer') return 'number'
  if (rawType === 'object') return 'object'
  if (rawType === 'array') return 'array'
  const value = currentValue != null ? currentValue : defaultValue
  if (typeof value === 'boolean') return 'boolean'
  if (typeof value === 'number') return 'number'
  if (Array.isArray(value)) return 'array'
  if (value && typeof value === 'object') return 'object'
  return 'string'
}

function configFieldHelper(field: ConfigField, hasValue: boolean): string {
  const parts: string[] = []
  if (field.description) parts.push(field.description)
  if (!hasValue && field.defaultValue != null && field.type !== 'object' && field.type !== 'array') {
    parts.push(`当前使用默认值：${String(field.defaultValue) || '（空）'}`)
  }
  return parts.join(' ')
}

function numberFromInput(value: string): number | '' {
  const trimmed = String(value || '').trim()
  if (!trimmed) return ''
  const n = Number(trimmed)
  return Number.isFinite(n) ? n : ''
}

function isBlankConfigValue(value: any): boolean {
  return value === undefined || value === null
}

function plainObject(value: any): Record<string, any> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value : {}
}

function stringField(value: any): string {
  return typeof value === 'string' ? value.trim() : ''
}

function stringArray(value: any): string[] {
  return Array.isArray(value) ? value.map((item) => String(item || '').trim()).filter(Boolean) : []
}

function arrayFromLines(value: string): string[] {
  return String(value || '').split('\n').map((line) => line.trim()).filter(Boolean)
}

function orderedUniqueStrings(values: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const value of values) {
    const key = String(value || '').trim()
    if (!key || seen.has(key)) continue
    seen.add(key)
    out.push(key)
  }
  return out
}

function hasOwn(obj: Record<string, any>, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(obj, key)
}
