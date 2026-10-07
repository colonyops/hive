import { onBeforeUnmount, onMounted } from 'vue'
import IconInfo from '~icons/lucide/info'
import IconMessagesSquare from '~icons/lucide/messages-square'
import IconPencil from '~icons/lucide/pencil'
import IconPinOff from '~icons/lucide/pin-off'
import IconPlay from '~icons/lucide/play'
import IconPlus from '~icons/lucide/plus'
import IconRecycle from '~icons/lucide/recycle'
import IconSquare from '~icons/lucide/square'
import IconTerminal from '~icons/lucide/terminal'
import IconTrash from '~icons/lucide/trash-2'
import { useCommands, useShellEscape, type Command } from '../../composables/useCommands'
import type { TerminalWindowTab } from '../../composables/useTerminalWindows'
import { activePaneOf } from '../../lib/terminalLayout'
import { paneMayAutoFocus, setTerminalTreeHandles } from '../../lib/terminalTree'
import type { TerminalSessionRow } from '../../stores/useTerminalSessions'
import type { MenuActionEntry, MenuEntry } from '../../types/menu'
import type { TerminalModeContext } from './terminalModeContext'
import { windowKey } from './useTreeKeyboardNav'

function actionsOf(entries: MenuEntry[]): MenuActionEntry[] {
  return entries.filter((entry): entry is MenuActionEntry => entry.kind === 'action')
}

/**
 * The palette rows for the attached session's own operations, under the
 * session's name. Window and attach rows are registered at App level instead
 * (useAppPaletteRows), since a fresh launch must list them before this async
 * component has mounted.
 */
export function attachedSessionCommands(context: TerminalModeContext, attached: TerminalSessionRow): Command[] {
  const { tree, ops, attach, sessions } = context
  const hive = tree.isHiveSession(attached)
  const scratch = tree.isScratch(attached)
  const chat = tree.isChat(attached)
  const live = attached.state === 'active'
  const running = tree.rowRunning(attached)
  const terminal = (hive && live) || scratch
  const agents = hive && live && running && !ops.agentWindowBusy.value ? ops.agentProfiles.value : []
  // A window action acts on the window on screen; the row menu carries the other
  // windows' copies. The hint names the window, because an action's label says
  // what it does, not what it does it to.
  const activeWindow = hive ? tree.windowRowsFor(attached).find((win) => win.active) : undefined
  const rows: (Command | false)[] = [
    terminal &&
      !running && {
        id: 'terminal:session:start',
        title: scratch ? 'Start terminal' : 'Start session',
        keywords: ['session', 'run', 'launch'],
        icon: IconPlay,
        run: () => void attach.startSession(attached.slug),
      },
    terminal && {
      id: 'terminal:session:kill',
      title: 'Kill terminal…',
      keywords: ['session', 'stop'],
      icon: IconSquare,
      run: () => ops.requestKill(attached),
    },
    ...agents.map((agent) => ({
      id: `terminal:session:agent:${agent}`,
      title: `New ${agent} agent`,
      keywords: ['peer', 'window', 'shared', 'checkout'],
      icon: IconPlus,
      run: () => void ops.newAgentWindowIn(attached, agent),
    })),
    hive && {
      id: 'terminal:session:detail',
      title: 'Session details…',
      keywords: ['session', 'info'],
      icon: IconInfo,
      run: () => void sessions.openDetail(attached),
    },
    hive && {
      id: 'terminal:session:rename',
      title: 'Rename session…',
      keywords: ['session'],
      icon: IconPencil,
      run: () => sessions.requestRename(attached),
    },
    hive &&
      live && {
        id: 'terminal:session:recycle',
        title: 'Recycle session…',
        keywords: ['session', 'reset'],
        icon: IconRecycle,
        run: () => void sessions.requestRecycle(attached),
      },
    hive && {
      id: 'terminal:session:delete',
      title: 'Delete session…',
      keywords: ['session', 'remove'],
      icon: IconTrash,
      run: () => void sessions.requestDelete(attached),
    },
    ...(hive ? actionsOf(ops.sessionActionEntries.value) : []).map((entry) => ({
      id: `terminal:session:${entry.id}`,
      title: entry.label,
      keywords: ['session', 'action'],
      iconName: entry.iconName,
      iconColor: entry.iconColor,
      run: () => ops.runSessionAction(attached, entry.id),
    })),
    ...(activeWindow
      ? actionsOf(ops.windowActionEntries.value).map((entry) => ({
          id: `terminal:window:action:${entry.id}`,
          title: entry.label,
          keywords: ['window', 'action', activeWindow.name],
          iconName: entry.iconName,
          iconColor: entry.iconColor,
          hint: activeWindow.name,
          run: () => ops.runWindowAction(attached, activeWindow.windowId, entry.id),
        }))
      : []),
    chat && {
      id: 'terminal:chat:open-in-agents',
      title: 'Open in Chats',
      keywords: ['chat', 'chats', 'agents'],
      icon: IconMessagesSquare,
      scope: 'goto',
      run: () => ops.openChatInAgents(attached),
    },
    chat && {
      id: 'terminal:chat:unpin',
      title: 'Unpin from Code',
      keywords: ['chat', 'pin'],
      icon: IconPinOff,
      run: () => tree.unpinSlug(attached.slug),
    },
  ]
  return rows
    .filter((row) => row !== false)
    .map((row) => ({ scope: 'actions', ...row, group: attached.name, order: -3 }))
}

