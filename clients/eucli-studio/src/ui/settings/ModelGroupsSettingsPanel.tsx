import * as React from 'react'
import { Box, Button, Divider, FormControl, InputLabel, MenuItem, Paper, Select, Stack, TextField, Typography } from '@mui/material'
import AddIcon from '@mui/icons-material/Add'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import RefreshIcon from '@mui/icons-material/Refresh'
import SaveIcon from '@mui/icons-material/Save'

type ModelGroupsSettingsPanelProps = {
  controller: any
  loading: boolean
  modelGroups: any
  providers: any[]
  topbarHeight: number
}

export function ModelGroupsSettingsPanel(props: ModelGroupsSettingsPanelProps) {
  const { controller, loading, modelGroups, providers, topbarHeight } = props
  const box = modelGroups && typeof modelGroups === 'object' ? modelGroups : {}
  const items = Array.isArray(box.items) ? box.items : []
  const busy = loading || !!box.loading || !!box.saving

  React.useEffect(() => {
    controller.actions.refreshModelGroups?.(false)
  }, [controller])

  return (
    <Box sx={{ flex: 1, minWidth: 0, minHeight: 0, overflow: 'auto', px: 2, pt: `calc(${topbarHeight}px + 16px)`, pb: 2, bgcolor: 'grey.50' }}>
      <Paper variant="outlined" sx={{ p: 1.5 }}>
        <Stack spacing={1.5}>
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography sx={{ fontWeight: 900 }}>模型组</Typography>
              <Typography variant="caption" color="text.secondary">把已登记的供应商模型组合成对外模型入口。</Typography>
            </Box>
            <Button startIcon={<RefreshIcon />} variant="outlined" onClick={() => controller.actions.refreshModelGroups?.(true)} disabled={busy}>{box.loading ? '刷新中…' : '刷新'}</Button>
            <Button startIcon={<AddIcon />} variant="outlined" onClick={() => controller.actions.createModelGroup?.()} disabled={busy}>新建模型组</Button>
            <Button startIcon={<SaveIcon />} variant="contained" onClick={() => controller.actions.saveModelGroups?.()} disabled={busy}>{box.saving ? '保存中…' : '保存'}</Button>
          </Stack>

          <Divider />

          {box.error ? <Typography variant="body2" color="error">{String(box.error || '')}</Typography> : null}
          {box.saveError ? <Typography variant="body2" color="error">{String(box.saveError || '')}</Typography> : null}

          <Stack spacing={1.5}>
            {items.length ? items.map((group: any) => (
              <ModelGroupCard key={String(group?.id || '')} controller={controller} group={group} providers={providers} busy={busy} />
            )) : (
              <Typography variant="body2" color="text.secondary">暂无模型组。</Typography>
            )}
          </Stack>
        </Stack>
      </Paper>
    </Box>
  )
}

function ModelGroupCard(props: { controller: any; group: any; providers: any[]; busy: boolean }) {
  const { controller, group, providers, busy } = props
  const groupId = String(group?.id || '')
  const models = Array.isArray(group?.models) ? group.models : []
  return (
    <Paper variant="outlined" sx={{ p: 1.25, borderRadius: 2 }}>
      <Stack spacing={1.25}>
        <Stack direction={{ xs: 'column', md: 'row' }} spacing={1} alignItems={{ xs: 'stretch', md: 'center' }}>
          <TextField size="small" label="模型组名称" value={String(group?.name || '')} onChange={(e) => controller.actions.setModelGroupField?.(groupId, 'name', e.target.value)} sx={{ flex: 1 }} />
          <Button size="small" startIcon={<AddIcon />} onClick={() => controller.actions.createModelGroupModel?.(groupId)} disabled={busy}>添加对外模型</Button>
          <Button size="small" color="error" startIcon={<DeleteOutlineIcon />} onClick={() => controller.actions.deleteModelGroup?.(groupId)} disabled={busy}>删除组</Button>
        </Stack>

        {models.length ? models.map((model: any) => (
          <ModelGroupModelCard key={String(model?.id || '')} controller={controller} groupId={groupId} model={model} providers={providers} busy={busy} />
        )) : (
          <Typography variant="body2" color="text.secondary">这个模型组还没有对外模型。</Typography>
        )}
      </Stack>
    </Paper>
  )
}

