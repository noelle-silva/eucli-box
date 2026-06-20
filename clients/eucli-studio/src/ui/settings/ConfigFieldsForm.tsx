import * as React from 'react'
import { Box, Button, FormControl, FormControlLabel, InputLabel, MenuItem, Paper, Select, Stack, Switch, TextField, Typography } from '@mui/material'

export type ConfigField = {
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

export function ConfigFieldsForm(props: {
  schema: any
  defaultConfig: any
  userConfig: any
  draftConfig: any
  emptyText: string
  onSetValue: (path: string[], value: any) => void
  onRemoveValue: (path: string[]) => void
}) {
  const fields = buildConfigFields(props.schema, props.defaultConfig, props.userConfig, props.draftConfig)
  if (!fields.length) {
    return <Typography variant="body2" color="text.secondary">{props.emptyText}</Typography>
  }
  return (
    <Stack spacing={1.25}>
      {fields.map((field) => <ConfigFieldControl key={field.path.join('.')} field={field} onSetValue={props.onSetValue} onRemoveValue={props.onRemoveValue} />)}
    </Stack>
  )
}

export function cloneConfigObject(value: any): Record<string, any> {
  return value && typeof value === 'object' && !Array.isArray(value) ? JSON.parse(JSON.stringify(value)) : {}
}

export function setConfigValueAtPath(source: Record<string, any>, path: string[], value: any): Record<string, any> {
  const draft = cloneConfigObject(source)
  const segments = normalizeConfigPath(path)
  if (!segments.length) return draft
  let cursor: Record<string, any> = draft
  for (const segment of segments.slice(0, -1)) {
    const next = cursor[segment]
    if (!next || typeof next !== 'object' || Array.isArray(next)) cursor[segment] = {}
    cursor = cursor[segment]
  }
  cursor[segments[segments.length - 1]] = value
  return draft
}

export function removeConfigValueAtPath(source: Record<string, any>, path: string[]): Record<string, any> {
  const draft = cloneConfigObject(source)
  const segments = normalizeConfigPath(path)
  if (!segments.length) return draft
  let cursor: Record<string, any> = draft
  for (const segment of segments.slice(0, -1)) {
    const next = cursor[segment]
    if (!next || typeof next !== 'object' || Array.isArray(next)) return draft
    cursor = next
  }
  delete cursor[segments[segments.length - 1]]
  return draft
}

function ConfigFieldControl(props: { field: ConfigField; onSetValue: (path: string[], value: any) => void; onRemoveValue: (path: string[]) => void }) {
  const { field, onSetValue, onRemoveValue } = props
  const hasValue = !isBlankConfigValue(field.currentValue)
  const displayValue = hasValue ? field.currentValue : field.defaultValue
  const helper = configFieldHelper(field, hasValue)
  const setValue = (value: any) => onSetValue(field.path, value)
  const removeValue = () => onRemoveValue(field.path)

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
      ? <ConfigArrayField field={field} hasValue={hasValue} value={displayValue} helper={helper} onSetValue={onSetValue} onRemoveValue={onRemoveValue} />
      : <ConfigObjectField field={field} hasValue={hasValue} helper={helper} onSetValue={onSetValue} onRemoveValue={onRemoveValue} />
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

function ConfigObjectField(props: { field: ConfigField; hasValue: boolean; helper: string; onSetValue: (path: string[], value: any) => void; onRemoveValue: (path: string[]) => void }) {
  const { field, hasValue, helper, onSetValue, onRemoveValue } = props
  const childFields = buildNestedConfigFields(field)
  return (
    <Paper variant="outlined" sx={{ p: 1.25, borderRadius: 2, bgcolor: hasValue ? 'background.paper' : 'grey.50' }}>
      <Stack spacing={1.25}>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}>
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Typography sx={{ fontWeight: 900 }}>{field.label}{field.required ? ' *' : ''}</Typography>
            {helper ? <Typography variant="caption" color="text.secondary">{helper}</Typography> : null}
          </Box>
          <Button size="small" onClick={() => onRemoveValue(field.path)} disabled={!hasValue}>恢复默认</Button>
        </Stack>
        {childFields.length ? (
          <Stack spacing={1.25} sx={{ pl: { xs: 0, sm: 1.5 }, borderLeft: { sm: '2px solid' }, borderColor: { sm: 'divider' } }}>
            {childFields.map((child) => <ConfigFieldControl key={child.path.join('.')} field={child} onSetValue={onSetValue} onRemoveValue={onRemoveValue} />)}
          </Stack>
        ) : (
          <Typography variant="body2" color="text.secondary">该对象当前没有可编辑子字段。</Typography>
        )}
      </Stack>
    </Paper>
  )
}

function ConfigArrayField(props: { field: ConfigField; hasValue: boolean; value: any; helper: string; onSetValue: (path: string[], value: any) => void; onRemoveValue: (path: string[]) => void }) {
  const { field, hasValue, value, helper, onSetValue, onRemoveValue } = props
  const lines = Array.isArray(value) ? value.map((item) => String(item ?? '')).join('\n') : ''
  return (
    <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'flex-start' }}>
      <TextField
        size="small"
        label={`${field.label}${field.required ? ' *' : ''}`}
        value={lines}
        onChange={(event) => onSetValue(field.path, arrayFromLines(event.target.value))}
        helperText={helper ? `${helper} 一行一个值。` : '一行一个值。'}
        fullWidth
        multiline
        minRows={3}
      />
      <Button size="small" onClick={() => onRemoveValue(field.path)} disabled={!hasValue} sx={{ mt: { sm: 0.5 } }}>恢复默认</Button>
    </Stack>
  )
}

function buildConfigFields(schemaRaw: any, defaultConfigRaw: any, userConfigRaw: any, draftConfigRaw: any): ConfigField[] {
  const schema = plainObject(schemaRaw)
  const defaultConfig = plainObject(defaultConfigRaw)
  const userConfig = plainObject(userConfigRaw)
  const draftConfig = plainObject(draftConfigRaw)
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
  if (!hasValue && field.defaultValue != null && field.type !== 'object' && field.type !== 'array') parts.push(`当前使用默认值：${String(field.defaultValue) || '（空）'}`)
  return parts.join(' ')
}

function numberFromInput(value: string): number | '' {
  const trimmed = String(value || '').trim()
  if (!trimmed) return ''
  const n = Number(trimmed)
  return Number.isFinite(n) ? n : ''
}

function normalizeConfigPath(path: any): string[] {
  return Array.isArray(path) ? path.map((segment) => String(segment || '').trim()).filter(Boolean) : []
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
