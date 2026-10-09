import { nextTick, onMounted, ref, type Ref } from 'vue'
import { useCommandPalette } from '../composables/useCommands'
import { useLaunchers } from '../composables/useLaunchers'
import { useNewSession } from '../composables/useNewSession'
import { useReportDialog } from '../composables/useReportDialog'
import { useWailsEvent } from '../composables/useWailsEvent'
import {
  commandById,
  launcherActionID,
  launcherCommandID,
  terminalWindowPosition,
  type CommandContext,
} from '../keybindings/catalog'
import { focusAgentsFilter, focusAgentsList, focusAgentsPane } from '../lib/agentsTree'
import {
  closeTerminalPane,
  closeTerminalWindow,
  codePaneFocused,
  findInTerminal,
  focusTerminalFilter,
  focusTerminalPane,
  focusTerminalPaneDirection,
  focusTerminalTree,
  newTerminalWindow,
  selectTerminalWindow,
  splitTerminalPane,
  stepTerminalWindow,
  zoomTerminalPane,
} from '../lib/terminalTree'
import { useCodeInstallation } from '../stores/useCodeInstallation'
import { usePopupTerminal } from '../stores/usePopupTerminal'
import { useTerminalFont } from '../stores/useTerminalFont'
import { useTerminalSessions } from '../stores/useTerminalSessions'
import type { AppNavigation, FeedState } from './useAppNavigation'
import type { useAppMode } from './useAppMode'
import { useAppPaletteRows } from './useAppPaletteRows'
import type { useFeedCommands } from './useFeedCommands'
import type { useHubOverlays } from './useHubOverlays'

export interface AppCommandDeps {
  feed: FeedState
  nav: AppNavigation
  appMode: ReturnType<typeof useAppMode>
  overlays: ReturnType<typeof useHubOverlays>
  feedCommands: ReturnType<typeof useFeedCommands>
  onboardingActive: Ref<boolean>
  shellLoaded: Ref<boolean>
  openNewProfile: () => void
  focusFeedSearch: () => void
}

/**
 * One handler per bindable command id. The global keymap and the command
 * palette both dispatch through runCommand, so each command has one
 * implementation and the palette can show its live shortcut.
 */
