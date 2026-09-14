import * as React from 'react'
import {
  AppBar,
  Avatar,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Collapse,
  CssBaseline,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  GlobalStyles,
  IconButton,
  InputAdornment,
  InputLabel,
  List,
  ListItemAvatar,
  ListItemButton,
  ListItemText,
  MenuItem,
  Paper,
  Popover,
  Select,
  Slider,
  Stack,
  Tab,
  Tabs,
  TextField,
  ThemeProvider,
  Toolbar,
  Tooltip,
  Typography,
} from '@mui/material'
import AddIcon from '@mui/icons-material/Add'
import CloseIcon from '@mui/icons-material/Close'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import EditOutlinedIcon from '@mui/icons-material/EditOutlined'
import HistoryIcon from '@mui/icons-material/History'
import SearchIcon from '@mui/icons-material/Search'
import ImageIcon from '@mui/icons-material/Image'
import AttachFileIcon from '@mui/icons-material/AttachFile'
import SettingsIcon from '@mui/icons-material/Settings'
import RestartAltIcon from '@mui/icons-material/RestartAlt'
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft'
import ChevronRightIcon from '@mui/icons-material/ChevronRight'
import AccountTreeIcon from '@mui/icons-material/AccountTree'
import AutorenewIcon from '@mui/icons-material/Autorenew'
import ArrowBackRoundedIcon from '@mui/icons-material/ArrowBackRounded'
import ExpandLessIcon from '@mui/icons-material/ExpandLess'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import StarBorderRoundedIcon from '@mui/icons-material/StarBorderRounded'
import FolderOutlinedIcon from '@mui/icons-material/FolderOutlined'
import DriveFileMoveOutlinedIcon from '@mui/icons-material/DriveFileMoveOutlined'
import UnfoldLessIcon from '@mui/icons-material/UnfoldLess'
import { useAiChatState } from './hooks/useAiChatState'
import { useDeferredChatSwitch } from './hooks/useDeferredChatSwitch'
import { useEvent } from './hooks/useEvent'
import { useLazyListWindow } from './hooks/useLazyListWindow'
import { clampNum } from './utils/numbers'
import { hotkeyFromKeyEvent, normalizeHotkeyString } from './utils/hotkeys'
import { TOPBAR_H } from './appConstants'
import { createChatGlobalStyles } from './globalStyles'
import { ProvidersDialog } from './dialogs/ProvidersDialog'
import { RoleDialog } from './dialogs/RoleDialog'
import { GroupDialog } from './dialogs/GroupDialog'
import { WorkspaceDialog } from './dialogs/WorkspaceDialog'

const isRenderableNode = (node: React.ReactNode): node is Exclude<React.ReactNode, null> => node !== null
import { ConfirmDialog } from './dialogs/ConfirmDialog'
import { MermaidDialog } from './dialogs/MermaidDialog'
import { ImageDialog } from './dialogs/ImageDialog'
import { StandaloneWindowControls, type WindowControlActions } from './components/StandaloneWindowControls'
import type { SettingsTabValue } from './settings/SettingsPageLayout'
import type { AiChatDataDirectory } from './settings/DataSettingsPanel'
import { PluginSettingsPage } from './settings/PluginSettingsPage'
import { providerSelectItems, registeredModelItems } from './settings/modelItemSelectors'
import { HookPromptSelector } from './components/HookPromptSelector'
import { EUCLI_STUDIO_CHAT_ROOT_ID } from '../runtime/eucliStudioGlobals'
import { ASSISTANT_RUNNING_CONTENT, assistantRunGenerationId, isAssistantGenerating } from '../domain/assistantRunState'
import { activeRunCardForAssistantMessage, isStaleAssistantPlaceholder, messageVisibleText } from '../domain/chatMessageDisplay'
import { formatModelRefDisplayText } from '../domain/modelRefUtils'
import { pendingChatForTarget } from '../domain/pendingChat'
import { chatNavigationFromOrderedChats } from '../domain/chatNavigation'
import { normalizeChatSessionRunStatus } from '../domain/chatSessionRunStatus'
import { sortChatListItemsForDisplay } from '../domain/chatListOrdering'
import { filterEbRoleRunCardsOnMessagePath, readActiveEbRunCardsForTarget } from '../domain/activeRunCards'
import { createMessageMutationGuard, type MessageMutationOperation } from '../domain/messageMutationConflicts'
import { formatTokenEstimate, formatTokenEstimateShort, sumMessageTokenEstimate } from '../domain/messageTokenUsage'
import { ChatSessionRunIndicator, type ChatSessionRunIndicatorKind } from './components/ChatSessionRunIndicator'
import { ChatMessageList } from './components/ChatMessageList'
import { CustomScrollArea } from './components/CustomScrollArea'
import { ComposerInputControls } from './composer/ComposerInputControls'
import { buildChatTreeLayout, normalizeChatTreeNodeRole, svgSafeId } from './chatTree/chatTreeLayout'
import { ChatTreeNodeShape } from './chatTree/ChatTreeNodeShape'
import { snippetText } from './utils/text'
import { numericTimeValue } from './utils/time'
import { SOFT_POPOVER_PAPER_SX, SOFT_POPOVER_HEADER_SX, SOFT_POPOVER_LIST_SX, SOFT_POPOVER_ITEM_SX, SOFT_POPOVER_ITEM_TOP_SX } from './softPopoverStyles'
import { chatSessionRunNoticeKey, chatSessionRunSummaryFromListItem, collectChatSessionRunObservations, type ChatSessionRunNoticeKind, type ChatSessionRunNotice, type ChatSessionRunObservation } from './chatSessionRunObservations'
import { workspaceRoleTargetId } from '../domain/workspaceRoleTarget'
import { chatSettingsTargetKey } from '../controller/chatSessionTarget'
import { REASONING_EFFORT_OPTIONS, chatReasoningEffort, effectiveReasoningEffort, modelReasoningProfileFromModelRef, reasoningEffortLabel } from '../domain/reasoning'
import { chatStreamEnabled } from '../domain/chatStream'
import { chatMessageMaterialKind } from '../domain/message'
import type { HookPromptLibrary } from '../domain/hookPrompt'
import type { PlaceholderLibrary } from '../domain/placeholder'
import type { ReleaseCandidatesView, StudioBootstrap } from '../domain/release'
import { resolveColorThemePreset } from '../domain/colorTheme'
import { colorMixVar, colorThemeCssVariables, createStudioMuiTheme } from './colorThemeStyles'

type SettingsTab = SettingsTabValue

const CHAT_HISTORY_PAGE_SIZE = 20
const CHAT_HISTORY_BOTTOM_THRESHOLD_RATIO = 0.25

function chatHistoryMatchesSearch(chat: any, fallbackTitle: string, queryText: string) {
  const q = String(queryText || '').trim().toLowerCase()
  if (!q) return true
  if (!chat) return false
  const title = String(chat?.title || fallbackTitle || '')
  const raw = String(chat?.lastMessagePreview || '')
  return (title + '\n' + raw).toLowerCase().includes(q)
}

type SendPathAnchor = {
  chatId: string
  branchId: string
  parentMid: string
  runId: string
  inputMessageId: string
  lastMessageId: string
  existingMessageIds: string[]
  nonce: number
}

function emptySendPathAnchor(): SendPathAnchor {
  return { chatId: '', branchId: '', parentMid: '', runId: '', inputMessageId: '', lastMessageId: '', existingMessageIds: [], nonce: 0 }
}

export type AiChatWindowControls = {
  standalone: boolean
  actions: WindowControlActions
}

function isNearBottom(el: HTMLElement, thresholdPx = 24) {
  const gap = el.scrollHeight - el.scrollTop - el.clientHeight
  return Math.ceil(gap) <= thresholdPx
}



const composerToolIconButtonSx = {
  width: 36,
  height: 36,
  borderRadius: '999px',
  bgcolor: 'transparent',
  color: 'text.secondary',
  '&:hover': { bgcolor: 'rgba(0,0,0,.08)', color: 'text.primary' },
  '&.Mui-disabled': { bgcolor: 'transparent' },
}

const composerToolTextButtonSx = {
  minWidth: 0,
  maxWidth: 220,
  height: 34,
  px: 1.25,
  borderRadius: '999px',
  bgcolor: 'transparent',
  color: 'text.secondary',
  border: 0,
  textTransform: 'none',
  fontWeight: 800,
  fontSize: 12,
  '&:hover': { bgcolor: 'rgba(0,0,0,.08)', color: 'text.primary', border: 0 },
  '&.Mui-disabled': { bgcolor: 'transparent', border: 0 },
}

const composerContextButtonSx = {
  ...composerToolTextButtonSx,
  borderRadius: 2,
  maxWidth: 120,
}


