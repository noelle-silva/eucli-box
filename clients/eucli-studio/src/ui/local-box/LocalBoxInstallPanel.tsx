import * as React from 'react'
import type { LocalBoxState } from '../../domain/localBox'

export function LocalBoxInstallPanel(props: { state: LocalBoxState; busy: boolean; onInstall: () => Promise<void> | void }) {
  const { state, busy, onInstall } = props
  const canInstall = state.status === 'ready_to_install' || state.status === 'not_installed' || state.status === 'failed'
  if (!canInstall) return null
  return (
    <div className="localBoxInstallPanel">
      {state.releaseNotes ? <div className="localBoxReleaseNotes">{state.releaseNotes}</div> : null}
      <button type="button" disabled={busy} onClick={onInstall}>
        {busy ? '处理中…' : state.installed ? '重新启动' : '安装业务端'}
      </button>
    </div>
  )
}