export function useAppCommands(deps: AppCommandDeps) {
  const { feed, nav, appMode, overlays, feedCommands } = deps
  const { terminalActive, agentsActive, onScreenSessionSlug, terminalSidebarCollapsed, agentsSidebarCollapsed } =
    appMode
  const codeInstallation = useCodeInstallation()
  const palette = useCommandPalette()
  const report = useReportDialog()
  const newSession = useNewSession()
  const terminalFont = useTerminalFont()
  const { sessionRepository } = useTerminalSessions()

  // Mounted on first use and then only hidden, because hiding it must not end
  // the shell inside it. It opens where the terminal on screen is, else in the
  // user's home (ADR ephemeral-popup-terminals).
  const popupTerminal = usePopupTerminal()
  const popupTerminalMounted = ref(false)

  function togglePopupTerminal(): void {
    if (terminalActive.value && codeInstallation.installation.value === 'remote') codeInstallation.select('local')
    popupTerminalMounted.value = true
    popupTerminal.toggle({ sessionSlug: onScreenSessionSlug.value || undefined })
  }

  // A launcher is the pop-up opened straight into a program, where the terminal
  // on screen is. Outside its context the core refuses the launch anyway, and a
  // pop-up that only reports that is worse than none (ADR
  // quick-terminal-launchers-are-session-scoped).
  function toggleLauncher(actionID: string): void {
    const command = commandById.value.get(launcherCommandID(actionID))
    if (command && !contextActive(command.context)) return
    popupTerminalMounted.value = true
    popupTerminal.toggle({ launcher: actionID, sessionSlug: onScreenSessionSlug.value || undefined })
  }

  // Launchers are palette and keymap commands before any pop-up has opened.
  const launchers = useLaunchers()
  onMounted(() => {
    void launchers.refresh()
  })
  useWailsEvent('actions:updated', () => {
    void launchers.refresh()
  })

  // Reaching for a list or its filter also brings a collapsed sidebar back:
  // a hidden panel is not an answer to the chord.
  function revealSidebar(collapsed: Ref<boolean>, focus: () => void): void {
    collapsed.value = false
    void nextTick(focus)
  }

  const runMap: Record<string, () => void | Promise<void>> = {
    'feed.next': feed.selectNext,
    'feed.prev': feed.selectPrev,
    'feed.open-in-browser': feed.openSelectedInBrowser,
    'feed.toggle-unread': () => nav.navigateUnreadFilter(!feed.unreadOnly.value),
    'feed.toggle-preview': appMode.togglePreview,
    'feed.refresh': feed.refreshSources,
    'feed.toggle-selection': () => {
      if (feed.itemSelectionActive.value) feed.cancelItemSelection()
      else feed.enterItemSelection()
    },
    'feed.toggle-archive': async () => {
      if (feed.selectedItem.value) await feed.toggleArchive(feed.selectedItem.value)
    },
    'feed.mark-unread': async () => {
      if (feed.selectedItem.value) await feed.markItemUnread(feed.selectedItem.value, true)
    },
    'feed.mark-all-read': feedCommands.markSelectedFeedRead,
    'feed.mark-workspace-read': feedCommands.requestMarkWorkspaceRead,
    'palette.toggle': () => palette.toggle(),
    'report.open': () => {
      void report.reportProblem()
    },
    'report.bundle': report.openBundleDialog,
    'tasks.toggle': overlays.toggleTasks,
    'canvas.toggle': overlays.toggleCanvas,
    'action-runs.toggle': () => overlays.toggleActionRuns(),
    'terminal.popup.toggle': togglePopupTerminal,
    'terminal.toggle-sidebar': appMode.toggleSidebar,
    'terminal.focus-sidebar': () => revealSidebar(terminalSidebarCollapsed, focusTerminalTree),
    'terminal.focus-pane': focusTerminalPane,
    // One combo, dispatched on whichever surface is on screen.
    'view.focus-search': () => {
      if (appMode.feedNavActive.value) void nextTick(deps.focusFeedSearch)
      else if (terminalActive.value) revealSidebar(terminalSidebarCollapsed, focusTerminalFilter)
      else if (agentsActive.value) revealSidebar(agentsSidebarCollapsed, focusAgentsFilter)
    },
    'terminal.new-window': newTerminalWindow,
    'terminal.close-window': closeTerminalWindow,
    'terminal.next-window': () => stepTerminalWindow(1),
    'terminal.prev-window': () => stepTerminalWindow(-1),
    'terminal.split-right': () => splitTerminalPane('horizontal'),
    'terminal.split-down': () => splitTerminalPane('vertical'),
    'terminal.close-pane': closeTerminalPane,
    'terminal.zoom-pane': zoomTerminalPane,
    'terminal.find': findInTerminal,
    'terminal.text-size-increase': () => terminalFont.stepFontSize(1),
    'terminal.text-size-decrease': () => terminalFont.stepFontSize(-1),
    'terminal.text-size-reset': terminalFont.resetFontSize,
    'terminal.focus-pane-left': () => focusTerminalPaneDirection('left'),
    'terminal.focus-pane-right': () => focusTerminalPaneDirection('right'),
    'terminal.focus-pane-up': () => focusTerminalPaneDirection('up'),
    'terminal.focus-pane-down': () => focusTerminalPaneDirection('down'),
    'agents.focus-sidebar': () => revealSidebar(agentsSidebarCollapsed, focusAgentsList),
    'agents.focus-pane': focusAgentsPane,
    'session.new': () => {
      codeInstallation.select('local')
      void newSession.openBlank(sessionRepository(onScreenSessionSlug.value))
    },
    'view.go-remote-code': () => {
      codeInstallation.select('remote')
      appMode.setMode('terminal')
    },
    'view.go-local-code': () => {
      codeInstallation.select('local')
      appMode.setMode('terminal')
    },
    'window.hide': feed.hideWindow,
    'view.go-inbox': () => appMode.setMode('hub'),
    'view.go-code': () => appMode.setMode('terminal'),
    'view.go-chats': () => appMode.setMode('agents'),
    'settings.open': () => nav.openSettings('application'),
    'history.back': () => nav.router.back(),
    'history.forward': () => nav.router.forward(),
    'palette.keys': () => palette.openWithScope('keys'),
  }

  // Launchers and the numbered window jumps are one implementation each,
  // parameterised by what the id names, rather than an entry per command.
  function runCommand(id: string): void {
    const launcher = launcherActionID(id)
    if (launcher !== null) {
      toggleLauncher(launcher)
      return
    }
    const window = terminalWindowPosition(id)
    if (window !== null) {
      selectTerminalWindow(window)
      return
    }
    void runMap[id]?.()
  }

  function contextActive(context: CommandContext): boolean {
    switch (context) {
      case 'feed':
        return appMode.feedNavActive.value
      case 'terminal':
        return terminalActive.value && codeInstallation.installation.value === 'local'
      // Terminal mode with nothing attached is the session picker, which has
      // no more terminal to work with than the feed does.
      case 'terminal-session':
        return terminalActive.value && !!onScreenSessionSlug.value
      case 'terminal-pane':
        return terminalActive.value && !!onScreenSessionSlug.value && codePaneFocused()
      case 'agents':
        return agentsActive.value
      case 'sidebar':
        return appMode.canToggleSidebar.value
      // Excludes the session picker for terminal-session's reason: with no
      // emulator drawn, the chord would change a size nobody can see.
      case 'any-terminal':
        return (
          (terminalActive.value && !!onScreenSessionSlug.value) || agentsActive.value || popupTerminal.visible.value
        )
      case 'global':
        return true
    }
  }

  useAppPaletteRows(deps, runCommand, contextActive)

  return { runCommand, contextActive, popupTerminalMounted }
}
