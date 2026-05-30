import { Button, Dialog, DialogActions, DialogContent, DialogTitle, Typography } from '@mui/material'

export function ToolConfirmationDialog(props: { open: boolean; controller: any; confirmation: any }) {
  const { open, controller, confirmation } = props

  if (!confirmation) return null

  const action = confirmation?.action
  const tool = confirmation?.tool
  const decision = confirmation?.decision

  const toolName = String(action?.toolName || tool?.name || '未知工具')
  const description = String(tool?.description || '')
  const actionId = String(action?.id || decision?.actionId || '')
  const args = action?.arguments && typeof action.arguments === 'object' ? action.arguments : null

  return (
    <Dialog open={open} onClose={() => controller.actions.closeToolConfirm()} fullWidth maxWidth="sm">
      <DialogTitle>工具调用确认</DialogTitle>
      <DialogContent dividers>
        <Typography variant="body1" fontWeight={600} gutterBottom>
          {toolName}
        </Typography>
        {description ? (
          <Typography variant="body2" color="text.secondary" gutterBottom>
            {description}
          </Typography>
        ) : null}
        {args ? (
          <Typography
            variant="caption"
            component="pre"
            sx={{
              mt: 1,
              p: 1.5,
              bgcolor: 'grey.100',
              borderRadius: 1,
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
              fontSize: '0.75rem',
              fontFamily: 'monospace',
              maxHeight: 160,
              overflow: 'auto',
            }}
          >
            {JSON.stringify(args, null, 2)}
          </Typography>
        ) : null}
      </DialogContent>
      <DialogActions>
        <Button onClick={() => controller.actions.closeToolConfirm()}>取消</Button>
        <Button color="primary" variant="contained" onClick={() => controller.actions.confirmTool(true)}>
          批准
        </Button>
      </DialogActions>
    </Dialog>
  )
}