/**
 * Everything outside the Code view's own DOM that reaches into it: the palette
 * rows, the `!` shell escape, and the `terminal.*` keymap handles. The mode is
 * mounted once and only hidden, so scope disposal never fires on a trip to the
 * hub; the registrations gate on `active` instead.
 */
export function useTerminalCommands(context: TerminalModeContext, active: () => boolean): void {
  const { pool, tree, nav, ops } = context
  const { current, activeSlug } = pool

  useCommands(() => {
    const attached = tree.attachedRow.value
    return active() && attached ? attachedSessionCommands(context, attached) : []
  })

  // `!` opens a window on the attached session running the rest of the line: a
  // shell in that checkout, beside the others, that outlives the command.
  useShellEscape((line) => {
    const attached = tree.attachedRow.value
    if (!active() || !attached) return []
    return [
      {
        id: 'shell:run',
        title: `Run: ${line}`,
        keywords: ['shell', 'terminal', 'window', 'run'],
        icon: IconTerminal,
        hint: `new window in ${attached.name}`,
        run: () => void current.value?.newWindow(line),
      },
    ]
  }, active)

  // A chord that names a window is an intent to type in it, so the tree cursor
  // follows and the pane may take focus.
  function goToWindow(tab: TerminalWindowTab | undefined): void {
    if (!tab) return
    const row = tree.attachedRow.value
    if (row) nav.cursorRequest.value = windowKey(row, tab.windowId)
    paneMayAutoFocus.value = true
    void current.value?.select(tab.windowId)
  }

  // Wrapping, where the tree's walk clamps: a session's windows are a ring in
  // every terminal emulator.
  function relativeWindow(delta: number): TerminalWindowTab | undefined {
    const tabs = current.value?.tabs.value ?? []
    const at = tabs.findIndex((tab) => tab.windowId === current.value?.activeWindowId.value)
    if (at < 0) return undefined
    return tabs[(at + delta + tabs.length) % tabs.length]
  }

  function activeTab(): TerminalWindowTab | undefined {
    return current.value?.tabs.value.find((tab) => tab.windowId === current.value?.activeWindowId.value)
  }

  onMounted(() =>
    setTerminalTreeHandles({
      focusTree: nav.focusTree,
      focusFilter: nav.focusFilter,
      focusPane: (): void => current.value?.focusActive(),
      // Counted along the strip rather than clamped: ⌘3 means the third window.
      selectWindow: (position: number): void => goToWindow(current.value?.tabs.value[position - 1]),
      stepWindow: (delta: number): void => goToWindow(relativeWindow(delta)),
      newWindow: (): void => {
        const row = tree.attachedRow.value
        if (!row) return
        paneMayAutoFocus.value = true
        // tmux names the window, so there is no key to point the cursor at yet;
        // it falls back to the active window, which is the new one once it lands.
        nav.cursorRequest.value = ''
        void ops.newWindowIn(row)
      },
      closeWindow: (): void => {
        const windowId = current.value?.activeWindowId.value
        if (windowId) void ops.requestCloseWindow(activeSlug.value, windowId, activeTab()?.name ?? '')
      },
      splitPane: (direction): void => {
        paneMayAutoFocus.value = true
        void current.value?.splitPane(direction)
      },
      closePane: (): void => {
        const tab = activeTab()
        const pane = tab && activePaneOf(tab)
        if (tab && pane) void ops.requestClosePane(activeSlug.value, pane.paneId, tab.name)
      },
      zoomPane: (): void => {
        paneMayAutoFocus.value = true
        void current.value?.zoomPane()
      },
      focusPaneDirection: (direction): void => {
        paneMayAutoFocus.value = true
        void current.value?.focusPane(direction)
      },
      find: (): void => current.value?.openSearch(),
    }),
  )
  onBeforeUnmount(() => setTerminalTreeHandles(null))
}
