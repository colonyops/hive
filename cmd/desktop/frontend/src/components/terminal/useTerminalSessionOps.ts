import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { SessionLaunchOptions } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type { useConfirmation } from '../../composables/useConfirmation'
import { useTerminalActions } from '../../composables/useTerminalActions'
import { useWailsEvent } from '../../composables/useWailsEvent'
import { appErrorMessage, errorText } from '../../lib/appError'
import type { TerminalClient, WindowForeground } from '../../lib/terminalClient'
import { useTerminalAvailability } from '../../stores/useTerminalAvailability'
import type { TerminalSessionRow } from '../../stores/useTerminalSessions'
import type { TerminalAttach } from './useTerminalAttach'
import type { TerminalPool } from './useTerminalPool'
import type { TerminalTree } from './useTerminalTree'

export type TerminalSessionOps = ReturnType<typeof useTerminalSessionOps>

/**
 * What a tree row, a keybinding, or the palette does to a session other than
 * attach it: kill it, add or close its windows and panes, and run the configured
 * terminal actions. A row can act on a session that is not the attached one, so
 * a failure here has no session to report through and lands in `treeError`.
 */
export function useTerminalSessionOps(options: {
  pool: TerminalPool
  tree: TerminalTree
  attach: TerminalAttach
  confirmation: ReturnType<typeof useConfirmation>
  active: () => boolean
}) {
  const { pool, tree, attach, confirmation } = options
  const router = useRouter()
  const { client } = useTerminalAvailability()
  const actions = useTerminalActions()
  useWailsEvent('actions:updated', () => void actions.load())
  onMounted(() => void actions.load())

  const treeError = ref('')
  const agentWindowBusy = ref(false)
  const agentProfiles = ref<string[]>([])
  const defaultAgentProfile = ref('')
  const agentProfilesLoading = ref(false)
  const agentProfilesFailed = ref(false)
  let agentProfilesRequest: Promise<void> | null = null

  function loadAgentProfiles(): Promise<void> {
    if (agentProfilesRequest) return agentProfilesRequest
    agentProfilesLoading.value = true
    agentProfilesFailed.value = false
    agentProfilesRequest = SessionLaunchOptions()
      .then((launch) => {
        agentProfiles.value = launch.agents ?? []
        defaultAgentProfile.value = launch.defaultAgent
      })
      .catch(() => {
        agentProfilesFailed.value = true
      })
      .finally(() => {
        agentProfilesLoading.value = false
        agentProfilesRequest = null
      })
    return agentProfilesRequest
  }

  watch(
    options.active,
    (active) => {
      if (active) void loadAgentProfiles()
    },
    { immediate: true },
  )

  // Killing ends the terminal and nothing else (the checkout, the record and the
  // work stay) but it stops whatever runs inside, so it is confirmed. The
  // scratch terminal *is* its tmux session, so there the dialog names the tabs.
  function requestKill(row: TerminalSessionRow): void {
    confirmation.request({
      title: 'Kill this terminal?',
      description: tree.isScratch(row)
        ? 'Every tab in the scratch terminal is closed and whatever is running in them stops. Starting it again opens an empty one.'
        : `The tmux session behind ${row.name} is killed, stopping the agent and anything else running in it. Its checkout and its work are untouched, and you can start it again from here.`,
      confirmLabel: 'Kill',
      onConfirm: () => killSession(row.slug),
    })
  }

  async function killSession(slug: string): Promise<void> {
    if (!client.value) return
    await client.value.kill(slug)
    // Re-attaching the killed session lands it on the start panel; a pooled one
    // that is not on screen is simply let go.
    if (slug === pool.activeSlug.value) void pool.pool.get(slug)?.reconnect()
    else pool.drop(slug)
    if (tree.showAllWindows.value) void tree.refreshListings(client.value, tree.attachable.value)
  }

  // Closing kills the tmux window or pane and everything in it, so one with a
  // foreground process is confirmed and a shell at its prompt closes on the
  // click. The state is read at the moment of the close: what a pane runs
  // changes without tmux announcing it. A read that failed is not evidence the
  // target is idle, so it counts as running.
  async function confirmClose(
    noun: 'tab' | 'pane',
    name: string,
    foreground: (transport: TerminalClient) => Promise<WindowForeground>,
    close: () => Promise<void>,
  ): Promise<void> {
    let state: WindowForeground = { running: true, command: '' }
    if (client.value) {
      try {
        state = await foreground(client.value)
      } catch {
        // Keep the running default.
      }
    }
    if (!state.running) {
      await close()
      return
    }
    confirmation.request({
      title: `Close this ${noun}?`,
      description: `${state.command || 'Something'} is still running ${name ? `in ${name}` : `in this ${noun}`}. Closing the ${noun} stops it.`,
      confirmLabel: `Close ${noun}`,
      onConfirm: close,
    })
  }

  async function requestCloseWindow(slug: string, windowId: string, name: string): Promise<void> {
    const pooled = pool.pool.get(slug)
    if (!pooled) return
    await confirmClose(
      'tab',
      name,
      (transport) => transport.windowForeground(slug, windowId),
      () => pooled.closeWindow(windowId),
    )
  }

  // tmux closes the window when its last pane closes, so a pane takes the tab's policy.
  async function requestClosePane(slug: string, paneId: string, name: string): Promise<void> {
    const pooled = pool.pool.get(slug)
    if (!pooled) return
    await confirmClose(
      'pane',
      name,
      (transport) => transport.paneForeground(slug, paneId),
      () => pooled.closePane(paneId),
    )
  }

  // Adding a window is a property of the session, not of what is on screen, so
  // a row offers it whether or not it is attached; a row with nothing running
  // starts instead. Every branch selects the row first: only the row's own pane
  // shows a start error. A pooled session goes through its own client, which
  // makes the new window active; an unattached running one takes the slug call,
  // and the attach the selection starts lists its windows fresh.
  async function newWindowIn(row: TerminalSessionRow): Promise<void> {
    const pooled = pool.pool.get(row.slug)
    if (pooled && pooled.status.value !== 'ended') {
      attach.selectSession(row.slug)
      await pooled.newWindow()
      return
    }
    if (!tree.rowRunning(row)) {
      attach.selectSession(row.slug)
      await attach.startSession(row.slug)
      return
    }
    const transport = client.value
    if (!transport) return
    attach.selectSession(row.slug)
    try {
      treeError.value = ''
      await transport.newWindow(row.slug)
    } catch (e) {
      treeError.value = appErrorMessage(e) || 'Could not create a window.'
    }
  }

  async function newAgentWindowIn(row: TerminalSessionRow, agent: string): Promise<void> {
    const transport = client.value
    if (!transport || agentWindowBusy.value) return
    agentWindowBusy.value = true
    treeError.value = ''
    try {
      const pooled = pool.pool.get(row.slug)
      if (pooled && pooled.status.value !== 'ended') {
        attach.selectSession(row.slug)
        await pooled.newAgentWindow(agent)
        return
      }
      const { windowId } = await transport.newAgentWindow(row.slug, agent)
      await router.push({ name: 'terminal', params: { slug: row.slug }, query: { window: windowId } })
      tree.sweepListings()
    } catch (e) {
      treeError.value = errorText(e, 'Could not start the agent.')
    } finally {
      agentWindowBusy.value = false
    }
  }

  // A configured action renders over the session it was invoked on, and the
  // scratch terminal has none to template from, so its tabs offer no actions.
  function rowHasWindowActions(row: TerminalSessionRow): boolean {
    return actions.hasWindowActions.value && !tree.isScratch(row)
  }

  function runSessionAction(row: TerminalSessionRow, entryID: string): void {
    void actions.select('session', entryID, { slug: row.slug, windowId: '' })
  }

  function runWindowAction(row: TerminalSessionRow, windowId: string, entryID: string): void {
    void actions.select('window', entryID, { slug: row.slug, windowId })
  }

  function openChatInAgents(row: TerminalSessionRow): void {
    const session = tree.recents.value.find((candidate) => candidate.slug === row.slug)
    if (!session) return
    void router.push({ name: 'agents', params: { workspace: session.workspace }, query: { chat: String(session.id) } })
  }

  return {
    actions,
    sessionActionEntries: actions.sessionEntries,
    windowActionEntries: actions.windowEntries,
    treeError,
    agentWindowBusy,
    agentProfiles,
    defaultAgentProfile,
    agentProfilesLoading,
    agentProfilesFailed,
    loadAgentProfiles,
    requestKill,
    requestCloseWindow,
    requestClosePane,
    newWindowIn,
    newAgentWindowIn,
    rowHasWindowActions,
    runSessionAction,
    runWindowAction,
    openChatInAgents,
  }
}
