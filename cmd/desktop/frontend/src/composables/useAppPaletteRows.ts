import { computed, nextTick, watch, type Ref } from 'vue'
import type { Router } from 'vue-router'
import IconGauge from '~icons/lucide/gauge'
import IconLayoutGrid from '~icons/lucide/layout-grid'
import IconList from '~icons/lucide/list'
import IconMessagesSquare from '~icons/lucide/messages-square'
import IconPalette from '~icons/lucide/palette'
import IconRss from '~icons/lucide/rss'
import IconSearch from '~icons/lucide/search'
import IconSquarePlus from '~icons/lucide/square-plus'
import IconTerminal from '~icons/lucide/terminal'
import IconWorkflow from '~icons/lucide/workflow'
import { commandById, commands as bindableCommands, terminalWindowCommandID, type CommandContext } from '../keybindings/catalog'
import { keymapRows, requestedEditorFilter } from '../keybindings/keymapRows'
import { paletteScopes, type PaletteScopeId } from '../palette/scopes'
import { formatCombo, useKeybindings } from './useKeybindings'
import { useCommands, useCommandPalette, useKeysScope, type Command } from './useCommands'
import { setTheme, themeLabels, themes } from './useTheme'
import { terminalSessionGroups, useTerminalSessions } from './useTerminalSessions'
import { useTerminalPinnedChats } from './useTerminalPinnedChats'
import { useAgentSessionsAll } from './useAgentSessionsAll'
import { useAgentWorkspaces } from './useAgentWorkspaces'
import { useAttachedTerminalWindows } from './useAttachedTerminalWindows'
import { applicationSettingsSections } from '../router'
import { applicationSettingsSectionMeta } from '../components/settings/sectionMeta'
import { actionTypeMeta } from '../lib/actionPresentation'
import { containerLine } from '../lib/itemPresentation'
import type { ActionView } from '../types/action'
import type { InboxItem, Profile, SidebarSelection } from '../types/feed'

// What each sigil is for, shown as the Keys scope's trailing legend row.
const sigilMeanings: Partial<Record<PaletteScopeId, string>> = {
  goto: 'Jump to a place, session, or setting',
  actions: 'Run a command',
  shell: 'Run a line in a new terminal window',
  keys: 'Search and rebind shortcuts',
}

function contextLabel(context: CommandContext): string {
  // The terminal contexts belong to the area the product names Code.
  if (context === 'terminal') return 'Code'
  if (context === 'terminal-session') return 'Code session'
  if (context === 'any-terminal') return 'Terminal'
  return context.split('-').map((word) => word[0]!.toUpperCase() + word.slice(1)).join(' ')
}

export interface AppPaletteDeps {
  /** Resolves a catalog command id to its implementation (App.vue's runMap). */
  runCommand: (id: string) => void
  /** Whether a catalog command's context fires from where the user is standing. */
  contextActive: (context: CommandContext) => boolean
  mode: Ref<'hub' | 'terminal' | 'agents'>
  shellLoaded: Ref<boolean>
  onboardingActive: Ref<boolean>
  hubActive: Ref<boolean>
  devToolsEnabled: Ref<boolean>
  router: Router
  profiles: Ref<Profile[]>
  activeProfile: Ref<Profile | null>
  requestSelectProfile: (id: string) => Promise<void>
  navigateSidebar: (selection: SidebarSelection) => void
  selectedItem: Ref<InboxItem | null>
  selectedItemIDs: Ref<number[]>
  openSelectedItemsSession: () => Promise<void>
  actions: Ref<ActionView[]>
  invokeAction: (id: string) => Promise<void>
  flowsActive: Ref<boolean>
  openFlows: (focusNodeId?: string) => void
  requestExitFlows: () => void
  openNewProfile: () => void
  /** The slug attached on screen, so its own attach row does not offer itself. */
  onScreenSessionSlug: Ref<string>
}

/**
 * Registers every App-level palette row source for the app's lifetime:
 * catalog commands, mode switches, and the hub's own objects (profiles, feeds,
 * themes), plus the Go-to rows for sessions, windows, settings sections, and
 * chats that are global rather than tied to a lazily mounted mode.
 */