export function AiChatApp(props: { controller: any; bootstrap?: StudioBootstrap; dataDirectory?: AiChatDataDirectory; windowControls?: AiChatWindowControls; releaseBusy: boolean; releaseView: ReleaseCandidatesView | null; onReleaseRead: (kind?: string) => Promise<void> | void; onReleaseRefresh: (kind?: string) => Promise<void> | void }) {
  const { controller, bootstrap, dataDirectory, windowControls, releaseBusy, releaseView, onReleaseRead, onReleaseRefresh } = props
  const s = useAiChatState(controller)
  const data = s.data
  const colorThemePreset = resolveColorThemePreset(data?.settings?.colorTheme)
  const theme = React.useMemo(() => createStudioMuiTheme(colorThemePreset), [colorThemePreset])
  const roles = Array.isArray(data?.roles) ? data.roles : []
  const groups = Array.isArray((data as any)?.groups) ? ((data as any).groups as any[]) : []
  const workspaces = Array.isArray((data as any)?.workspaces) ? ((data as any).workspaces as any[]) : []
  const providers = Array.isArray(data?.settings?.providers) ? data.settings.providers : []
  const modelGroups = Array.isArray((s as any)?.modelGroups?.items) ? (s as any).modelGroups.items : []
  const hookPrompts = (s as any)?.hookPrompts && typeof (s as any).hookPrompts === 'object' ? (s as any).hookPrompts : { loading: false, error: '', library: { presets: [] } as HookPromptLibrary }
  const placeholders = (s as any)?.placeholders && typeof (s as any).placeholders === 'object' ? (s as any).placeholders : { loading: false, error: '', library: { placeholders: [], folders: [] } as PlaceholderLibrary, preview: { text: '', problems: [] }, problems: [], dependencyTree: { name: '' } }
  const systemPlugins = (s as any)?.systemPlugins && typeof (s as any).systemPlugins === 'object' ? (s as any).systemPlugins : { loading: false, error: '', items: [], selectedPluginId: '', selectedPlugin: null, availableInterfaces: [] }
  const favorites = (data as any)?.favorites && typeof (data as any).favorites === 'object' ? (data as any).favorites : { folders: [], chatRefsByFolderId: {} }
  const favoriteFolders = Array.isArray((favorites as any)?.folders) ? ((favorites as any).folders as any[]) : []
  const favoriteChatRefsByFolderId =
    (favorites as any)?.chatRefsByFolderId && typeof (favorites as any).chatRefsByFolderId === 'object' ? (favorites as any).chatRefsByFolderId : {}
  const transparentChatBg = !!data?.settings?.transparentChatBg
  const chatBgOpacity = clampNum(Number(data?.settings?.chatBgOpacity ?? 0), 0, 100)
  const chatBgBlur = clampNum(Number(data?.settings?.chatBgBlur ?? 0), 0, 24)
  const topbarOpacity = clampNum(Number(data?.settings?.topbarOpacity ?? 100), 0, 100)
  const topbarBlur = clampNum(Number(data?.settings?.topbarBlur ?? 0), 0, 24)
  const composerOpacity = clampNum(Number(data?.settings?.composerOpacity ?? 86), 40, 100)
  const composerBlur = clampNum(Number(data?.settings?.composerBlur ?? 10), 0, 24)
  const renderSafetyPolicy = (() => {
    const v = String((data?.settings as any)?.renderSafetyPolicy || 'original').trim()
    return v === 'unsafe' ? 'unsafe' : v === 'baseline' ? 'baseline' : 'original'
  })()
  const userMessageCollapseEnabled = !!data?.settings?.userMessageCollapseEnabled
  const userMessageCollapseLines = clampNum(Number(data?.settings?.userMessageCollapseLines ?? 8), 1, 50)
  const attachSendLimitChars = clampNum(Number(data?.settings?.attachments?.sendLimitChars ?? 80000), 1000, 2000000)
  const attachMaxFileSizeMbByKind0 = (data?.settings?.attachments as any)?.maxFileSizeMbByKind
  const attachMaxFileSizeMbByKind = attachMaxFileSizeMbByKind0 && typeof attachMaxFileSizeMbByKind0 === 'object' ? attachMaxFileSizeMbByKind0 : {}
  const attachMaxFileSizeMbTxt = clampNum(Number((attachMaxFileSizeMbByKind as any)?.txt ?? 10), 0, 2048)
  const attachMaxFileSizeMbMd = clampNum(Number((attachMaxFileSizeMbByKind as any)?.md ?? 10), 0, 2048)
  const attachMaxFileSizeMbPdf = clampNum(Number((attachMaxFileSizeMbByKind as any)?.pdf ?? 10), 0, 2048)
  const attachMaxFileSizeMbDocx = clampNum(Number((attachMaxFileSizeMbByKind as any)?.docx ?? 10), 0, 2048)
  const attachMaxFileSizeMbPpt = clampNum(Number((attachMaxFileSizeMbByKind as any)?.ppt ?? 10), 0, 2048)
  const stickersEnabled = !!data?.settings?.stickers?.enabled
  const stickerMap = data?.settings?.stickers?.map
  const stickerCategories = Array.isArray(data?.settings?.stickers?.categories) ? data.settings.stickers.categories : []
  const bgAlpha = transparentChatBg ? Math.max(chatBgOpacity / 100, chatBgBlur > 0 ? 0.01 : 0) : 1

  const activeTargetKind0 = String((s.draft as any)?.activeTargetKind || (data?.ui as any)?.activeTargetKind || 'role').trim()
  const activeTargetKind = activeTargetKind0 === 'group' ? 'group' : activeTargetKind0 === 'workspace' ? 'workspace' : 'role'
  const activeGroupId = String((s.draft as any)?.activeGroupId || (data?.ui as any)?.activeGroupId || '')
  const activeWorkspaceId = String((s.draft as any)?.activeWorkspaceId || (data?.ui as any)?.activeWorkspaceId || '')
  const activeRole = controller.activeRole()
  const activeGroup = activeTargetKind === 'group' ? (groups.find((g: any) => String(g?.id || '') === activeGroupId) || null) : null
  const activeWorkspace = activeTargetKind === 'workspace' ? (workspaces.find((workspace: any) => String(workspace?.id || '') === activeWorkspaceId) || null) : null
  const activeWorkspaceChatTargetId = workspaceRoleTargetId(activeWorkspaceId, (activeRole as any)?.id)
  const activeChatTargetId = activeTargetKind === 'group' ? activeGroupId : activeTargetKind === 'workspace' ? activeWorkspaceChatTargetId : String(activeRole?.id || '')
  const activeChatSelectionBox = activeChatTargetId
    ? activeTargetKind === 'group'
      ? (data as any)?.chatsByGroup?.[activeChatTargetId]
      : activeTargetKind === 'workspace'
        ? (data as any)?.chatsByWorkspace?.[activeChatTargetId]
        : data?.chatsByRole?.[activeChatTargetId]
    : null
  const activeChat = controller.activeChat()
  const selectedActiveChatId = String(activeChat?.id || activeChatSelectionBox?.activeChatId || '').trim()
  const selectedActiveChatKey = selectedActiveChatId ? `${activeTargetKind}:${activeChatTargetId}:${selectedActiveChatId}` : ''
  const chatSwitch = useDeferredChatSwitch(controller, activeChat, selectedActiveChatId, selectedActiveChatKey)
  const renderChat = chatSwitch.renderChat
  const renderChatId = chatSwitch.renderChatId
  const activeChatId = chatSwitch.activeChatId
  const chatSettingsSavingByTarget = (s as any)?.chatSettings?.savingByTarget
  const activeChatSettingsTargetKey = activeChatTargetId && activeChatId
    ? chatSettingsTargetKey({ kind: activeTargetKind, targetId: activeChatTargetId, sessionId: activeChatId })
    : ''
  const chatSettingsSavingStatuses = chatSettingsSavingByTarget && typeof chatSettingsSavingByTarget === 'object'
    ? Object.values(chatSettingsSavingByTarget).filter((item: any) => (
        chatSettingsTargetKey({ kind: String(item?.kind || '') as any, targetId: String(item?.targetId || ''), sessionId: String(item?.sessionId || '') }) === activeChatSettingsTargetKey
      )) as any[]
    : []
  const chatSettingsSavingFor = (action: string) => chatSettingsSavingStatuses.find((item: any) => String(item?.action || '') === action) || null
  const chatSettingsModelSaving = !!chatSettingsSavingFor('model')
  const chatSettingsReasoningSaving = !!chatSettingsSavingFor('reasoning')
  const chatSettingsStreamSaving = !!chatSettingsSavingFor('stream')
  const chatSettingsHookSaving = !!chatSettingsSavingFor('hook')
  const openingChatMeta = (() => {
    if (activeChat || !data) return null
    const targetId = String(activeChatTargetId || '').trim()
    if (!targetId) return null
    const box = activeChatSelectionBox
    const openingChatId = String(activeChatId || box?.activeChatId || '').trim()
    if (!openingChatId) return null
    const metas = Array.isArray(box?.chatMetas) ? box.chatMetas : []
    return metas.find((m: any) => String(m?.id || '') === openingChatId) || null
  })()
  const savedTreeDir = (() => {
    const raw = String(((data?.settings as any)?.branchTree?.dir ?? '') as any).trim()
    return raw === 'lr' || raw === 'tb' || raw === 'bt' || raw === 'rl' ? (raw as any) : 'lr'
  })() as 'lr' | 'tb' | 'bt' | 'rl'
  const savedTreeView = (() => {
    const raw = String(((data?.settings as any)?.branchTree?.view ?? '') as any).trim()
    return raw === 'right' || raw === 'float' ? (raw as any) : 'right'
  })() as 'right' | 'float'
  const savedTreeModalHotkey = (() => {
    const raw = String(((data?.settings as any)?.branchTree?.modalHotkey ?? '') as any).trim()
    return normalizeHotkeyString(raw)
  })()
  const savedTreeFollowSelected = (() => {
    const raw = (data?.settings as any)?.branchTree?.followSelected
    return typeof raw === 'boolean' ? raw : true
  })() as boolean
  const branchDraftRaw: any = (s as any)?.branchDraft
  const branchDraft =
    activeTargetKind === 'role' &&
    branchDraftRaw &&
    typeof branchDraftRaw === 'object' &&
    String(branchDraftRaw?.roleId || '') === String(activeRole?.id || '') &&
    String(branchDraftRaw?.chatId || '') === String(activeChat?.id || '')
      ? branchDraftRaw
      : null
  const branchDraftKey = branchDraft ? `${String(branchDraft?.chatId || '')}:${String(branchDraft?.forkFromMid || '')}:${String(branchDraft?.createdAt || '')}` : ''
  const [treeOpen, setTreeOpen] = React.useState(false)
  const [treeViewOverride, setTreeViewOverride] = React.useState<'' | 'right' | 'float'>('')
  const [treePan, setTreePan] = React.useState<{ x: number; y: number }>({ x: 18, y: 18 })
  const [treeScale, setTreeScale] = React.useState(1)
  const [treeDir, setTreeDir] = React.useState<'lr' | 'tb' | 'bt' | 'rl'>(() => savedTreeDir)
  const [treeSelectedMid, setTreeSelectedMid] = React.useState('')
  const [sendPathAnchor, setSendPathAnchor] = React.useState<SendPathAnchor>(() => emptySendPathAnchor())
  const clearSendPathAnchor = useEvent(() => setSendPathAnchor(emptySendPathAnchor()))
  const sendPathAnchorNonceRef = React.useRef(0)
  const [treePop, setTreePop] = React.useState<{ id: string; at: number }>({ id: '', at: 0 })
  const [treeDragging, setTreeDragging] = React.useState(false)
  const treeDragRef = React.useRef<{ pid: number; sx: number; sy: number; ox: number; oy: number; moved: boolean } | null>(null)
  const treeSuppressClickRef = React.useRef(false)
  const treeViewportRef = React.useRef<SVGGElement | null>(null)
  const treeViewRef = React.useRef<{ x: number; y: number; scale: number }>({ x: 18, y: 18, scale: 1 })
  const treeViewRafRef = React.useRef<number>(0)
  const treeHostRightRef = React.useRef<HTMLDivElement | null>(null)
  const treeHostFloatRef = React.useRef<HTMLDivElement | null>(null)
  const treeFollowRafRef = React.useRef<number>(0)
  const treeFollowAnimRef = React.useRef<{ targetX: number; targetY: number; lastT: number } | null>(null)
  const treeOpenTokenRef = React.useRef(0)
  const treeInitialCenterRafRef = React.useRef<number>(0)
  const treeNeedInitialCenterRef = React.useRef(false)

  const effectiveTreeView = (treeViewOverride || savedTreeView) as 'right' | 'float'
  const [chatSessionRunNotices, setChatSessionRunNotices] = React.useState<Record<string, ChatSessionRunNotice>>({})
  const chatSessionRunStatusRef = React.useRef<Record<string, ChatSessionRunObservation>>({})

  React.useEffect(() => {
    const observations = collectChatSessionRunObservations(data)
    const previousByKey = chatSessionRunStatusRef.current
    const liveKeys = new Set<string>()
    const nextByKey: Record<string, ChatSessionRunObservation> = {}

    for (const observation of observations) {
      liveKeys.add(observation.key)
      nextByKey[observation.key] = observation
    }

    setChatSessionRunNotices((prev) => {
      let changed = false
      const next = { ...prev }

      for (const observation of observations) {
        const previous = previousByKey[observation.key]
        const terminal = observation.status === 'completed' || observation.status === 'interrupted'
        if (observation.status === 'running') {
          if (next[observation.key]) {
            delete next[observation.key]
            changed = true
          }
          continue
        }
        if (terminal && previous?.status === 'running') {
          const kind = observation.status as ChatSessionRunNoticeKind
          const changedAt = Number(observation.changedAt || 0)
          if (!next[observation.key] || next[observation.key].kind !== kind || Number(next[observation.key].changedAt || 0) !== changedAt) {
            next[observation.key] = { kind, changedAt }
            changed = true
          }
        }
      }

      for (const key of Object.keys(next)) {
        if (!liveKeys.has(key)) {
          delete next[key]
          changed = true
        }
      }

      return changed ? next : prev
    })

    chatSessionRunStatusRef.current = nextByKey
  })

  React.useEffect(() => {
    const key = chatSessionRunNoticeKey(activeTargetKind, activeChatTargetId, activeChatId)
    if (!key) return
    setChatSessionRunNotices((prev) => {
      if (!prev[key]) return prev
      const next = { ...prev }
      delete next[key]
      return next
    })
  })

  const clearChatSessionRunNotice = useEvent((targetKind: 'role' | 'group' | 'workspace', targetId: string, chatId: string) => {
    const key = chatSessionRunNoticeKey(targetKind, targetId, chatId)
    if (!key) return
    setChatSessionRunNotices((prev) => {
      if (!prev[key]) return prev
      const next = { ...prev }
      delete next[key]
      return next
    })
  })

  const chatSessionRunIndicatorKind = React.useCallback(
    (targetKind: 'role' | 'group' | 'workspace', targetId: string, chat: any, selected: boolean): ChatSessionRunIndicatorKind | '' => {
      const chatId = String(chat?.id || '').trim()
      const key = chatSessionRunNoticeKey(targetKind, targetId, chatId)
      if (!key) return ''
      const summary = chatSessionRunSummaryFromListItem(chat)
      if (summary.status === 'running') return 'running'
      if (selected) return ''
      const notice = chatSessionRunNotices[key]
      return notice ? notice.kind : ''
    },
    [chatSessionRunNotices],
  )

  const isSendingThisChat = React.useCallback(
    (targetKind: 'role' | 'group' | 'workspace', targetId: string, chatId: string) => {
      const tid = String(targetId || '')
      const cid = String(chatId || '')
      if (!tid || !cid) return false
      const box = targetKind === 'group' ? data?.chatsByGroup?.[tid] : targetKind === 'workspace' ? (data as any)?.chatsByWorkspace?.[tid] : data?.chatsByRole?.[tid]
      const meta = Array.isArray(box?.chatMetas) ? box.chatMetas.find((m: any) => String(m?.id || '') === cid) : null
      return readActiveEbRunCardsForTarget(s, targetKind, tid, cid).length > 0 || normalizeChatSessionRunStatus(meta?.runStatus || meta?.status) === 'running'
    },
    [s, data],
  )

  const favoriteChildrenMap = React.useMemo(() => {
    const map: Record<string, any[]> = {}
    for (const f of favoriteFolders) {
      const pid = String((f as any)?.parentId || '')
      if (!map[pid]) map[pid] = []
      map[pid].push(f)
    }
    for (const list of Object.values(map)) {
      list.sort((a: any, b: any) => Number(a?.createdAt || 0) - Number(b?.createdAt || 0))
    }
    return map
  }, [favoriteFolders])

  const collectFavoriteFolderSubtreeIds = React.useCallback(
    (folderId: string) => {
      const fid = String(folderId || '')
      if (!fid) return []
      const out: string[] = []
      const stack = [fid]
      while (stack.length) {
        const cur = String(stack.pop() || '')
        if (!cur || out.includes(cur)) continue
        out.push(cur)
        const children = favoriteChildrenMap[cur] || []
        for (const child of children) {
          const cid = String(child?.id || '')
          if (cid) stack.push(cid)
        }
      }
      return out
    },
    [favoriteChildrenMap],
  )

  const getChatByFavoriteRef = React.useCallback(
    (targetKind: 'role' | 'group' | 'workspace', targetId: string, chatId: string) => {
      const tid = String(targetId || '')
      const cid = String(chatId || '')
      if (!tid || !cid) return null
      if (targetKind === 'group') {
        const box = (data as any)?.chatsByGroup?.[tid]
        const chats = Array.isArray(box?.chats) ? box.chats : []
        const chat = chats.find((c: any) => String(c?.id || '') === cid) || null
        const meta = Array.isArray(box?.chatMetas) ? box.chatMetas.find((c: any) => String(c?.id || '') === cid) : null
        return chat && meta ? { ...chat, ...meta } : chat || meta || null
      }
      if (targetKind === 'workspace') {
        const box = (data as any)?.chatsByWorkspace?.[tid]
        const chats = Array.isArray(box?.chats) ? box.chats : []
        const chat = chats.find((c: any) => String(c?.id || '') === cid) || null
        const meta = Array.isArray(box?.chatMetas) ? box.chatMetas.find((c: any) => String(c?.id || '') === cid) : null
        return chat && meta ? { ...chat, ...meta } : chat || meta || null
      }
      const box = data?.chatsByRole?.[tid]
      const chats = Array.isArray(box?.chats) ? box.chats : []
      const chat = chats.find((c: any) => String(c?.id || '') === cid) || null
      const meta = Array.isArray(box?.chatMetas) ? box.chatMetas.find((c: any) => String(c?.id || '') === cid) : null
      return chat && meta ? { ...chat, ...meta } : chat || meta || null
    },
    [data],
  )

  const getFavoriteTargetMeta = React.useCallback(
    (targetKind: 'role' | 'group' | 'workspace', targetId: string) => {
      const tid = String(targetId || '')
      if (!tid) return null
      if (targetKind === 'group') {
        const g = groups.find((it: any) => String(it?.id || '') === tid) || null
        if (!g) return null
        return {
          name: String(g?.name || '群聊'),
          avatar: String(g?.avatar || '👥'),
          avatarImage: String(g?.avatarImage || ''),
        }
      }
      if (targetKind === 'workspace') {
        const workspace = workspaces.find((it: any) => String(it?.id || '') === tid) || null
        if (!workspace) return null
        return {
          name: String(workspace?.name || '工作区'),
          avatar: '📁',
          avatarImage: '',
        }
      }
      const r = roles.find((it: any) => String(it?.id || '') === tid) || null
      if (!r) return null
      return {
        name: String(r?.name || '角色'),
        avatar: String(r?.avatar || '🙂'),
        avatarImage: String(r?.avatarImage || ''),
      }
    },
    [groups, roles, workspaces],
  )

  const formatModelRefText = React.useCallback(
    (modelRef: any) => {
      return formatModelRefDisplayText(modelRef, providers, modelGroups)
    },
    [providers, modelGroups],
  )

  const openCreateFavoriteFolder = useEvent((parentId = '') => {
    setCreateFavoriteFolder({ open: true, parentId: String(parentId || ''), name: '' })
  })
  const closeCreateFavoriteFolder = useEvent(() => setCreateFavoriteFolder({ open: false, parentId: '', name: '' }))
  const closeFavoriteFolderMenu = useEvent(() => setFavoriteFolderMenu({ folderId: '', parentId: '', x: 0, y: 0 }))
  const openRenameFavoriteFolder = useEvent((folderId: string) => {
    const fid = String(folderId || '')
    const folder = favoriteFolders.find((f: any) => String(f?.id || '') === fid) || null
    if (!folder) return
    closeFavoriteFolderMenu()
    setRenameFavoriteFolder({ open: true, folderId: fid, name: String(folder?.name || '') })
  })
  const closeRenameFavoriteFolder = useEvent(() => setRenameFavoriteFolder({ open: false, folderId: '', name: '' }))
  const submitRenameFavoriteFolder = useEvent(() => {
    const fid = String(renameFavoriteFolder.folderId || '')
    const name = String(renameFavoriteFolder.name || '').trim()
    if (!fid || !name) return
    controller.actions.renameFavoriteFolder?.(fid, name)
    closeRenameFavoriteFolder()
  })
  const openDeleteFavoriteFolderConfirm = useEvent((folderId: string, mode: 'keep' | 'tree') => {
    closeFavoriteFolderMenu()
    setConfirmDeleteFavoriteFolder({ open: true, folderId: String(folderId || ''), mode })
  })
  const closeDeleteFavoriteFolderConfirm = useEvent(() => setConfirmDeleteFavoriteFolder({ open: false, folderId: '', mode: 'keep' }))
  const closeMoveFavoriteFolderContents = useEvent(() => setMoveFavoriteFolderContents({ open: false, folderId: '', targetFolderId: '' }))
  const closeMoveFavoriteFolderDialog = useEvent(() => setMoveFavoriteFolderDialog({ open: false, folderId: '', parentId: '' }))
  const closeConfirmClearFavoriteFolder = useEvent(() => setConfirmClearFavoriteFolder({ open: false, folderId: '' }))
  const submitDeleteFavoriteFolder = useEvent(() => {
    const fid = String(confirmDeleteFavoriteFolder.folderId || '')
    if (!fid) return
    const folder = favoriteFolders.find((f: any) => String(f?.id || '') === fid) || null
    const isTop = !String((folder as any)?.parentId || '').trim()
    const refs = Array.isArray((favoriteChatRefsByFolderId as any)?.[fid]) ? (favoriteChatRefsByFolderId as any)[fid] : []
    if (confirmDeleteFavoriteFolder.mode === 'tree') {
      controller.actions.deleteFavoriteFolderTree?.(fid)
      closeDeleteFavoriteFolderConfirm()
      return
    }
    if (isTop && refs.length) {
      const fallback = favoriteFolders.find((f: any) => String(f?.id || '') !== fid) || null
      setMoveFavoriteFolderContents({ open: true, folderId: fid, targetFolderId: String((fallback as any)?.id || '') })
      closeDeleteFavoriteFolderConfirm()
      return
    }
    controller.actions.deleteFavoriteFolderKeepContents?.(fid)
    closeDeleteFavoriteFolderConfirm()
  })
  const submitMoveFavoriteFolderContents = useEvent(() => {
    const fid = String(moveFavoriteFolderContents.folderId || '')
    const targetFolderId = String(moveFavoriteFolderContents.targetFolderId || '')
    if (!fid || !targetFolderId) return
    controller.actions.deleteFavoriteFolderKeepContents?.(fid, targetFolderId)
    closeMoveFavoriteFolderContents()
  })
  const submitClearFavoriteFolder = useEvent(() => {
    const fid = String(confirmClearFavoriteFolder.folderId || '')
    if (!fid) return
    controller.actions.clearFavoriteFolderRefs?.(fid)
    closeConfirmClearFavoriteFolder()
  })
  const submitMoveFavoriteFolder = useEvent(() => {
    const fid = String(moveFavoriteFolderDialog.folderId || '')
    if (!fid) return
    controller.actions.moveFavoriteFolder?.(fid, moveFavoriteFolderDialog.parentId)
    const moved = favoriteFolders.find((f: any) => String(f?.id || '') === fid) || null
    const nextParentId = String(moveFavoriteFolderDialog.parentId || '')
    setFavoriteFolderExpanded((p) => {
      const next = { ...p, [fid]: true }
      if (nextParentId) next[nextParentId] = true
      const prevParentId = String((moved as any)?.parentId || '')
      if (prevParentId) next[prevParentId] = true
      return next
    })
    closeMoveFavoriteFolderDialog()
  })
  const collapseAllFavoriteFolders = useEvent(() => {
    const next: Record<string, boolean> = {}
    for (const folder of favoriteFolders) {
      const fid = String(folder?.id || '')
      if (fid) next[fid] = false
    }
    setFavoriteFolderExpanded(next)
  })
  const toggleFavoriteFolderExpanded = useEvent((folderId: string) => {
    const fid = String(folderId || '')
    if (!fid) return
    setFavoriteFolderExpanded((p) => ({ ...p, [fid]: !p[fid] }))
  })
  const toggleFavoriteFolderChecked = useEvent((folderId: string) => {
    const fid = String(folderId || '')
    if (!fid) return
    setFavoriteCheckedFolderIds((p) => (p.includes(fid) ? p.filter((x) => x !== fid) : p.concat(fid)))
  })
  const openFavoriteDialog = useEvent((targetKind: 'role' | 'group' | 'workspace', targetId: string, chatId: string, title: string) => {
    const tid = String(targetId || '')
    const cid = String(chatId || '')
    if (!tid || !cid) return
    const ids = Array.isArray(controller.actions.getChatFavoriteFolderIds?.(targetKind, tid, cid))
      ? controller.actions.getChatFavoriteFolderIds(targetKind, tid, cid)
      : []
    setFavoriteCheckedFolderIds(ids.map((x: any) => String(x || '')).filter(Boolean))
    setFavoriteDialog({ open: true, targetKind, targetId: tid, chatId: cid, title: String(title || '') })
  })
  const closeFavoriteDialog = useEvent(() => {
    setFavoriteDialog({ open: false, targetKind: 'role', targetId: '', chatId: '', title: '' })
    setFavoriteCheckedFolderIds([])
  })
  const saveFavoriteDialog = useEvent(() => {
    const { targetKind, targetId, chatId } = favoriteDialog
    if (!targetId || !chatId) return
    controller.actions.setChatFavoriteFolders?.(targetKind, targetId, chatId, favoriteCheckedFolderIds)
    closeFavoriteDialog()
  })
  const submitCreateFavoriteFolder = useEvent(() => {
    const name = String(createFavoriteFolder.name || '').trim()
    if (!name) return
    const parentId = String(createFavoriteFolder.parentId || '')
    const id = controller.actions.createFavoriteFolder?.(name, parentId)
    if (id) {
      setFavoriteFolderExpanded((p) => {
        const next = { ...p, [String(id || '')]: true }
        if (parentId) next[parentId] = true
        return next
      })
      closeCreateFavoriteFolder()
    }
  })
  const openFavoritedChat = useEvent((targetKind: 'role' | 'group' | 'workspace', targetId: string, chatId: string) => {
    const tid = String(targetId || '')
    const cid = String(chatId || '')
    if (!tid || !cid) return
    clearChatSessionRunNotice(targetKind, tid, cid)
    if (targetKind === 'group') controller.actions.setActiveGroup?.(tid)
    else if (targetKind === 'workspace') controller.actions.setActiveWorkspace?.(tid)
    else controller.actions.setActiveRole?.(tid)
    chatSwitch.requestSwitch(cid, { force: true })
    closeChatPicker()
  })

  const chatRootRef = React.useRef<HTMLDivElement | null>(null)
  const stickToBottomRef = React.useRef(true)
  const autoScrollBlockUntilRef = React.useRef(0)
  const composerRef = React.useRef<HTMLDivElement | null>(null)
  const [composerHeight, setComposerHeight] = React.useState(0)
  const chatPaneRef = React.useRef<HTMLDivElement | null>(null)
  const [treePanelW, setTreePanelW] = React.useState(360)
  const treePanelWRef = React.useRef(360)
  const [treeResizing, setTreeResizing] = React.useState(false)
  const treeResizeRef = React.useRef<{ pid: number; right: number } | null>(null)
  const treeResizeLastXRef = React.useRef(0)
  const treeResizeRafRef = React.useRef<number>(0)

  const [page, setPage] = React.useState<'chat' | 'settings'>('chat')
  const [settingsTab, setSettingsTab] = React.useState<SettingsTab>('roles')

  const [expandedUserMsgIds, setExpandedUserMsgIds] = React.useState(() => new Set<string>())
  const [expandedToolMsgIds, setExpandedToolMsgIds] = React.useState(() => new Set<string>())
  const [branchNav, setBranchNav] = React.useState<{ mid: string; at: number }>({ mid: '', at: 0 })

  React.useEffect(() => {
    setExpandedUserMsgIds(() => new Set())
    setExpandedToolMsgIds(() => new Set())
  }, [String(activeChat?.id || '')])

  React.useEffect(() => {
    setTreePan({ x: 18, y: 18 })
    setTreeScale(1)
    setTreeSelectedMid('')
    clearSendPathAnchor()
    treeViewRef.current = { x: 18, y: 18, scale: 1 }
  }, [String(activeChat?.id || ''), clearSendPathAnchor])

  React.useEffect(() => {
    if (treeDir === savedTreeDir) return
    setTreeDir(savedTreeDir)
  }, [savedTreeDir, treeDir])

  // float 模态窗不需要“吸附/定位”逻辑

  const applyTreeViewTransform = useEvent(() => {
    const g = treeViewportRef.current
    if (!g) return
    const v = treeViewRef.current
    const x = Math.round(Number(v?.x || 0))
    const y = Math.round(Number(v?.y || 0))
    const s = clampNum(Number(v?.scale || 1), 0.35, 2.6)
    try {
      g.setAttribute('transform', `translate(${x},${y}) scale(${s})`)
    } catch (_) {}
  })

  const scheduleTreeViewTransform = useEvent(() => {
    if (treeViewRafRef.current) return
    treeViewRafRef.current = requestAnimationFrame(() => {
      treeViewRafRef.current = 0
      applyTreeViewTransform()
    })
  })

  const stopTreeFollow = useEvent(() => {
    treeFollowAnimRef.current = null
    if (treeFollowRafRef.current) {
      cancelAnimationFrame(treeFollowRafRef.current)
      treeFollowRafRef.current = 0
    }
  })

  React.useLayoutEffect(() => {
    treeViewRef.current = { x: treePan.x, y: treePan.y, scale: treeScale }
    applyTreeViewTransform()
  }, [treeOpen, treePan.x, treePan.y, treeScale, applyTreeViewTransform])

  React.useEffect(() => {
    const id = String(treePop.id || '').trim()
    if (!id) return
    const t = window.setTimeout(() => setTreePop({ id: '', at: 0 }), 420)
    return () => window.clearTimeout(t)
  }, [treePop.id, treePop.at])

  React.useEffect(() => {
    treePanelWRef.current = treePanelW
  }, [treePanelW])

  React.useEffect(() => {
    if (treeOpen) return
    treeDragRef.current = null
    treeSuppressClickRef.current = false
    setTreeDragging(false)
    setTreeViewOverride('')
    stopTreeFollow()
  }, [treeOpen, stopTreeFollow])

  const endTreeResize = useEvent((e: React.PointerEvent) => {
    const st = treeResizeRef.current
    if (!st) return
    if (Number(e.pointerId) !== Number(st.pid)) return
    treeResizeRef.current = null
    if (treeResizeRafRef.current) {
      cancelAnimationFrame(treeResizeRafRef.current)
      treeResizeRafRef.current = 0
    }
    setTreeResizing(false)
  })

  const onTreeSplitterPointerDown = useEvent((e: React.PointerEvent) => {
    if (!treeOpen) return
    if (e.button !== 0) return
    const host = chatPaneRef.current
    let right = 0
    try {
      right = host ? Number(host.getBoundingClientRect().right || 0) : 0
    } catch (_) {
      right = 0
    }
    treeResizeLastXRef.current = Number(e.clientX || 0)
    treeResizeRef.current = { pid: Number(e.pointerId), right }
    setTreeResizing(true)
    e.preventDefault()
    e.stopPropagation()
    try {
      ;(e.currentTarget as any)?.setPointerCapture?.(e.pointerId)
    } catch (_) {}
  })

  const onTreeSplitterPointerMove = useEvent((e: React.PointerEvent) => {
    const st = treeResizeRef.current
    if (!st) return
    if (Number(e.pointerId) !== Number(st.pid)) return
    treeResizeLastXRef.current = Number(e.clientX || 0)
    if (treeResizeRafRef.current) return
    treeResizeRafRef.current = requestAnimationFrame(() => {
      treeResizeRafRef.current = 0
      const cur = treeResizeRef.current
      if (!cur) return
      const raw = Math.max(0, Number(cur.right || 0) - Number(treeResizeLastXRef.current || 0))
      const next = clampNum(Math.round(raw), 240, 860)
      setTreePanelW(next)
    })
  })

  React.useEffect(() => {
    if (!treeResizing) return
    const onUp = () => {
      treeResizeRef.current = null
      setTreeResizing(false)
    }
    window.addEventListener('mouseup', onUp)
    window.addEventListener('blur', onUp)
    return () => {
      window.removeEventListener('mouseup', onUp)
      window.removeEventListener('blur', onUp)
    }
  }, [treeResizing])

  React.useEffect(() => {
    if (!treeDragging) return
    const onUp = () => {
      treeDragRef.current = null
      setTreeDragging(false)
      treeSuppressClickRef.current = false
    }
    window.addEventListener('mouseup', onUp)
    window.addEventListener('blur', onUp)
    return () => {
      window.removeEventListener('mouseup', onUp)
      window.removeEventListener('blur', onUp)
    }
  }, [treeDragging])

  React.useEffect(() => {
    if (!userMessageCollapseEnabled) setExpandedUserMsgIds(() => new Set())
  }, [userMessageCollapseEnabled])

  const toggleExpandedUserMsg = useEvent((mid: string) => {
    const id = String(mid || '')
    if (!id) return
    setExpandedUserMsgIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  })

  const toggleExpandedToolMsg = useEvent((mid: string) => {
    const id = String(mid || '')
    if (!id) return
    setExpandedToolMsgIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  })

  const [rolePickerEl, setRolePickerEl] = React.useState<HTMLElement | null>(null)
  const [rolePickerTab, setRolePickerTab] = React.useState<'roles' | 'groups' | 'workspaces'>('roles')
  const [rolePickerMode, setRolePickerMode] = React.useState<'global' | 'workspaceRole'>('global')
  const [chatPickerEl, setChatPickerEl] = React.useState<HTMLElement | null>(null)
  const [chatPickerView, setChatPickerView] = React.useState<'history' | 'favorites'>('history')
  const [chatPickerSearchOpen, setChatPickerSearchOpen] = React.useState(false)
  const [chatPickerSearchText, setChatPickerSearchText] = React.useState('')
  const chatPickerSearchInputRef = React.useRef<HTMLInputElement | null>(null)
  const chatHistoryTotal = React.useMemo(() => {
    if (activeTargetKind === 'group') {
      if (!activeGroup) return 0
      const box = (data as any)?.chatsByGroup?.[String((activeGroup as any).id || '')]
      const chats = sortChatListItemsForDisplay(Array.isArray(box?.chatMetas) && box.chatMetas.length ? box.chatMetas : Array.isArray(box?.chats) ? box.chats : [])
      return chats.filter((chat: any) => chatHistoryMatchesSearch(chat, '群聊', chatPickerSearchText)).length
    }
    if (activeTargetKind === 'workspace') {
      if (!activeWorkspace) return 0
      const box = (data as any)?.chatsByWorkspace?.[String(activeChatTargetId || '')]
      const chats = sortChatListItemsForDisplay(Array.isArray(box?.chatMetas) && box.chatMetas.length ? box.chatMetas : Array.isArray(box?.chats) ? box.chats : [])
      return chats.filter((chat: any) => chatHistoryMatchesSearch(chat, '工作区会话', chatPickerSearchText)).length
    }
    const role = activeRole
    if (!role) return 0
    const box = data?.chatsByRole?.[String(role.id)]
    const chats = sortChatListItemsForDisplay(Array.isArray(box?.chatMetas) && box.chatMetas.length ? box.chatMetas : Array.isArray(box?.chats) ? box.chats : [])
    return chats.filter((chat: any) => chatHistoryMatchesSearch(chat, '新聊天', chatPickerSearchText)).length
  }, [activeTargetKind, activeGroup, activeWorkspace, activeChatTargetId, activeRole, data, chatPickerSearchText])
  const { scrollRef: chatHistoryScrollRef, onScrollPositionChange: onChatHistoryScrollPositionChange, visibleCount: chatHistoryVisibleCount } = useLazyListWindow({
    resetKey: [chatPickerEl ? 'open' : 'closed', chatPickerView, activeTargetKind, activeChatTargetId, chatPickerSearchText].join(':'),
    total: chatHistoryTotal,
    pageSize: CHAT_HISTORY_PAGE_SIZE,
    bottomThresholdRatio: CHAT_HISTORY_BOTTOM_THRESHOLD_RATIO,
  })
  const [favoriteSearchOpen, setFavoriteSearchOpen] = React.useState(false)
  const [favoriteSearchText, setFavoriteSearchText] = React.useState('')
  const favoriteSearchInputRef = React.useRef<HTMLInputElement | null>(null)
  const [favoriteFolderExpanded, setFavoriteFolderExpanded] = React.useState<Record<string, boolean>>({})
  const [favoriteDialog, setFavoriteDialog] = React.useState<{ open: boolean; targetKind: 'role' | 'group' | 'workspace'; targetId: string; chatId: string; title: string }>({
    open: false,
    targetKind: 'role',
    targetId: '',
    chatId: '',
    title: '',
  })
  const [favoriteCheckedFolderIds, setFavoriteCheckedFolderIds] = React.useState<string[]>([])
  const [createFavoriteFolder, setCreateFavoriteFolder] = React.useState<{ open: boolean; parentId: string; name: string }>({
    open: false,
    parentId: '',
    name: '',
  })
  const [favoriteFolderMenu, setFavoriteFolderMenu] = React.useState<{ folderId: string; parentId: string; x: number; y: number }>({ folderId: '', parentId: '', x: 0, y: 0 })
  const [renameFavoriteFolder, setRenameFavoriteFolder] = React.useState<{ open: boolean; folderId: string; name: string }>({ open: false, folderId: '', name: '' })
  const [confirmDeleteFavoriteFolder, setConfirmDeleteFavoriteFolder] = React.useState<{ open: boolean; folderId: string; mode: 'keep' | 'tree' }>({
    open: false,
    folderId: '',
    mode: 'keep',
  })
  const [moveFavoriteFolderContents, setMoveFavoriteFolderContents] = React.useState<{ open: boolean; folderId: string; targetFolderId: string }>({
    open: false,
    folderId: '',
    targetFolderId: '',
  })
  const [moveFavoriteFolderDialog, setMoveFavoriteFolderDialog] = React.useState<{ open: boolean; folderId: string; parentId: string }>({
    open: false,
    folderId: '',
    parentId: '',
  })
  const [confirmClearFavoriteFolder, setConfirmClearFavoriteFolder] = React.useState<{ open: boolean; folderId: string }>({ open: false, folderId: '' })
  const [attachmentPickerEl, setAttachmentPickerEl] = React.useState<HTMLElement | null>(null)
  const [tempModelPickerEl, setTempModelPickerEl] = React.useState<HTMLElement | null>(null)
  const [asyncToolTasksEl, setAsyncToolTasksEl] = React.useState<HTMLElement | null>(null)
  const [asyncToolTasks, setAsyncToolTasks] = React.useState<any[]>([])
  const [asyncToolTasksLoading, setAsyncToolTasksLoading] = React.useState(false)
  const [reasoningPickerEl, setReasoningPickerEl] = React.useState<HTMLElement | null>(null)
  const [fileAdjust, setFileAdjust] = React.useState<{ el: HTMLElement | null; id: string }>({ el: null, id: '' })
  const [tempModelProviderId, setTempModelProviderId] = React.useState('')
  const [tempModelPick, setTempModelPick] = React.useState('')
  const composerInputRef = React.useRef<HTMLTextAreaElement | HTMLInputElement | null>(null)
  const draftFilePickerInputRef = React.useRef<HTMLInputElement | null>(null)
  const roleSessionControlsEnabled = activeTargetKind !== 'group' && !!activeRole

  React.useEffect(() => {
    if (roleSessionControlsEnabled) return
    setTempModelPickerEl(null)
    setReasoningPickerEl(null)
  }, [roleSessionControlsEnabled])

  const focusComposerSoon = useEvent(() => {
    if (page !== 'chat') return
    requestAnimationFrame(() => {
      setTimeout(() => {
        const el = composerInputRef.current
        if (!el) return
        try {
          el.focus?.()
          const v = typeof (el as any).value === 'string' ? String((el as any).value || '') : ''
          if (typeof (el as any).setSelectionRange === 'function') (el as any).setSelectionRange(v.length, v.length)
        } catch (_) {}
      }, 0)
    })
  })

  const closeTreeModal = useEvent((focusComposer: boolean) => {
    setTreeViewOverride('')
    setTreeOpen(false)
    if (focusComposer) focusComposerSoon()
  })

  const onTopbarPointerDown = useEvent((e: React.PointerEvent) => {
    if (e.button !== 0) return
    const t = e.target as any
    if (!t || typeof t.closest !== 'function') return
    if (t.closest('button, a, input, textarea, select, [role="button"], [data-window-controls="true"]')) return
    controller?.capabilities?.ui?.startDragging?.()
  })

  const standaloneWindowControls = windowControls?.standalone ? <StandaloneWindowControls actions={windowControls.actions} /> : null

  const onClickOpenImageViewer = useEvent((e: React.MouseEvent) => {
    const t = e.target as any
    if (!(t instanceof Element)) return
    const img = t.closest?.('img[data-fw-img="1"]')
    if (!img) return
    if (!(img instanceof HTMLImageElement)) return
    const src = String(img.getAttribute('src') || '').trim()
    if (!src) return
    e.preventDefault()
    e.stopPropagation()
    controller.actions.openImageViewer(e.currentTarget as any, img)
  })

  const closeFileAdjust = useEvent(() => setFileAdjust({ el: null, id: '' }))
  const closeReasoningPicker = useEvent(() => setReasoningPickerEl(null))
  const openFileAdjust = useEvent((e: React.MouseEvent<HTMLElement>, fileId: string) => {
    const id = String(fileId || '')
    if (!id) return
    e.preventDefault()
    e.stopPropagation()
    setFileAdjust({ el: e.currentTarget, id })
  })

  const [attachView, setAttachView] = React.useState<{ el: HTMLElement | null; mid: string; idx: number }>({ el: null, mid: '', idx: -1 })
  const closeAttachView = useEvent(() => setAttachView({ el: null, mid: '', idx: -1 }))
  const openAttachView = useEvent((e: React.MouseEvent<HTMLElement>, mid: string, idx: number) => {
    const id = String(mid || '').trim()
    if (!id) return
    e.preventDefault()
    e.stopPropagation()
    setAttachView({ el: e.currentTarget, mid: id, idx: Math.max(-1, Math.floor(Number(idx || 0))) })
  })

  const endTreeDrag = useEvent((e: React.PointerEvent) => {
    const st = treeDragRef.current
    if (!st) return
    if (Number(e.pointerId) !== Number(st.pid)) return
    treeDragRef.current = null
    setTreeDragging(false)
    const v = treeViewRef.current
    setTreePan({ x: Number(v?.x || 0), y: Number(v?.y || 0) })
    setTreeScale(clampNum(Number(v?.scale || 1), 0.35, 2.6))
    if (st.moved) {
      treeSuppressClickRef.current = true
      setTimeout(() => {
        treeSuppressClickRef.current = false
      }, 0)
    }
  })

  const onTreePointerDown = useEvent((e: React.PointerEvent) => {
    if (!treeOpen) return
    if (e.button !== 0) return
    try {
      const t = e.target as any
      if (t && typeof t.closest === 'function' && t.closest('[data-tree-node="1"]')) return
    } catch (_) {}
    stopTreeFollow()
    treeSuppressClickRef.current = false
    const v = treeViewRef.current
    treeDragRef.current = { pid: Number(e.pointerId), sx: Number(e.clientX), sy: Number(e.clientY), ox: Number(v?.x || 0), oy: Number(v?.y || 0), moved: false }
    setTreeDragging(true)
    e.preventDefault()
    try {
      ;(e.currentTarget as any)?.setPointerCapture?.(e.pointerId)
    } catch (_) {}
  })

  const onTreePointerMove = useEvent((e: React.PointerEvent) => {
    const st = treeDragRef.current
    if (!st) return
    if (Number(e.pointerId) !== Number(st.pid)) return
    const dx = Number(e.clientX) - Number(st.sx)
    const dy = Number(e.clientY) - Number(st.sy)
    if (!st.moved && Math.hypot(dx, dy) > 3) st.moved = true
    treeViewRef.current = { ...treeViewRef.current, x: st.ox + dx, y: st.oy + dy }
    scheduleTreeViewTransform()
  })

  const onTreeWheel = useEvent((e: React.WheelEvent) => {
    if (!treeOpen) return
    const el = e.currentTarget as HTMLElement | null
    if (!el) return
    const dy = Number((e as any)?.deltaY || 0)
    if (!isFinite(dy) || dy === 0) return
    e.preventDefault()

    const rect = el.getBoundingClientRect()
    const cx = Number(e.clientX) - Number(rect.left)
    const cy = Number(e.clientY) - Number(rect.top)

    const scale0 = Number(treeViewRef.current?.scale || 1)
    const factor = dy > 0 ? 1 / 1.12 : 1.12
    const nextScale = clampNum(scale0 * factor, 0.35, 2.6)
    if (Math.abs(nextScale - scale0) < 1e-6) return

    const pan0 = treeViewRef.current
    const wx = (cx - Number(pan0?.x || 0)) / scale0
    const wy = (cy - Number(pan0?.y || 0)) / scale0
    const nx = cx - wx * nextScale
    const ny = cy - wy * nextScale

    treeViewRef.current = { x: nx, y: ny, scale: nextScale }
    scheduleTreeViewTransform()
  })

  // 分支树：Ctrl + 上/下 缩放（以视窗中心为锚点）
  React.useEffect(() => {
    if (!treeOpen) return
    const onKeyDown = (e: KeyboardEvent) => {
      if (!treeOpen) return
      if (e.defaultPrevented) return
      if ((e as any).isComposing) return
      if (!e.ctrlKey) return

      const key = String(e.key || '')
      if (key !== 'ArrowUp' && key !== 'ArrowDown') return

      const target = e.target as any
      const tag = String(target?.tagName || '').toUpperCase()
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || !!target?.isContentEditable) return

      const host = (effectiveTreeView === 'float' ? treeHostFloatRef.current : treeHostRightRef.current) as HTMLDivElement | null
      if (!host) return
      const w = Number(host.clientWidth || 0)
      const h = Number(host.clientHeight || 0)
      if (w < 20 || h < 20) return

      e.preventDefault()
      e.stopPropagation()

      stopTreeFollow()

      const cx = w / 2
      const cy = h / 2

      const scale0 = Number(treeViewRef.current?.scale || 1)
      const factor = key === 'ArrowUp' ? 1.12 : 1 / 1.12
      const nextScale = clampNum(scale0 * factor, 0.35, 2.6)
      if (Math.abs(nextScale - scale0) < 1e-6) return

      const pan0 = treeViewRef.current
      const wx = (cx - Number(pan0?.x || 0)) / scale0
      const wy = (cy - Number(pan0?.y || 0)) / scale0
      const nx = cx - wx * nextScale
      const ny = cy - wy * nextScale

      treeViewRef.current = { x: nx, y: ny, scale: nextScale }
      scheduleTreeViewTransform()
      setTreePan({ x: nx, y: ny })
      setTreeScale(nextScale)
    }

    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
  }, [treeOpen, effectiveTreeView, scheduleTreeViewTransform, stopTreeFollow])

  const cycleTreeDir = useEvent(() => {
    const order: Array<'lr' | 'tb' | 'bt' | 'rl'> = ['lr', 'tb', 'bt', 'rl']
    const i = Math.max(0, order.indexOf(treeDir))
    const next = order[(i + 1) % order.length]
    setTreeDir(next)
    controller.actions.setBranchTreeDir?.(next)
    setTreePan({ x: 18, y: 18 })
    setTreeScale(1)
    treeViewRef.current = { x: 18, y: 18, scale: 1 }
    scheduleTreeViewTransform()
  })

  const chatAllMessagesRaw: any[] = Array.isArray(renderChat?.messages) ? (renderChat.messages as any[]) : []
  const chatAllById = React.useMemo(() => {
    const m = new Map<string, any>()
    for (const it of chatAllMessagesRaw) {
      const id = String(it?.id || '').trim()
      if (!id || m.has(id)) continue
      m.set(id, it)
    }
    return m
  }, [chatAllMessagesRaw, chatAllMessagesRaw.length, Number((renderChat as any)?.updatedAt || 0)])
  const chatAllIndexById = React.useMemo(() => {
    const m = new Map<string, number>()
    for (let i = 0; i < chatAllMessagesRaw.length; i++) {
      const id = String(chatAllMessagesRaw[i]?.id || '').trim()
      if (!id || m.has(id)) continue
      m.set(id, i)
    }
    return m
  }, [chatAllMessagesRaw, chatAllMessagesRaw.length, Number((renderChat as any)?.updatedAt || 0)])
  const prevAiMidByAssistantId = React.useMemo(() => {
    const out = new Map<string, string>()
    const msgs = chatAllMessagesRaw

    const findPrevAi = (assistantMid: string) => {
      const aiIndex = chatAllIndexById.get(assistantMid)
      if (aiIndex == null || aiIndex < 0) return ''
      const target = msgs[aiIndex]
      if (!target || target.role !== 'assistant') return ''

      let userMid = String((target as any)?.parentMid || '').trim()
      let userMsg = userMid ? (chatAllById.get(userMid) || null) : null
      if (!userMsg || userMsg.role !== 'user') {
        for (let i = aiIndex - 1; i >= 0; i--) {
          const m = msgs[i]
          if (m && m.role === 'user') {
            userMsg = m
            userMid = String(m?.id || '').trim()
            break
          }
          if (m && m.role === 'assistant') break
        }
      }
      if (!userMsg || userMsg.role !== 'user') return ''

      const p0 = String((userMsg as any)?.parentMid || '').trim()
      const pMsg = p0 ? (chatAllById.get(p0) || null) : null
      if (pMsg && pMsg.role === 'assistant') return String(pMsg?.id || '').trim()

      const uidx = userMid ? (chatAllIndexById.get(userMid) ?? -1) : -1
      const start = uidx >= 0 ? uidx - 1 : aiIndex - 1
      for (let i = start; i >= 0; i--) {
        const m = msgs[i]
        if (m && m.role === 'assistant') return String(m?.id || '').trim()
      }
      return ''
    }

    for (const m of msgs) {
      if (!m || m.role !== 'assistant') continue
      const mid = String(m?.id || '').trim()
      if (!mid || out.has(mid)) continue
      out.set(mid, findPrevAi(mid))
    }
    return out
  }, [chatAllMessagesRaw, chatAllMessagesRaw.length, Number((renderChat as any)?.updatedAt || 0), chatAllById, chatAllIndexById])
  const activeSessionRunCards = readActiveEbRunCardsForTarget(s, activeTargetKind, String(activeChatTargetId || ''), renderChatId)
  const activeSessionRunCardsKey = activeSessionRunCards
    .map((card: any) => `${String(card?.runId || '')}:${String(card?.lastMessageId || '')}:${String(card?.status || '')}:${String(card?.retry?.attempt || '')}:${String(card?.retry?.retryAt || '')}:${String(card?.retry?.failure?.message || '')}:${Number(card?.updatedAt || 0)}`)
    .join('|')
  const activeChatRunCards = renderChatId === activeChatId ? activeSessionRunCards : readActiveEbRunCardsForTarget(s, activeTargetKind, String(activeChatTargetId || ''), activeChatId)
  const treeLayout = React.useMemo(() => {
    if (!renderChat || !treeOpen) return null
    const treeMessages = chatAllMessagesRaw.filter((message: any) => {
      const hasActiveRun = !!activeRunCardForAssistantMessage(activeSessionRunCards, message) && isAssistantGenerating(message)
      return !isStaleAssistantPlaceholder(message, hasActiveRun)
    })
    return buildChatTreeLayout(treeMessages, { maxNodes: 900 })
  }, [renderChatId, Number((renderChat as any)?.updatedAt || 0), treeOpen, chatAllMessagesRaw.length, activeSessionRunCardsKey])
  const treeRender = React.useMemo(() => {
    const tl: any = treeLayout
    if (!tl || !Array.isArray(tl.nodes) || tl.nodes.length === 0) return null

    const nodeW = Number(tl.nodeW || 168)
    const nodeH = Number(tl.nodeH || 44)
    const gapX = Number(tl.gapX || 120)
    const gapY = Number(tl.gapY || 70)
    const pad = Number(tl.pad || 22)
    const maxDepth = Math.max(0, Math.floor(Number(tl.maxDepth || 0)))
    const maxLane = Math.max(0, Number(tl.maxLane || 0))

    const stepDepthX = nodeW + gapX
    const stepDepthY = nodeH + gapY
    const stepLaneX = nodeW + gapX
    const stepLaneY = gapY

    const nodes = (tl.nodes as any[]).map((n: any) => {
      const depth = Math.max(0, Math.floor(Number(n?.depth || 0)))
      const lane = Math.max(0, Number(n?.lane || 0))
      let x = 0
      let y = 0

      if (treeDir === 'lr' || treeDir === 'rl') {
        const d = treeDir === 'rl' ? maxDepth - depth : depth
        x = pad + d * stepDepthX
        y = pad + lane * stepLaneY
      } else {
        const d = treeDir === 'bt' ? maxDepth - depth : depth
        x = pad + lane * stepLaneX
        y = pad + d * stepDepthY
      }

      return { ...n, depth, lane, x, y }
    })

    const byId = new Map<string, any>()
    for (const n of nodes) {
      const id = String(n?.id || '').trim()
      if (id && !byId.has(id)) byId.set(id, n)
    }

    const edges = Array.isArray(tl.edges) ? (tl.edges as any[]) : []

    let maxX = 0
    let maxY = 0
    for (const n of nodes) {
      maxX = Math.max(maxX, Number(n.x || 0) + nodeW)
      maxY = Math.max(maxY, Number(n.y || 0) + nodeH)
    }

    const size = { w: maxX + pad, h: maxY + pad }
    return { nodes, edges, byId, nodeW, nodeH, size }
  }, [treeLayout, treeDir])
  const activeBranchIdUi = String((activeChat as any)?.branching?.activeBranchId || '')
  const activeSendPathAnchorMid =
    String(sendPathAnchor.chatId || '') === String(activeChat?.id || '') ? String(sendPathAnchor.parentMid || '').trim() : ''
  const activeSendPathRunId = activeSendPathAnchorMid ? String(sendPathAnchor.runId || '').trim() : ''
  const activeSendPathRunCard = activeSendPathRunId ? activeSessionRunCards.find((card: any) => String(card?.runId || '').trim() === activeSendPathRunId) || null : null
  const activeSendPathFollowMid = React.useMemo(() => {
    if (!renderChat || !activeSendPathAnchorMid) return ''
    const existingIds = new Set(Array.isArray(sendPathAnchor.existingMessageIds) ? sendPathAnchor.existingMessageIds.map((id: any) => String(id || '').trim()).filter(Boolean) : [])
    const canFollowRunMid = (mid0: any) => {
      const mid = String(mid0 || '').trim()
      return !!mid && chatAllById.has(mid) && !existingIds.has(mid)
    }
    const runCardMid = String(activeSendPathRunCard?.lastMessageId || '').trim()
    if (canFollowRunMid(runCardMid)) return runCardMid

    if (activeSendPathRunId) {
      for (let i = chatAllMessagesRaw.length - 1; i >= 0; i--) {
        const message = chatAllMessagesRaw[i]
        if (!message || String(message?.role || '') !== 'assistant') continue
        if (assistantRunGenerationId(message) === activeSendPathRunId) return String(message?.id || '').trim()
      }
    }

    const lastMid = String(sendPathAnchor.lastMessageId || '').trim()
    if (canFollowRunMid(lastMid)) return lastMid

    const inputMid = String(sendPathAnchor.inputMessageId || '').trim()
    if (canFollowRunMid(inputMid)) return inputMid
    return activeSendPathAnchorMid
  }, [renderChatId, activeSendPathAnchorMid, activeSendPathRunCard, activeSendPathRunId, sendPathAnchor.lastMessageId, sendPathAnchor.inputMessageId, sendPathAnchor.existingMessageIds, chatAllById, chatAllMessagesRaw, chatAllMessagesRaw.length])
  const treeFocusMid = String(treeSelectedMid || activeSendPathFollowMid || activeSendPathAnchorMid || '').trim()
  const allMessages: any[] = React.useMemo(() => {
    const chat: any = renderChat
    const msgs = chatAllMessagesRaw
    if (!chat || !Array.isArray(msgs) || msgs.length === 0) return []

    const branching = chat?.branching
    const activeBranchId = String(branching?.activeBranchId || 'main').trim() || 'main'
    const branches = Array.isArray(branching?.branches) ? branching.branches : []
    const b = branches.find((x: any) => String(x?.id || '') === activeBranchId) || null

    const activeHeadMid = String(b?.headMid || '').trim()
    let headMid = activeHeadMid
    if (branchDraft) headMid = String(branchDraft?.forkFromMid || '').trim() || headMid
    if (!branchDraft && treeSelectedMid) headMid = String(treeSelectedMid || '').trim() || headMid
    else if (!branchDraft && activeSendPathAnchorMid) headMid = activeSendPathFollowMid || activeSendPathAnchorMid
    if (!headMid) headMid = String(msgs[msgs.length - 1]?.id || '').trim()
    if (!headMid) return msgs

    const byId = new Map<string, any>()
    for (const m of msgs) {
      const id = String(m?.id || '').trim()
      if (!id || byId.has(id)) continue
      byId.set(id, m)
    }

    const out: any[] = []
    const seen = new Set<string>()
    let cur = headMid
    while (cur && !seen.has(cur)) {
      seen.add(cur)
      const m = byId.get(cur) || null
      if (!m) break
      out.push(m)
      cur = String((m as any)?.parentMid || '').trim()
    }

    out.reverse()
    return out.length ? out : msgs
  }, [renderChatId, Number((renderChat as any)?.updatedAt || 0), activeBranchIdUi, branchDraftKey, treeSelectedMid, activeSendPathAnchorMid, activeSendPathFollowMid, chatAllMessagesRaw, chatAllMessagesRaw.length])
  const activeVisibleMessageIds = React.useMemo(() => new Set(allMessages.map((message: any) => String(message?.id || '').trim()).filter(Boolean)), [allMessages])
  const activeVisibleHeadMid = allMessages.length ? String(allMessages[allMessages.length - 1]?.id || '').trim() : ''
  const activeVisibleRunCards = React.useMemo(
    () => filterEbRoleRunCardsOnMessagePath(activeSessionRunCards, activeVisibleMessageIds, activeVisibleHeadMid),
    [activeSessionRunCardsKey, activeVisibleMessageIds, activeVisibleHeadMid],
  )
  const latestActiveVisibleRunCard = activeVisibleRunCards.length ? activeVisibleRunCards[activeVisibleRunCards.length - 1] : null
  const activeStopRunCard = activeChatRunCards.length ? activeChatRunCards[activeChatRunCards.length - 1] : null
  const activeStopRunId = String(activeStopRunCard?.runId || latestActiveVisibleRunCard?.runId || '').trim()
  const activeRunCardForMessage = (message: any) => {
    return activeRunCardForAssistantMessage(activeVisibleRunCards, message)
  }

  const assistantSiblingsByPrevAiMid = React.useMemo(() => {
    const map = new Map<string, any[]>()
    for (const m of chatAllMessagesRaw) {
      if (!m || m.role !== 'assistant') continue
      const mid = String(m?.id || '').trim()
      if (!mid) continue
      const prevAiMid = String(prevAiMidByAssistantId.get(mid) || '').trim()
      if (!prevAiMid) continue
      const list = map.get(prevAiMid) || []
      list.push(m)
      map.set(prevAiMid, list)
    }
    for (const [k, list] of map.entries()) {
      list.sort((a: any, b: any) => {
        const da = Number(a?.createdAt || 0)
        const db = Number(b?.createdAt || 0)
        if (da !== db) return da - db
        return String(a?.id || '').localeCompare(String(b?.id || ''))
      })
      map.set(k, list)
    }
    return map
  }, [chatAllMessagesRaw, chatAllMessagesRaw.length, Number((renderChat as any)?.updatedAt || 0), prevAiMidByAssistantId])
  const msgIndexById = React.useMemo(() => {
    const m = new Map<string, number>()
    for (let i = 0; i < allMessages.length; i++) {
      const id = String(allMessages[i]?.id || '')
      if (!id) continue
      if (!m.has(id)) m.set(id, i)
    }
    return m
  }, [allMessages])
  const groupedAttMsgsByRootMid = React.useMemo(() => {
    const map = new Map<string, any[]>()
    for (const m of allMessages) {
      if (!m || chatMessageMaterialKind(m) !== 'user') continue
      if (String(m?.groupRole || '') !== 'attachment') continue
      const parent = String(m?.groupParentMid || '').trim()
      if (!parent) continue
      const list = map.get(parent) || []
      list.push(m)
      map.set(parent, list)
    }
    return map
  }, [allMessages])
  const renderMessages = React.useMemo(() => {
    const out: any[] = []
    for (const m of allMessages) {
      if (!m) continue
      const hasActiveRun = !!activeRunCardForMessage(m) && isAssistantGenerating(m)
      if (isStaleAssistantPlaceholder(m, hasActiveRun)) continue
      if (chatMessageMaterialKind(m) !== 'user') {
        out.push(m)
        continue
      }
      if (String(m?.groupRole || '') === 'attachment' && String(m?.groupParentMid || '').trim()) continue
      out.push(m)
    }
    return out
  }, [allMessages, activeSessionRunCardsKey, activeVisibleRunCards])
  const showActiveRunTailPending = !!latestActiveVisibleRunCard && !renderMessages.some((message: any) => !!activeRunCardForMessage(message) && isAssistantGenerating(message))
  const activeRunTailPendingMessage = React.useMemo(() => {
    if (!showActiveRunTailPending || !latestActiveVisibleRunCard) return null
    const runId = String(latestActiveVisibleRunCard?.runId || '').trim()
    if (!runId) return null
    const t = Number(latestActiveVisibleRunCard?.createdAt || latestActiveVisibleRunCard?.updatedAt || Date.now())
    return {
      id: `__ui_active_run_tail_pending:${runId}`,
      role: 'assistant',
      content: ASSISTANT_RUNNING_CONTENT,
      pending: true,
      streaming: !!latestActiveVisibleRunCard?.stream,
      createdAt: isFinite(t) && t > 0 ? t : Date.now(),
      speakerRoleId: activeTargetKind === 'group' ? String(latestActiveVisibleRunCard?.roleId || '').trim() : '',
      displayOnlyPendingRunTail: true,
      assistantRun: {
        generationId: runId,
        status: 'running',
        mode: 'new',
        stream: !!latestActiveVisibleRunCard?.stream,
        startedAt: isFinite(t) && t > 0 ? t : Date.now(),
        updatedAt: Number(latestActiveVisibleRunCard?.updatedAt || t || Date.now()),
      },
    }
  }, [showActiveRunTailPending, latestActiveVisibleRunCard, activeTargetKind])
  const displayRenderMessages = React.useMemo(() => activeRunTailPendingMessage ? [...renderMessages, activeRunTailPendingMessage] : renderMessages, [activeRunTailPendingMessage, renderMessages])
  const activeContextTokenUsage = React.useMemo(() => sumMessageTokenEstimate(allMessages), [allMessages])
  const activeContextTokenUsageText = React.useMemo(() => formatTokenEstimate(activeContextTokenUsage), [activeContextTokenUsage])
  const activeContextTokenUsageShortText = React.useMemo(() => formatTokenEstimateShort(activeContextTokenUsage), [activeContextTokenUsage])
  const activeAsyncToolTasks = React.useMemo(() => (Array.isArray((activeChat as any)?.asyncToolTasks) ? (activeChat as any).asyncToolTasks : []), [activeChat, Number((activeChat as any)?.updatedAt || 0)])
  const activeAsyncToolTaskRunningCount = activeAsyncToolTasks.filter((task: any) => String(task?.status || '').trim() === 'running').length

  const lastMsg = renderMessages.length ? renderMessages[renderMessages.length - 1] : null
  const lastMsgId = String(lastMsg?.id || '')
  const lastMsgText = messageVisibleText(lastMsg)
  const treeHighlightEdgeKeys = React.useMemo(() => {
    const tr: any = treeRender
    const target = String(lastMsgId || '').trim()
    if (!tr || !target) return new Set<string>()
    const byId = tr?.byId
    if (!byId || typeof byId.get !== 'function') return new Set<string>()

    const out = new Set<string>()
    const seen = new Set<string>()
    let cur = target
    let guard = 0
    while (cur && !seen.has(cur) && guard < 6000) {
      guard++
      seen.add(cur)
      const n = byId.get(cur) || null
      if (!n) break
      const p = String(n?.parentId || '').trim()
      if (!p) break
      out.add(`${p}->${cur}`)
      cur = p
    }
    return out
  }, [treeRender, lastMsgId])
  const chatOverride = (activeChat && typeof activeChat === 'object' ? (activeChat as any).modelOverride : null) as any
  const overrideProviderId = String(chatOverride?.providerId || '').trim()
  const overrideModelId = String(chatOverride?.modelId || '').trim()
  const hasChatOverride = !!overrideProviderId && !!overrideModelId

  const roleProviderId = String((activeRole as any)?.modelRef?.providerId || '').trim()
  const roleModelId = String((activeRole as any)?.modelRef?.modelId || '').trim()
  const roleModelRef = (activeRole as any)?.modelRef || null

  const effectiveProviderId = hasChatOverride ? overrideProviderId : roleProviderId
  const effectiveModelId = hasChatOverride ? overrideModelId : roleModelId
  const effectiveModelRef = hasChatOverride
    ? { kind: 'provider', providerId: overrideProviderId, modelId: overrideModelId }
    : roleModelRef
  const reasoningProfile = activeTargetKind !== 'group'
    ? modelReasoningProfileFromModelRef(effectiveModelRef, providers, modelGroups)
    : { supportsReasoning: false, defaultReasoningEffort: '' as const }
  const activeChatReasoningEffort = chatReasoningEffort(activeChat)
  const activeEffectiveReasoningEffort = effectiveReasoningEffort(activeChat, reasoningProfile)
  const activeReasoningLabel = reasoningEffortLabel(activeEffectiveReasoningEffort)
  const hasChatReasoningOverride = !!activeChatReasoningEffort
  const activeHookPromptMode = String((activeChat as any)?.hookPromptMode || '').trim() === 'none' ? 'none' : String((activeChat as any)?.hookPromptMode || '').trim() === 'preset' ? 'preset' : 'inherit'
  const activeHookPromptPresetId = String((activeChat as any)?.hookPromptPresetId || '').trim()
  const activeStreamOn = chatStreamEnabled(activeChat)
  const roleDefaultHookPromptPresetId = String((activeRole as any)?.hookPromptPresetId || '').trim()
  const hookPromptSelectorDisabled = s.loading || !activeChat
  const hookPromptSelectorDisabledReason = !activeChat
    ? '请先创建或选择会话'
    : chatSettingsHookSaving
      ? 'hook 提示词保存中…'
      : ''

  const uiBusy = !!s.loading
  const messageMutationGuard = React.useMemo(
    () => createMessageMutationGuard(renderChat, { activeRunCards: activeSessionRunCards }),
    [renderChat, renderChatId, Number((renderChat as any)?.updatedAt || 0), activeSessionRunCardsKey],
  )
  const messageMutationBlocked = useEvent((mid: any, operation: MessageMutationOperation = 'edit') => {
    if (s.loading || uiBusy) return true
    return messageMutationGuard.blocked(mid, operation)
  })
  const jumpToMessage = useEvent((mid0: string) => {
    // 重要：树视图的缩放/平移是用 ref 更新的（为了性能不频繁 setState）。
    // 但一旦触发 React render（比如点击节点），useLayoutEffect 会用 treePan/treeScale 覆盖 ref。
    // 所以这里先把 ref 的最新值同步回 state，避免“缩放后第一次点击不居中、第二次才正常”的错位。
    const vv = treeViewRef.current
    setTreePan({ x: Number(vv?.x || 0), y: Number(vv?.y || 0) })
    setTreeScale(clampNum(Number(vv?.scale || 1), 0.35, 2.6))

    const mid = String(mid0 || '').trim()
    if (!mid) return
    if (!activeChat) return
    const msg = chatAllById.get(mid) || null
    if (!msg) return
    clearSendPathAnchor()
    setTreeSelectedMid(mid)

    const branching = (activeChat as any)?.branching
    const curBid = String(branching?.activeBranchId || 'main').trim() || 'main'
    const branches = Array.isArray(branching?.branches) ? branching.branches : []

    const containsMid = (headMid0: any) => {
      let cur = String(headMid0 || '').trim()
      const seen = new Set<string>()
      let guard = 0
      while (cur && !seen.has(cur) && guard < 6000) {
        guard++
        seen.add(cur)
        if (cur === mid) return true
        const m = chatAllById.get(cur) || null
        if (!m) break
        cur = String((m as any)?.parentMid || '').trim()
      }
      return false
    }

    const rawBid = String((msg as any)?.branchId || '').trim()
    let bid = rawBid || curBid || 'main'

    const curBranch = branches.find((b: any) => String(b?.id || '').trim() === curBid) || null
    const curHead = String((curBranch as any)?.headMid || '').trim()
    if (curHead && containsMid(curHead)) bid = curBid || bid
    else {
      let picked = ''
      for (const b of branches) {
        const id = String(b?.id || '').trim()
        const head = String((b as any)?.headMid || '').trim()
        if (!id || !head) continue
        if (!containsMid(head)) continue
        picked = id
        break
      }
      if (picked) bid = picked
    }

    stickToBottomRef.current = false
    autoScrollBlockUntilRef.current = Date.now() + 1200
    setBranchNav({ mid, at: Date.now() })
    if (bid && bid !== curBid) controller.actions.setActiveBranch?.(bid)
  })

  // 选中节点 → 视角自动追踪到中心（可开关，全局持久化）
  React.useEffect(() => {
    stopTreeFollow()
    if (!treeOpen) return
    if (!savedTreeFollowSelected) return
    const mid = String(treeFocusMid || '').trim()
    if (!mid) return
    const tr: any = treeRender
    if (!tr || !tr.byId || typeof tr.byId.get !== 'function') return

    const node = tr.byId.get(mid) || null
    if (!node) return

    const host = (effectiveTreeView === 'float' ? treeHostFloatRef.current : treeHostRightRef.current) as HTMLDivElement | null
    if (!host) return
    const w = Number(host.clientWidth || 0)
    const h = Number(host.clientHeight || 0)
    if (w < 20 || h < 20) return

    const nodeW = Number(tr.nodeW || 168)
    const nodeH = Number(tr.nodeH || 44)
    const wx = Number(node?.x || 0) + nodeW / 2
    const wy = Number(node?.y || 0) + nodeH / 2

    const v0 = treeViewRef.current
    const s = clampNum(Number(v0?.scale || 1), 0.35, 2.6)
    const cx = w / 2
    const cy = h / 2
    const targetX = cx - wx * s
    const targetY = cy - wy * s

    const start = () => {
      const speed = 1800 // px / s（屏幕坐标）
      const tick = (t: number) => {
        treeFollowRafRef.current = 0
        if (!treeOpen || !savedTreeFollowSelected) {
          treeFollowAnimRef.current = null
          return
        }
        if (treeDragRef.current) {
          treeFollowAnimRef.current = null
          return
        }

        const a = treeFollowAnimRef.current
        if (!a) return

        const last = Number(a.lastT || 0) || t
        const dt = Math.min(0.05, Math.max(0.001, (t - last) / 1000))
        a.lastT = t

        const v = treeViewRef.current
        const x0 = Number(v?.x || 0)
        const y0 = Number(v?.y || 0)
        const dx = Number(a.targetX) - x0
        const dy = Number(a.targetY) - y0
        const dist = Math.hypot(dx, dy)

        if (dist < 0.8) {
          treeViewRef.current = { ...treeViewRef.current, x: Number(a.targetX), y: Number(a.targetY) }
          scheduleTreeViewTransform()
          setTreePan({ x: Number(a.targetX), y: Number(a.targetY) })
          treeFollowAnimRef.current = null
          return
        }

        const step = Math.min(dist, speed * dt)
        const nx = x0 + (dx / dist) * step
        const ny = y0 + (dy / dist) * step
        treeViewRef.current = { ...treeViewRef.current, x: nx, y: ny }
        scheduleTreeViewTransform()

        treeFollowRafRef.current = requestAnimationFrame(tick)
      }

      treeFollowAnimRef.current = { targetX, targetY, lastT: performance.now() }
      treeFollowRafRef.current = requestAnimationFrame(tick)
    }

    start()
    return () => stopTreeFollow()
  }, [treeOpen, effectiveTreeView, savedTreeFollowSelected, treeFocusMid, treeRender, scheduleTreeViewTransform, stopTreeFollow])

  // 分支树（悬浮模态窗）：键盘方向键切换选中节点（按“深度/平级/分支最深”策略）
  React.useEffect(() => {
    if (!treeOpen || effectiveTreeView !== 'float') return
    const tr: any = treeRender
    if (!tr || !Array.isArray(tr.nodes) || tr.nodes.length === 0) return
    const byId = tr?.byId
    if (!byId || typeof byId.get !== 'function') return

    const nodes = tr.nodes as any[]

    const nodesAtDepth = new Map<number, any[]>()
    const childrenById = new Map<string, any[]>()
    const hasChild = new Set<string>()

    for (const n of nodes) {
      const id = String(n?.id || '').trim()
      if (!id) continue
      const depth = Math.floor(Number(n?.depth || 0))
      const list = nodesAtDepth.get(depth) || []
      list.push(n)
      nodesAtDepth.set(depth, list)

      const pid = String(n?.parentId || '').trim()
      if (pid) {
        hasChild.add(pid)
        const kids = childrenById.get(pid) || []
        kids.push(n)
        childrenById.set(pid, kids)
      }
    }

    const sortByLane = (a: any, b: any) => {
      const la = Number(a?.lane ?? Number.POSITIVE_INFINITY)
      const lb = Number(b?.lane ?? Number.POSITIVE_INFINITY)
      if (la !== lb) return la - lb
      const da = Math.floor(Number(a?.depth || 0))
      const db = Math.floor(Number(b?.depth || 0))
      if (da !== db) return da - db
      return String(a?.id || '').localeCompare(String(b?.id || ''))
    }

    for (const [d, list] of nodesAtDepth.entries()) nodesAtDepth.set(d, list.slice().sort(sortByLane))
    for (const [pid, kids] of childrenById.entries()) childrenById.set(pid, kids.slice().sort(sortByLane))

    const leaves = nodes.filter((n) => {
      const id = String(n?.id || '').trim()
      if (!id) return false
      return !hasChild.has(id)
    })
    leaves.sort(sortByLane)

    const onKeyDown = (e: KeyboardEvent) => {
      if (!treeOpen || effectiveTreeView !== 'float') return
      if (e.defaultPrevented) return
      if ((e as any).isComposing) return
      if (e.metaKey || e.ctrlKey || e.altKey) return

      const target = e.target as any
      const tag = String(target?.tagName || '').toUpperCase()
      if (tag === 'INPUT' || tag === 'TEXTAREA' || !!target?.isContentEditable) return

      const key = String(e.key || '')
      const isArrow = key === 'ArrowLeft' || key === 'ArrowRight' || key === 'ArrowUp' || key === 'ArrowDown'
      if (!isArrow) return

      const action = (() => {
        // 让方向键按“屏幕方向”适配 4 种树生长方向：
        // - 深度轴（父/子）跟随树的生长方向；
        // - 平级（lane）轴跟随与深度轴正交的方向。
        if (treeDir === 'tb') {
          if (key === 'ArrowUp') return 'parent'
          if (key === 'ArrowDown') return 'child'
          if (key === 'ArrowLeft') return 'lanePrev'
          return 'laneNext'
        }
        if (treeDir === 'bt') {
          if (key === 'ArrowDown') return 'parent'
          if (key === 'ArrowUp') return 'child'
          if (key === 'ArrowLeft') return 'lanePrev'
          return 'laneNext'
        }
        if (treeDir === 'lr') {
          if (key === 'ArrowLeft') return 'parent'
          if (key === 'ArrowRight') return 'child'
          if (key === 'ArrowUp') return 'lanePrev'
          return 'laneNext'
        }
        // rl
        if (key === 'ArrowRight') return 'parent'
        if (key === 'ArrowLeft') return 'child'
        if (key === 'ArrowUp') return 'lanePrev'
        return 'laneNext'
      })() as 'parent' | 'child' | 'lanePrev' | 'laneNext'

      const pickStartId = () => {
      const a = String(treeFocusMid || '').trim()
        if (a && byId.get(a)) return a
        const b = String(lastMsgId || '').trim()
        if (b && byId.get(b)) return b
        const first = tr.nodes.find((n: any) => !!String(n?.id || '').trim()) || null
        const fid = String(first?.id || '').trim()
        return fid && byId.get(fid) ? fid : ''
      }

      const curId = pickStartId()
      const cur = curId ? byId.get(curId) : null
      if (!cur) return

      const curDepth = Math.floor(Number(cur?.depth || 0))
      const curLane = Number(cur?.lane ?? 0)

      // 策略：
      // - 平级：在“同深度”上按 lane 移动；如果同深度边界无节点，则按 lane 去找“相邻分支”的叶子（最深节点）。
      // - 深度：parent/child。
      let nextId = ''
      if (action === 'lanePrev') {
        const list = nodesAtDepth.get(curDepth) || []
        const idx = list.findIndex((x: any) => String(x?.id || '').trim() === curId)
        if (idx > 0) nextId = String(list[idx - 1]?.id || '').trim()
        else {
          const cand = list
            .slice()
            .reverse()
            .find((x: any) => Number(x?.lane ?? Number.POSITIVE_INFINITY) < curLane && String(x?.id || '').trim() !== curId)
          nextId = cand ? String(cand?.id || '').trim() : ''
        }
        if (!nextId) {
          const leaf = leaves
            .slice()
            .reverse()
            .find((x: any) => Number(x?.lane ?? Number.POSITIVE_INFINITY) < curLane && String(x?.id || '').trim() !== curId)
          nextId = leaf ? String(leaf?.id || '').trim() : ''
        }
      } else if (action === 'laneNext') {
        const list = nodesAtDepth.get(curDepth) || []
        const idx = list.findIndex((x: any) => String(x?.id || '').trim() === curId)
        if (idx >= 0 && idx + 1 < list.length) nextId = String(list[idx + 1]?.id || '').trim()
        else {
          const cand = list.find((x: any) => Number(x?.lane ?? Number.POSITIVE_INFINITY) > curLane && String(x?.id || '').trim() !== curId)
          nextId = cand ? String(cand?.id || '').trim() : ''
        }
        if (!nextId) {
          const leaf = leaves.find((x: any) => Number(x?.lane ?? Number.POSITIVE_INFINITY) > curLane && String(x?.id || '').trim() !== curId)
          nextId = leaf ? String(leaf?.id || '').trim() : ''
        }
      } else if (action === 'parent') {
        const pid = String(cur?.parentId || '').trim()
        if (pid && byId.get(pid)) nextId = pid
      } else if (action === 'child') {
        const kids = childrenById.get(curId) || []
        if (kids.length === 1) nextId = String(kids[0]?.id || '').trim()
        else if (kids.length > 1) {
          let best = ''
          let bestD = Number.POSITIVE_INFINITY
          for (const k of kids) {
            const id = String(k?.id || '').trim()
            if (!id || !byId.get(id)) continue
            const lane = Number(k?.lane ?? Number.POSITIVE_INFINITY)
            const d = Math.abs(lane - curLane)
            if (d < bestD) {
              bestD = d
              best = id
            }
          }
          nextId = best
        }
      }

      if (!nextId || nextId === curId) return
      e.preventDefault()
      e.stopPropagation()
      jumpToMessage(nextId)
    }

    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
  }, [treeOpen, effectiveTreeView, treeDir, treeRender, treeFocusMid, lastMsgId, jumpToMessage])

  // 分支树（悬浮模态窗）：按 Enter 关闭
  React.useEffect(() => {
    if (!treeOpen || effectiveTreeView !== 'float') return
    const onKeyDown = (e: KeyboardEvent) => {
      if (!treeOpen || effectiveTreeView !== 'float') return
      if (e.defaultPrevented) return
      if ((e as any).isComposing) return
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (String(e.key || '') !== 'Enter') return

      const target = e.target as any
      const tag = String(target?.tagName || '').toUpperCase()
      if (tag === 'INPUT' || tag === 'TEXTAREA' || !!target?.isContentEditable) return

      e.preventDefault()
      e.stopPropagation()
      closeTreeModal(true)
    }
    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
  }, [treeOpen, effectiveTreeView, closeTreeModal])

  // 全局：快捷键打开/关闭“分支树模态窗”（不改变默认显示方式）
  React.useEffect(() => {
    if (page !== 'chat') return
    const hk = String(savedTreeModalHotkey || '').trim()
    if (!hk) return

    const onKeyDown = (e: KeyboardEvent) => {
      if (e.defaultPrevented) return
      if ((e as any).isComposing) return
      const cur = hotkeyFromKeyEvent(e)
      if (!cur || cur !== hk) return

      e.preventDefault()
      e.stopPropagation()

      if (treeOpen && effectiveTreeView === 'float') {
        closeTreeModal(true)
        return
      }

      setTreeViewOverride('float')
      setTreeOpen(true)
    }

    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
  }, [page, savedTreeModalHotkey, treeOpen, effectiveTreeView, closeTreeModal])
  const activeBranchHeadMid = React.useMemo(() => {
    const chat: any = activeChat
    if (!chat) return ''
    const branching = chat?.branching
    const bid = String(branching?.activeBranchId || 'main').trim() || 'main'
    const branches = Array.isArray(branching?.branches) ? branching.branches : []
    const b = branches.find((x: any) => String(x?.id || '') === bid) || null
    return String(b?.headMid || '').trim()
  }, [String(activeChat?.id || ''), Number((activeChat as any)?.updatedAt || 0), activeBranchIdUi])

  // 打开分支树时：让视角先落在“当前选中节点”上（默认取当前分支 head）
  React.useEffect(() => {
    if (!treeOpen) return
    treeOpenTokenRef.current++
    treeNeedInitialCenterRef.current = true
  }, [treeOpen, effectiveTreeView, String(activeChat?.id || '')])

  React.useEffect(() => {
    if (!treeOpen) return
    if (!treeNeedInitialCenterRef.current) return

    const token = treeOpenTokenRef.current
    let tries = 0

    const pickMid = () => {
      if (branchDraft) return String((branchDraft as any)?.forkFromMid || '').trim()
      const chosen = String(treeFocusMid || '').trim()
      if (chosen) return chosen
      const head = String(activeBranchHeadMid || '').trim()
      if (head) return head
      const msgs: any[] = Array.isArray((activeChat as any)?.messages) ? ((activeChat as any).messages as any[]) : []
      return msgs.length ? String(msgs[msgs.length - 1]?.id || '').trim() : ''
    }

    const tryCenterOnce = () => {
      if (!treeOpen) return true
      if (token !== treeOpenTokenRef.current) return true
      const tr: any = treeRender
      if (!tr || !tr.byId || typeof tr.byId.get !== 'function') return false

      const mid = pickMid()
      if (!mid) return false
      const node = tr.byId.get(mid) || null
      if (!node) return false

      const host = (effectiveTreeView === 'float' ? treeHostFloatRef.current : treeHostRightRef.current) as HTMLDivElement | null
      if (!host) return false
      const w = Number(host.clientWidth || 0)
      const h = Number(host.clientHeight || 0)
      if (w < 20 || h < 20) return false

      stopTreeFollow()

      const nodeW = Number(tr.nodeW || 168)
      const nodeH = Number(tr.nodeH || 44)
      const wx = Number(node?.x || 0) + nodeW / 2
      const wy = Number(node?.y || 0) + nodeH / 2

      const v0 = treeViewRef.current
      const s = clampNum(Number(v0?.scale || 1), 0.35, 2.6)
      const cx = w / 2
      const cy = h / 2
      const targetX = cx - wx * s
      const targetY = cy - wy * s

      treeViewRef.current = { ...treeViewRef.current, x: targetX, y: targetY }
      setTreePan({ x: targetX, y: targetY })
      scheduleTreeViewTransform()

      if (!String(treeSelectedMid || treeFocusMid || '').trim()) setTreeSelectedMid(mid)
      treeNeedInitialCenterRef.current = false
      return true
    }

    const tick = () => {
      treeInitialCenterRafRef.current = 0
      if (!treeNeedInitialCenterRef.current) return
      tries++
      if (tries > 14) {
        treeNeedInitialCenterRef.current = false
        return
      }
      const ok = tryCenterOnce()
      if (ok) return
      treeInitialCenterRafRef.current = requestAnimationFrame(tick)
    }

    if (treeInitialCenterRafRef.current) cancelAnimationFrame(treeInitialCenterRafRef.current)
    treeInitialCenterRafRef.current = requestAnimationFrame(tick)

    return () => {
      if (treeInitialCenterRafRef.current) cancelAnimationFrame(treeInitialCenterRafRef.current)
      treeInitialCenterRafRef.current = 0
    }
  }, [
    treeOpen,
    effectiveTreeView,
    treeRender,
    branchDraftKey,
    treeSelectedMid,
    treeFocusMid,
    activeBranchHeadMid,
    String(activeChat?.id || ''),
    stopTreeFollow,
    scheduleTreeViewTransform,
  ])
  const draftFiles: any[] = Array.isArray((s.draft as any)?.files) ? ((s.draft as any).files as any[]) : []
  const hasDraftFiles = draftFiles.length > 0
  const draftFilesPending = hasDraftFiles && draftFiles.some((f: any) => !!f?.pending)
  const draftFilesWarn =
    hasDraftFiles &&
    draftFiles.some((f: any) => {
      if (!f || f.pending) return false
      if (String(f?.error || '').trim()) return false
      const rawLen = String(f?.text || '').trim().length
      if (!rawLen) return false
      const pct = clampNum(Math.round(Number(f?.sendPct ?? 100)), 0, 100)
      const sendLen = Math.max(0, Math.ceil((rawLen * pct) / 100))
      return sendLen > attachSendLimitChars
    })

  const activeRoleId = String(activeRole?.id || '')
  const attachViewItem = (() => {
    const mid = String(attachView.mid || '').trim()
    const idx = Math.floor(Number(attachView.idx ?? -1))
    if (!mid || !renderChat || !Array.isArray((renderChat as any).messages) || idx < 0) return null
    const m = (renderChat as any).messages.find((x: any) => String(x?.id || '') === mid) || null
    const atts = m && Array.isArray(m.attachments) ? m.attachments : []
    const a = idx >= 0 && idx < atts.length ? atts[idx] : null
    if (!a) return null
    return { message: m, attachment: a }
  })()
  const chatNav = (() => {
    const loading = !!s.loading
    if (loading) return { olderId: '', newerId: '', lockedReason: '正在加载中' }
    let targetId = ''
    if (activeTargetKind === 'group') {
      targetId = String((activeGroup as any)?.id || activeGroupId || '').trim()
      if (!targetId) return { olderId: '', newerId: '', lockedReason: '请先选择群组' }
    } else if (activeTargetKind === 'workspace') {
      targetId = String((activeWorkspace as any)?.id || activeWorkspaceId || '').trim()
      if (!targetId) return { olderId: '', newerId: '', lockedReason: '请先选择工作区' }
    } else {
      targetId = String(activeRoleId || '').trim()
      if (!targetId) return { olderId: '', newerId: '', lockedReason: '请先选择角色' }
    }
    if (!data) return { olderId: '', newerId: '', lockedReason: '数据未就绪' }

    const box = activeTargetKind === 'group' ? (data as any)?.chatsByGroup?.[targetId] : activeTargetKind === 'workspace' ? (data as any)?.chatsByWorkspace?.[targetId] : data?.chatsByRole?.[targetId]
    const chats = sortChatListItemsForDisplay(Array.isArray(box?.chatMetas) && box.chatMetas.length ? box.chatMetas : Array.isArray(box?.chats) ? box.chats : [])
    const pendingChat = pendingChatForTarget(s, activeTargetKind, targetId)
    const currentChatId = String(activeChat?.id || box?.activeChatId || String(chats[0]?.id || '') || '')
    return chatNavigationFromOrderedChats({ orderedChats: chats, activeChatId: currentChatId, pendingChat })
  })()

  React.useLayoutEffect(() => {
    if (page !== 'chat') return
    const el = composerRef.current
    if (!el) return

    const measure = () => {
      try {
        setComposerHeight(Math.ceil(el.getBoundingClientRect().height || 0))
      } catch (_) {}
    }

    const raf = requestAnimationFrame(measure)

    if (typeof ResizeObserver === 'undefined') {
      return () => cancelAnimationFrame(raf)
    }

    const ro = new ResizeObserver(() => measure())
    ro.observe(el)
    return () => {
      cancelAnimationFrame(raf)
      ro.disconnect()
    }
  }, [page])

  React.useEffect(() => {
    if (page !== 'chat') return
    const el = chatRootRef.current
    if (!el) return
    const onScroll = () => {
      stickToBottomRef.current = isNearBottom(el)
    }
    onScroll()
    el.addEventListener('scroll', onScroll, { passive: true } as any)
    return () => el.removeEventListener('scroll', onScroll as any)
  }, [page, activeRole?.id, activeChat?.id, activeBranchIdUi, branchDraftKey])

  React.useEffect(() => {
    if (page !== 'chat') return
    const el = chatRootRef.current
    if (!el) return
    if (Date.now() < autoScrollBlockUntilRef.current) return
    if (branchNav.mid) return
    stickToBottomRef.current = true
    requestAnimationFrame(() => {
      try {
        el.scrollTop = el.scrollHeight
      } catch (_) {}
    })
  }, [page, activeRole?.id, activeChat?.id, activeBranchIdUi, branchDraftKey, branchNav.mid])

  React.useEffect(() => {
    if (page !== 'chat') return
    const el = chatRootRef.current
    if (!el) return
    if (Date.now() < autoScrollBlockUntilRef.current) return
    if (!stickToBottomRef.current) return
    requestAnimationFrame(() => {
      try {
        el.scrollTop = el.scrollHeight
      } catch (_) {}
    })
  }, [page, allMessages.length, lastMsgId, lastMsgText, activeBranchIdUi, branchDraftKey])

  React.useEffect(() => {
    if (page !== 'chat') return
    const mid = String(branchNav.mid || '').trim()
    if (!mid) return

    const root = chatRootRef.current
    if (!root) return

    stickToBottomRef.current = false

    let tries = 0
    let canceled = false

    const tick = () => {
      if (canceled) return
      tries++
      const esc = typeof CSS !== 'undefined' && typeof (CSS as any).escape === 'function' ? (CSS as any).escape(mid) : mid.replace(/"/g, '\\"')
      const node = root.querySelector(`[data-mid="${esc}"]`) as HTMLElement | null
      if (node) {
        try {
          const rr = root.getBoundingClientRect()
          const nr = node.getBoundingClientRect()
          const top = nr.top - rr.top + root.scrollTop
          const target = Math.max(0, Math.floor(top - 12))
          root.scrollTop = target
        } catch (_) {}
        setBranchNav({ mid: '', at: 0 })
        return
      }
      if (tries >= 10) {
        setBranchNav({ mid: '', at: 0 })
        return
      }
      requestAnimationFrame(tick)
    }

    requestAnimationFrame(tick)
    return () => {
      canceled = true
    }
  }, [page, activeRole?.id, activeChat?.id, activeBranchIdUi, branchNav.mid, branchNav.at])

  const [sendWarn, setSendWarn] = React.useState<{ open: boolean; items: any[] }>({ open: false, items: [] })
  const closeSendWarn = useEvent(() => setSendWarn({ open: false, items: [] }))
  const beginRunPathFollow = useEvent((parentMid0: string) => {
    const parentMid = String(parentMid0 || '').trim()
    if (!parentMid || !activeChat) return null
    const chatId = String(activeChat?.id || '')
    const nonce = ++sendPathAnchorNonceRef.current
    const existingMessageIds = Array.isArray(chatAllMessagesRaw)
      ? chatAllMessagesRaw.map((message: any) => String(message?.id || '').trim()).filter(Boolean)
      : []
    stickToBottomRef.current = true
    setSendPathAnchor({ ...emptySendPathAnchor(), chatId, branchId: activeBranchIdUi, parentMid, existingMessageIds, nonce })
    setTreeSelectedMid('')

    const onRunState = (run: any) => {
      const runId = String(run?.id || '').trim()
      setSendPathAnchor((current) => {
        if (String(current?.chatId || '') !== chatId) return current
        if (String(current?.parentMid || '') !== parentMid) return current
        if (Number(current?.nonce || 0) !== nonce) return current
        return {
          ...current,
          runId,
          inputMessageId: String(run?.inputMessageId || current.inputMessageId || '').trim(),
          lastMessageId: String(run?.lastMessageId || current.lastMessageId || run?.inputMessageId || current.inputMessageId || '').trim(),
        }
      })
    }

    const clear = () => {
      setSendPathAnchor((current) => {
        if (String(current?.chatId || '') !== chatId) return current
        if (String(current?.parentMid || '') !== parentMid) return current
        if (Number(current?.nonce || 0) !== nonce) return current
        return emptySendPathAnchor()
      })
    }

    return { onRunState, clear }
  })

  const sendFromComposer = useEvent(() => {
    const selectedMid = String(treeSelectedMid || '').trim()
    const branchDraftMid = String((branchDraft as any)?.forkFromMid || '').trim()
    const mid = selectedMid || branchDraftMid
    if (mid && activeChat) {
      const follow = beginRunPathFollow(mid)
      if (!follow) return
      Promise.resolve()
        .then(() => controller.actions.sendFromMid?.(mid, { onRunState: follow.onRunState }))
        .finally(() => follow.clear())
      return
    }
    clearSendPathAnchor()
    setTreeSelectedMid('')
    controller.actions.send()
  })
  const confirmSendWarn = useEvent(() => {
    setSendWarn({ open: false, items: [] })
    sendFromComposer()
  })

  const onSend = useEvent(() => {
    if (draftFilesPending) return controller?.capabilities?.ui?.showToast?.('文件解析中，请稍候…')
    const warns = draftFiles
      .map((f: any) => {
        if (!f || f.pending) return null
        const err = String(f?.error || '').trim()
        if (err) return null
        const rawLen = String(f?.text || '').trim().length
        if (!rawLen) return null
        const pct = clampNum(Math.round(Number(f?.sendPct ?? 100)), 0, 100)
        const sendLen = Math.max(0, Math.ceil((rawLen * pct) / 100))
        if (sendLen <= attachSendLimitChars) return null
        return { id: String(f?.id || ''), name: String(f?.name || '文件'), pct, rawLen, sendLen }
      })
      .filter(Boolean)
    if (warns.length) return setSendWarn({ open: true, items: warns })
    sendFromComposer()
  })
  const onStop = useEvent(() => controller.actions.stop?.(activeStopRunId))
  const closeAttachmentPicker = useEvent(() => setAttachmentPickerEl(null))
  const openAttachmentPicker = useEvent((e: React.MouseEvent<HTMLElement>) => setAttachmentPickerEl(e.currentTarget))
  const onPickDraftImages = useEvent(() => {
    controller.actions.pickDraftImages()
    closeAttachmentPicker()
  })
  const onPickDraftFiles = useEvent(() => {
    draftFilePickerInputRef.current?.click?.()
    closeAttachmentPicker()
  })
  const onPickFilesChanged = useEvent((e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files ? Array.from(e.target.files) : []
    e.target.value = ''
    if (!files.length) return
    controller.actions.addDraftFilesFromFiles?.(files)
  })
  const closeTempModelPicker = useEvent(() => setTempModelPickerEl(null))
  const closeAsyncToolTasks = useEvent(() => setAsyncToolTasksEl(null))
  const openTempModelPicker = useEvent((e: React.MouseEvent<HTMLElement>) => {
    if (!roleSessionControlsEnabled) return
    if (!activeRole) return
    if (!providers.length) return controller?.capabilities?.ui?.showToast?.('暂无供应商', { kind: 'error' })

    const pid0 = effectiveProviderId || String((providers[0] as any)?.id || '')
    const mid0 = effectiveModelId || ''

    const pid = String(pid0 || '').trim()
    const mid = String(mid0 || '').trim()

    setTempModelProviderId(pid)

    const p = providers.find((x: any) => String(x?.id || '') === pid) || null
    const items = registeredModelItems(p)
    const inList = !!mid && items.some((x: any) => x.id === mid)

    setTempModelPick(inList ? mid : '')
    setTempModelPickerEl(e.currentTarget)
  })

  const refreshAsyncToolTasks = useEvent(async () => {
    const request = controller?.capabilities?.net?.request
    if (typeof request !== 'function') return
    const params = new URLSearchParams()
    if (activeTargetKind === 'group') params.set('groupId', String(activeGroupId || ''))
    else if (activeTargetKind === 'workspace') params.set('workspaceId', String(activeWorkspaceId || ''))
    if (activeRole?.id) params.set('roleId', String(activeRole.id || ''))
    if (activeChatId) params.set('sessionId', String(activeChatId || ''))
    setAsyncToolTasksLoading(true)
    try {
      const response = await request({ method: 'GET', path: `/api/async-tool-tasks?${params.toString()}`, timeoutMs: 15000 })
      setAsyncToolTasks(Array.isArray(response?.body) ? response.body : [])
    } catch (err: any) {
      controller?.capabilities?.ui?.showToast?.(String(err?.message || err || '异步任务列表加载失败'), { kind: 'error' })
    } finally {
      setAsyncToolTasksLoading(false)
    }
  })

  const openAsyncToolTasks = useEvent((e: React.MouseEvent<HTMLElement>) => {
    if (!activeRole || !activeChatId) return
    setAsyncToolTasksEl(e.currentTarget)
    setAsyncToolTasks(activeAsyncToolTasks)
    refreshAsyncToolTasks()
  })

  const onTempProviderChanged = useEvent((nextProviderId: string) => {
    const pid = String(nextProviderId || '').trim()
    setTempModelProviderId(pid)
    setTempModelPick('')
  })

  const saveTempModelOverride = useEvent(() => {
    const pid = String(tempModelProviderId || '').trim()
    const mid = String(tempModelPick || '').trim()

    if (!pid) return controller?.capabilities?.ui?.showToast?.('请选择供应商', { kind: 'error' })
    if (!mid) return controller?.capabilities?.ui?.showToast?.('请选择模型', { kind: 'error' })

    controller.actions.setChatModelOverride?.(pid, mid)
    closeTempModelPicker()
  })

  const clearTempModelOverride = useEvent(() => {
    controller.actions.clearChatModelOverride?.()
    closeTempModelPicker()
  })
  const openReasoningPicker = useEvent((e: React.MouseEvent<HTMLElement>) => {
    if (!roleSessionControlsEnabled) return
    if (!reasoningProfile.supportsReasoning) return
    setReasoningPickerEl(e.currentTarget)
  })
  const pickReasoningEffort = useEvent((effort: string) => {
    controller.actions.setChatReasoningEffort?.(effort)
    closeReasoningPicker()
  })
  const clearReasoningEffort = useEvent(() => {
    controller.actions.setChatReasoningEffort?.('')
    closeReasoningPicker()
  })
  const [regen, setRegen] = React.useState<{ mid: string; role: 'assistant' | 'user' }>({ mid: '', role: 'assistant' })
  const [msgMenu, setMsgMenu] = React.useState<{ mid: string; role: 'user' | 'assistant'; x: number; y: number }>({
    mid: '',
    role: 'assistant',
    x: 0,
    y: 0,
  })
  const [treeNodeMenu, setTreeNodeMenu] = React.useState<{ mid: string; role: 'user' | 'assistant'; x: number; y: number }>({
    mid: '',
    role: 'assistant',
    x: 0,
    y: 0,
  })
  const [confirmDelMsg, setConfirmDelMsg] = React.useState<{ mid: string; role: 'user' | 'assistant' }>({ mid: '', role: 'assistant' })
  const [confirmDelTree, setConfirmDelTree] = React.useState<{ mid: string; role: 'user' | 'assistant' }>({
    mid: '',
    role: 'assistant',
  })
  const [editingMsg, setEditingMsg] = React.useState<{ mid: string; text: string }>({ mid: '', text: '' })
  const [chatMenu, setChatMenu] = React.useState<{
    targetKind: 'role' | 'group' | 'workspace'
    targetId: string
    chatId: string
    title: string
    x: number
    y: number
  }>({
    targetKind: 'role',
    targetId: '',
    chatId: '',
    title: '',
    x: 0,
    y: 0,
  })
  const [favoriteChatMenu, setFavoriteChatMenu] = React.useState<{
    folderId: string
    targetKind: 'role' | 'group' | 'workspace'
    targetId: string
    chatId: string
    title: string
    x: number
    y: number
  }>({
    folderId: '',
    targetKind: 'role',
    targetId: '',
    chatId: '',
    title: '',
    x: 0,
    y: 0,
  })
  const [confirmDelChat, setConfirmDelChat] = React.useState<{ targetKind: 'role' | 'group' | 'workspace'; targetId: string; chatId: string }>({
    targetKind: 'role',
    targetId: '',
    chatId: '',
  })
  const [editingChatTitle, setEditingChatTitle] = React.useState<{ targetKind: 'role' | 'group' | 'workspace'; targetId: string; chatId: string; text: string }>({
    targetKind: 'role',
    targetId: '',
    chatId: '',
    text: '',
  })

  const closeEditingChatTitle = useEvent(() => setEditingChatTitle({ targetKind: 'role', targetId: '', chatId: '', text: '' }))
  const saveEditingChatTitle = useEvent(async () => {
    const { targetKind, targetId, chatId, text } = editingChatTitle
    if (!targetId || !chatId || s.loading) return
    const action = targetKind === 'group' ? controller.actions.renameGroupChat : targetKind === 'workspace' ? controller.actions.renameWorkspaceChat : controller.actions.renameChat
    const ok = await Promise.resolve(action?.(targetId, chatId, String(text ?? '')))
    if (ok === true) closeEditingChatTitle()
  })

  React.useEffect(() => {
    setEditingMsg({ mid: '', text: '' })
  }, [page, activeRole?.id, activeChat?.id, activeBranchIdUi, branchDraftKey])

  React.useEffect(() => {
    setChatMenu({ targetKind: 'role', targetId: '', chatId: '', title: '', x: 0, y: 0 })
    setFavoriteChatMenu({ folderId: '', targetKind: 'role', targetId: '', chatId: '', title: '', x: 0, y: 0 })
    setConfirmDelChat({ targetKind: 'role', targetId: '', chatId: '' })
    setEditingChatTitle({ targetKind: 'role', targetId: '', chatId: '', text: '' })
  }, [page, activeRole?.id, (activeGroup as any)?.id, activeTargetKind])

  React.useEffect(() => {
    if (!favoriteDialog.open) return
    setFavoriteCheckedFolderIds((p) => p.filter((id) => favoriteFolders.some((f: any) => String(f?.id || '') === String(id || ''))))
  }, [favoriteDialog.open, favoriteFolders])

  React.useEffect(() => {
    if (!moveFavoriteFolderContents.open) return
    const fid = String(moveFavoriteFolderContents.folderId || '')
    setMoveFavoriteFolderContents((p) => {
      const cur = String(p.targetFolderId || '')
      if (cur && cur !== fid && favoriteFolders.some((f: any) => String(f?.id || '') === cur)) return p
      const fallback = favoriteFolders.find((f: any) => String(f?.id || '') !== fid) || null
      return { ...p, targetFolderId: String((fallback as any)?.id || '') }
    })
  }, [moveFavoriteFolderContents.open, moveFavoriteFolderContents.folderId, favoriteFolders])

  React.useEffect(() => {
    if (!moveFavoriteFolderDialog.open) return
    const fid = String(moveFavoriteFolderDialog.folderId || '')
    const subtree = new Set(collectFavoriteFolderSubtreeIds(fid))
    setMoveFavoriteFolderDialog((p) => {
      const cur = String(p.parentId || '')
      if (!cur) return p
      if (cur !== fid && !subtree.has(cur) && favoriteFolders.some((f: any) => String(f?.id || '') === cur)) return p
      return { ...p, parentId: '' }
    })
  }, [moveFavoriteFolderDialog.open, moveFavoriteFolderDialog.folderId, favoriteFolders, collectFavoriteFolderSubtreeIds])

  const renderFavoriteFolderPicker = React.useCallback(
    (parentId = '', depth = 0): React.ReactNode => {
      const items = favoriteChildrenMap[String(parentId || '')] || []
      return items.map((folder: any) => {
        const fid = String(folder?.id || '')
        const children = favoriteChildrenMap[fid] || []
        const expanded = !!favoriteFolderExpanded[fid]
        return (
          <React.Fragment key={`pick-${fid}`}>
            <ListItemButton sx={{ ...SOFT_POPOVER_ITEM_SX, pl: 1 + depth * 2, pr: 1 }} onClick={() => toggleFavoriteFolderChecked(fid)}>
              <Checkbox
                size="small"
                edge="start"
                checked={favoriteCheckedFolderIds.includes(fid)}
                onClick={(e) => e.stopPropagation()}
                onChange={() => toggleFavoriteFolderChecked(fid)}
              />
              {children.length ? (
                <IconButton
                  size="small"
                  edge="start"
                  onClick={(e) => {
                    e.stopPropagation()
                    toggleFavoriteFolderExpanded(fid)
                  }}
                  sx={{ mr: 0.5 }}
                >
                  {expanded ? <ExpandLessIcon fontSize="small" /> : <ExpandMoreIcon fontSize="small" />}
                </IconButton>
              ) : (
                <Box sx={{ width: 28 }} />
              )}
              <FolderOutlinedIcon sx={{ fontSize: 18, color: 'text.secondary', ml: 0.5, mr: 1 }} />
              <ListItemText primary={String(folder?.name || '未命名文件夹')} />
            </ListItemButton>
            {children.length ? <Collapse in={expanded}>{renderFavoriteFolderPicker(fid, depth + 1)}</Collapse> : null}
          </React.Fragment>
        )
      })
    },
    [favoriteChildrenMap, favoriteFolderExpanded, favoriteCheckedFolderIds, toggleFavoriteFolderExpanded, toggleFavoriteFolderChecked],
  )

  const renderFavoriteFolderSinglePicker = React.useCallback(
    (selectedId: string, onSelect: (folderId: string) => void, options?: { includeRoot?: boolean; filter?: (folder: any) => boolean }, parentId = '', depth = 0): React.ReactNode => {
      const items = favoriteChildrenMap[String(parentId || '')] || []
      const nodes: React.ReactNode[] = []
      if (!parentId && options?.includeRoot) {
        nodes.push(
          <ListItemButton key="pick-root" sx={{ ...SOFT_POPOVER_ITEM_SX, pl: 1, pr: 1 }} selected={!selectedId} onClick={() => onSelect('')}>
            <Box sx={{ width: 20 }} />
            <FolderOutlinedIcon sx={{ fontSize: 18, color: 'text.secondary', ml: 0.5, mr: 1 }} />
            <ListItemText primary="顶层" secondary="移动到最外层" />
          </ListItemButton>,
        )
      }
      for (const folder of items) {
        if (options?.filter && !options.filter(folder)) continue
        const fid = String(folder?.id || '')
        const children = favoriteChildrenMap[fid] || []
        const visibleChildren = options?.filter ? children.filter((child: any) => options.filter?.(child)) : children
        const expanded = !!favoriteFolderExpanded[fid]
        nodes.push(
          <React.Fragment key={`single-pick-${fid}`}>
            <ListItemButton sx={{ ...SOFT_POPOVER_ITEM_SX, pl: 1 + depth * 2, pr: 1 }} selected={selectedId === fid} onClick={() => onSelect(fid)}>
              {visibleChildren.length ? (
                <IconButton
                  size="small"
                  edge="start"
                  onClick={(e) => {
                    e.stopPropagation()
                    toggleFavoriteFolderExpanded(fid)
                  }}
                  sx={{ mr: 0.5 }}
                >
                  {expanded ? <ExpandLessIcon fontSize="small" /> : <ExpandMoreIcon fontSize="small" />}
                </IconButton>
              ) : (
                <Box sx={{ width: 28 }} />
              )}
              <FolderOutlinedIcon sx={{ fontSize: 18, color: 'text.secondary', ml: 0.5, mr: 1 }} />
              <ListItemText primary={String(folder?.name || '未命名文件夹')} />
            </ListItemButton>
            {visibleChildren.length ? <Collapse in={expanded}>{renderFavoriteFolderSinglePicker(selectedId, onSelect, options, fid, depth + 1)}</Collapse> : null}
          </React.Fragment>,
        )
      }
      return nodes
    },
    [favoriteChildrenMap, favoriteFolderExpanded, toggleFavoriteFolderExpanded],
  )

  const renderFavoriteFolderTree = React.useCallback(
    (parentId = '', depth = 0): React.ReactNode => {
      const q = String(favoriteSearchText || '').trim().toLowerCase()
      const itemMatches = (folder: any, refs: any[]) => {
        if (!q) return true
        const folderName = String(folder?.name || '').toLowerCase()
        if (folderName.includes(q)) return true
        for (const ref of refs) {
          const targetKindText = String(ref?.targetKind || '').trim()
          const targetKind = targetKindText === 'group' ? 'group' : targetKindText === 'workspace' ? 'workspace' : 'role'
          const targetId = String(ref?.targetId || '')
          const chatId = String(ref?.chatId || '')
            const chat = getChatByFavoriteRef(targetKind as any, targetId, chatId)
            const targetMeta = getFavoriteTargetMeta(targetKind as any, targetId)
            const hay = [
              String((chat as any)?.title || ''),
              snippetText((chat as any)?.lastMessagePreview || (Array.isArray((chat as any)?.messages) ? (chat as any).messages : []).slice(-1)[0]?.content || ''),
              String(targetMeta?.name || ''),
          ]
            .join('\n')
            .toLowerCase()
          if (hay.includes(q)) return true
        }
        return false
      }
      const renderNode = (folder: any, depth2: number): React.ReactNode => {
        const fid = String(folder?.id || '')
        const children = favoriteChildrenMap[fid] || []
        const refs = Array.isArray((favoriteChatRefsByFolderId as any)?.[fid]) ? (favoriteChatRefsByFolderId as any)[fid] : []
        const visibleChildren = children
          .map((child: any) => renderNode(child, depth2 + 1))
          .filter(isRenderableNode)
        const visibleRefs = refs
          .map((ref: any) => {
            const targetKindText = String(ref?.targetKind || '').trim()
            const targetKind = targetKindText === 'group' ? 'group' : targetKindText === 'workspace' ? 'workspace' : 'role'
            const targetId = String(ref?.targetId || '')
            const chatId = String(ref?.chatId || '')
            const chat = getChatByFavoriteRef(targetKind as any, targetId, chatId)
            if (!chat) return null
            const targetMeta = getFavoriteTargetMeta(targetKind as any, targetId)
            const targetName = String(targetMeta?.name || (targetKind === 'group' ? '群聊' : targetKind === 'workspace' ? '工作区' : '角色'))
            const snippet = snippetText((chat as any)?.lastMessagePreview || (Array.isArray((chat as any)?.messages) ? (chat as any).messages : []).slice(-1)[0]?.content || '')
            const selected = targetKind === activeTargetKind && String(targetId || '') === activeChatTargetId && String(chatId || '') === activeChatId
            const indicatorKind = chatSessionRunIndicatorKind(targetKind as any, targetId, chat, selected)
            if (q) {
              const hay = [String((chat as any)?.title || ''), snippet, targetName].join('\n').toLowerCase()
              if (!hay.includes(q)) return null
            }
            return (
              <ListItemButton
                key={`${fid}:${targetKind}:${targetId}:${chatId}`}
                selected={selected}
                sx={{ ...SOFT_POPOVER_ITEM_TOP_SX, pl: 3 + depth2 * 2, pr: 1, gap: 1 }}
                onClick={() => openFavoritedChat(targetKind as any, targetId, chatId)}
                onContextMenu={(e) => onFavoriteChatContextMenu(e, fid, targetKind as any, targetId, chatId, String((chat as any)?.title || ''))}
              >
                <Stack spacing={0.5} alignItems="center" sx={{ width: 48, flex: '0 0 48px', pt: 0.25 }}>
                  <Avatar src={String(targetMeta?.avatarImage || '') || undefined} sx={{ width: 28, height: 28, fontSize: 14 }}>
                    {String(targetMeta?.avatar || (targetKind === 'group' ? '👥' : targetKind === 'workspace' ? '📁' : '🙂'))}
                  </Avatar>
                  <Typography variant="caption" color="text.secondary" noWrap sx={{ maxWidth: '100%' }}>
                    {targetName}
                  </Typography>
                </Stack>
                <ListItemText sx={{ minWidth: 0, mt: 0.25 }} primary={String((chat as any)?.title || (targetKind === 'group' ? '群聊' : targetKind === 'workspace' ? '工作区会话' : '新聊天'))} secondary={snippet} />
                {indicatorKind ? <ChatSessionRunIndicator kind={indicatorKind} /> : null}
              </ListItemButton>
            )
          })
          .filter(isRenderableNode)
        if (!itemMatches(folder, refs) && !visibleChildren.length && !visibleRefs.length) return null
        const expanded = q ? true : favoriteFolderExpanded[fid] ?? true
        return (
          <React.Fragment key={fid}>
            <ListItemButton
              sx={{ ...SOFT_POPOVER_ITEM_SX, pl: 1 + depth2 * 2, pr: 1 }}
              onClick={() => toggleFavoriteFolderExpanded(fid)}
              onContextMenu={(e) => {
                e.preventDefault()
                e.stopPropagation()
                const folderParentId = String(folder?.parentId || '')
                setFavoriteFolderMenu({ folderId: fid, x: e.clientX, y: e.clientY, parentId: folderParentId })
              }}
            >
              {visibleChildren.length || visibleRefs.length ? expanded ? <ExpandLessIcon fontSize="small" /> : <ExpandMoreIcon fontSize="small" /> : <Box sx={{ width: 20 }} />}
              <FolderOutlinedIcon sx={{ fontSize: 18, color: 'text.secondary', ml: 0.5, mr: 1 }} />
              <ListItemText primary={String(folder?.name || '未命名文件夹')} secondary={visibleRefs.length ? `${visibleRefs.length} 条收藏` : undefined} />
            </ListItemButton>
            <Collapse in={expanded}>
              {visibleChildren}
              {visibleRefs}
            </Collapse>
          </React.Fragment>
        )
      }
      const items = favoriteChildrenMap[String(parentId || '')] || []
      return items.map((folder: any) => renderNode(folder, depth)).filter(isRenderableNode)
    },
    [
      favoriteChildrenMap,
      favoriteChatRefsByFolderId,
      favoriteFolderExpanded,
      favoriteSearchText,
      getChatByFavoriteRef,
      getFavoriteTargetMeta,
      activeTargetKind,
      activeChatTargetId,
      activeChatId,
      chatSessionRunIndicatorKind,
      openFavoritedChat,
      toggleFavoriteFolderExpanded,
    ],
  )

  const closeMsgMenu = useEvent(() => setMsgMenu({ mid: '', role: 'assistant', x: 0, y: 0 }))
  const onMessageContextMenu = useEvent((e: React.MouseEvent, mid: string, role: 'user' | 'assistant') => {
    if (!mid) return
    e.preventDefault()
    e.stopPropagation()
    setMsgMenu({ mid, role, x: e.clientX, y: e.clientY })
  })

  const closeTreeNodeMenu = useEvent(() => setTreeNodeMenu({ mid: '', role: 'assistant', x: 0, y: 0 }))
  const onTreeNodeContextMenu = useEvent((e: any, mid: string, role: 'user' | 'assistant') => {
    const id = String(mid || '').trim()
    if (!id) return
    try {
      e.preventDefault?.()
      e.stopPropagation?.()
    } catch (_) {}
    treeSuppressClickRef.current = true
    setTimeout(() => {
      treeSuppressClickRef.current = false
    }, 0)
    setTreeNodeMenu({ mid: id, role, x: Number(e?.clientX || 0), y: Number(e?.clientY || 0) })
  })

  const closeChatMenu = useEvent(() => setChatMenu({ targetKind: 'role', targetId: '', chatId: '', title: '', x: 0, y: 0 }))
  const closeFavoriteChatMenu = useEvent(() => setFavoriteChatMenu({ folderId: '', targetKind: 'role', targetId: '', chatId: '', title: '', x: 0, y: 0 }))
  const onChatContextMenu = useEvent((e: React.MouseEvent, targetKind: 'role' | 'group' | 'workspace', targetId: string, chatId: string, title: string) => {
    const kind = targetKind === 'group' ? 'group' : targetKind === 'workspace' ? 'workspace' : 'role'
    const tid = String(targetId || '')
    const cid = String(chatId || '')
    if (!tid || !cid) return
    e.preventDefault()
    e.stopPropagation()
    setChatMenu({ targetKind: kind, targetId: tid, chatId: cid, title: String(title ?? ''), x: e.clientX, y: e.clientY })
  })
  const onFavoriteChatContextMenu = useEvent(
    (e: React.MouseEvent, folderId: string, targetKind: 'role' | 'group' | 'workspace', targetId: string, chatId: string, title: string) => {
      const fid = String(folderId || '')
      const kind = targetKind === 'group' ? 'group' : targetKind === 'workspace' ? 'workspace' : 'role'
      const tid = String(targetId || '')
      const cid = String(chatId || '')
      if (!fid || !tid || !cid) return
      e.preventDefault()
      e.stopPropagation()
      setFavoriteChatMenu({ folderId: fid, targetKind: kind, targetId: tid, chatId: cid, title: String(title ?? ''), x: e.clientX, y: e.clientY })
    },
  )

  React.useEffect(() => {
    if (page !== 'chat') return
    const mid = String(editingMsg.mid || '')
    if (!mid) return
    const msgs = Array.isArray(renderChat?.messages) ? renderChat.messages : []
    if (!msgs.some((m: any) => String(m?.id || '') === mid)) setEditingMsg({ mid: '', text: '' })
  }, [page, renderChatId, (renderChat?.messages || []).length, editingMsg.mid])

  const startEditMessage = useEvent((mid: string, text: string) => {
    if (!mid) return
    if (messageMutationBlocked(mid, 'edit')) return
    setEditingMsg({ mid, text: String(text ?? '') })
  })
  const setEditingMsgText = useEvent((text: string) => setEditingMsg((p) => ({ ...p, text: String(text ?? '') })))
  const cancelEditMessage = useEvent(() => setEditingMsg({ mid: '', text: '' }))
  const saveEditMessage = useEvent(async () => {
    const mid = String(editingMsg.mid || '')
    if (!mid) return
    if (messageMutationBlocked(mid, 'edit')) return
    const ok = await Promise.resolve(controller.actions.editMessage?.(mid, String(editingMsg.text ?? '')))
    if (ok === true) setEditingMsg({ mid: '', text: '' })
  })

  const copyMessageText = useEvent((text: unknown) => {
    const writeText = controller.capabilities?.clipboard?.writeText
    if (typeof writeText !== 'function') return controller.capabilities?.ui?.showToast?.('未授权：clipboard.writeText', { kind: 'error' })
    Promise.resolve()
      .then(() => writeText(String(text ?? '')))
      .then(
        () => controller.capabilities?.ui?.showToast?.('已复制', { kind: 'success' }),
        () => controller.capabilities?.ui?.showToast?.('复制失败', { kind: 'error' }),
      )
  })

  const switchBranchSibling = useEvent((mid: string, direction: -1 | 1, nextMid: string) => {
    const id = String(mid || '').trim()
    if (!id) return
    stickToBottomRef.current = false
    autoScrollBlockUntilRef.current = Date.now() + 1200
    clearSendPathAnchor()
    const followMid = String(nextMid || '').trim()
    if (followMid) setBranchNav({ mid: followMid, at: Date.now() })
    controller.actions.switchBranchSibling?.(id, direction)
  })

  const openRegenConfirm = useEvent((mid: string, role: 'assistant' | 'user') => {
    const id = String(mid || '').trim()
    if (!id) return
    setRegen({ mid: id, role: role === 'user' ? 'user' : 'assistant' })
  })

  const regenPathParentMid = useEvent((mid0: string, role0: 'assistant' | 'user') => {
    const mid = String(mid0 || '').trim()
    if (!mid) return ''
    if (role0 === 'user') return mid
    const message = chatAllById.get(mid) || null
    const parentMid = String((message as any)?.parentMid || '').trim()
    const parent = parentMid ? chatAllById.get(parentMid) || null : null
    return parent ? parentMid : ''
  })

  const openDeleteMessageConfirm = useEvent((mid: string, role: 'assistant' | 'user') => {
    const id = String(mid || '').trim()
    if (!id) return
    setConfirmDelMsg({ mid: id, role: role === 'user' ? 'user' : 'assistant' })
  })

  const openRolePicker = useEvent((e: React.MouseEvent<HTMLElement>) => {
    setRolePickerMode('global')
    setRolePickerTab(activeTargetKind === 'group' ? 'groups' : activeTargetKind === 'workspace' ? 'workspaces' : 'roles')
    setRolePickerEl(e.currentTarget)
  })
  const openWorkspaceRolePicker = useEvent((e: React.MouseEvent<HTMLElement>) => {
    setRolePickerMode('workspaceRole')
    setRolePickerTab('roles')
    setRolePickerEl(e.currentTarget)
  })
  const closeRolePicker = useEvent(() => {
    setRolePickerEl(null)
    setRolePickerMode('global')
  })
  const openChatPicker = useEvent((e: React.MouseEvent<HTMLElement>) => {
    setChatPickerView('history')
    setChatPickerEl(e.currentTarget)
  })
  const closeChatPicker = useEvent(() => {
    setChatPickerEl(null)
    setChatPickerView('history')
    closeChatMenu()
    closeFavoriteChatMenu()
    closeFavoriteFolderMenu()
    closeFavoriteDialog()
    closeCreateFavoriteFolder()
    closeRenameFavoriteFolder()
    closeDeleteFavoriteFolderConfirm()
    closeMoveFavoriteFolderContents()
    closeMoveFavoriteFolderDialog()
    closeConfirmClearFavoriteFolder()
    setChatPickerSearchOpen(false)
    setChatPickerSearchText('')
    setFavoriteSearchOpen(false)
    setFavoriteSearchText('')
  })

  const openPluginSettings = useEvent(
    (tab: SettingsTab = 'roles') => {
    setRolePickerEl(null)
    setChatPickerEl(null)
    setChatPickerSearchOpen(false)
    setChatPickerSearchText('')
    setSettingsTab(tab)
    setPage('settings')
  })
  const closePluginSettings = useEvent(() => setPage('chat'))

  React.useEffect(() => {
    if (!chatPickerEl) return
    if (!chatPickerSearchOpen) return
    requestAnimationFrame(() => {
      setTimeout(() => {
        const el = chatPickerSearchInputRef.current
        if (!el) return
        try {
          el.focus?.()
          el.select?.()
        } catch (_) {}
      }, 0)
    })
  }, [chatPickerEl, chatPickerSearchOpen])

  const onPaste = useEvent((e: React.ClipboardEvent) => {
    if (s.loading) return
    const items = e.clipboardData?.items ? Array.from(e.clipboardData.items) : []
    const files: File[] = []
    for (const it of items) {
      if (!it || it.kind !== 'file') continue
      const type = String(it.type || '')
      if (!type.startsWith('image/')) continue
      const f = it.getAsFile?.()
      if (f) files.push(f)
    }
    if (!files.length) return
    e.preventDefault()
    controller.actions.addDraftImagesFromFiles(files)
  })

  const msgMenuMid = String(msgMenu.mid || '')
  const msgMenuMessages = Array.isArray(renderChat?.messages) ? renderChat.messages : []
  const msgMenuIndex = msgMenuMid ? msgMenuMessages.findIndex((m: any) => String(m?.id || '') === msgMenuMid) : -1
  const msgMenuMsg = msgMenuIndex >= 0 ? msgMenuMessages[msgMenuIndex] : null
  const msgMenuText = messageVisibleText(msgMenuMsg)
  const msgMenuIsToolResponse = msgMenuText.startsWith('<<<[TOOL_RESPONSE]>>>')
  const msgMenuCanEdit = !!msgMenuMid && !messageMutationBlocked(msgMenuMid, 'edit')

  let msgMenuRegenMid = msgMenuMid
  let msgMenuRegenRole: 'assistant' | 'user' = msgMenu.role === 'user' ? 'user' : 'assistant'
  let msgMenuRegenBlocked = msgMenu.role === 'assistant' ? messageMutationBlocked(msgMenuRegenMid, 'edit') : false
  if (msgMenu.role === 'user' && msgMenuIndex >= 0) {
    for (let j = msgMenuIndex + 1; j < msgMenuMessages.length; j++) {
      const next = msgMenuMessages[j]
      if (!next) continue
      if (next.role === 'assistant') {
        msgMenuRegenRole = 'assistant'
        msgMenuRegenMid = String(next?.id || '')
        msgMenuRegenBlocked = messageMutationBlocked(msgMenuRegenMid, 'edit')
        break
      }
    }
  }
  const msgMenuCanRegen =
    !!msgMenuRegenMid &&
    !s.loading &&
    !uiBusy &&
    !(msgMenuRegenRole === 'assistant' && msgMenuRegenBlocked)

  const fileAdjustItem = fileAdjust.id ? draftFiles.find((x: any) => String(x?.id || '') === String(fileAdjust.id || '')) : null
  const fileAdjustName = String(fileAdjustItem?.name || '文件')
  const fileAdjustPending = !!fileAdjustItem?.pending
  const fileAdjustError = String(fileAdjustItem?.error || '').trim()
  const fileAdjustRaw = String(fileAdjustItem?.text || '').trim()
  const fileAdjustFullLen = fileAdjustRaw.length
  const fileAdjustPct = clampNum(Math.round(Number(fileAdjustItem?.sendPct ?? 100)), 0, 100)
  const fileAdjustSendLen = Math.max(0, Math.ceil((fileAdjustFullLen * fileAdjustPct) / 100))
  const fileAdjustTooLong = !fileAdjustPending && !fileAdjustError && fileAdjustFullLen > 0 && fileAdjustSendLen > attachSendLimitChars

  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <GlobalStyles styles={createChatGlobalStyles({ colorThemePreset, transparentChatBg, bgAlpha, chatBgBlur })} />

      <Box sx={{ height: '100%', minWidth: 0, overflow: 'hidden', display: 'flex', flexDirection: 'column', position: 'relative', color: 'var(--studio-text-primary)', background: 'var(--studio-app-background)' }}>
        <AppBar
          position="absolute"
          elevation={0}
          sx={{
            bgcolor: colorMixVar('--studio-topbar', topbarOpacity),
            color: 'text.primary',
            borderBottom: 'none',
            backdropFilter: topbarBlur > 0 ? `blur(${topbarBlur}px)` : 'none',
            WebkitBackdropFilter: topbarBlur > 0 ? `blur(${topbarBlur}px)` : 'none',
            top: 0,
            left: 0,
            right: 0,
          }}
        >
          <Toolbar
            variant="dense"
            sx={{
              gap: 0.5,
              minHeight: 40,
              px: 1,
              '&.MuiToolbar-root': { minHeight: 40 },
            }}
            onPointerDown={onTopbarPointerDown}
          >
            {page === 'settings' ? (
              <>
                <IconButton onClick={closePluginSettings} size="small">
                  <ChevronLeftIcon fontSize="small" />
                </IconButton>

                <Typography variant="subtitle2" sx={{ fontWeight: 900, mr: 0.5 }}>
                  设置
                </Typography>

                <Box sx={{ flex: 1, minWidth: 8 }} />
                {standaloneWindowControls}
              </>
            ) : (
              <>
                <Button
                  variant="text"
                  size="small"
                  onClick={openRolePicker}
                  disabled={s.loading || (!roles.length && !groups.length && !workspaces.length)}
                  sx={{ borderRadius: 999, px: 1, py: 0.25, minWidth: 0, gap: 0.75, borderColor: 'divider' }}
                >
                  <Avatar
                    src={
                      activeTargetKind === 'group'
                        ? String((activeGroup as any)?.avatarImage || '') || undefined
                        : activeTargetKind === 'workspace'
                          ? undefined
                        : String(activeRole?.avatarImage || '') || undefined
                    }
                    sx={{ width: 22, height: 22, fontSize: 12 }}
                  >
                    {activeTargetKind === 'group' ? String((activeGroup as any)?.avatar || '👥') : activeTargetKind === 'workspace' ? '📁' : String(activeRole?.avatar || '🙂')}
                  </Avatar>
                  <Typography variant="body2" sx={{ fontWeight: 900, maxWidth: 180 }} noWrap>
                    {activeTargetKind === 'group'
                      ? activeGroup
                        ? String((activeGroup as any)?.name || '')
                        : '请选择群组'
                      : activeTargetKind === 'workspace'
                        ? activeWorkspace
                          ? String((activeWorkspace as any)?.name || '')
                          : '请选择工作区'
                      : activeRole
                        ? String(activeRole?.name || '')
                        : '请选择角色'}
                  </Typography>
                </Button>

                {activeTargetKind === 'workspace' ? (
                  <Button
                    variant="text"
                    size="small"
                    onClick={openWorkspaceRolePicker}
                    disabled={s.loading || !roles.length}
                    sx={{ borderRadius: 999, px: 1, py: 0.25, minWidth: 0, gap: 0.75, borderColor: 'divider' }}
                  >
                    <Avatar src={String(activeRole?.avatarImage || '') || undefined} sx={{ width: 22, height: 22, fontSize: 12 }}>
                      {String(activeRole?.avatar || '🙂')}
                    </Avatar>
                    <Typography variant="body2" sx={{ fontWeight: 900, maxWidth: 160 }} noWrap>
                      {activeRole ? String(activeRole?.name || '') : '请选择角色'}
                    </Typography>
                  </Button>
                ) : null}

                <Box sx={{ flex: 1 }} />

                <Tooltip title={chatNav.lockedReason || (chatNav.olderId ? '切换到较旧会话' : '没有更旧的会话')}>
                  <span>
                    <IconButton
                      onClick={() => chatSwitch.requestSwitch(chatNav.olderId)}
                      size="small"
                      disabled={!!chatNav.lockedReason || !chatNav.olderId}
                      aria-label="切换到较旧会话"
                    >
                      <ChevronLeftIcon fontSize="small" />
                    </IconButton>
                  </span>
                </Tooltip>
                <Tooltip title={chatNav.lockedReason || (chatNav.newerId ? '切换到较新会话' : '没有更新的会话')}>
                  <span>
                    <IconButton
                      onClick={() => chatSwitch.requestSwitch(chatNav.newerId)}
                      size="small"
                      disabled={!!chatNav.lockedReason || !chatNav.newerId}
                      aria-label="切换到较新会话"
                    >
                      <ChevronRightIcon fontSize="small" />
                    </IconButton>
                  </span>
                </Tooltip>
                <Tooltip title="聊天记录">
                  <IconButton onClick={openChatPicker} size="small">
                    <HistoryIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
                <Tooltip title="新建聊天">
                  <span>
                    <IconButton
                      onClick={() => controller.actions.createChat()}
                      size="small"
                      disabled={activeTargetKind === 'group' ? !activeGroup : activeTargetKind === 'workspace' ? !activeWorkspace || !activeRole : !activeRole}
                      aria-label="新建聊天"
                    >
                      <AddIcon fontSize="small" />
                    </IconButton>
                  </span>
                </Tooltip>
                <Tooltip title={treeOpen ? '收起分支树' : '展开分支树'}>
                  <IconButton onClick={() => setTreeOpen((v) => !v)} size="small" aria-label={treeOpen ? '收起分支树' : '展开分支树'}>
                    <AccountTreeIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
                <Tooltip title="设置">
                  <IconButton onClick={() => openPluginSettings(activeTargetKind === 'workspace' ? 'workspaces' : activeTargetKind === 'group' ? 'groups' : 'roles')} size="small">
                    <SettingsIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
                {standaloneWindowControls}
              </>
            )}
          </Toolbar>
        </AppBar>

        <Box sx={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
        {page === 'chat' ? (
          <>
            <Box
              ref={chatPaneRef}
              sx={{
                flex: 1,
                minWidth: 0,
                minHeight: 0,
                display: 'flex',
                flexDirection: 'column',
                position: 'relative',
                bgcolor: transparentChatBg ? 'transparent' : 'var(--studio-canvas)',
              }}
            >
             <CustomScrollArea
                ref={chatRootRef}
                onClick={onClickOpenImageViewer}
                hostSx={{
                  flex: 1,
                  minHeight: 0,
                }}
                scrollSx={{
                  height: '100%',
                  overflowX: 'hidden',
                  pl: 2,
                  pr: treeOpen && effectiveTreeView === 'right' ? `calc(16px + ${Math.round(treePanelW)}px)` : 2,
                  pt: `calc(${TOPBAR_H}px + 16px)`,
                  bgcolor: transparentChatBg ? 'transparent' : 'var(--studio-paper-muted)',
                  paddingBottom: `calc(${Math.max(0, composerHeight)}px + 24px)`,
               }}
              >
                {s.loading ? (
                  <Typography variant="body2" color="text.secondary">
                    加载中…
                  </Typography>
                ) : activeTargetKind === 'group' && !activeGroup ? (
                  <Typography variant="body2" color="text.secondary">
                    请选择群组
                  </Typography>
                ) : activeTargetKind === 'workspace' && !activeWorkspace ? (
                  <Typography variant="body2" color="text.secondary">
                    请选择工作区
                  </Typography>
                ) : activeTargetKind === 'workspace' && !activeRole ? (
                  <Typography variant="body2" color="text.secondary">
                    请选择角色
                  </Typography>
                ) : activeTargetKind !== 'group' && !activeRole ? (
                  <Typography variant="body2" color="text.secondary">
                    请选择角色
                  </Typography>
                ) : chatSwitch.switching ? (
                  <Box role="status" aria-live="polite" sx={{ display: 'inline-flex', alignItems: 'center', gap: 1, color: 'text.secondary' }}>
                    <CircularProgress size={18} thickness={4.4} color="inherit" />
                    <Typography variant="body2" color="text.secondary">
                      正在切换会话…
                    </Typography>
                  </Box>
                ) : !renderChat ? (
                  openingChatMeta ? (
                    <Box role="status" aria-live="polite" sx={{ display: 'inline-flex', alignItems: 'center', gap: 1, color: 'text.secondary' }}>
                      <CircularProgress size={18} thickness={4.4} color="inherit" />
                      <Typography variant="body2" color="text.secondary">
                        正在打开「{String((openingChatMeta as any)?.title || '会话')}」…
                      </Typography>
                    </Box>
                  ) : (
                    <Typography variant="body2" color="text.secondary">
                      还没有消息。输入内容并发送。
                    </Typography>
                  )
              ) : !Array.isArray(renderChat.messages) || renderChat.messages.length === 0 ? (
                <Typography variant="body2" color="text.secondary">
                  还没有消息。输入内容并发送。
                </Typography>
              ) : (
                <ChatMessageList
                  controller={controller}
                  messages={displayRenderMessages}
                  roles={roles}
                  activeRole={activeRole}
                  activeTargetKind={activeTargetKind}
                  activeVisibleRunCards={activeVisibleRunCards}
                  groupedAttMsgsByRootMid={groupedAttMsgsByRootMid}
                  prevAiMidByAssistantId={prevAiMidByAssistantId}
                  assistantSiblingsByPrevAiMid={assistantSiblingsByPrevAiMid}
                  chatAllMessagesRaw={chatAllMessagesRaw}
                  expandedToolMsgIds={expandedToolMsgIds}
                  expandedUserMsgIds={expandedUserMsgIds}
                  editingMsg={editingMsg}
                  loading={s.loading}
                  uiBusy={uiBusy}
                  userMessageCollapseEnabled={userMessageCollapseEnabled}
                  userMessageCollapseLines={userMessageCollapseLines}
                  stickersEnabled={stickersEnabled}
                  stickerMap={stickerMap}
                  renderSafetyPolicyKey={renderSafetyPolicy}
                  chatRootRef={chatRootRef}
                  formatModelRefText={formatModelRefText}
                  messageMutationBlocked={messageMutationBlocked}
                  onMessageContextMenu={onMessageContextMenu}
                  onToggleToolMessage={toggleExpandedToolMsg}
                  onToggleUserMessage={toggleExpandedUserMsg}
                  onEditTextChange={setEditingMsgText}
                  onCancelEditMessage={cancelEditMessage}
                  onSaveEditMessage={saveEditMessage}
                  onStartEditMessage={startEditMessage}
                  onCopyMessageText={copyMessageText}
                  onOpenAttachView={openAttachView}
                  onSwitchBranchSibling={switchBranchSibling}
                  onRegenerate={openRegenConfirm}
                  onDeleteMessage={openDeleteMessageConfirm}
                />
              )}
             </CustomScrollArea>

             <Popover
               open={!!attachView.el && !!attachViewItem}
               anchorEl={attachView.el}
               onClose={closeAttachView}
               anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
               transformOrigin={{ vertical: 'top', horizontal: 'left' }}
             >
               <Box sx={{ width: 520, maxWidth: '84vw', p: 1.25 }}>
                 <Stack direction="row" spacing={1} alignItems="center" justifyContent="space-between" sx={{ mb: 0.75 }}>
                   <Typography sx={{ fontWeight: 900 }}>
                     {String(attachViewItem?.attachment?.name || '附件')}
                   </Typography>
                   <Stack direction="row" spacing={0.5}>
                     <Tooltip title="复制文本">
                       <IconButton
                         size="small"
                         aria-label="复制附件文本"
                         onClick={() => {
                           const text = String(attachViewItem?.attachment?.text || '')
                            const writeText = controller.capabilities?.clipboard?.writeText
                            if (typeof writeText !== 'function') return controller.capabilities?.ui?.showToast?.('未授权：clipboard.writeText', { kind: 'error' })
                            Promise.resolve()
                              .then(() => writeText(text))
                              .then(() => controller.capabilities?.ui?.showToast?.('已复制', { kind: 'success' }))
                              .catch(() => controller.capabilities?.ui?.showToast?.('复制失败', { kind: 'error' }))
                         }}
                       >
                         <ContentCopyIcon fontSize="inherit" />
                       </IconButton>
                     </Tooltip>
                     <Tooltip title="关闭">
                       <IconButton size="small" aria-label="关闭附件预览" onClick={closeAttachView}>
                         <CloseIcon fontSize="inherit" />
                       </IconButton>
                     </Tooltip>
                   </Stack>
                 </Stack>

                 <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.75 }}>
                   将发送：{Math.round(Number(attachViewItem?.attachment?.sendLen ?? 0))}/{Math.round(Number(attachViewItem?.attachment?.fullLen ?? 0))}（
                   {clampNum(Math.round(Number(attachViewItem?.attachment?.sendPct ?? 100)), 0, 100)}%）
                 </Typography>

                 <TextField
                   fullWidth
                   multiline
                   minRows={8}
                   maxRows={20}
                   size="small"
                   value={String(attachViewItem?.attachment?.text || '')}
                   inputProps={{ readOnly: true, style: { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace' } }}
                 />
               </Box>
             </Popover>

              <Popover
                open={!!msgMenu.mid}
                onClose={closeMsgMenu}
                anchorReference="anchorPosition"
                anchorPosition={msgMenu.mid ? { top: msgMenu.y, left: msgMenu.x } : undefined}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
              >
                <Box sx={{ minWidth: 160, p: 0.5 }}>
                  {msgMenuIsToolResponse ? (
                    <>
                      <MenuItem
                        disabled={!msgMenuMid}
                        onClick={() => {
                          const text = msgMenuText
                          closeMsgMenu()
                          copyMessageText(text)
                        }}
                        sx={{ gap: 1 }}
                      >
                        <ContentCopyIcon fontSize="small" />
                        复制
                      </MenuItem>

                      <MenuItem
                        disabled={!msgMenuMid}
                        onClick={() => {
                          const mid = msgMenuMid
                          closeMsgMenu()
                          if (!mid) return
                          toggleExpandedToolMsg(mid)
                        }}
                        sx={{ gap: 1 }}
                      >
                        {msgMenuMid && expandedToolMsgIds.has(msgMenuMid) ? <ExpandLessIcon fontSize="small" /> : <ExpandMoreIcon fontSize="small" />}
                        {msgMenuMid && expandedToolMsgIds.has(msgMenuMid) ? '收起' : '展开'}
                      </MenuItem>

                      <MenuItem
                        disabled={!msgMenuCanEdit}
                        onClick={() => {
                          const mid = msgMenuMid
                          const text = msgMenuText
                          closeMsgMenu()
                          startEditMessage(mid, text)
                        }}
                        sx={{ gap: 1 }}
                      >
                        <EditOutlinedIcon fontSize="small" />
                        编辑
                      </MenuItem>

                      <MenuItem
                        disabled={!msgMenuMid || messageMutationBlocked(msgMenuMid, 'delete')}
                        onClick={() => {
                          const mid = msgMenuMid
                          const role = msgMenu.role
                          closeMsgMenu()
                          setConfirmDelMsg({ mid, role })
                        }}
                        sx={{ gap: 1 }}
                      >
                        <DeleteOutlineIcon fontSize="small" />
                        删除
                      </MenuItem>
                    </>
                  ) : (
                    <>
                      <MenuItem
                        disabled={!msgMenuMid || msgMenu.role !== 'assistant' || messageMutationBlocked(msgMenuMid, 'edit') || s.loading || uiBusy}
                        onClick={() => {
                          const mid = msgMenuMid
                          closeMsgMenu()
                          if (!mid) return
                          controller.actions.createBranchFromAssistant?.(mid)
                        }}
                        sx={{ gap: 1 }}
                      >
                        <AddIcon fontSize="small" />
                        新建分支
                      </MenuItem>

                      <MenuItem
                        disabled={!msgMenuCanRegen}
                        onClick={() => {
                          const mid = msgMenuRegenMid
                          const role = msgMenuRegenRole
                          closeMsgMenu()
                          if (!mid) return
                          setRegen({ mid, role })
                        }}
                        sx={{ gap: 1 }}
                      >
                        <RestartAltIcon fontSize="small" />
                        重新回复
                      </MenuItem>

                      <MenuItem
                        disabled={!msgMenuCanEdit}
                        onClick={() => {
                          const mid = msgMenuMid
                          const text = msgMenuText
                          closeMsgMenu()
                          startEditMessage(mid, text)
                        }}
                        sx={{ gap: 1 }}
                      >
                        <EditOutlinedIcon fontSize="small" />
                        编辑
                      </MenuItem>

                      <MenuItem
                        disabled={!msgMenuMid}
                        onClick={() => {
                          const text = msgMenuText
                          closeMsgMenu()
                          copyMessageText(text)
                        }}
                        sx={{ gap: 1 }}
                      >
                        <ContentCopyIcon fontSize="small" />
                        复制
                      </MenuItem>

                      <MenuItem
                        disabled={!msgMenuMid || messageMutationBlocked(msgMenuMid, 'delete')}
                        onClick={() => {
                          const mid = msgMenuMid
                          const role = msgMenu.role
                          closeMsgMenu()
                          setConfirmDelMsg({ mid, role })
                        }}
                        sx={{ gap: 1 }}
                      >
                        <DeleteOutlineIcon fontSize="small" />
                        删除
                      </MenuItem>
                    </>
                  )}
                </Box>
              </Popover>

              <Popover
                open={!!treeNodeMenu.mid}
                onClose={closeTreeNodeMenu}
                anchorReference="anchorPosition"
                anchorPosition={treeNodeMenu.mid ? { top: treeNodeMenu.y, left: treeNodeMenu.x } : undefined}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
              >
                <Box sx={{ minWidth: 220, p: 0.5 }}>
                  <MenuItem
                    disabled={!treeNodeMenu.mid || messageMutationBlocked(treeNodeMenu.mid, 'delete')}
                    onClick={() => {
                      const mid = String(treeNodeMenu.mid || '').trim()
                      const role = treeNodeMenu.role
                      closeTreeNodeMenu()
                      if (!mid) return
                      setConfirmDelMsg({ mid, role })
                    }}
                    sx={{ gap: 1 }}
                  >
                    <DeleteOutlineIcon fontSize="small" />
                    仅删除当前节点
                  </MenuItem>

                  <MenuItem
                    disabled={!treeNodeMenu.mid || messageMutationBlocked(treeNodeMenu.mid, 'delete-subtree')}
                    onClick={() => {
                      const mid = String(treeNodeMenu.mid || '').trim()
                      const role = treeNodeMenu.role
                      closeTreeNodeMenu()
                      if (!mid) return
                      setConfirmDelTree({ mid, role })
                    }}
                    sx={{ gap: 1 }}
                  >
                    <DeleteOutlineIcon fontSize="small" />
                    删除节点及子节点
                  </MenuItem>
                </Box>
              </Popover>

             <Dialog
               open={!!confirmDelMsg.mid}
               onClose={() => setConfirmDelMsg({ mid: '', role: 'assistant' })}
               maxWidth="xs"
               fullWidth
             >
               <DialogTitle>确认删除这条消息？</DialogTitle>
               <DialogContent>
                 <Typography variant="body2" color="text.secondary">
                   仅删除当前这条{confirmDelMsg.role === 'assistant' ? ' AI 回复' : '用户消息'}，不影响其他记录。
                 </Typography>
               </DialogContent>
               <DialogActions>
                 <Button onClick={() => setConfirmDelMsg({ mid: '', role: 'assistant' })}>取消</Button>
                 <Button
                   variant="contained"
                   color="error"
                    onClick={async () => {
                      const mid = confirmDelMsg.mid
                      const ok = await Promise.resolve(controller.actions.deleteMessage?.(mid))
                      if (ok === true) setConfirmDelMsg({ mid: '', role: 'assistant' })
                    }}
                    disabled={!confirmDelMsg.mid || messageMutationBlocked(confirmDelMsg.mid, 'delete')}
                 >
                   删除
                 </Button>
               </DialogActions>
             </Dialog>

              <Dialog
                open={!!confirmDelTree.mid}
                onClose={() => setConfirmDelTree({ mid: '', role: 'assistant' })}
                maxWidth="xs"
                fullWidth
              >
                <DialogTitle>确认删除该节点及其子节点？</DialogTitle>
                <DialogContent>
                  <Typography variant="body2" color="text.secondary">
                    将删除该{confirmDelTree.role === 'assistant' ? ' AI 回复' : '用户消息'}节点，以及它后面所有分支上的子节点。
                  </Typography>
                </DialogContent>
                <DialogActions>
                  <Button onClick={() => setConfirmDelTree({ mid: '', role: 'assistant' })}>取消</Button>
                  <Button
                    variant="contained"
                    color="error"
                    onClick={async () => {
                      const mid = confirmDelTree.mid
                      const ok = await Promise.resolve(controller.actions.deleteMessageSubtree?.(mid))
                      if (ok === true) setConfirmDelTree({ mid: '', role: 'assistant' })
                    }}
                    disabled={!confirmDelTree.mid || messageMutationBlocked(confirmDelTree.mid, 'delete-subtree')}
                  >
                    删除
                  </Button>
                </DialogActions>
              </Dialog>

              <Dialog
                open={!!regen.mid}
                onClose={() => setRegen({ mid: '', role: 'assistant' })}
                maxWidth="xs"
                fullWidth
              >
                <DialogTitle>确认重新回复？</DialogTitle>
                <DialogContent>
                  <Typography variant="body2" color="text.secondary">
                    {regen.role === 'assistant' ? '这会基于该回复之前的上下文生成一个新的 AI 回复版本。' : '这会基于该用户消息生成一条新的 AI 回复。'}
                  </Typography>
                </DialogContent>
                <DialogActions>
                  <Button onClick={() => setRegen({ mid: '', role: 'assistant' })}>取消</Button>
                  <Button
                    variant="contained"
                    color="warning"
                    onClick={() => {
                      const mid = regen.mid
                      const role = regen.role
                      setRegen({ mid: '', role: 'assistant' })
                      const parentMid = regenPathParentMid(mid, role)
                      const follow = parentMid ? beginRunPathFollow(parentMid) : null
                      Promise.resolve()
                        .then(() => {
                          if (role === 'assistant') return controller.actions.regenerateAssistant?.(mid, follow ? { onRunState: follow.onRunState } : undefined)
                          return controller.actions.replyFromUserMessage?.(mid, follow ? { onRunState: follow.onRunState } : undefined)
                        })
                        .finally(() => follow?.clear())
                    }}
                   disabled={!regen.mid || s.loading || uiBusy}
                  >
                    重新回复
                  </Button>
                </DialogActions>
              </Dialog>

              <Dialog open={sendWarn.open} onClose={closeSendWarn} maxWidth="xs" fullWidth>
                <DialogTitle>附件超长提醒</DialogTitle>
                <DialogContent>
                  <Stack spacing={1.25} sx={{ pt: 0.5 }}>
                    <Typography variant="body2" color="text.secondary">
                      以下附件当前“实际发送长度”超过阈值（{Math.round(attachSendLimitChars)} 字符）。仍然发送可能导致响应慢或消耗更多 token。
                    </Typography>
                    <Stack spacing={0.75}>
                      {sendWarn.items.map((it: any, i: number) => (
                        <Paper key={String(it?.id || i)} variant="outlined" sx={{ p: 1 }}>
                          <Typography sx={{ fontWeight: 900 }} noWrap>
                            {String(it?.name || '文件')}
                          </Typography>
                          <Typography variant="caption" color="text.secondary">
                            当前发送 {Math.round(Number(it?.pct ?? 100))}%：{Number(it?.sendLen || 0)}/{Number(it?.rawLen || 0)} 字符
                          </Typography>
                        </Paper>
                      ))}
                    </Stack>
                  </Stack>
                </DialogContent>
                <DialogActions>
                  <Button onClick={closeSendWarn}>取消</Button>
                  <Button variant="contained" color="warning" onClick={confirmSendWarn}>
                    仍然发送
                  </Button>
                </DialogActions>
              </Dialog>

              <Box
                sx={{
                  position: 'absolute',
                  top: TOPBAR_H,
                  right: 0,
                  bottom: 0,
                  width: treeOpen && effectiveTreeView === 'right' ? Math.round(treePanelW) : 0,
                  transition: treeResizing ? 'none' : 'width 180ms ease',
                  overflow: 'hidden',
                  pointerEvents: treeOpen && effectiveTreeView === 'right' ? 'auto' : 'none',
                  borderLeft: treeOpen && effectiveTreeView === 'right' ? '1px solid rgba(0,0,0,.10)' : '1px solid transparent',
                  bgcolor: transparentChatBg ? colorMixVar('--studio-paper', Math.max(72, bgAlpha * 100)) : 'var(--studio-paper)',
                  zIndex: 1000,
                }}
              >
                {treeOpen && effectiveTreeView === 'right' ? (
                  <>
                  <Box
                    onPointerDown={onTreeSplitterPointerDown}
                    onPointerMove={onTreeSplitterPointerMove}
                    onPointerUp={endTreeResize}
                    onPointerCancel={endTreeResize}
                    sx={{
                      position: 'absolute',
                      left: -4,
                      top: 0,
                      bottom: 0,
                      width: 8,
                      zIndex: 4,
                      cursor: 'col-resize',
                      touchAction: 'none',
                      userSelect: 'none',
                      WebkitUserSelect: 'none',
                      display: 'flex',
                      alignItems: 'stretch',
                      justifyContent: 'center',
                      '& .fw-split-line': {
                        opacity: treeResizing ? 1 : 0,
                        bgcolor: treeResizing ? 'rgba(25,118,210,.55)' : 'rgba(0,0,0,.18)',
                        transition: 'opacity 120ms ease, background-color 120ms ease',
                      },
                      '&:hover .fw-split-line': { opacity: 1, bgcolor: 'rgba(25,118,210,.55)' },
                    }}
                  >
                    <Box className="fw-split-line" sx={{ width: 1, bgcolor: 'rgba(0,0,0,.18)' }} />
                  </Box>
                <Box sx={{ position: 'relative', width: '100%', height: '100%', userSelect: 'none', WebkitUserSelect: 'none' }}>
                  <Tooltip title="重置视图">
                    <span>
                      <IconButton
                        size="small"
                        onClick={(e) => {
                          setTreePan({ x: 18, y: 18 })
                          setTreeScale(1)
                          treeViewRef.current = { x: 18, y: 18, scale: 1 }
                          scheduleTreeViewTransform()
                          try {
                            ;(e.currentTarget as any)?.blur?.()
                          } catch (_) {}
                        }}
                        disabled={!treeOpen}
                        sx={{
                          position: 'absolute',
                          top: 10,
                          right: 10,
                          zIndex: 2,
                          bgcolor: 'rgba(255,255,255,.72)',
                          border: '1px solid rgba(0,0,0,.12)',
                          backdropFilter: 'blur(8px)',
                          WebkitBackdropFilter: 'blur(8px)',
                          '&:hover': { bgcolor: 'rgba(255,255,255,.82)' },
                        }}
                      >
                        <RestartAltIcon fontSize="inherit" />
                      </IconButton>
                    </span>
                  </Tooltip>

                  <Tooltip
                    title={
                      treeDir === 'lr'
                        ? '切换方向（当前：左→右）'
                        : treeDir === 'rl'
                          ? '切换方向（当前：右→左）'
                          : treeDir === 'tb'
                            ? '切换方向（当前：上→下）'
                            : '切换方向（当前：下→上）'
                    }
                  >
                      <span>
                        <IconButton
                          size="small"
                          onClick={(e) => {
                            cycleTreeDir()
                            try {
                              ;(e.currentTarget as any)?.blur?.()
                            } catch (_) {}
                          }}
                          disabled={!treeOpen}
                          sx={{
                            position: 'absolute',
                            top: 10,
                          right: 52,
                          zIndex: 2,
                          bgcolor: 'rgba(255,255,255,.72)',
                          border: '1px solid rgba(0,0,0,.12)',
                          backdropFilter: 'blur(8px)',
                          WebkitBackdropFilter: 'blur(8px)',
                          '&:hover': { bgcolor: 'rgba(255,255,255,.82)' },
                        }}
                      >
                        <AutorenewIcon fontSize="inherit" />
                      </IconButton>
                    </span>
                  </Tooltip>

                <Box
                    onPointerDown={onTreePointerDown}
                    onPointerMove={onTreePointerMove}
                    onPointerUp={endTreeDrag}
                    onPointerCancel={endTreeDrag}
                    onWheel={onTreeWheel}
                    ref={(el: any) => {
                      treeHostRightRef.current = el
                    }}
                    sx={{
                      position: 'absolute',
                      inset: 0,
                      bgcolor: 'rgba(0,0,0,.03)',
                      overflow: 'hidden',
                      cursor: treeDragging ? 'grabbing' : 'grab',
                      touchAction: 'none',
                      userSelect: 'none',
                      WebkitUserSelect: 'none',
                    }}
                  >
                    {treeRender && Array.isArray((treeRender as any).nodes) && (treeRender as any).nodes.length ? (
                      <svg width="100%" height="100%" style={{ display: 'block' }}>
                        <g
                          ref={(el) => {
                            treeViewportRef.current = el
                            if (el) requestAnimationFrame(() => applyTreeViewTransform())
                          }}
                          transform="translate(0,0) scale(1)"
                        >
                          {Array.isArray((treeRender as any).edges)
                            ? ((treeRender as any).edges as any[]).map((e: any) => {
                                const from = String(e?.from || '').trim()
                                const to = String(e?.to || '').trim()
                                if (!from || !to) return null
                                const a = (treeRender as any)?.byId?.get(from) || null
                                const b = (treeRender as any)?.byId?.get(to) || null
                                if (!a || !b) return null
                                const w = Number((treeRender as any).nodeW || 168)
                                const h = Number((treeRender as any).nodeH || 44)
                                const strokeW = 6
                                const capR = strokeW / 2
                                const aiGap = 2
                                const selectedMid = String(treeFocusMid || '')

                                const ax = Number(a.x || 0)
                                const ay = Number(a.y || 0)
                                const bx = Number(b.x || 0)
                                const by = Number(b.y || 0)
                                const aIsAi = String(a?.role || '') === 'assistant'
                                const bIsAi = String(b?.role || '') === 'assistant'
                                const aDotR = aIsAi && from === selectedMid ? 10 : 8
                                const bDotR = bIsAi && to === selectedMid ? 10 : 8
                                const aOff = aIsAi ? aDotR + capR + aiGap : 0
                                const bOff = bIsAi ? bDotR + capR + aiGap : 0

                                const horizontal = treeDir === 'lr' || treeDir === 'rl'
                                let sx = 0
                                let sy = 0
                                let tx = 0
                                let ty = 0
                                let d = ''

                                if (horizontal) {
                                  const forward = bx >= ax
                                  const acx = ax + w / 2
                                  const acy = ay + h / 2
                                  const bcx = bx + w / 2
                                  const bcy = by + h / 2

                                  sx = aIsAi ? acx + (forward ? aOff : -aOff) : ax + (forward ? w : 0)
                                  sy = aIsAi ? acy : ay + h / 2
                                  tx = bIsAi ? bcx + (forward ? -bOff : bOff) : bx + (forward ? 0 : w)
                                  ty = bIsAi ? bcy : by + h / 2
                                  const dd = Math.max(42, Math.abs(tx - sx) * 0.5)
                                  d = `M ${sx} ${sy} C ${sx + (forward ? dd : -dd)} ${sy} ${tx - (forward ? dd : -dd)} ${ty} ${tx} ${ty}`
                                } else {
                                  const forward = by >= ay
                                  const acx = ax + w / 2
                                  const acy = ay + h / 2
                                  const bcx = bx + w / 2
                                  const bcy = by + h / 2

                                  sx = aIsAi ? acx : ax + w / 2
                                  sy = aIsAi ? acy + (forward ? aOff : -aOff) : ay + (forward ? h : 0)
                                  tx = bIsAi ? bcx : bx + w / 2
                                  ty = bIsAi ? bcy + (forward ? -bOff : bOff) : by + (forward ? 0 : h)
                                  const dd = Math.max(42, Math.abs(ty - sy) * 0.5)
                                  d = `M ${sx} ${sy} C ${sx} ${sy + (forward ? dd : -dd)} ${tx} ${ty - (forward ? dd : -dd)} ${tx} ${ty}`
                                }
                                const key = `${from}->${to}`
                                const hi = treeHighlightEdgeKeys.has(key)
                                return (
                                  <path
                                    key={key}
                                    d={d}
                                    fill="none"
                                    stroke={hi ? 'rgba(34,197,94,.85)' : 'rgba(0,0,0,.16)'}
                                    strokeWidth={strokeW}
                                    strokeLinecap="round"
                                    strokeLinejoin="round"
                                  />
                                )
                              })
                            : null}

                          {((treeRender as any).nodes as any[]).map((n: any) => {
                            const id = String(n?.id || '').trim()
                            if (!id) return null
                            const x = Number(n?.x || 0)
                            const y = Number(n?.y || 0)
                            const w = Number((treeRender as any).nodeW || 168)
                            const h = Number((treeRender as any).nodeH || 44)
                            const role = normalizeChatTreeNodeRole(n?.role)
                            const text = String(n?.text || '')
                            const isSelected = id === String(treeFocusMid || '')
                            const clipId = `fw-tree-clip-${svgSafeId(id)}`

                            return (
                              <g key={id} transform={`translate(${Math.round(x)},${Math.round(y)})`}>
                                <g
                                  className="fw-tree-node"
                                  data-pop={treePop.id === id ? '1' : undefined}
                                  style={{ cursor: 'pointer' }}
                                  data-tree-node="1"
                                  onClick={(ev) => {
                                    if (treeSuppressClickRef.current) {
                                      treeSuppressClickRef.current = false
                                      ev.preventDefault()
                                      ev.stopPropagation()
                                      return
                                    }
                                    setTreeSelectedMid(id)
                                    setTreePop({ id, at: Date.now() })
                                    jumpToMessage(id)
                                  }}
                                  onContextMenu={role === 'system' ? undefined : (ev) => onTreeNodeContextMenu(ev, id, role === 'assistant' ? 'assistant' : 'user')}
                                  onPointerDown={(ev) => {
                                    ev.stopPropagation()
                                  }}
                                >
                                  <ChatTreeNodeShape role={role} text={text} isSelected={isSelected} w={w} h={h} clipId={clipId} />
                                </g>
                              </g>
                            )
                          })}
                        </g>
                      </svg>
                    ) : (
                      <Box sx={{ p: 1.5 }}>
                        <Typography variant="body2" color="text.secondary">
                          暂无可展示的树（至少需要 1 条消息）。
                        </Typography>
                      </Box>
                    )}
                  </Box>
                </Box>
                  </>
                ) : null}
              </Box>

              <Dialog
                open={treeOpen && effectiveTreeView === 'float'}
                onClose={() => closeTreeModal(true)}
                maxWidth={false}
                disableRestoreFocus
                PaperProps={{
                  sx: {
                    width: 'min(92vw, 980px)',
                    height: 'min(80vh, 760px)',
                    borderRadius: 3,
                    overflow: 'hidden',
                    bgcolor: transparentChatBg ? colorMixVar('--studio-paper', Math.max(72, bgAlpha * 100)) : 'var(--studio-paper)',
                  },
                }}
              >
                <Box sx={{ position: 'relative', width: '100%', height: '100%' }}>
                  <Box sx={{ position: 'absolute', top: 10, right: 10, zIndex: 3, display: 'flex', gap: 1 }}>
                    <Tooltip
                      title={
                        treeDir === 'lr'
                          ? '切换方向（当前：左→右）'
                          : treeDir === 'rl'
                            ? '切换方向（当前：右→左）'
                            : treeDir === 'tb'
                              ? '切换方向（当前：上→下）'
                              : '切换方向（当前：下→上）'
                      }
                    >
                      <span>
                        <IconButton
                          size="small"
                          onClick={(e) => {
                            cycleTreeDir()
                            try {
                              ;(e.currentTarget as any)?.blur?.()
                            } catch (_) {}
                          }}
                          sx={{
                            bgcolor: 'rgba(255,255,255,.72)',
                            border: '1px solid rgba(0,0,0,.12)',
                            backdropFilter: 'blur(8px)',
                            WebkitBackdropFilter: 'blur(8px)',
                            '&:hover': { bgcolor: 'rgba(255,255,255,.82)' },
                          }}
                        >
                          <AutorenewIcon fontSize="inherit" />
                        </IconButton>
                      </span>
                    </Tooltip>
                    <Tooltip title="重置视图">
                      <span>
                        <IconButton
                          size="small"
                          onClick={() => {
                            setTreePan({ x: 18, y: 18 })
                            setTreeScale(1)
                            treeViewRef.current = { x: 18, y: 18, scale: 1 }
                            scheduleTreeViewTransform()
                          }}
                          onMouseUp={(e) => {
                            try {
                              ;(e.currentTarget as any)?.blur?.()
                            } catch (_) {}
                          }}
                          sx={{
                            bgcolor: 'rgba(255,255,255,.72)',
                            border: '1px solid rgba(0,0,0,.12)',
                            backdropFilter: 'blur(8px)',
                            WebkitBackdropFilter: 'blur(8px)',
                            '&:hover': { bgcolor: 'rgba(255,255,255,.82)' },
                          }}
                        >
                          <RestartAltIcon fontSize="inherit" />
                        </IconButton>
                      </span>
                    </Tooltip>
                  </Box>
                  <Box
                    onPointerDown={onTreePointerDown}
                    onPointerMove={onTreePointerMove}
                    onPointerUp={endTreeDrag}
                    onPointerCancel={endTreeDrag}
                    onWheel={onTreeWheel}
                    ref={(el: any) => {
                      treeHostFloatRef.current = el
                    }}
                    sx={{
                      position: 'absolute',
                      inset: 0,
                      bgcolor: 'rgba(0,0,0,.03)',
                      overflow: 'hidden',
                      cursor: treeDragging ? 'grabbing' : 'grab',
                      touchAction: 'none',
                      userSelect: 'none',
                      WebkitUserSelect: 'none',
                      contain: 'strict',
                    }}
                  >
                    {treeRender && Array.isArray((treeRender as any).nodes) && (treeRender as any).nodes.length ? (
                      <svg width="100%" height="100%" style={{ display: 'block' }}>
                        <g
                          ref={(el) => {
                            treeViewportRef.current = el
                            if (el) requestAnimationFrame(() => applyTreeViewTransform())
                          }}
                          transform="translate(0,0) scale(1)"
                        >
                          {Array.isArray((treeRender as any).edges)
                            ? ((treeRender as any).edges as any[]).map((e: any) => {
                                const from = String(e?.from || '').trim()
                                const to = String(e?.to || '').trim()
                                if (!from || !to) return null
                                const a = (treeRender as any)?.byId?.get(from) || null
                                const b = (treeRender as any)?.byId?.get(to) || null
                                if (!a || !b) return null
                                const w = Number((treeRender as any).nodeW || 168)
                                const h = Number((treeRender as any).nodeH || 44)
                                const ax = Number(a.x || 0)
                                const ay = Number(a.y || 0)
                                const bx = Number(b.x || 0)
                                const by = Number(b.y || 0)
                                const strokeW = 6
                                const capR = strokeW / 2
                                const aiGap = 2
                                const selectedMid = String(treeFocusMid || '')
                                const aIsAi = String(a?.role || '') === 'assistant'
                                const bIsAi = String(b?.role || '') === 'assistant'
                                const aDotR = aIsAi && from === selectedMid ? 10 : 8
                                const bDotR = bIsAi && to === selectedMid ? 10 : 8
                                const aOff = aIsAi ? aDotR + capR + aiGap : 0
                                const bOff = bIsAi ? bDotR + capR + aiGap : 0

                                const horizontal = treeDir === 'lr' || treeDir === 'rl'
                                let sx = 0
                                let sy = 0
                                let tx = 0
                                let ty = 0
                                let d = ''

                                if (horizontal) {
                                  const forward = bx >= ax
                                  const acx = ax + w / 2
                                  const acy = ay + h / 2
                                  const bcx = bx + w / 2
                                  const bcy = by + h / 2

                                  sx = aIsAi ? acx + (forward ? aOff : -aOff) : ax + (forward ? w : 0)
                                  sy = aIsAi ? acy : ay + h / 2
                                  tx = bIsAi ? bcx + (forward ? -bOff : bOff) : bx + (forward ? 0 : w)
                                  ty = bIsAi ? bcy : by + h / 2
                                  const dd = Math.max(42, Math.abs(tx - sx) * 0.5)
                                  d = `M ${sx} ${sy} C ${sx + (forward ? dd : -dd)} ${sy} ${tx - (forward ? dd : -dd)} ${ty} ${tx} ${ty}`
                                } else {
                                  const forward = by >= ay
                                  const acx = ax + w / 2
                                  const acy = ay + h / 2
                                  const bcx = bx + w / 2
                                  const bcy = by + h / 2

                                  sx = aIsAi ? acx : ax + w / 2
                                  sy = aIsAi ? acy + (forward ? aOff : -aOff) : ay + (forward ? h : 0)
                                  tx = bIsAi ? bcx : bx + w / 2
                                  ty = bIsAi ? bcy + (forward ? -bOff : bOff) : by + (forward ? 0 : h)
                                  const dd = Math.max(42, Math.abs(ty - sy) * 0.5)
                                  d = `M ${sx} ${sy} C ${sx} ${sy + (forward ? dd : -dd)} ${tx} ${ty - (forward ? dd : -dd)} ${tx} ${ty}`
                                }
                                const key = `${from}->${to}`
                                const hi = treeHighlightEdgeKeys.has(key)
                                return (
                                  <path
                                    key={key}
                                    d={d}
                                    fill="none"
                                    stroke={hi ? 'rgba(34,197,94,.85)' : 'rgba(0,0,0,.16)'}
                                    strokeWidth={strokeW}
                                    strokeLinecap="round"
                                    strokeLinejoin="round"
                                  />
                                )
                              })
                            : null}

                          {((treeRender as any).nodes as any[]).map((n: any) => {
                            const id = String(n?.id || '').trim()
                            if (!id) return null
                            const x = Number(n?.x || 0)
                            const y = Number(n?.y || 0)
                            const w = Number((treeRender as any).nodeW || 168)
                            const h = Number((treeRender as any).nodeH || 44)
                            const role = normalizeChatTreeNodeRole(n?.role)
                            const text = String(n?.text || '')
                            const isSelected = id === String(treeFocusMid || '')
                            const clipId = `fw-tree-clip-${svgSafeId(id)}`

                            return (
                              <g key={id} transform={`translate(${Math.round(x)},${Math.round(y)})`}>
                                <g
                                  className="fw-tree-node"
                                  data-pop={treePop.id === id ? '1' : undefined}
                                  style={{ cursor: 'pointer' }}
                                  data-tree-node="1"
                                  onClick={(ev) => {
                                    if (treeSuppressClickRef.current) {
                                      treeSuppressClickRef.current = false
                                      ev.preventDefault()
                                      ev.stopPropagation()
                                      return
                                    }
                                    setTreeSelectedMid(id)
                                    setTreePop({ id, at: Date.now() })
                                    jumpToMessage(id)
                                  }}
                                  onContextMenu={role === 'system' ? undefined : (ev) => onTreeNodeContextMenu(ev, id, role === 'assistant' ? 'assistant' : 'user')}
                                  onPointerDown={(ev) => {
                                    ev.stopPropagation()
                                  }}
                                >
                                  <ChatTreeNodeShape role={role} text={text} isSelected={isSelected} w={w} h={h} clipId={clipId} />
                                </g>
                              </g>
                            )
                          })}
                        </g>
                      </svg>
                    ) : (
                      <Box sx={{ p: 1.5 }}>
                        <Typography variant="body2" color="text.secondary">
                          暂无可展示的树（至少需要 1 条消息）。
                        </Typography>
                      </Box>
                    )}
                  </Box>
                </Box>
              </Dialog>

               <Box
                 ref={composerRef}
                 onClick={onClickOpenImageViewer}
                 sx={{
                 position: 'absolute',
                 left: 16,
                 right: treeOpen && effectiveTreeView === 'right' ? 16 + Math.round(treePanelW) : 16,
                 bottom: 16,
                zIndex: 1299,
                p: 1.5,
                borderRadius: 18,
                bgcolor: `rgba(255,255,255,${composerOpacity / 100})`,
                boxShadow: '0 12px 28px rgba(0,0,0,.18)',
                backdropFilter: composerBlur > 0 ? `blur(${composerBlur}px)` : 'none',
                WebkitBackdropFilter: composerBlur > 0 ? `blur(${composerBlur}px)` : 'none',
              }}
            >
              <Stack spacing={1}>
                {Array.isArray(s.draft?.images) && s.draft.images.length ? (
                  <Stack direction="row" spacing={1} sx={{ flexWrap: 'wrap' }}>
                    {s.draft.images.map((img: any) => (
                      <Box key={String(img?.id || '')} sx={{ position: 'relative' }}>
                        <Box
                          component="img"
                          data-fw-img="1"
                          src={String(img?.dataUrl || '')}
                          alt={String(img?.name || '图片')}
                          sx={{ width: 64, height: 64, objectFit: 'cover', borderRadius: 2, border: '1px solid', borderColor: 'divider', cursor: 'zoom-in' }}
                        />
                        <IconButton
                          size="small"
                          onClick={() => controller.actions.removeDraftImage(String(img?.id || ''))}
                          sx={{ position: 'absolute', top: 4, right: 4, bgcolor: 'rgba(255,255,255,.85)', border: '1px solid', borderColor: 'divider' }}
                        >
                          <CloseIcon fontSize="inherit" />
                        </IconButton>
                      </Box>
                    ))}
                  </Stack>
                ) : null}

                {Array.isArray((s.draft as any)?.files) && (s.draft as any).files.length ? (
                  <Stack direction="row" spacing={1} sx={{ flexWrap: 'wrap' }}>
                    {(s.draft as any).files.map((f: any) => {
                      const id = String(f?.id || '')
                      const name = String(f?.name || '文件')
                      const pending = !!f?.pending
                      const err = String(f?.error || '').trim()
                      const pct0 = Math.round(Number(f?.sendPct ?? 100))
                      const pct = clampNum(pct0, 0, 100)
                      const rawLen = String(f?.text || '').trim().length
                      const sendLen = Math.max(0, Math.ceil((rawLen * pct) / 100))
                      const warn = !pending && !err && rawLen > 0 && sendLen > attachSendLimitChars
                      const label = pending
                        ? `${name}（解析中…）`
                        : err
                          ? `${name}（失败）`
                          : warn
                            ? `${name}（超长提醒）`
                            : pct < 100
                              ? `${name}（${pct}%）`
                              : name
                      return (
                        <Chip
                          key={id || name}
                          size="small"
                          label={label}
                          variant="outlined"
                          color={err ? 'error' : warn ? 'warning' : 'default'}
                          onClick={id ? (e) => openFileAdjust(e as any, id) : undefined}
                          onDelete={id ? () => controller.actions.removeDraftFile?.(id) : undefined}
                          sx={{ maxWidth: 320 }}
                        />
                      )
                    })}
                  </Stack>
                ) : null}

                <input
                  ref={draftFilePickerInputRef}
                  hidden
                  type="file"
                  multiple
                  accept=".txt,.md,.pdf,.docx,.ppt,.pptx,text/plain,text/markdown,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/vnd.ms-powerpoint,application/vnd.openxmlformats-officedocument.presentationml.presentation"
                  onChange={onPickFilesChanged}
                />

                <ComposerInputControls
                  controller={controller}
                  draftKey={String((s as any).activeSessionComposerDraftKey || `${activeTargetKind}:${activeChatTargetId}:${activeChatId || '__new__'}`)}
                  initialValue={String(s.draft?.input || '')}
                  inputRef={composerInputRef}
                  disabled={s.loading || !activeRole}
                  draftFilesPending={draftFilesPending}
                  draftFilesWarn={draftFilesWarn}
                  hasDraftNonText={!!((s.draft?.images || []).length || hasDraftFiles)}
                  activeTargetKind={activeTargetKind}
                  activeGroup={activeGroup}
                  roles={roles}
                  activeStopRunId={activeStopRunId}
                  formatModelRefText={formatModelRefText}
                  toolbarStart={(
                    <>
                      <Tooltip title="添加图片或文件">
                        <span>
                          <IconButton
                            aria-label="添加图片或文件"
                            onClick={openAttachmentPicker}
                            disabled={s.loading || !activeRole}
                            size="small"
                            sx={composerToolIconButtonSx}
                          >
                            <AddIcon fontSize="small" />
                          </IconButton>
                        </span>
                      </Tooltip>

                      <HookPromptSelector
                        library={hookPrompts.library || { presets: [] }}
                        selectedMode={activeHookPromptMode as any}
                        selectedPresetId={activeHookPromptPresetId}
                        roleDefaultPresetId={roleDefaultHookPromptPresetId}
                        disabled={hookPromptSelectorDisabled || chatSettingsHookSaving}
                        saving={chatSettingsHookSaving}
                        disabledReason={hookPromptSelectorDisabledReason}
                        onSelect={(mode, presetId) => controller.actions.selectHookPromptForActiveChat?.(mode, presetId)}
                      />

                      {roleSessionControlsEnabled ? (
                        <Tooltip title={chatSettingsModelSaving ? '临时模型保存中…' : hasChatOverride ? `临时模型：${formatModelRefText(chatOverride)}` : `角色模型：${effectiveModelId || '未配置模型'}`}>
                          <span>
                            <Button
                              aria-label="临时切换模型"
                              onClick={openTempModelPicker}
                              disabled={s.loading || chatSettingsModelSaving || !activeRole || !providers.length}
                              size="small"
                              variant="text"
                              sx={{ ...composerToolTextButtonSx, color: hasChatOverride ? 'primary.main' : 'text.secondary' }}
                            >
                              {chatSettingsModelSaving ? <CircularProgress size={14} thickness={5} color="inherit" sx={{ mr: 0.5 }} /> : null}
                              <Box component="span" sx={{ minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                {chatSettingsModelSaving ? '保存中…' : effectiveModelId || '未配置模型'}
                              </Box>
                            </Button>
                          </span>
                        </Tooltip>
                      ) : null}

                      {roleSessionControlsEnabled && reasoningProfile.supportsReasoning ? (
                        <Tooltip title={chatSettingsReasoningSaving ? '思考等级保存中…' : `思考等级：${activeReasoningLabel || '默认'}`}>
                          <span>
                            <Button
                              aria-label="选择思考等级"
                              onClick={openReasoningPicker}
                              disabled={s.loading || chatSettingsReasoningSaving || !activeRole}
                              size="small"
                              variant="text"
                              sx={{ ...composerToolTextButtonSx, color: hasChatReasoningOverride ? 'primary.main' : 'text.secondary' }}
                            >
                              {activeReasoningLabel || '默认'}
                            </Button>
                          </span>
                        </Tooltip>
                      ) : null}

                      <Tooltip title={`上下文约 ${activeContextTokenUsageText}`}>
                        <span>
                          <Button aria-label={`上下文约 ${activeContextTokenUsageText}`} size="small" variant="text" sx={composerContextButtonSx}>
                            {activeContextTokenUsageShortText}
                          </Button>
                        </span>
                      </Tooltip>

                      <Tooltip title="异步工具任务">
                        <span>
                          <Button
                            aria-label="异步工具任务"
                            onClick={openAsyncToolTasks}
                            disabled={!activeRole || !activeChatId}
                            size="small"
                            variant="text"
                            startIcon={<AutorenewIcon fontSize="small" />}
                            sx={composerToolTextButtonSx}
                          >
                            异步 {activeAsyncToolTaskRunningCount}
                          </Button>
                        </span>
                      </Tooltip>

                      <Tooltip title={activeStreamOn ? '流式输出：已开启（本会话）' : '非流式：已关闭（本会话）'}>
                        <span>
                          <Button
                            aria-label="切换会话流式输出"
                            onClick={() => controller.actions.toggleChatStreamEnabled?.()}
                            disabled={s.loading || chatSettingsStreamSaving || !roleSessionControlsEnabled || !activeChat}
                            size="small"
                            variant="text"
                            sx={{ ...composerToolTextButtonSx, width: 88, justifyContent: 'center', color: activeStreamOn ? 'primary.main' : 'text.secondary' }}
                          >
                            {activeStreamOn ? '流' : '非流'}
                          </Button>
                        </span>
                      </Tooltip>
                    </>
                  )}
                  onSend={onSend}
                  onStop={onStop}
                  onPaste={onPaste}
                />
              </Stack>
            </Box>
        </Box>

        <Popover
          open={!!attachmentPickerEl}
          anchorEl={attachmentPickerEl}
          onClose={closeAttachmentPicker}
          anchorOrigin={{ vertical: 'top', horizontal: 'left' }}
          transformOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        >
          <Box data-area="composer-attachment-picker" sx={{ width: 248, p: 1 }}>
            <Stack spacing={0.5}>
              <Button startIcon={<ImageIcon fontSize="small" />} variant="text" onClick={onPickDraftImages} disabled={s.loading || !activeRole} sx={{ justifyContent: 'flex-start', borderRadius: 2 }}>
                图片
              </Button>
              <Button startIcon={<AttachFileIcon fontSize="small" />} variant="text" onClick={onPickDraftFiles} disabled={s.loading || !activeRole} sx={{ justifyContent: 'flex-start', borderRadius: 2 }}>
                文件（txt/md/pdf/docx/ppt/pptx）
              </Button>
            </Stack>
          </Box>
        </Popover>

        <Popover
          open={!!asyncToolTasksEl}
          anchorEl={asyncToolTasksEl}
          onClose={closeAsyncToolTasks}
          anchorOrigin={{ vertical: 'top', horizontal: 'left' }}
          transformOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        >
          <Box data-area="async-tool-tasks" sx={{ width: 380, p: 1.5 }}>
            <Stack spacing={1.25}>
              <Stack direction="row" spacing={1} alignItems="center">
                <Typography variant="subtitle2" sx={{ fontWeight: 900 }}>异步工具任务</Typography>
                <Box sx={{ flex: 1 }} />
                <Button size="small" onClick={refreshAsyncToolTasks} disabled={asyncToolTasksLoading}>刷新</Button>
                <Button size="small" onClick={closeAsyncToolTasks}>关闭</Button>
              </Stack>
              <Typography variant="caption" color="text.secondary">只读展示当前会话的后台工具任务。</Typography>
              {asyncToolTasksLoading ? (
                <Stack direction="row" spacing={1} alignItems="center"><CircularProgress size={16} /><Typography variant="body2">加载中…</Typography></Stack>
              ) : asyncToolTasks.length ? (
                <Stack spacing={1}>
                  {asyncToolTasks.map((task: any) => {
                    const status = String(task?.status || '').trim() || 'unknown'
                    const submittedAt = numericTimeValue(task?.submittedAt)
                    const startedAt = numericTimeValue(task?.startedAt)
                    const finishedAt = numericTimeValue(task?.finishedAt)
                    const elapsed = startedAt && finishedAt ? `${Math.max(0, Math.round((finishedAt - startedAt) / 1000))}s` : startedAt ? '运行中' : '-'
                    return (
                      <Paper key={String(task?.id || `${task?.toolName}-${task?.submittedAt}`)} variant="outlined" sx={{ p: 1, borderRadius: 2 }}>
                        <Stack spacing={0.5}>
                          <Stack direction="row" spacing={1} alignItems="center">
                            <Typography variant="body2" sx={{ fontWeight: 800, minWidth: 0, flex: 1 }} noWrap>{String(task?.taskName || task?.toolName || '工具任务')}</Typography>
                            <Chip size="small" label={status} color={status === 'succeeded' || status === 'completed' ? 'success' : status === 'failed' ? 'error' : 'warning'} />
                          </Stack>
                          <Typography variant="caption" color="text.secondary">工具：{String(task?.toolName || '-')}</Typography>
                          <Typography variant="caption" color="text.secondary">提交：{submittedAt ? new Date(submittedAt).toLocaleString() : '-'}</Typography>
                          <Typography variant="caption" color="text.secondary">耗时：{elapsed}</Typography>
                        </Stack>
                      </Paper>
                    )
                  })}
                </Stack>
              ) : (
                <Typography variant="body2" color="text.secondary">当前会话暂无异步工具任务。</Typography>
              )}
            </Stack>
          </Box>
        </Popover>

        <Popover
          open={roleSessionControlsEnabled && !!tempModelPickerEl}
          anchorEl={tempModelPickerEl}
          onClose={closeTempModelPicker}
          anchorOrigin={{ vertical: 'top', horizontal: 'left' }}
          transformOrigin={{ vertical: 'bottom', horizontal: 'left' }}
          PaperProps={{ sx: SOFT_POPOVER_PAPER_SX }}
        >
          <Box data-area="temp-model" sx={{ width: 420, p: 1.75 }}>
            <Stack spacing={1.25}>
              <Stack direction="row" spacing={1} alignItems="center">
                <Typography variant="subtitle2" sx={{ fontWeight: 900 }}>
                  当前会话临时模型
                </Typography>
                <Box sx={{ flex: 1 }} />
                <Button size="small" onClick={closeTempModelPicker}>
                  关闭
                </Button>
              </Stack>

              <Typography variant="caption" color="text.secondary">
                仅影响当前会话；不修改角色设置。
              </Typography>

              <FormControl size="small" fullWidth>
                <InputLabel id="chat-override-provider">供应商</InputLabel>
                <Select
                  labelId="chat-override-provider"
                  label="供应商"
                  value={String(tempModelProviderId || '')}
                  onChange={(e) => onTempProviderChanged(String(e.target.value || ''))}
                  disabled={s.loading || !providers.length}
                >
                  {providerSelectItems(providers)}
                </Select>
              </FormControl>

              {(() => {
                const pid = String(tempModelProviderId || '')
                const p = providers.find((x: any) => String(x?.id || '') === pid) || null
                const items = registeredModelItems(p)

                return (
                  <Stack spacing={1}>
                    <FormControl size="small" fullWidth>
                      <InputLabel id="chat-override-model">登记模型</InputLabel>
                      <Select
                        labelId="chat-override-model"
                        label="登记模型"
                        value={String(tempModelPick || '')}
                        onChange={(e) => setTempModelPick(String(e.target.value || ''))}
                        disabled={s.loading || !pid}
                      >
                        <MenuItem value="">
                          <em>请选择…</em>
                        </MenuItem>
                        {items.map((item: any) => (
                          <MenuItem key={item.id} value={item.id}>
                            {item.hint ? `${item.label} / ${item.hint}` : item.label}
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>

                    <Typography variant="caption" color="text.secondary">
                      这里仅显示供应商设置中已登记的模型；原始模型列表请到供应商设置中刷新并登记。
                    </Typography>
                  </Stack>
                )
              })()}

              <Stack direction="row" spacing={1} justifyContent="space-between" alignItems="center">
                <Button variant="text" onClick={clearTempModelOverride} disabled={!hasChatOverride || s.loading}>
                  清除临时模型（跟随角色）
                </Button>
                <Button variant="contained" onClick={saveTempModelOverride} disabled={s.loading || !tempModelProviderId}>
                  保存
                </Button>
              </Stack>
            </Stack>
          </Box>
        </Popover>

        <Popover
          open={roleSessionControlsEnabled && !!reasoningPickerEl}
          anchorEl={reasoningPickerEl}
          onClose={closeReasoningPicker}
          anchorOrigin={{ vertical: 'top', horizontal: 'left' }}
          transformOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        >
          <Box data-area="reasoning-effort" sx={{ width: 300, p: 1.5 }}>
            <Stack spacing={1.25}>
              <Stack direction="row" spacing={1} alignItems="center">
                <Typography variant="subtitle2" sx={{ fontWeight: 900 }}>
                  当前会话思考等级
                </Typography>
                <Box sx={{ flex: 1 }} />
                <Button size="small" onClick={closeReasoningPicker}>关闭</Button>
              </Stack>

              <Typography variant="caption" color="text.secondary">
                仅影响当前会话；不修改模型默认设置。
              </Typography>

              <Stack spacing={0.75}>
                {REASONING_EFFORT_OPTIONS.map((option) => {
                  const selected = String(activeEffectiveReasoningEffort || '') === option.value
                  const sessionSelected = String(activeChatReasoningEffort || '') === option.value
                  return (
                    <Button
                      key={option.value}
                      variant={selected ? 'contained' : 'outlined'}
                      onClick={() => pickReasoningEffort(option.value)}
                      disabled={s.loading}
                      sx={{ justifyContent: 'space-between', borderRadius: 2 }}
                    >
                      <span>{option.label}</span>
                      <span style={{ fontSize: 12, opacity: 0.75 }}>{sessionSelected ? '当前会话' : selected ? '默认生效' : ''}</span>
                    </Button>
                  )
                })}
              </Stack>

              <Button variant="text" onClick={clearReasoningEffort} disabled={!hasChatReasoningOverride || s.loading}>
                恢复模型默认
              </Button>
            </Stack>
          </Box>
        </Popover>

        <Popover
          open={!!fileAdjust.el}
          anchorEl={fileAdjust.el}
          onClose={closeFileAdjust}
          anchorOrigin={{ vertical: 'top', horizontal: 'left' }}
          transformOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        >
          <Box data-area="file-adjust" sx={{ width: 420, p: 1.5 }}>
            <Stack spacing={1.25}>
              <Stack direction="row" spacing={1} alignItems="center">
                <Typography variant="subtitle2" sx={{ fontWeight: 900 }} noWrap>
                  附件：{fileAdjustName}
                </Typography>
                <Box sx={{ flex: 1 }} />
                <Button size="small" onClick={closeFileAdjust}>
                  关闭
                </Button>
              </Stack>

              {!fileAdjustItem ? (
                <Typography variant="body2" color="text.secondary">
                  未找到该附件。
                </Typography>
              ) : fileAdjustPending ? (
                <Typography variant="body2" color="text.secondary">
                  解析中…
                </Typography>
              ) : fileAdjustError ? (
                <Typography variant="body2" color="error">
                  {fileAdjustError}
                </Typography>
              ) : (
                <>
                  <Typography variant="caption" color="text.secondary">
                    文件文本长度：{fileAdjustFullLen}；阈值：{attachSendLimitChars}；当前将发送：{fileAdjustSendLen}
                  </Typography>

                  <Box>
                    <Stack direction="row" spacing={1} alignItems="center">
                      <Typography variant="body2" sx={{ fontWeight: 900 }}>
                        发送百分比
                      </Typography>
                      <Box sx={{ flex: 1 }} />
                      <Typography variant="caption" color={fileAdjustTooLong ? 'error' : 'text.secondary'}>
                        {fileAdjustPct}%
                      </Typography>
                    </Stack>
                    <Slider
                      size="small"
                      value={fileAdjustPct}
                      min={0}
                      max={100}
                      step={1}
                      onChange={(_e, v) => controller.actions.setDraftFileSendPct?.(String(fileAdjust.id || ''), v)}
                    />
                    {fileAdjustTooLong ? (
                      <Typography variant="caption" sx={{ color: 'warning.main' }}>
                        超过阈值：点击发送时会弹出确认提醒。
                      </Typography>
                    ) : (
                      <Typography variant="caption" color="text.secondary">
                        发送时仅取文件开头部分；不会自动截断。超过阈值会在点击发送时提示确认。
                      </Typography>
                    )}
                  </Box>

                  <Paper variant="outlined" sx={{ bgcolor: 'grey.50' }}>
                    <CustomScrollArea hostSx={{ maxHeight: 200 }} scrollSx={{ maxHeight: 200 }}>
                      <Box sx={{ p: 1 }}>
                        <Typography variant="caption" sx={{ whiteSpace: 'pre-wrap', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace' }}>
                          {(() => {
                            const sendLen = clampNum(fileAdjustSendLen, 0, fileAdjustFullLen)
                            const snippet = fileAdjustRaw.slice(0, sendLen)
                            if (snippet.length <= 4000) return snippet
                            const head = snippet.slice(0, 1500).trimEnd()
                            const tail = snippet.slice(Math.max(0, snippet.length - 1500)).trimStart()
                            return `${head}\n\n…（中间省略 ${Math.max(0, snippet.length - head.length - tail.length)} 字符）…\n\n${tail}`
                          })()}
                        </Typography>
                      </Box>
                    </CustomScrollArea>
                  </Paper>
                  <Typography variant="caption" color="text.secondary">
                    预览显示“将发送内容”的开头与截断点附近片段（超过 4000 字符会省略中间）。
                  </Typography>
                </>
              )}
            </Stack>
          </Box>
        </Popover>

        <Popover
          open={!!rolePickerEl}
          anchorEl={rolePickerEl}
          onClose={closeRolePicker}
          anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
          transformOrigin={{ vertical: 'top', horizontal: 'left' }}
          PaperProps={{ sx: SOFT_POPOVER_PAPER_SX }}
        >
          <CustomScrollArea hostSx={{ width: 380, maxHeight: '70vh' }} scrollSx={{ maxHeight: '70vh' }}>
            {rolePickerMode === 'global' ? (
              <Box sx={{ px: 1.5, pt: 1.25, pb: 0.5 }}>
                <Tabs
                  value={rolePickerTab}
                  onChange={(_e, v) => setRolePickerTab(v === 'groups' ? 'groups' : v === 'workspaces' ? 'workspaces' : 'roles')}
                  variant="fullWidth"
                >
                  <Tab value="roles" label="选择角色" />
                  <Tab value="groups" label="群组" />
                  <Tab value="workspaces" label="工作区" />
                </Tabs>
              </Box>
            ) : null}
            {rolePickerTab === 'roles' ? (
              <List dense sx={SOFT_POPOVER_LIST_SX}>
                {roles.map((r: any) => {
                  const on = String(r?.id || '') === String(s.draft?.activeRoleId || '')
                  const modelRefText = formatModelRefText(r?.modelRef)
                  const selected = rolePickerMode === 'workspaceRole'
                    ? on && activeTargetKind === 'workspace'
                    : on && activeTargetKind === 'role'
                  return (
                      <ListItemButton
                        key={String(r?.id || '')}
                        selected={selected}
                        onClick={() => {
                          if (rolePickerMode === 'workspaceRole' && activeTargetKind === 'workspace') controller.actions.setWorkspaceRole?.(String(r?.id || ''))
                          else controller.actions.setActiveRole(String(r?.id || ''))
                          closeRolePicker()
                        }}
                        sx={SOFT_POPOVER_ITEM_SX}
                    >
                      <ListItemAvatar>
                        <Avatar src={String(r?.avatarImage || '') || undefined} sx={{ width: 28, height: 28, fontSize: 14 }}>
                          {String(r?.avatar || '🙂')}
                        </Avatar>
                      </ListItemAvatar>
                      <ListItemText
                        sx={{ minWidth: 0 }}
                        primary={
                          <Typography sx={{ fontWeight: 900, fontSize: 13 }} noWrap>
                            {String(r?.name || '')}
                          </Typography>
                        }
                        secondary={
                          <Typography variant="caption" color="text.secondary" noWrap>
                            {modelRefText || '未配置模型'}
                          </Typography>
                        }
                      />
                      <Tooltip title="设置">
                        <IconButton
                          size="small"
                          onClick={(e) => {
                            e.preventDefault()
                            e.stopPropagation()
                            closeRolePicker()
                            controller.actions.openRoleEditor(String(r?.id || ''))
                          }}
                        >
                          <SettingsIcon fontSize="inherit" />
                        </IconButton>
                      </Tooltip>
                    </ListItemButton>
                  )
                })}
              </List>
            ) : rolePickerTab === 'groups' ? groups.length ? (
              <List dense sx={SOFT_POPOVER_LIST_SX}>
                {groups.map((g: any) => {
                  const on = String(g?.id || '') === String(activeGroupId || '')
                  return (
                    <ListItemButton
                      key={String(g?.id || '')}
                      selected={on && activeTargetKind === 'group'}
                      onClick={() => {
                        controller.actions.setActiveGroup?.(String(g?.id || ''))
                        closeRolePicker()
                      }}
                      sx={SOFT_POPOVER_ITEM_SX}
                    >
                      <ListItemAvatar>
                        <Avatar src={String(g?.avatarImage || '') || undefined} sx={{ width: 28, height: 28, fontSize: 14 }}>
                          {String(g?.avatar || '👥')}
                        </Avatar>
                      </ListItemAvatar>
                      <ListItemText
                        sx={{ minWidth: 0 }}
                        primary={
                          <Typography sx={{ fontWeight: 900, fontSize: 13 }} noWrap>
                            {String(g?.name || '')}
                          </Typography>
                        }
                        secondary={
                          <Typography variant="caption" color="text.secondary" noWrap>
                            {Array.isArray(g?.memberRoleIds) ? `${g.memberRoleIds.length} 个成员` : '群聊'}
                          </Typography>
                        }
                      />
                      <Tooltip title="设置">
                        <IconButton
                          size="small"
                          onClick={(e) => {
                            e.preventDefault()
                            e.stopPropagation()
                            closeRolePicker()
                            controller.actions.openGroupEditor?.(String(g?.id || ''))
                          }}
                        >
                          <SettingsIcon fontSize="inherit" />
                        </IconButton>
                      </Tooltip>
                    </ListItemButton>
                  )
                })}
              </List>
            ) : (
              <Box sx={{ p: 2 }}>
                <Typography variant="body2" color="text.secondary">
                  还没有群组。
                </Typography>
                <Button
                  size="small"
                  variant="contained"
                  sx={{ mt: 1 }}
                  onClick={() => {
                    closeRolePicker()
                    openPluginSettings('groups')
                  }}
                >
                  去创建群组
                </Button>
              </Box>
            ) : workspaces.length ? (
              <List dense sx={SOFT_POPOVER_LIST_SX}>
                {workspaces.map((workspace: any) => {
                  const workspaceId = String(workspace?.id || '')
                  const on = workspaceId === String(activeWorkspaceId || '')
                  const directoryCount = Array.isArray(workspace?.directories) ? workspace.directories.length : 0
                  return (
                    <ListItemButton
                      key={workspaceId}
                      selected={on && activeTargetKind === 'workspace'}
                      onClick={() => {
                        controller.actions.setActiveWorkspace?.(workspaceId)
                        closeRolePicker()
                      }}
                      sx={SOFT_POPOVER_ITEM_SX}
                    >
                      <ListItemAvatar>
                        <Avatar sx={{ width: 28, height: 28, fontSize: 14, bgcolor: 'rgba(59,130,246,.12)', color: 'primary.main' }}>
                          📁
                        </Avatar>
                      </ListItemAvatar>
                      <ListItemText
                        sx={{ minWidth: 0 }}
                        primary={
                          <Typography sx={{ fontWeight: 900, fontSize: 13 }} noWrap>
                            {String(workspace?.name || '')}
                          </Typography>
                        }
                        secondary={
                          <Typography variant="caption" color="text.secondary" noWrap>
                            {directoryCount ? `${directoryCount} 个目录` : '暂未登记目录'}
                          </Typography>
                        }
                      />
                      <Tooltip title="设置">
                        <IconButton
                          size="small"
                          onClick={(e) => {
                            e.preventDefault()
                            e.stopPropagation()
                            closeRolePicker()
                            controller.actions.openWorkspaceEditor?.(workspaceId)
                          }}
                        >
                          <SettingsIcon fontSize="inherit" />
                        </IconButton>
                      </Tooltip>
                    </ListItemButton>
                  )
                })}
              </List>
            ) : (
              <Box sx={{ p: 2 }}>
                <Typography variant="body2" color="text.secondary">
                  还没有工作区。
                </Typography>
                <Button
                  size="small"
                  variant="contained"
                  sx={{ mt: 1 }}
                  onClick={() => {
                    closeRolePicker()
                    openPluginSettings('workspaces')
                  }}
                >
                  去创建工作区
                </Button>
              </Box>
            )}
          </CustomScrollArea>
        </Popover>

        <Popover
          open={!!chatPickerEl}
          anchorEl={chatPickerEl}
          onClose={closeChatPicker}
          anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
          transformOrigin={{ vertical: 'top', horizontal: 'right' }}
          PaperProps={{ sx: SOFT_POPOVER_PAPER_SX }}
        >
          <Box sx={{ width: 420, maxHeight: '70vh', overflow: 'hidden' }}>
            <Box
              sx={{
                width: 840,
                display: 'flex',
                transform: chatPickerView === 'favorites' ? 'translateX(-420px)' : 'translateX(0)',
                transition: 'transform 220ms ease',
              }}
            >
              <CustomScrollArea ref={chatHistoryScrollRef} onScrollPositionChange={onChatHistoryScrollPositionChange} hostSx={{ width: 420, maxHeight: '70vh', flex: '0 0 420px' }} scrollSx={{ maxHeight: '70vh' }}>
                <Box sx={SOFT_POPOVER_HEADER_SX}>
                  <Tooltip title={chatPickerSearchOpen ? '关闭搜索' : '搜索'}>
                    <IconButton
                      size="small"
                      onClick={() => {
                        setChatPickerSearchOpen((p) => !p)
                        if (chatPickerSearchOpen) setChatPickerSearchText('')
                      }}
                    >
                      {chatPickerSearchOpen ? <CloseIcon fontSize="inherit" /> : <SearchIcon fontSize="inherit" />}
                    </IconButton>
                  </Tooltip>
                  <Box sx={{ flex: 1 }} />
                  <Tooltip title="收藏夹">
                    <IconButton size="small" onClick={() => setChatPickerView('favorites')}>
                      <StarBorderRoundedIcon fontSize="inherit" />
                    </IconButton>
                  </Tooltip>
                </Box>
                <Collapse in={chatPickerSearchOpen}>
                  <Box sx={{ px: 1.5, pb: 1 }}>
                    <TextField
                      inputRef={chatPickerSearchInputRef}
                      fullWidth
                      size="small"
                      placeholder="搜索会话…"
                      value={String(chatPickerSearchText || '')}
                      onChange={(e) => setChatPickerSearchText(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Escape') {
                          e.preventDefault()
                          setChatPickerSearchOpen(false)
                          setChatPickerSearchText('')
                        }
                      }}
                      InputProps={{
                        startAdornment: (
                          <InputAdornment position="start">
                            <SearchIcon fontSize="small" />
                          </InputAdornment>
                        ),
                      }}
                    />
                  </Box>
                </Collapse>
                {(() => {
                  if (activeTargetKind === 'group') {
                    if (!activeGroup) {
                      return (
                        <Box sx={{ p: 2 }}>
                          <Typography variant="body2" color="text.secondary">
                            先选择群组
                          </Typography>
                        </Box>
                      )
                    }
                    const box = (data as any)?.chatsByGroup?.[String((activeGroup as any).id || '')]
                    const chats = sortChatListItemsForDisplay(Array.isArray(box?.chatMetas) && box.chatMetas.length ? box.chatMetas : Array.isArray(box?.chats) ? box.chats : [])
                    const activeChatId = String(box?.activeChatId || '')
                    const pendingChat =
                      (s as any)?.pendingGroupChat && String((s as any).pendingGroupChat?.groupId || '') === String((activeGroup as any)?.id || '')
                        ? (s as any).pendingGroupChat.chat
                        : null
                    const hasPending = !!pendingChat
                    const showPending = hasPending && chatHistoryMatchesSearch(pendingChat, '群聊', chatPickerSearchText)
                    const shownChats = chats.filter((c: any) => chatHistoryMatchesSearch(c, '群聊', chatPickerSearchText)).slice(0, chatHistoryVisibleCount)
                    if (!showPending && !shownChats.length) {
                      return (
                        <Box sx={{ p: 2 }}>
                          <Typography variant="body2" color="text.secondary">
                            没有匹配的会话
                          </Typography>
                        </Box>
                      )
                    }
                    return (
                      <List dense sx={SOFT_POPOVER_LIST_SX}>
                        {showPending ? (
                          <ListItemButton selected sx={SOFT_POPOVER_ITEM_TOP_SX}>
                            <ListItemText
                              sx={{ minWidth: 0 }}
                              primary={
                                <Stack direction="row" alignItems="center" spacing={1} sx={{ minWidth: 0 }}>
                                  <Typography sx={{ fontWeight: 900, fontSize: 13, flex: 1, minWidth: 0 }} noWrap>
                                    {String(pendingChat?.title || '群聊')}（未发送）
                                  </Typography>
                                  <Typography variant="caption" color="text.secondary">
                                    {controller.fmtTime(Number(pendingChat?.updatedAt || pendingChat?.createdAt || 0))}
                                  </Typography>
                                </Stack>
                              }
                              secondary={
                                <Typography variant="caption" color="text.secondary" noWrap sx={{ display: 'block', minWidth: 0 }}>
                                  （草稿）
                                </Typography>
                              }
                            />
                          </ListItemButton>
                        ) : null}
                        {shownChats.map((c: any) => {
                          const on = !showPending && String(c?.id || '') === activeChatId
                          const raw = String(c?.lastMessagePreview || '').replace(/\s+/g, ' ').trim()
                          const snippet = raw.length > 40 ? raw.slice(0, 40) + '…' : raw
                          const time = controller.fmtTime(Number(c?.updatedAt || c?.createdAt || 0))
                          const indicatorKind = chatSessionRunIndicatorKind('group', String((activeGroup as any)?.id || ''), c, on)
                          return (
                            <ListItemButton
                              key={String(c?.id || '')}
                              selected={on}
                              onClick={() => {
                                clearChatSessionRunNotice('group', String((activeGroup as any)?.id || ''), String(c?.id || ''))
                                chatSwitch.requestSwitch(String(c?.id || ''))
                                closeChatPicker()
                              }}
                              onContextMenu={(e) =>
                                onChatContextMenu(e, 'group', String((activeGroup as any)?.id || ''), String(c?.id || ''), String(c?.title || '群聊'))
                              }
                              sx={SOFT_POPOVER_ITEM_TOP_SX}
                            >
                              <ListItemText
                                sx={{ minWidth: 0 }}
                                primary={
                                  <Stack direction="row" alignItems="center" spacing={1} sx={{ minWidth: 0 }}>
                                    <Typography sx={{ fontWeight: 900, fontSize: 13, flex: 1, minWidth: 0 }} noWrap>
                                      {String(c?.title || '群聊')}
                                    </Typography>
                                     <Typography variant="caption" color="text.secondary">
                                       {time}
                                     </Typography>
                                     {indicatorKind ? <ChatSessionRunIndicator kind={indicatorKind} /> : null}
                                   </Stack>
                                 }
                                secondary={
                                  <Typography variant="caption" color="text.secondary" noWrap sx={{ display: 'block', minWidth: 0 }}>
                                    {snippet || '（空）'}
                                  </Typography>
                                }
                              />
                            </ListItemButton>
                          )
                        })}
                      </List>
                    )
                  }

                  if (activeTargetKind === 'workspace') {
                    if (!activeWorkspace) {
                      return (
                        <Box sx={{ p: 2 }}>
                          <Typography variant="body2" color="text.secondary">
                            先选择工作区
                          </Typography>
                        </Box>
                      )
                    }
                    const box = (data as any)?.chatsByWorkspace?.[String(activeChatTargetId || '')]
                    const chats = sortChatListItemsForDisplay(Array.isArray(box?.chatMetas) && box.chatMetas.length ? box.chatMetas : Array.isArray(box?.chats) ? box.chats : [])
                    const activeChatId = String(box?.activeChatId || '')
                    const pendingChat =
                      (s as any)?.pendingWorkspaceChat && workspaceRoleTargetId((s as any).pendingWorkspaceChat?.workspaceId, (s as any).pendingWorkspaceChat?.roleId) === String(activeChatTargetId || '')
                        ? (s as any).pendingWorkspaceChat.chat
                        : null
                    const hasPending = !!pendingChat
                    const showPending = hasPending && chatHistoryMatchesSearch(pendingChat, '工作区会话', chatPickerSearchText)
                    const shownChats = chats.filter((c: any) => chatHistoryMatchesSearch(c, '工作区会话', chatPickerSearchText)).slice(0, chatHistoryVisibleCount)
                    if (!showPending && !shownChats.length) {
                      return (
                        <Box sx={{ p: 2 }}>
                          <Typography variant="body2" color="text.secondary">
                            没有匹配的会话
                          </Typography>
                        </Box>
                      )
                    }
                    return (
                      <List dense sx={SOFT_POPOVER_LIST_SX}>
                        {showPending ? (
                          <ListItemButton selected sx={SOFT_POPOVER_ITEM_TOP_SX}>
                            <ListItemText
                              sx={{ minWidth: 0 }}
                              primary={
                                <Stack direction="row" alignItems="center" spacing={1} sx={{ minWidth: 0 }}>
                                  <Typography sx={{ fontWeight: 900, fontSize: 13, flex: 1, minWidth: 0 }} noWrap>
                                    {String(pendingChat?.title || '工作区会话')}（未发送）
                                  </Typography>
                                  <Typography variant="caption" color="text.secondary">
                                    {controller.fmtTime(Number(pendingChat?.updatedAt || pendingChat?.createdAt || 0))}
                                  </Typography>
                                </Stack>
                              }
                              secondary={
                                <Typography variant="caption" color="text.secondary" noWrap sx={{ display: 'block', minWidth: 0 }}>
                                  （草稿）
                                </Typography>
                              }
                            />
                          </ListItemButton>
                        ) : null}
                        {shownChats.map((c: any) => {
                          const on = !showPending && String(c?.id || '') === activeChatId
                          const raw = String(c?.lastMessagePreview || '').replace(/\s+/g, ' ').trim()
                          const snippet = raw.length > 40 ? raw.slice(0, 40) + '…' : raw
                          const time = controller.fmtTime(Number(c?.updatedAt || c?.createdAt || 0))
                    const indicatorKind = chatSessionRunIndicatorKind('workspace', String(activeChatTargetId || ''), c, on)
                          return (
                            <ListItemButton
                              key={String(c?.id || '')}
                              selected={on}
                              onClick={() => {
                                clearChatSessionRunNotice('workspace', String(activeChatTargetId || ''), String(c?.id || ''))
                                chatSwitch.requestSwitch(String(c?.id || ''))
                                closeChatPicker()
                              }}
                              onContextMenu={(e) =>
                                onChatContextMenu(e, 'workspace', String(activeChatTargetId || ''), String(c?.id || ''), String(c?.title || '工作区会话'))
                              }
                              sx={SOFT_POPOVER_ITEM_TOP_SX}
                            >
                              <ListItemText
                                sx={{ minWidth: 0 }}
                                primary={
                                  <Stack direction="row" alignItems="center" spacing={1} sx={{ minWidth: 0 }}>
                                    <Typography sx={{ fontWeight: 900, fontSize: 13, flex: 1, minWidth: 0 }} noWrap>
                                      {String(c?.title || '工作区会话')}
                                    </Typography>
                                    <Typography variant="caption" color="text.secondary">
                                      {time}
                                    </Typography>
                                    {indicatorKind ? <ChatSessionRunIndicator kind={indicatorKind} /> : null}
                                  </Stack>
                                }
                                secondary={
                                  <Typography variant="caption" color="text.secondary" noWrap sx={{ display: 'block', minWidth: 0 }}>
                                    {snippet || '（空）'}
                                  </Typography>
                                }
                              />
                            </ListItemButton>
                          )
                        })}
                      </List>
                    )
                  }

                  const role = activeRole
                  if (!role) {
                    return (
                      <Box sx={{ p: 2 }}>
                        <Typography variant="body2" color="text.secondary">
                          先选择角色
                        </Typography>
                      </Box>
                    )
                  }
                  const box = data?.chatsByRole?.[String(role.id)]
                  const chats = sortChatListItemsForDisplay(Array.isArray(box?.chatMetas) && box.chatMetas.length ? box.chatMetas : Array.isArray(box?.chats) ? box.chats : [])
                  const activeChatId = String(box?.activeChatId || '')
                  const pendingChat = s?.pendingChat && String(s.pendingChat?.roleId || '') === String(role.id) ? s.pendingChat.chat : null
                  const hasPending = !!pendingChat
                  const showPending = hasPending && chatHistoryMatchesSearch(pendingChat, '新聊天', chatPickerSearchText)
                  const shownChats = chats.filter((c: any) => chatHistoryMatchesSearch(c, '新聊天', chatPickerSearchText)).slice(0, chatHistoryVisibleCount)
                  if (!showPending && !shownChats.length) {
                    return (
                      <Box sx={{ p: 2 }}>
                        <Typography variant="body2" color="text.secondary">
                          没有匹配的会话
                        </Typography>
                      </Box>
                    )
                  }
                  return (
                    <List dense sx={SOFT_POPOVER_LIST_SX}>
                      {showPending ? (
                        <ListItemButton selected sx={SOFT_POPOVER_ITEM_TOP_SX}>
                          <ListItemText
                            sx={{ minWidth: 0 }}
                            primary={
                              <Stack direction="row" alignItems="center" spacing={1} sx={{ minWidth: 0 }}>
                                <Typography sx={{ fontWeight: 900, fontSize: 13, flex: 1, minWidth: 0 }} noWrap>
                                  {String(pendingChat?.title || '新聊天')}（未发送）
                                </Typography>
                                <Typography variant="caption" color="text.secondary">
                                  {controller.fmtTime(Number(pendingChat?.updatedAt || pendingChat?.createdAt || 0))}
                                </Typography>
                              </Stack>
                            }
                            secondary={
                              <Typography variant="caption" color="text.secondary" noWrap sx={{ display: 'block', minWidth: 0 }}>
                                （草稿）
                              </Typography>
                            }
                          />
                        </ListItemButton>
                      ) : null}
                      {shownChats.map((c: any) => {
                        const on = !showPending && String(c?.id || '') === activeChatId
                        const raw = String(c?.lastMessagePreview || '').replace(/\s+/g, ' ').trim()
                        const snippet = raw.length > 40 ? raw.slice(0, 40) + '…' : raw
                        const time = controller.fmtTime(Number(c?.updatedAt || c?.createdAt || 0))
                        const indicatorKind = chatSessionRunIndicatorKind('role', String(role?.id || ''), c, on)
                        return (
                          <ListItemButton
                            key={String(c?.id || '')}
                            selected={on}
                            onClick={() => {
                              clearChatSessionRunNotice('role', String(role?.id || ''), String(c?.id || ''))
                              chatSwitch.requestSwitch(String(c?.id || ''))
                              closeChatPicker()
                            }}
                            onContextMenu={(e) =>
                              onChatContextMenu(e, 'role', String(role?.id || ''), String(c?.id || ''), String(c?.title || '新聊天'))
                            }
                            sx={SOFT_POPOVER_ITEM_TOP_SX}
                          >
                            <ListItemText
                              sx={{ minWidth: 0 }}
                              primary={
                                <Stack direction="row" alignItems="center" spacing={1} sx={{ minWidth: 0 }}>
                                  <Typography sx={{ fontWeight: 900, fontSize: 13, flex: 1, minWidth: 0 }} noWrap>
                                    {String(c?.title || '新聊天')}
                                  </Typography>
                                   <Typography variant="caption" color="text.secondary">
                                     {time}
                                   </Typography>
                                   {indicatorKind ? <ChatSessionRunIndicator kind={indicatorKind} /> : null}
                                 </Stack>
                               }
                              secondary={
                                <Typography variant="caption" color="text.secondary" noWrap sx={{ display: 'block', minWidth: 0 }}>
                                  {snippet || '（空）'}
                                </Typography>
                              }
                            />
                          </ListItemButton>
                        )
                      })}
                    </List>
                  )
                })()}
              </CustomScrollArea>

              <CustomScrollArea hostSx={{ width: 420, maxHeight: '70vh', flex: '0 0 420px' }} scrollSx={{ maxHeight: '70vh' }}>
                <Box sx={SOFT_POPOVER_HEADER_SX}>
                  <Tooltip title="返回历史记录">
                    <IconButton size="small" onClick={() => setChatPickerView('history')}>
                      <ArrowBackRoundedIcon fontSize="inherit" />
                    </IconButton>
                  </Tooltip>
                  <Typography sx={{ fontWeight: 800, flex: 1 }}>收藏夹</Typography>
                  <Tooltip title={favoriteSearchOpen ? '关闭搜索' : '搜索'}>
                    <IconButton
                      size="small"
                      onClick={() => {
                        setFavoriteSearchOpen((p) => !p)
                        if (favoriteSearchOpen) setFavoriteSearchText('')
                      }}
                    >
                      {favoriteSearchOpen ? <CloseIcon fontSize="inherit" /> : <SearchIcon fontSize="inherit" />}
                    </IconButton>
                  </Tooltip>
                  <Tooltip title="折叠全部">
                    <span>
                      <IconButton size="small" onClick={collapseAllFavoriteFolders} disabled={!favoriteFolders.length}>
                        <UnfoldLessIcon fontSize="inherit" />
                      </IconButton>
                    </span>
                  </Tooltip>
                  <Tooltip title="新建文件夹">
                    <IconButton size="small" onClick={() => openCreateFavoriteFolder('')}>
                      <AddIcon fontSize="inherit" />
                    </IconButton>
                  </Tooltip>
                </Box>
                <Collapse in={favoriteSearchOpen}>
                  <Box sx={{ px: 1.5, pb: 1 }}>
                    <TextField
                      inputRef={favoriteSearchInputRef}
                      fullWidth
                      size="small"
                      placeholder="搜索收藏夹…"
                      value={String(favoriteSearchText || '')}
                      onChange={(e) => setFavoriteSearchText(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Escape') {
                          e.preventDefault()
                          setFavoriteSearchOpen(false)
                          setFavoriteSearchText('')
                        }
                      }}
                      InputProps={{
                        startAdornment: (
                          <InputAdornment position="start">
                            <SearchIcon fontSize="small" />
                          </InputAdornment>
                        ),
                      }}
                    />
                  </Box>
                </Collapse>
                {!favoriteFolders.length ? (
                  <Box sx={{ p: 2.5 }}>
                    <Typography variant="body2" color="text.secondary">
                      还没有收藏夹，点击右上角加号新建。
                    </Typography>
                  </Box>
                ) : (
                  <List dense sx={SOFT_POPOVER_LIST_SX}>{renderFavoriteFolderTree('', 0)}</List>
                )}
              </CustomScrollArea>
            </Box>
          </Box>
        </Popover>

        <Popover
          open={!!favoriteFolderMenu.folderId}
          onClose={closeFavoriteFolderMenu}
          anchorReference="anchorPosition"
          anchorPosition={favoriteFolderMenu.folderId ? { top: favoriteFolderMenu.y, left: favoriteFolderMenu.x } : undefined}
          transformOrigin={{ vertical: 'top', horizontal: 'left' }}
        >
          <Box sx={{ minWidth: 220, p: 0.5 }}>
            <MenuItem
              onClick={() => {
                const fid = String(favoriteFolderMenu.folderId || '')
                if (!fid) return
                openCreateFavoriteFolder(fid)
                closeFavoriteFolderMenu()
              }}
              sx={{ gap: 1 }}
            >
              <AddIcon fontSize="small" />
              新建子文件夹
            </MenuItem>
            <MenuItem
              onClick={() => {
                openCreateFavoriteFolder(String(favoriteFolderMenu.parentId || ''))
                closeFavoriteFolderMenu()
              }}
              sx={{ gap: 1 }}
            >
              <AddIcon fontSize="small" />
              新建同级文件夹
            </MenuItem>
            <MenuItem
              onClick={() => {
                const fid = String(favoriteFolderMenu.folderId || '')
                if (!fid) return
                closeFavoriteFolderMenu()
                setMoveFavoriteFolderDialog({ open: true, folderId: fid, parentId: String(favoriteFolderMenu.parentId || '') })
              }}
              sx={{ gap: 1 }}
            >
              <DriveFileMoveOutlinedIcon fontSize="small" />
              移动到...
            </MenuItem>
            <MenuItem onClick={() => openRenameFavoriteFolder(favoriteFolderMenu.folderId)} sx={{ gap: 1 }}>
              <EditOutlinedIcon fontSize="small" />
              重命名
            </MenuItem>
            <MenuItem
              onClick={() => {
                closeFavoriteFolderMenu()
                setConfirmClearFavoriteFolder({ open: true, folderId: String(favoriteFolderMenu.folderId || '') })
              }}
              sx={{ gap: 1 }}
            >
              <DeleteOutlineIcon fontSize="small" />
              清空当前文件夹收藏
            </MenuItem>
            <MenuItem onClick={() => openDeleteFavoriteFolderConfirm(favoriteFolderMenu.folderId, 'keep')} sx={{ gap: 1 }}>
              <DeleteOutlineIcon fontSize="small" />
              删除文件夹（内容保留）
            </MenuItem>
            <MenuItem onClick={() => openDeleteFavoriteFolderConfirm(favoriteFolderMenu.folderId, 'tree')} sx={{ gap: 1 }}>
              <DeleteOutlineIcon fontSize="small" />
              删除文件夹及其子内容
            </MenuItem>
          </Box>
        </Popover>

        <Popover
          open={!!favoriteChatMenu.chatId}
          onClose={closeFavoriteChatMenu}
          anchorReference="anchorPosition"
          anchorPosition={favoriteChatMenu.chatId ? { top: favoriteChatMenu.y, left: favoriteChatMenu.x } : undefined}
          transformOrigin={{ vertical: 'top', horizontal: 'left' }}
        >
          <Box sx={{ minWidth: 200, p: 0.5 }}>
            <MenuItem
              disabled={!favoriteChatMenu.chatId || !favoriteChatMenu.targetId || s.loading}
              onClick={() => {
                const { targetKind, targetId, chatId, title } = favoriteChatMenu
                closeFavoriteChatMenu()
                openFavoriteDialog(targetKind, targetId, chatId, title)
              }}
              sx={{ gap: 1 }}
            >
              <StarBorderRoundedIcon fontSize="small" />
              收藏到...
            </MenuItem>
            <MenuItem
              disabled={!favoriteChatMenu.chatId || !favoriteChatMenu.targetId || !favoriteChatMenu.folderId || s.loading}
              onClick={() => {
                const { folderId, targetKind, targetId, chatId } = favoriteChatMenu
                const currentIds = Array.isArray(controller.actions.getChatFavoriteFolderIds?.(targetKind, targetId, chatId))
                  ? controller.actions.getChatFavoriteFolderIds(targetKind, targetId, chatId)
                  : []
                closeFavoriteChatMenu()
                controller.actions.setChatFavoriteFolders?.(
                  targetKind,
                  targetId,
                  chatId,
                  currentIds.filter((id: any) => String(id || '') !== String(folderId || '')),
                )
              }}
              sx={{ gap: 1 }}
            >
              <DeleteOutlineIcon fontSize="small" />
              从当前文件夹移除
            </MenuItem>
            <MenuItem
              disabled={!favoriteChatMenu.chatId || !favoriteChatMenu.targetId || s.loading}
              onClick={() => {
                const { targetKind, targetId, chatId, title } = favoriteChatMenu
                closeFavoriteChatMenu()
                setEditingChatTitle({ targetKind, targetId, chatId, text: String(title ?? '') })
              }}
              sx={{ gap: 1 }}
            >
              <EditOutlinedIcon fontSize="small" />
              编辑标题
            </MenuItem>
            <MenuItem
              disabled={
                !favoriteChatMenu.chatId ||
                !favoriteChatMenu.targetId ||
                s.loading ||
                isSendingThisChat(favoriteChatMenu.targetKind, favoriteChatMenu.targetId, favoriteChatMenu.chatId)
              }
              onClick={() => {
                const { targetKind, targetId, chatId } = favoriteChatMenu
                closeFavoriteChatMenu()
                Promise.resolve()
                  .then(() => {
                    if (targetKind === 'group') return controller.actions.aiGenerateGroupChatTitle?.(targetId, chatId)
                    if (targetKind === 'workspace') return controller.actions.aiGenerateWorkspaceChatTitle?.(targetId, chatId)
                    return controller.actions.aiGenerateChatTitle?.(targetId, chatId)
                  })
                  .catch(() => {})
              }}
              sx={{ gap: 1 }}
            >
              <AutorenewIcon fontSize="small" />
              AI 生成标题
            </MenuItem>
          </Box>
        </Popover>

        <Popover
          open={!!chatMenu.chatId}
          onClose={closeChatMenu}
          anchorReference="anchorPosition"
          anchorPosition={chatMenu.chatId ? { top: chatMenu.y, left: chatMenu.x } : undefined}
          transformOrigin={{ vertical: 'top', horizontal: 'left' }}
        >
          <Box sx={{ minWidth: 180, p: 0.5 }}>
            <MenuItem
              disabled={!chatMenu.chatId || !chatMenu.targetId || s.loading}
              onClick={() => {
                const { targetKind, targetId, chatId, title } = chatMenu
                closeChatMenu()
                setEditingChatTitle({ targetKind, targetId, chatId, text: String(title ?? '') })
              }}
              sx={{ gap: 1 }}
            >
              <EditOutlinedIcon fontSize="small" />
              编辑标题
            </MenuItem>
            <MenuItem
              disabled={!chatMenu.chatId || !chatMenu.targetId || s.loading || isSendingThisChat(chatMenu.targetKind, chatMenu.targetId, chatMenu.chatId)}
              onClick={() => {
                const { targetKind, targetId, chatId } = chatMenu
                closeChatMenu()
                Promise.resolve()
                  .then(() => {
                    if (targetKind === 'group') return controller.actions.aiGenerateGroupChatTitle?.(targetId, chatId)
                    if (targetKind === 'workspace') return controller.actions.aiGenerateWorkspaceChatTitle?.(targetId, chatId)
                    return controller.actions.aiGenerateChatTitle?.(targetId, chatId)
                  })
                  .catch(() => {})
              }}
              sx={{ gap: 1 }}
            >
              <AutorenewIcon fontSize="small" />
              AI 生成标题
            </MenuItem>
            <MenuItem
              disabled={!chatMenu.chatId || !chatMenu.targetId || s.loading}
              onClick={() => {
                const { targetKind, targetId, chatId, title } = chatMenu
                closeChatMenu()
                openFavoriteDialog(targetKind, targetId, chatId, title)
              }}
              sx={{ gap: 1 }}
            >
              <StarBorderRoundedIcon fontSize="small" />
              收藏到...
            </MenuItem>
            <MenuItem
              disabled={!chatMenu.chatId || !chatMenu.targetId || s.loading || isSendingThisChat(chatMenu.targetKind, chatMenu.targetId, chatMenu.chatId)}
              onClick={() => {
                const { targetKind, targetId, chatId } = chatMenu
                closeChatMenu()
                setConfirmDelChat({ targetKind, targetId, chatId })
              }}
              sx={{ gap: 1 }}
            >
              <DeleteOutlineIcon fontSize="small" />
              删除
            </MenuItem>
          </Box>
        </Popover>

        <Dialog open={createFavoriteFolder.open} onClose={closeCreateFavoriteFolder} maxWidth="xs" fullWidth>
          <DialogTitle>{createFavoriteFolder.parentId ? '新建子文件夹' : '新建文件夹'}</DialogTitle>
          <DialogContent>
            <Stack spacing={1.25} sx={{ pt: 0.5 }}>
              <TextField
                autoFocus
                size="small"
                label="文件夹名"
                value={createFavoriteFolder.name}
                onChange={(e) => setCreateFavoriteFolder((p) => ({ ...p, name: e.target.value }))}
                placeholder="例如：工作 / 灵感 / 需求"
                fullWidth
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    submitCreateFavoriteFolder()
                  }
                }}
              />
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={closeCreateFavoriteFolder}>取消</Button>
            <Button variant="contained" onClick={submitCreateFavoriteFolder} disabled={!String(createFavoriteFolder.name || '').trim() || s.loading}>
              创建
            </Button>
          </DialogActions>
        </Dialog>

        <Dialog open={renameFavoriteFolder.open} onClose={closeRenameFavoriteFolder} maxWidth="xs" fullWidth>
          <DialogTitle>重命名文件夹</DialogTitle>
          <DialogContent>
            <Stack spacing={1.25} sx={{ pt: 0.5 }}>
              <TextField
                autoFocus
                size="small"
                label="文件夹名"
                value={renameFavoriteFolder.name}
                onChange={(e) => setRenameFavoriteFolder((p) => ({ ...p, name: e.target.value }))}
                fullWidth
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    submitRenameFavoriteFolder()
                  }
                }}
              />
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={closeRenameFavoriteFolder}>取消</Button>
            <Button variant="contained" onClick={submitRenameFavoriteFolder} disabled={!String(renameFavoriteFolder.name || '').trim() || s.loading}>
              保存
            </Button>
          </DialogActions>
        </Dialog>

        <Dialog open={confirmDeleteFavoriteFolder.open} onClose={closeDeleteFavoriteFolderConfirm} maxWidth="xs" fullWidth>
          <DialogTitle>{confirmDeleteFavoriteFolder.mode === 'tree' ? '删除文件夹及其子内容？' : '删除文件夹（内容保留）？'}</DialogTitle>
          <DialogContent>
            <Typography variant="body2" color="text.secondary">
              {confirmDeleteFavoriteFolder.mode === 'tree'
                ? '这会删除当前文件夹、它的子文件夹，以及这一整棵树里的收藏关系。'
                : '这会删除当前文件夹，并尽量保留内容：子文件夹会上移到上一层；当前文件夹里的收藏会移动到父文件夹。若它是带收藏的顶层文件夹，下一步会让你选择迁移目标。'}
            </Typography>
          </DialogContent>
          <DialogActions>
            <Button onClick={closeDeleteFavoriteFolderConfirm}>取消</Button>
            <Button variant="contained" color="error" onClick={submitDeleteFavoriteFolder} disabled={!confirmDeleteFavoriteFolder.folderId || s.loading}>
              删除
            </Button>
          </DialogActions>
        </Dialog>

        <Dialog open={moveFavoriteFolderContents.open} onClose={closeMoveFavoriteFolderContents} maxWidth="xs" fullWidth>
          <DialogTitle>选择内容迁移目标</DialogTitle>
          <DialogContent>
            <Stack spacing={1.25} sx={{ pt: 0.5 }}>
              <Typography variant="body2" color="text.secondary">
                这个顶层文件夹里有收藏内容。删除前，请先选择一个文件夹来承接这些内容和子文件夹。
              </Typography>
              {!favoriteFolders.filter((f: any) => String(f?.id || '') !== String(moveFavoriteFolderContents.folderId || '')).length ? (
                <Box sx={{ py: 1 }}>
                  <Typography variant="body2" color="text.secondary">
                    当前没有可承接内容的其他文件夹。
                  </Typography>
                </Box>
              ) : (
                <List dense sx={{ py: 0 }}>
                  {renderFavoriteFolderSinglePicker(
                    String(moveFavoriteFolderContents.targetFolderId || ''),
                    (folderId) => setMoveFavoriteFolderContents((p) => ({ ...p, targetFolderId: folderId })),
                    { filter: (folder: any) => String(folder?.id || '') !== String(moveFavoriteFolderContents.folderId || '') },
                  )}
                </List>
              )}
              <Button variant="outlined" startIcon={<AddIcon />} onClick={() => openCreateFavoriteFolder('')} sx={{ alignSelf: 'flex-start' }}>
                新建文件夹
              </Button>
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={closeMoveFavoriteFolderContents}>取消</Button>
            <Button
              variant="contained"
              onClick={submitMoveFavoriteFolderContents}
              disabled={!String(moveFavoriteFolderContents.targetFolderId || '').trim() || s.loading}
            >
              确认迁移并删除
            </Button>
          </DialogActions>
        </Dialog>

        <Dialog open={confirmClearFavoriteFolder.open} onClose={closeConfirmClearFavoriteFolder} maxWidth="xs" fullWidth>
          <DialogTitle>清空当前文件夹收藏？</DialogTitle>
          <DialogContent>
            <Typography variant="body2" color="text.secondary">
              只会清空这个文件夹里直接挂着的聊天收藏，不会删除文件夹本身，也不会影响子文件夹。
            </Typography>
          </DialogContent>
          <DialogActions>
            <Button onClick={closeConfirmClearFavoriteFolder}>取消</Button>
            <Button variant="contained" color="error" onClick={submitClearFavoriteFolder} disabled={!confirmClearFavoriteFolder.folderId || s.loading}>
              清空
            </Button>
          </DialogActions>
        </Dialog>

        <Dialog open={moveFavoriteFolderDialog.open} onClose={closeMoveFavoriteFolderDialog} maxWidth="xs" fullWidth>
          <DialogTitle>移动到...</DialogTitle>
          <DialogContent>
            <Stack spacing={1.25} sx={{ pt: 0.5 }}>
              <Typography variant="body2" color="text.secondary">
                选择这个文件夹的新父文件夹。点“顶层”就是把它移动回最外层。
              </Typography>
              <List dense sx={{ py: 0 }}>
                {renderFavoriteFolderSinglePicker(
                  String(moveFavoriteFolderDialog.parentId || ''),
                  (folderId) => setMoveFavoriteFolderDialog((p) => ({ ...p, parentId: folderId })),
                  {
                    includeRoot: true,
                    filter: (folder: any) => {
                      const fid = String(folder?.id || '')
                      if (!fid || fid === String(moveFavoriteFolderDialog.folderId || '')) return false
                      return !collectFavoriteFolderSubtreeIds(String(moveFavoriteFolderDialog.folderId || '')).includes(fid)
                    },
                  },
                )}
              </List>
              <Button variant="outlined" startIcon={<AddIcon />} onClick={() => openCreateFavoriteFolder('')} sx={{ alignSelf: 'flex-start' }}>
                新建文件夹
              </Button>
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={closeMoveFavoriteFolderDialog}>取消</Button>
            <Button variant="contained" onClick={submitMoveFavoriteFolder} disabled={!moveFavoriteFolderDialog.folderId || s.loading}>
              确认移动
            </Button>
          </DialogActions>
        </Dialog>

        <Dialog open={favoriteDialog.open} onClose={closeFavoriteDialog} maxWidth="xs" fullWidth>
          <DialogTitle>收藏到文件夹</DialogTitle>
          <DialogContent>
            <Stack spacing={1.25} sx={{ pt: 0.5 }}>
              <Typography variant="body2" color="text.secondary">
                {String(favoriteDialog.title || '未命名会话')}
              </Typography>
              {!favoriteFolders.length ? (
                <Box sx={{ py: 1 }}>
                  <Typography variant="body2" color="text.secondary">
                    还没有收藏夹，请先新建文件夹。
                  </Typography>
                </Box>
              ) : (
                <List dense sx={{ py: 0 }}>{renderFavoriteFolderPicker('', 0)}</List>
              )}
              <Button variant="outlined" startIcon={<AddIcon />} onClick={() => openCreateFavoriteFolder('')} sx={{ alignSelf: 'flex-start' }}>
                新建文件夹
              </Button>
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={closeFavoriteDialog}>取消</Button>
            <Button variant="contained" onClick={saveFavoriteDialog} disabled={!favoriteDialog.targetId || !favoriteDialog.chatId || s.loading}>
              保存
            </Button>
          </DialogActions>
        </Dialog>

        <Dialog
          open={!!editingChatTitle.chatId}
          onClose={closeEditingChatTitle}
          maxWidth="xs"
          fullWidth
        >
          <DialogTitle>编辑会话标题</DialogTitle>
          <DialogContent>
            <TextField
              autoFocus
              fullWidth
              size="small"
              label="标题"
              placeholder="例如：需求讨论 / bug 复盘 / …"
              value={String(editingChatTitle.text ?? '')}
              onChange={(e) => setEditingChatTitle((p) => ({ ...p, text: e.target.value }))}
              onKeyDown={(e) => {
                if (e.key === 'Escape') closeEditingChatTitle()
                if (e.key === 'Enter') {
                  e.preventDefault()
                  void saveEditingChatTitle()
                }
              }}
              sx={{ mt: 1 }}
            />
          </DialogContent>
          <DialogActions>
            <Button onClick={closeEditingChatTitle}>取消</Button>
            <Button
              variant="contained"
              onClick={() => { void saveEditingChatTitle() }}
              disabled={!editingChatTitle.targetId || !editingChatTitle.chatId || s.loading}
            >
              保存
            </Button>
          </DialogActions>
        </Dialog>

        <Dialog
          open={!!confirmDelChat.chatId}
          onClose={() => setConfirmDelChat({ targetKind: 'role', targetId: '', chatId: '' })}
          maxWidth="xs"
          fullWidth
        >
          <DialogTitle>确认删除这个会话？</DialogTitle>
          <DialogContent>
            <Typography variant="body2" color="text.secondary">
              这会删除该会话下的全部消息记录，且不可恢复；同时会尝试删除该会话引用的本地图片文件（若其它会话仍引用则会保留）。
            </Typography>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setConfirmDelChat({ targetKind: 'role', targetId: '', chatId: '' })}>取消</Button>
            <Button
              variant="contained"
              color="error"
              onClick={() => {
                const { targetKind, targetId, chatId } = confirmDelChat
                setConfirmDelChat({ targetKind: 'role', targetId: '', chatId: '' })
                if (!targetId || !chatId) return
                if (targetKind === 'group') controller.actions.deleteGroupChat?.(targetId, chatId)
                else if (targetKind === 'workspace') controller.actions.deleteWorkspaceChat?.(targetId, chatId)
                else controller.actions.deleteChat?.(targetId, chatId)
              }}
              disabled={
                !confirmDelChat.targetId ||
                !confirmDelChat.chatId ||
                s.loading ||
                isSendingThisChat(confirmDelChat.targetKind, confirmDelChat.targetId, confirmDelChat.chatId)
              }
            >
              删除
            </Button>
          </DialogActions>
        </Dialog>
          </>
        ) : (
          <PluginSettingsPage
            controller={controller}
            loading={!!s.loading}
            data={data}
            roles={roles}
            groups={groups}
            workspaces={workspaces}
            providers={providers}
            modelGroups={(s as any).modelGroups}
            models={s.models}
            tools={(s as any).tools}
            modelRequestConfig={(s as any).modelRequestConfig}
            bootstrap={bootstrap}
            releaseBusy={releaseBusy}
            releaseView={releaseView}
            onReleaseRead={onReleaseRead}
            onReleaseRefresh={onReleaseRefresh}
            accessSettings={(s as any)?.accessSettings}
            hookPrompts={hookPrompts}
            placeholders={placeholders}
            systemPlugins={systemPlugins}
            draft={s.draft}
            activeRoleId={String(s.draft?.activeRoleId || '')}
            activeWorkspaceId={String((s.draft as any)?.activeWorkspaceId || '')}
            activeTargetKind={activeTargetKind}
            tab={settingsTab}
            onTabChange={setSettingsTab}
            dataDirectory={dataDirectory}
          />
        )}
        </Box>

        <ProvidersDialog open={s.modal === 'providers'} controller={controller} providers={providers} draft={s.draft} models={s.models} />
        <RoleDialog open={s.modal === 'role'} controller={controller} providers={providers} modelGroups={modelGroups} draft={s.draft} models={s.models} tools={(s as any).tools} hookPrompts={hookPrompts} />
        <GroupDialog open={s.modal === 'group'} controller={controller} roles={roles} draft={s.draft} />
        <WorkspaceDialog open={s.modal === 'workspace'} controller={controller} draft={s.draft} />
        <ConfirmDialog open={s.modal === 'confirm'} controller={controller} draft={s.draft} roles={roles} groups={groups} providers={providers} workspaces={workspaces} />
        <MermaidDialog open={s.modal === 'mermaid'} controller={controller} mermaid={s.mermaid} />
        <ImageDialog open={s.modal === 'image'} controller={controller} viewer={s.imageViewer} />
      </Box>
    </ThemeProvider>
  )
}

