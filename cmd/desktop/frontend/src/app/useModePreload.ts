import { useTimeoutFn, whenever } from '@vueuse/core'
import { createLogger } from '../lib/log'
import { useAgentSessionsAll } from '../stores/useAgentSessionsAll'
import { useAgentWorkspaces } from '../stores/useAgentWorkspaces'
import { useCodeViews } from '../stores/useCodeViews'
import { useTerminalAvailability } from '../stores/useTerminalAvailability'
import { useTerminalSessions } from '../stores/useTerminalSessions'
import { useTerminalShowWindows } from '../stores/useTerminalShowWindows'

const log = createLogger('mode-preload')

// Long enough that the hub's own first paint and first load finish undisturbed.
const PRELOAD_DELAY_MS = 500

export const loadTerminalMode = () => import('../components/TerminalMode.vue')
export const loadAgentsMode = () => import('../components/AgentsMode.vue')

// A failed fetch here only loses the head start: the mount loads it again and reports it.
function reportFailure(error: unknown): void {
  log.warn('mode chunk preload failed', error)
}

function preload(prepareCode: () => void): void {
  loadTerminalMode().then(prepareCode).catch(reportFailure)
  loadAgentsMode().catch(reportFailure)
  void useTerminalAvailability().probe()
  void useTerminalSessions().reload()
  useTerminalShowWindows()
  useCodeViews()
  void useAgentWorkspaces().reloadWorkspaces()
  void useAgentSessionsAll().reload()
}

/**
 * Readies Code and Chats shortly after the shell is up, so the first entry to
 * either paints its real tree. This only reads: it attaches no tmux client and
 * resumes no chat, which stay tied to entering the mode.
 */
export function useModePreload(shellReady: () => boolean, prepareCode: () => void): void {
  const { start } = useTimeoutFn(() => preload(prepareCode), PRELOAD_DELAY_MS, { immediate: false })
  whenever(shellReady, () => start(), { immediate: true, once: true })
}