function ModelGroupModelCard(props: { controller: any; groupId: string; model: any; providers: any[]; busy: boolean }) {
  const { controller, groupId, model, providers, busy } = props
  const modelId = String(model?.id || '')
  const members = Array.isArray(model?.members) ? model.members : []
  return (
    <Paper variant="outlined" sx={{ p: 1, bgcolor: 'grey.50' }}>
      <Stack spacing={1}>
        <Stack direction={{ xs: 'column', md: 'row' }} spacing={1} alignItems={{ xs: 'stretch', md: 'center' }}>
          <TextField size="small" label="对外模型 ID" value={modelId} onChange={(e) => controller.actions.setModelGroupModelField?.(groupId, modelId, 'id', e.target.value)} sx={{ flex: 1 }} />
          <TextField size="small" label="显示名称" value={String(model?.name || '')} onChange={(e) => controller.actions.setModelGroupModelField?.(groupId, modelId, 'name', e.target.value)} sx={{ flex: 1 }} />
          <FormControl size="small" sx={{ minWidth: 160 }}>
            <InputLabel>策略</InputLabel>
            <Select label="策略" value={String(model?.strategy || '') === 'weighted_random' ? 'weighted_random' : 'sequential'} onChange={(e) => controller.actions.setModelGroupModelField?.(groupId, modelId, 'strategy', e.target.value)}>
              <MenuItem value="sequential">顺序轮询</MenuItem>
              <MenuItem value="weighted_random">权重随机</MenuItem>
            </Select>
          </FormControl>
          <Button size="small" startIcon={<AddIcon />} onClick={() => controller.actions.createModelGroupMember?.(groupId, modelId)} disabled={busy}>添加成员</Button>
          <Button size="small" color="error" startIcon={<DeleteOutlineIcon />} onClick={() => controller.actions.deleteModelGroupModel?.(groupId, modelId)} disabled={busy}>删除模型</Button>
        </Stack>

        {members.length ? members.map((member: any, index: number) => (
          <ModelGroupMemberRow key={index} controller={controller} groupId={groupId} modelId={modelId} member={member} memberIndex={index} providers={providers} busy={busy} />
        )) : (
          <Typography variant="caption" color="text.secondary">暂无成员模型。</Typography>
        )}
      </Stack>
    </Paper>
  )
}

function ModelGroupMemberRow(props: { controller: any; groupId: string; modelId: string; member: any; memberIndex: number; providers: any[]; busy: boolean }) {
  const { controller, groupId, modelId, member, memberIndex, providers, busy } = props
  const providerId = String(member?.providerId || '')
  const provider = providers.find((item: any) => String(item?.id || '') === providerId) || null
  const registeredModels = Array.isArray(provider?.registeredModels) ? provider.registeredModels : []
  return (
    <Stack direction={{ xs: 'column', md: 'row' }} spacing={1} alignItems={{ xs: 'stretch', md: 'center' }}>
      <FormControl size="small" sx={{ flex: 1 }}>
        <InputLabel>供应商</InputLabel>
        <Select label="供应商" value={providerId} onChange={(e) => controller.actions.setModelGroupMemberField?.(groupId, modelId, memberIndex, 'providerId', e.target.value)}>
          <MenuItem value=""><em>请选择供应商</em></MenuItem>
          {providers.map((providerItem: any) => <MenuItem key={String(providerItem?.id || '')} value={String(providerItem?.id || '')}>{String(providerItem?.name || '')}</MenuItem>)}
        </Select>
      </FormControl>
      <FormControl size="small" sx={{ flex: 1 }}>
        <InputLabel>登记模型</InputLabel>
        <Select label="登记模型" value={String(member?.modelId || '')} onChange={(e) => controller.actions.setModelGroupMemberField?.(groupId, modelId, memberIndex, 'modelId', e.target.value)} disabled={!providerId}>
          <MenuItem value=""><em>请选择模型</em></MenuItem>
          {registeredModels.map((model: any) => <MenuItem key={String(model?.id || '')} value={String(model?.id || '')}>{String(model?.name || model?.id || '')}</MenuItem>)}
        </Select>
      </FormControl>
      <TextField size="small" label="权重" type="number" value={String(member?.weight || 1)} onChange={(e) => controller.actions.setModelGroupMemberField?.(groupId, modelId, memberIndex, 'weight', e.target.value)} inputProps={{ min: 1, step: 1 }} sx={{ width: { xs: '100%', md: 120 } }} />
      <Button size="small" color="error" startIcon={<DeleteOutlineIcon />} onClick={() => controller.actions.deleteModelGroupMember?.(groupId, modelId, memberIndex)} disabled={busy}>删除</Button>
    </Stack>
  )
}