export function useAppPaletteRows(deps: AppPaletteDeps): void {
  const {
    runCommand, contextActive, mode, shellLoaded, onboardingActive, hubActive,
    devToolsEnabled, router, profiles, activeProfile, requestSelectProfile, navigateSidebar, selectedItem,
    selectedItemIDs, openSelectedItemsSession, actions, invokeAction, flowsActive, openFlows, requestExitFlows, openNewProfile,
    onScreenSessionSlug,
  } = deps

  const { combosFor } = useKeybindings()
  const hintFor = (id: string): string => formatCombo(combosFor(id)[0] ?? '')

  // The app is usable — past onboarding, the shell resolved — regardless of
  // which mode is on screen. This is the gate the #306 fix widens the hub's
  // own Go-to rows to: their run()s already land in the hub from anywhere.
  const appReady = computed(() => shellLoaded.value && !onboardingActive.value)

  // Session and window rows read the same module singletons the sidebar tree
  // itself reads, so they are global rather than tied to TerminalMode ever
  // having mounted.
  const { sessions: terminalSessionRows, scratch: terminalScratchRow, reload: reloadTerminalSessions } = useTerminalSessions()
  const { rows: pinnedChatRows } = useTerminalPinnedChats()
  const { attached: attachedTerminal } = useAttachedTerminalWindows()
  const activeTerminalSessions = computed(() => terminalSessionRows.value.filter((row) => row.state === 'active'))
  const terminalGroups = computed(() => terminalSessionGroups(activeTerminalSessions.value, terminalScratchRow.value, pinnedChatRows.value))

  // Chat rows read the same recents listing the Code view's pinned-chats
  // section and the Agents area's own sidebar do. session.workspace is the
  // workspace's directory key, not its display name, so the group label needs
  // the same dir → name join AgentsSidebar's own tree does.
  const { recents: chatRecents, reloadRecents } = useAgentSessionsAll()
  const { workspaces: agentWorkspaces, reloadWorkspaces } = useAgentWorkspaces()
  const workspaceNameByDir = computed(() => {
    const map = new Map<string, string>()
    for (const w of agentWorkspaces.value) map.set(w.dir, w.name)
    return map
  })

  useCommands(computed(() => {
    const cmds: Command[] = []

    for (const command of bindableCommands.value) {
      if (command.paletteHidden || !contextActive(command.context)) continue
      cmds.push({
        id: command.id,
        title: command.title,
        group: command.group,
        keywords: command.keywords,
        icon: command.icon,
        scope: command.scope,
        hint: hintFor(command.id),
        run: () => runCommand(command.id),
      })
    }

    // view.focus-search is palette-hidden because one command answers `/` in
    // three unrelated surfaces, and a single row for it would no-op wherever
    // another surface is on screen. These named rows stand in per surface,
    // gated the same way the surface's own commands are, sharing its hint.
    const focusSearchHint = hintFor('view.focus-search')
    if (contextActive('feed')) {
      cmds.push({
        id: 'view.focus-search:feed',
        title: 'Search items…',
        group: 'Feeds',
        scope: 'actions',
        keywords: ['find', 'filter', 'search'],
        icon: IconSearch,
        hint: focusSearchHint,
        run: () => runCommand('view.focus-search'),
      })
    }
    if (contextActive('terminal')) {
      cmds.push({
        id: 'view.focus-search:terminal',
        title: 'Filter sessions',
        group: 'Code',
        scope: 'actions',
        keywords: ['terminal', 'filter', 'search', 'find', 'session'],
        icon: IconSearch,
        hint: focusSearchHint,
        run: () => runCommand('view.focus-search'),
      })
    }
    if (contextActive('agents')) {
      cmds.push({
        id: 'view.focus-search:agents',
        title: 'Filter workspaces',
        group: 'Chats',
        scope: 'actions',
        keywords: ['chats', 'agents', 'filter', 'search', 'find', 'workspace'],
        icon: IconSearch,
        hint: focusSearchHint,
        run: () => runCommand('view.focus-search'),
      })
    }

    // A row per mode this one is not: with the other modes' objects hidden,
    // these keep a mode change reachable without the title bar. Title, icon
    // and keywords are the paired view.go-* command's — App.vue's runMap is
    // the one implementation both the keymap and this row dispatch through.
    if (appReady.value) {
      const modeRow = (id: 'mode:hub' | 'mode:terminal' | 'mode:agents', catalogID: string): void => {
        const catalog = commandById.value.get(catalogID)
        if (!catalog) return // a stale mode row is worse than a missing one
        cmds.push({
          id,
          title: catalog.title,
          group: 'View',
          scope: 'goto',
          keywords: catalog.keywords,
          icon: catalog.icon,
          hint: hintFor(catalogID),
          run: () => runCommand(catalogID),
        })
      }
      if (mode.value !== 'hub') modeRow('mode:hub', 'view.go-inbox')
      if (mode.value !== 'terminal') modeRow('mode:terminal', 'view.go-code')
      if (mode.value !== 'agents') modeRow('mode:agents', 'view.go-chats')

      // Profiles, feeds and Trash, and themes reach across every mode (#306):
      // requestSelectProfile, navigateSidebar and setTheme all push a route
      // that lands in the hub, so there is nothing hub-specific left in them.
      for (const p of profiles.value) {
        cmds.push({
          id: `profile:${p.id}`,
          title: p.name,
          group: 'Profiles',
          scope: 'goto',
          kind: 'profile',
          keywords: ['profile', 'switch', 'workspace'],
          icon: IconLayoutGrid,
          run: () => requestSelectProfile(p.id),
        })
      }

      // Trash and the feed rows below sit under the active profile's own
      // name, so they carry its group's order (-1) rather than the default —
      // sorting them together as one block, roughly where the old flat
      // 'Feeds' group sat, ahead of Flow/Profiles/Settings/Theme/View.
      const feedGroup = activeProfile.value?.name ?? 'Feeds'

      cmds.push({
        id: 'view:trash',
        title: 'Trash',
        group: feedGroup,
        order: -1,
        scope: 'goto',
        kind: 'view',
        keywords: ['trash', 'open'],
        icon: IconList,
        run: () => navigateSidebar({ type: 'trash' }),
      })

      for (const f of activeProfile.value?.feeds ?? []) {
        cmds.push({
          id: `feed:${f.id}`,
          title: f.name,
          group: feedGroup,
          order: -1,
          scope: 'goto',
          kind: 'feed',
          keywords: ['feed', 'select'],
          icon: IconRss,
          run: () => navigateSidebar({ type: 'feed', feedId: f.id }),
        })
      }

      for (const t of themes) {
        cmds.push({
          id: `theme:${t}`,
          title: `Theme: ${themeLabels[t]}`,
          group: 'Theme',
          scope: 'actions',
          keywords: ['theme', 'appearance', t],
          icon: IconPalette,
          run: () => setTheme(t),
        })
      }

      // One row per application settings section — titled the way the nav
      // panel itself labels them, so the palette and Settings agree.
      for (const section of applicationSettingsSections) {
        const meta = applicationSettingsSectionMeta[section]
        cmds.push({
          id: `settings:${section}`,
          title: meta.label,
          group: 'Settings',
          scope: 'goto',
          kind: 'settings',
          keywords: ['settings'],
          icon: meta.icon,
          run: () => void router.push({ name: 'application-settings', params: { section } }),
        })
      }
    }

    if (hubActive.value) {
      // The selected item's configured actions, under its own reference — the
      // same set the detail pane draws as cards. Running one from here goes
      // through invokeAction, so an action that declares inputs opens its form
      // and an interactive launch-session opens the session dialog, exactly as
      // a card click does.
      if (contextActive('feed') && selectedItem.value) {
        const itemGroup = containerLine(selectedItem.value) || 'Item'
        for (const action of actions.value) {
          const meta = actionTypeMeta(action.type)
          cmds.push({
            id: `item:action:${action.id}`,
            title: action.label,
            group: itemGroup,
            order: -3,
            scope: 'actions',
            keywords: ['action', 'item', action.type],
            iconName: meta.icon,
            iconColor: meta.color,
            run: () => void invokeAction(action.id),
          })
        }
      }

      if (contextActive('feed') && selectedItemIDs.value.length > 0) {
        cmds.push({
          id: 'feed:create-session-from-selection',
          title: 'Create session from selected items…',
          group: 'Feeds',
          scope: 'actions',
          keywords: ['session', 'multiple', 'bulk', 'selected'],
          icon: IconSquarePlus,
          run: () => void openSelectedItemsSession(),
        })
      }

      cmds.push({
        id: 'profile:new',
        title: 'New profile…',
        group: 'Profiles',
        scope: 'actions',
        keywords: ['workspace', 'create'],
        run: openNewProfile,
      })

      // View — enter/exit the flows canvas for the active profile. The canvas
      // is profile-bound, so this and the node rows below stay hub-gated.
      cmds.push({
        id: 'flow:edit',
        title: flowsActive.value ? 'Back to feed' : 'Edit flow…',
        group: 'View',
        scope: 'actions',
        keywords: ['flows', 'pipeline', 'nodes', 'canvas', 'editor'],
        icon: IconWorkflow,
        run: () => { flowsActive.value ? requestExitFlows() : openFlows() },
      })

    }

    // The palette is the only way in outside a Vite build, where the dev strip
    // carries the link.
    if (devToolsEnabled.value) {
      cmds.push({
        id: 'dev:open',
        title: 'Open developer tools',
        group: 'View',
        scope: 'goto',
        keywords: ['runtime', 'performance', 'memory', 'cpu', 'diagnostics'],
        icon: IconGauge,
        run: () => { void router.push({ name: 'dev' }) },
      })
    }

    // Session attach rows, grouped the way the sidebar tree groups them
    // (pinned chats, scratch, then one per repo). The route is the attach
    // state in and out of Code alike, so the row on screen already skips
    // itself.
    for (const group of terminalGroups.value) {
      for (const row of group.sessions) {
        if (row.slug === onScreenSessionSlug.value) continue
        cmds.push({
          id: `terminal:attach:${row.slug}`,
          title: row.name,
          group: group.name,
          scope: 'goto',
          kind: 'session',
          keywords: [row.slug, group.name, 'session', 'attach', 'switch', 'open'],
          icon: IconTerminal,
          run: () => void router.push({ name: 'terminal', params: { slug: row.slug } }),
        })
      }
    }

    // Window rows for whichever session is attached (the module projection
    // TerminalMode writes, since the live tab list is otherwise
    // component-local). Hints carry the numbered jump chords the keymap
    // already binds to each position in the strip.
    if (attachedTerminal.value) {
      const { slug, name, windows } = attachedTerminal.value
      windows.forEach((win, index) => {
        cmds.push({
          id: `terminal:window:${win.windowId}`,
          title: win.name,
          group: name,
          scope: 'goto',
          kind: 'window',
          keywords: ['window', 'tab', 'jump', 'switch'],
          icon: IconTerminal,
          hint: hintFor(terminalWindowCommandID(index + 1)),
          run: () => void router.push({ name: 'terminal', params: { slug }, query: { window: win.windowId } }),
        })
      })
    }

    // Chat rows — every recent session across every workspace; empty and
    // absent wherever the Agents area itself is unavailable. Grouped by
    // workspace, at the order the old flat 'Chats' group held (ahead of the
    // profile's Feeds/Trash group and everything below it).
    for (const session of chatRecents.value) {
      cmds.push({
        id: `chat:${session.id}`,
        title: session.name,
        group: workspaceNameByDir.value.get(session.workspace) || session.workspace,
        order: -2,
        scope: 'goto',
        kind: 'chat',
        keywords: ['chat'],
        icon: IconMessagesSquare,
        run: async () => {
          await router.push({ name: 'agents', params: { workspace: session.workspace }, query: { chat: String(session.id) } })
          await nextTick()
          runCommand('agents.focus-pane')
        },
      })
    }

    return cmds
  }))

  const { open: paletteOpen, setScope, visibleScopes } = useCommandPalette()

  // The ? scope: every bindable command (launchers included), rows whose
  // context is live right now promoted ahead of the rest under their own
  // group, and a trailing legend explaining the other sigils. Registered for
  // the app's lifetime, so the tab is effectively always visible.
  useKeysScope(() => {
    const rows: Command[] = keymapRows.value.map((row) => {
      const active = contextActive(row.context)
      return {
        id: row.id,
        title: row.title,
        group: active ? `${contextLabel(row.context)} · active context` : row.group,
        order: active ? -1 : 0,
        hint: row.formatted.join(' · '),
        keywords: row.keywords,
        // Every row routes to the pre-filtered editor — there is no disabled
        // state, active context or not.
        run: () => {
          requestedEditorFilter.value = row.title
          void router.push({ name: 'application-settings', params: { section: 'keybindings' } })
        },
      }
    })

    for (const scope of paletteScopes) {
      const meaning = sigilMeanings[scope.id]
      if (!scope.sigil || !meaning) continue
      // Only scopes whose tab is currently shown: outside the Code view the
      // Shell tab is hidden and typing `!` is a literal character, so a
      // legend row for it would run setScope into a scope with no tab and no
      // possible rows — a disabled row in disguise.
      if (!visibleScopes.value.some((visible) => visible.id === scope.id)) continue
      rows.push({
        id: `keys:sigil:${scope.id}`,
        title: meaning,
        group: 'Sigils',
        order: 1,
        hint: scope.sigil,
        keepOpen: true,
        run: () => setScope(scope.id),
      })
    }

    return rows
  })

  // Session, chat, and workspace rows are read from module singletons the
  // sidebar trees keep warm elsewhere; a palette open is the moment they are
  // about to be shown, so that is when staleness is worth paying to fix. All
  // three reloads keep last-good rows on failure. Workspaces in particular can
  // still be empty here on a fresh launch — the Agents area may never have
  // mounted — which is what the chat rows' dir → name join needs populated.
  watch(paletteOpen, (open) => {
    if (!open) return
    void reloadTerminalSessions()
    void reloadRecents()
    void reloadWorkspaces()
  })
}
