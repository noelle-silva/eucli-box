import { Box, Button, Dialog, DialogContent, DialogTitle, FormControl, IconButton, InputLabel, MenuItem, Paper, Select, Stack, TextField, Typography } from '@mui/material'
import AddIcon from '@mui/icons-material/Add'
import CloseIcon from '@mui/icons-material/Close'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import StorageIcon from '@mui/icons-material/Storage'
import { ApiKeyField } from '../components/fields/ApiKeyField'

export function ProvidersDialog(props: { open: boolean; controller: any; providers: any[]; draft: any }) {
  const { open, controller, providers, draft } = props
  const editingId = String(draft?.editProviderId || '')

  return (
    <Dialog open={open} onClose={() => controller.actions.closeModal()} fullWidth maxWidth="md">
      <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        <StorageIcon fontSize="small" />
        供应商
        <Box sx={{ flex: 1 }} />
        <Button startIcon={<AddIcon />} onClick={() => controller.actions.createProvider()}>
          新建
        </Button>
        <IconButton onClick={() => controller.actions.closeModal()} size="small">
          <CloseIcon fontSize="small" />
        </IconButton>
      </DialogTitle>
      <DialogContent dividers>
        <Stack spacing={1.5}>
          {providers.map((p: any) => {
            const pid = String(p?.id || '')
            const isEditing = pid && pid === editingId
            return (
              <Paper key={pid} variant="outlined" sx={{ p: 1.5 }}>
                <Stack direction="row" spacing={1} alignItems="center">
                  <Typography sx={{ fontWeight: 900 }}>{String(p?.name || '')}</Typography>
                  <Typography variant="caption" color="text.secondary" sx={{ flex: 1, minWidth: 0 }} noWrap>
                    {providerProtocolLabel(p?.protocol)} · {String(p?.baseUrl || '')}
                  </Typography>
                  <Button
                    size="small"
                    variant={isEditing ? 'outlined' : 'text'}
                    onClick={() => (isEditing ? controller.actions.closeProviderEditor() : controller.actions.openProviderEditor(pid))}
                  >
                    {isEditing ? '收起' : '编辑'}
                  </Button>
                  <Button size="small" color="error" startIcon={<DeleteOutlineIcon />} onClick={() => controller.actions.askDeleteProvider(pid)}>
                    删除
                  </Button>
                </Stack>

                {isEditing ? (
                  <Stack spacing={1.5} sx={{ mt: 1.5 }}>
                    <TextField label="名称" value={String(draft?.providerName || '')} onChange={(e) => controller.actions.setDraft('providerName', e.target.value)} />
                    <TextField
                      label="Base URL"
                      value={String(draft?.providerBaseUrl || '')}
                      onChange={(e) => controller.actions.setDraft('providerBaseUrl', e.target.value)}
                      placeholder="https://api.openai.com/v1"
                    />
                    <FormControl fullWidth>
                      <InputLabel id={`provider-dialog-protocol-${pid}`}>协议</InputLabel>
                      <Select
                        labelId={`provider-dialog-protocol-${pid}`}
                        label="协议"
                        value={String(draft?.providerProtocol || '')}
                        onChange={(e) => controller.actions.setDraft('providerProtocol', e.target.value)}
                      >
                        <MenuItem value="">
                          <em>请选择协议</em>
                        </MenuItem>
                        <MenuItem value="openai">OpenAI 兼容</MenuItem>
                        <MenuItem value="anthropic">Anthropic</MenuItem>
                      </Select>
                    </FormControl>
                    <ApiKeyField value={String(draft?.providerApiKey || '')} onValueChange={(next) => controller.actions.setDraft('providerApiKey', next)} />
                    <Stack direction="row" spacing={1} justifyContent="flex-end">
                      <Button variant="contained" onClick={() => controller.actions.saveProvider()}>
                        保存
                      </Button>
                    </Stack>
                  </Stack>
                ) : null}
              </Paper>
            )
          })}
        </Stack>
      </DialogContent>
    </Dialog>
  )
}

function providerProtocolLabel(protocol: unknown) {
  const value = String(protocol || '').trim()
  if (value === 'openai') return 'OpenAI 兼容'
  if (value === 'anthropic') return 'Anthropic'
  return '未选择协议'
}

