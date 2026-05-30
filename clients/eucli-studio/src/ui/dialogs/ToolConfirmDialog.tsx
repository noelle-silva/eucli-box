import { Button, Dialog, DialogActions, DialogContent, DialogTitle, Typography } from '@mui/material'

type ToolConfirmDialogProps = {
  open: boolean
  controller: any
  pendingConfirmation: any
}

export function ToolConfirmDialog(props: ToolConfirmDialogProps) {
  const { open, controller, pendingConfirmation } = props
  const conf = pendingConfirmation as any

  const payload = (conf?.event?.payload) as any
  const toolName = String(payload?.toolName || conf?.toolName || '未知工具')
  const description = String(payload?.toolDescription || conf?.description || conf?.message || '')

  return (
    <Dialog open={open} onClose={() => controller.actions.closeConfirmation()} fullWidth maxWidth="sm">
      <DialogTitle>工具调用确认</DialogTitle>
      <DialogContent dividers>
        <Typography variant="body1" sx={{ fontWeight: 600, mb: 1 }}>
          工具: {toolName}
        </Typography>
        {description ? (
          <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
            {description}
          </Typography>
        ) : (
          <Typography variant="body2" color="text.secondary">
            该工具正在请求执行许可，请确认是否允许。
          </Typography>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={() => controller.actions.rejectConfirmation()} color="error">
          拒绝
        </Button>
        <Button onClick={() => controller.actions.approveConfirmation()} color="primary" variant="contained">
          允许
        </Button>
      </DialogActions>
    </Dialog>
  )
}
