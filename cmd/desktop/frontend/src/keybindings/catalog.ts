import { computed, ref, type Component, type ComputedRef } from 'vue'
import IconArrowDown from '~icons/lucide/arrow-down'
import IconArrowLeft from '~icons/lucide/arrow-left'
import IconArrowRight from '~icons/lucide/arrow-right'
import IconArrowUp from '~icons/lucide/arrow-up'
import IconMessagesSquare from '~icons/lucide/messages-square'
import IconBug from '~icons/lucide/bug'
import IconLifeBuoy from '~icons/lucide/life-buoy'
import IconChevronLeft from '~icons/lucide/chevron-left'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconCode from '~icons/lucide/code'
import IconCommand from '~icons/lucide/command'
import IconExternalLink from '~icons/lucide/external-link'
import IconEye from '~icons/lucide/eye'
import IconInbox from '~icons/lucide/inbox'
import IconKeyboard from '~icons/lucide/keyboard'
import IconListTodo from '~icons/lucide/list-todo'
import IconMailCheck from '~icons/lucide/mail-check'
import IconMaximize2 from '~icons/lucide/maximize-2'
import IconMinus from '~icons/lucide/minus'
import IconPanelLeft from '~icons/lucide/panel-left'
import IconPanelRight from '~icons/lucide/panel-right'
import IconPlus from '~icons/lucide/plus'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import IconRotateCcw from '~icons/lucide/rotate-ccw'
import IconSearch from '~icons/lucide/search'
import IconSettings from '~icons/lucide/settings'
import IconSquareCheckBig from '~icons/lucide/square-check-big'
import IconSquarePlus from '~icons/lucide/square-plus'
import IconSquareSplitHorizontal from '~icons/lucide/square-split-horizontal'
import IconSquareSplitVertical from '~icons/lucide/square-split-vertical'
import IconTerminal from '~icons/lucide/terminal'
import IconX from '~icons/lucide/x'
import type { CommandScope } from '../palette/scopes'

// The single declarative source of truth for *bindable* commands — the stable
// app actions a user can rebind from Settings ▸ Keybindings and that also seed
// the command palette. Dynamic palette entries (switch profile, select feed,
// jump to node) are data, not commands, and are NOT bindable, so they live in
// App.vue's palette registration rather than here.
//
// Launchers are the one exception, and they are commands rather than data: a
// `terminal-popup` action exists to be reached by a chord of its own, so each
// one contributes a `launcher.<action-id>` command through setLauncherCommands
// below. Read `commands` — not `commandCatalog` — anywhere the answer has to
// include them.
//
// `context` gates where a bare (modifier-less) binding fires: `feed` commands
// only run when the feed is actually on screen, `terminal` commands only inside
// terminal mode, `terminal-session` commands only while a session is attached
// there, `agents` commands only inside the Agents area, `sidebar` commands in
// any view with a left panel, `any-terminal` commands wherever an emulator is
// drawn — an attached Code session, the Agents area, or the pop-up panel over
// any view; `global` commands run anywhere.
// `defaultCombos` are canonical combo strings (see useKeybindings.comboFromEvent)
// — an empty array means "bindable, but unbound by default".
//
// A combo resolves to exactly one command — the first in this list that claims
// it — so `context` narrows *when* a command fires, it does not let two
// commands share a chord. Widget-local navigation (the palette's own ↑↓, the
// session tree's) is therefore not modelled here: it would have to fight the
// feed's `j`/`k` for the same combo. Those keys stay handlers on the widget
// that owns focus.
// `terminal-session` is the only context nothing in the static catalog below
// claims: it belongs to a launcher that opens where its terminal is, which is a
// command whose whole meaning is the terminal it runs in (ADR
// quick-terminal-launchers-are-session-scoped, ADR a-new-tab-and-a-launcher-open-where-the-terminal-s-active-pane-is).
// Any slug the Code view attaches counts, including the scratch terminal and a
// pinned chat — what the launcher needs is a pane, not a hive record.
export type CommandContext = 'global' | 'feed' | 'terminal' | 'terminal-session' | 'any-terminal' | 'agents' | 'sidebar'

export interface BindableCommand {
  id: string
  /** Human label shown in the palette row and the keybindings editor. */
  title: string
  /** Section header, shared with the palette grouping. */
  group: string
  /** Extra palette match terms. */
  keywords?: string[]
  /** Leading icon for the palette row / settings row. */
  icon?: Component
  /** Canonical default combos; `[]` = bindable but unbound. */
  defaultCombos: string[]
  /**
   * Non-macOS defaults for shifted escaping chords that Ctrl+Shift
   * normalization cannot resolve. Read through defaultCombosFor.
   */
  ctrlDefaultCombos?: string[]
  context: CommandContext
  /** Palette scope for the seeded row. Default 'actions'. */
  scope?: CommandScope
  /** Omit from the command palette (still bindable + listed in settings). */
  paletteHidden?: boolean
  /**
   * Fires over a focused terminal pane, and on the terminal escape chord rather
   * than on the binding alone — Command on macOS, Ctrl+Shift where there is no
   * Command (useKeybindings.terminalEscapeCombo). Without it the pane keeps the
   * key, which is what leaves Ctrl+T as readline's transpose on a platform
   * where `mod` is Ctrl.
   */
  escapesPane?: boolean
  /**
   * Fires over a focused terminal pane on the binding alone, whatever
   * modifiers it carries — the narrower opt-in for a chord `escapesPane`
   * cannot express, since terminalEscapeCombo only qualifies Command and
   * Ctrl+Shift. Prefer `escapesPane`: this one takes the chord away from the
   * shell outright, so `alt+t` stops being readline's transpose-words.
   * `non-mac` limits this policy to Windows/Linux, preserving Control on macOS.
   */
  piercesPane?: boolean | 'non-mac'
}

// How far the digit row reaches. A session with more windows than this is
// walked from the tree; there is no chord for the tenth.
const DIRECT_WINDOW_COMMANDS = 9
const WINDOW_PREFIX = 'terminal.select-window-'

/** The jump command for the 1-based `position` in the window strip. */
export function terminalWindowCommandID(position: number): string {
  return WINDOW_PREFIX + position
}

/** The 1-based window a jump command names, or null for any other command. */
export function terminalWindowPosition(commandID: string): number | null {
  if (!commandID.startsWith(WINDOW_PREFIX)) return null
  const position = Number(commandID.slice(WINDOW_PREFIX.length))
  return Number.isInteger(position) && position >= 1 && position <= DIRECT_WINDOW_COMMANDS ? position : null
}

// Generated rather than written out: nine entries that differ in a digit, and
// one implementation behind them. They are palette-hidden because the Code view
// contributes a palette row per real window, named and carrying these combos as
// hints — nine positional aliases beside those would say less and double the
// list.
const windowJumpCommands: BindableCommand[] = Array.from({ length: DIRECT_WINDOW_COMMANDS }, (_, index) => {
  const position = index + 1
  return {
    id: terminalWindowCommandID(position),
    title: `Go to window ${position}`,
    group: 'Code',
    keywords: ['terminal', 'window', 'tab', 'switch', String(position)],
    icon: IconTerminal,
    defaultCombos: [`mod+${position}`],
    context: 'terminal',
    paletteHidden: true,
    piercesPane: true,
  }
})

export const commandCatalog: BindableCommand[] = [
  {
    id: 'feed.next',
    title: 'Next item',
    group: 'Feeds',
    keywords: ['down', 'next', 'navigate'],
    icon: IconArrowDown,
    defaultCombos: ['j', 'arrowdown'],
    context: 'feed',
  },
  {
    id: 'feed.prev',
    title: 'Previous item',
    group: 'Feeds',
    keywords: ['up', 'previous', 'navigate'],
    icon: IconArrowUp,
    defaultCombos: ['k', 'arrowup'],
    context: 'feed',
  },
  {
    id: 'feed.open-in-browser',
    title: 'Open item in browser',
    group: 'Feeds',
    keywords: ['open', 'browser', 'github', 'link', 'visit'],
    icon: IconExternalLink,
    defaultCombos: ['o', 'enter'],
    context: 'feed',
  },
  {
    id: 'feed.toggle-unread',
    title: 'Toggle unread filter',
    group: 'Feeds',
    keywords: ['unread', 'filter', 'seen'],
    icon: IconEye,
    defaultCombos: ['u'],
    context: 'feed',
  },
  {
    id: 'feed.toggle-preview',
    title: 'Toggle preview pane',
    group: 'Feeds',
    keywords: ['preview', 'detail', 'pane', 'panel', 'reading', 'close'],
    icon: IconPanelRight,
    defaultCombos: ['p'],
    context: 'feed',
  },
  { id: 'feed.toggle-archive', title: 'Archive / unarchive item', group: 'Feeds', keywords: ['archive', 'done', 'complete', 'dismiss'], defaultCombos: ['e'], context: 'feed' },
  { id: 'feed.mark-unread', title: 'Mark unread', group: 'Feeds', keywords: ['read', 'seen', 'unseen'], defaultCombos: ['shift+u'], context: 'feed' },
  // Scoped to the selected feed; a no-op in Trash, which carries no unread
  // semantics. The profile variant stays unbound by default: it clears every
  // feed at once and there is no undo, so it should be asked for by name.
  {
    id: 'feed.mark-all-read',
    title: 'Mark all as read',
    group: 'Feeds',
    keywords: ['read', 'unread', 'clear', 'catch up', 'bulk'],
    icon: IconMailCheck,
    defaultCombos: ['shift+a'],
    context: 'feed',
  },
  {
    id: 'feed.mark-workspace-read',
    title: 'Mark all feeds as read',
    group: 'Feeds',
    keywords: ['read', 'unread', 'clear', 'catch up', 'bulk', 'profile', 'everything'],
    icon: IconMailCheck,
    defaultCombos: [],
    context: 'feed',
  },
  {
    id: 'feed.refresh',
    title: 'Refresh feeds',
    group: 'Feeds',
    keywords: ['reload', 'sync'],
    icon: IconRefreshCw,
    defaultCombos: ['r'],
    context: 'feed',
  },
  {
    id: 'feed.toggle-selection',
    title: 'Toggle item selection',
    group: 'Feeds',
    keywords: ['select', 'multiple', 'bulk', 'items'],
    icon: IconSquareCheckBig,
    defaultCombos: [],
    context: 'feed',
  },
  {
    id: 'palette.toggle',
    title: 'Command palette',
    group: 'General',
    keywords: ['search', 'commands', 'palette'],
    icon: IconCommand,
    defaultCombos: ['mod+k'],
    context: 'global',
    paletteHidden: true,
    escapesPane: true,
  },
  // ⌘N, the chord every app spells "new thing" with — a session is this app's.
  // It escapes a focused pane because Code is where a second session is most
  // often wanted, and a pane owns every key there otherwise.
  {
    id: 'session.new',
    title: 'New session…',
    group: 'General',
    keywords: ['session', 'create', 'hive', 'agent', 'launch', 'new'],
    icon: IconSquarePlus,
    defaultCombos: ['mod+n'],
    context: 'global',
    escapesPane: true,
  },
  {
    id: 'terminal.popup.toggle',
    title: 'Terminal pop-up',
    group: 'General',
    keywords: ['terminal', 'shell', 'popup', 'console', 'run', 'command', 'lazygit'],
    icon: IconTerminal,
    defaultCombos: ['mod+`'],
    context: 'global',
    piercesPane: true,
  },
  {
    // This id is persisted in settings.yaml. Keep it even though the command
    // now follows the active sidebar in every app mode.
    id: 'terminal.toggle-sidebar',
    title: 'Toggle sidebar',
    group: 'View',
    keywords: ['sidebar', 'panel', 'inbox', 'code', 'chats', 'show', 'hide', 'collapse'],
    icon: IconPanelLeft,
    defaultCombos: ['mod+b'],
    context: 'sidebar',
    escapesPane: true,
    piercesPane: 'non-mac',
  },
  // Directional rather than one toggle: which pane you land on should be
  // readable off the chord, not off where focus happened to be.
  //
  // Only the sidebar half has to escape a focused pane, so it is the one chord
  // terminal mode takes away from tmux (App.vue's dispatcher, and xterm's own
  // handler in useTerminalWindows). `mod` is Cmd on macOS and Ctrl elsewhere,
  // where Ctrl+← is readline's backward-word — rebind it there if the pane
  // needs it back.
  {
    id: 'terminal.focus-sidebar',
    title: 'Focus session tree',
    group: 'Code',
    keywords: ['terminal', 'sidebar', 'sessions', 'tree', 'focus', 'left'],
    icon: IconPanelLeft,
    defaultCombos: ['mod+arrowleft'],
    context: 'terminal',
    piercesPane: true,
  },
  {
    id: 'terminal.focus-pane',
    title: 'Focus terminal',
    group: 'Code',
    keywords: ['terminal', 'pane', 'focus', 'right'],
    icon: IconPanelRight,
    defaultCombos: ['mod+arrowright'],
    context: 'terminal',
  },
  // Bare `/`, the way every list this is modelled on spells it. A focused pane
  // keeps the key — it is a character — so this fires from the tree, which is
  // where a search for a session starts anyway. One combo resolves to one
  // command, so the feed's search box and the session filter share this one,
  // whose run() dispatches on whichever is on screen; mod+f rides the same
  // command (xterm's own Cmd+F stays widget-local). paletteHidden: a visible
  // global row would no-op wherever neither surface is on screen, which the
  // palette forbids (hide, don't disable) — the per-view named rows below
  // carry the hint instead.
  {
    id: 'view.focus-search',
    title: 'Focus search',
    group: 'General',
    keywords: ['find', 'filter', 'search', 'slash'],
    icon: IconSearch,
    defaultCombos: ['/', 'mod+f'],
    context: 'global',
    paletteHidden: true,
  },
  // The ? reference. Bare '?' is dead in editables and panes automatically, so
  // it fires from list surfaces, which is where the genre binds it.
  {
    id: 'palette.keys',
    title: 'Keyboard shortcuts…',
    group: 'General',
    keywords: ['keys', 'shortcuts', 'keymap', 'help', 'cheatsheet'],
    icon: IconKeyboard,
    defaultCombos: ['?'],
    context: 'global',
    scope: 'goto',
  },
  // The window lifecycle, on the chords a terminal emulator already spells them
  // with: ⌘T, ⌘W, and ⌘⇧] / ⌘⇧[ to walk the list. They escape a focused pane
  // rather than piercing it, so where `mod` is Ctrl the pane keeps Ctrl+T for
  // readline and the app answers Ctrl+Shift+T instead.
  //
  // `mod+}` is not a slip for `mod+shift+]`: Shift already changed the
  // character, so the combo names `}` (see useKeybindings.shouldRecordShift),
  // and that one spelling is what ⌘⇧] and Ctrl+Shift+] both produce.
  {
    id: 'terminal.new-window',
    title: 'New window',
    group: 'Code',
    keywords: ['terminal', 'window', 'tab', 'new', 'create', 'open'],
    icon: IconPlus,
    defaultCombos: ['mod+t'],
    context: 'terminal',
    escapesPane: true,
  },
  {
    id: 'terminal.close-window',
    title: 'Close window',
    group: 'Code',
    keywords: ['terminal', 'window', 'tab', 'close', 'kill'],
    icon: IconX,
    defaultCombos: ['mod+w'],
    context: 'terminal',
    escapesPane: true,
  },
  {
    id: 'terminal.next-window',
    title: 'Next window',
    group: 'Code',
    keywords: ['terminal', 'window', 'tab', 'next', 'cycle', 'switch'],
    icon: IconChevronRight,
    defaultCombos: ['mod+}'],
    context: 'terminal',
    escapesPane: true,
  },
  {
    id: 'terminal.prev-window',
    title: 'Previous window',
    group: 'Code',
    keywords: ['terminal', 'window', 'tab', 'previous', 'cycle', 'switch'],
    icon: IconChevronLeft,
    defaultCombos: ['mod+{'],
    context: 'terminal',
    escapesPane: true,
  },
  // A position in the window strip, not a tmux window index: the strip is what
  // is on screen, and tmux's indices have gaps as soon as a window is closed.
  ...windowJumpCommands,
  // Match iTerm2 on macOS. Ctrl platforms need unshifted alternatives because
  // Ctrl+Shift normalizes to mod. Avoid Ctrl+X/Z because these commands can
  // fire in editable fields.
  {
    id: 'terminal.split-right',
    title: 'Split pane right',
    group: 'Code',
    keywords: ['terminal', 'pane', 'split', 'horizontal', 'right', 'tmux'],
    icon: IconSquareSplitHorizontal,
    defaultCombos: ['mod+d'],
    context: 'terminal',
    escapesPane: true,
  },
  {
    id: 'terminal.split-down',
    title: 'Split pane down',
    group: 'Code',
    keywords: ['terminal', 'pane', 'split', 'vertical', 'down', 'below', 'tmux'],
    icon: IconSquareSplitVertical,
    defaultCombos: ['mod+shift+d'],
    ctrlDefaultCombos: ['mod+o'],
    context: 'terminal',
    escapesPane: true,
  },
  {
    id: 'terminal.close-pane',
    title: 'Close pane',
    group: 'Code',
    keywords: ['terminal', 'pane', 'close', 'kill', 'tmux'],
    icon: IconX,
    defaultCombos: ['mod+shift+w'],
    ctrlDefaultCombos: ['mod+q'],
    context: 'terminal',
    escapesPane: true,
  },
  {
    id: 'terminal.zoom-pane',
    title: 'Zoom pane',
    group: 'Code',
    keywords: ['terminal', 'pane', 'zoom', 'maximize', 'fullscreen', 'toggle', 'tmux'],
    icon: IconMaximize2,
    defaultCombos: ['mod+shift+enter'],
    ctrlDefaultCombos: ['mod+m'],
    context: 'terminal',
    escapesPane: true,
  },
  // ADR the-zoom-chords-step-the-terminal-text-size-instead-of-magnifying-the-webview.
  // Increase carries every spelling of one physical key: `=`, the shifted `+` a
  // ⌘+ press sends (`plus` is a named key, so its Shift survives), and the bare
  // `+` of a layout with its own plus key. Where `mod` is Ctrl the pane escape
  // drops the Shift but keeps the character it produced, so the reachable
  // spellings there are `plus`, `_` and `)`.
  {
    id: 'terminal.text-size-increase',
    title: 'Increase text size',
    group: 'Code',
    keywords: ['terminal', 'font', 'text', 'size', 'bigger', 'larger', 'zoom', 'in'],
    icon: IconPlus,
    defaultCombos: ['mod+=', 'mod+shift+plus', 'mod+plus'],
    ctrlDefaultCombos: ['mod+=', 'mod+plus'],
    context: 'any-terminal',
    escapesPane: true,
  },
  {
    id: 'terminal.text-size-decrease',
    title: 'Decrease text size',
    group: 'Code',
    keywords: ['terminal', 'font', 'text', 'size', 'smaller', 'zoom', 'out'],
    icon: IconMinus,
    defaultCombos: ['mod+-'],
    ctrlDefaultCombos: ['mod+-', 'mod+_'],
    context: 'any-terminal',
    escapesPane: true,
  },
  {
    id: 'terminal.text-size-reset',
    title: 'Reset text size',
    group: 'Code',
    keywords: ['terminal', 'font', 'text', 'size', 'reset', 'default', 'actual'],
    icon: IconRotateCcw,
    defaultCombos: ['mod+0'],
    ctrlDefaultCombos: ['mod+0', 'mod+)'],
    context: 'any-terminal',
    escapesPane: true,
  },
  // Alt-arrow cannot use terminal escape normalization, so these bindings pierce
  // the pane. On Ctrl platforms readline leaves Ctrl+Alt+Arrow unbound by default.
  {
    id: 'terminal.focus-pane-left',
    title: 'Focus pane left',
    group: 'Code',
    keywords: ['terminal', 'pane', 'focus', 'select', 'left', 'tmux'],
    icon: IconArrowLeft,
    defaultCombos: ['mod+alt+arrowleft'],
    context: 'terminal',
    piercesPane: true,
  },
  {
    id: 'terminal.focus-pane-right',
    title: 'Focus pane right',
    group: 'Code',
    keywords: ['terminal', 'pane', 'focus', 'select', 'right', 'tmux'],
    icon: IconArrowRight,
    defaultCombos: ['mod+alt+arrowright'],
    context: 'terminal',
    piercesPane: true,
  },
  {
    id: 'terminal.focus-pane-up',
    title: 'Focus pane up',
    group: 'Code',
    keywords: ['terminal', 'pane', 'focus', 'select', 'up', 'above', 'tmux'],
    icon: IconArrowUp,
    defaultCombos: ['mod+alt+arrowup'],
    context: 'terminal',
    piercesPane: true,
  },
  {
    id: 'terminal.focus-pane-down',
    title: 'Focus pane down',
    group: 'Code',
    keywords: ['terminal', 'pane', 'focus', 'select', 'down', 'below', 'tmux'],
    icon: IconArrowDown,
    defaultCombos: ['mod+alt+arrowdown'],
    context: 'terminal',
    piercesPane: true,
  },
  // The Chats area is a plain two-level list beside a pane, not a tree, so it
  // needs only the pair terminal mode's focus chords have — no filter, no
  // window jumps. Combos are new ones, not terminal.*'s: a combo resolves to
  // exactly one command, so reusing mod+arrowleft/-right here would shadow
  // whichever command claims it first rather than binding both.
  {
    id: 'agents.focus-sidebar',
    title: 'Focus workspace list',
    group: 'Chats',
    keywords: ['chats', 'agents', 'workspace', 'sidebar', 'list', 'focus', 'left'],
    icon: IconMessagesSquare,
    defaultCombos: ['mod+shift+arrowleft'],
    context: 'agents',
    piercesPane: true,
  },
  {
    id: 'agents.focus-pane',
    title: 'Focus chat',
    group: 'Chats',
    keywords: ['chats', 'agents', 'session', 'pane', 'focus', 'right'],
    icon: IconMessagesSquare,
    defaultCombos: ['mod+shift+arrowright'],
    context: 'agents',
  },
  {
    id: 'report.open',
    title: 'Report a problem',
    group: 'General',
    keywords: ['bug', 'issue', 'feedback', 'github', 'crash', 'report'],
    icon: IconBug,
    defaultCombos: ['mod+shift+b'],
    context: 'global',
  },
  {
    id: 'report.bundle',
    title: 'Save a diagnostic bundle',
    group: 'General',
    keywords: ['bundle', 'diagnostics', 'logs', 'support', 'report'],
    icon: IconLifeBuoy,
    // Palette-only: asked for once, when a maintainer requests a bundle.
    defaultCombos: [],
    context: 'global',
  },
  // Cmd+, is the macOS settings standard; g s is the genre chord.
  // paletteHidden: the Settings › <section> rows are the named rows.
  {
    id: 'settings.open',
    title: 'Open Settings',
    group: 'General',
    keywords: ['settings', 'preferences'],
    icon: IconSettings,
    defaultCombos: ['mod+,', 'g s'],
    context: 'global',
    paletteHidden: true,
    scope: 'goto',
  },
  // Router history, matching the title-bar buttons. Not escapesPane: a
  // focused pane keeps the key.
  {
    id: 'history.back',
    title: 'Back',
    group: 'General',
    keywords: ['back', 'navigate', 'history'],
    icon: IconArrowLeft,
    defaultCombos: ['mod+['],
    context: 'global',
    paletteHidden: true,
  },
  {
    id: 'history.forward',
    title: 'Forward',
    group: 'General',
    keywords: ['forward', 'navigate', 'history'],
    icon: IconArrowRight,
    defaultCombos: ['mod+]'],
    context: 'global',
    paletteHidden: true,
  },
  // Palette-hidden like the window jumps: the dynamic "Go to Inbox/Code/Chats"
  // rows are the named rows and carry these combos as hints.
  {
    id: 'view.go-inbox',
    title: 'Go to Inbox',
    group: 'View',
    keywords: ['inbox', 'hub', 'feed', 'mode'],
    icon: IconInbox,
    defaultCombos: ['g i'],
    context: 'global',
    paletteHidden: true,
    scope: 'goto',
  },
  {
    id: 'view.go-code',
    title: 'Go to Code',
    group: 'View',
    keywords: ['code', 'terminal', 'sessions', 'mode'],
    icon: IconCode,
    defaultCombos: ['g c'],
    context: 'global',
    paletteHidden: true,
    scope: 'goto',
  },
  {
    id: 'view.go-chats',
    title: 'Go to Chats',
    group: 'View',
    keywords: ['chats', 'agents', 'chat', 'mode'],
    icon: IconMessagesSquare,
    defaultCombos: ['g a'],
    context: 'global',
    paletteHidden: true,
    scope: 'goto',
  },
  // Grouped with the mode rows (Go to Inbox/Code/Chats) rather than General:
  // it toggles a view on screen the same way those switch one, and that is
  // where a user opening the palette to find it would look first.
  {
    id: 'tasks.toggle',
    title: 'Toggle Tasks',
    group: 'View',
    keywords: ['tasks', 'honeycomb', 'hc', 'epics'],
    icon: IconListTodo,
    defaultCombos: ['mod+shift+t', 'g t'],
    context: 'global',
    // Pierces rather than escapes: Tasks is the overlay most often wanted from
    // inside a session, and a user who rebinds it to an alt chord gets nothing
    // through terminalEscapeCombo.
    piercesPane: true,
    scope: 'goto',
  },
  {
    id: 'window.hide',
    title: 'Hide window',
    group: 'Window',
    keywords: ['minimize', 'close'],
    icon: IconMinus,
    defaultCombos: [],
    context: 'global',
  },
]

// Read off the static catalog rather than off `commands`: a launcher pierces a
// focused pane through its own path, and nothing loaded from actions.yml gets
// to claim the escape chord.
const paneEscapes = new Set(commandCatalog.filter((command) => command.escapesPane).map((command) => command.id))
const panePierces = new Map(commandCatalog.filter((command) => command.piercesPane).map((command) => [command.id, command.piercesPane]))

/** Whether the command fires over a focused terminal pane on the escape chord. */
export function commandEscapesPane(commandID: string): boolean {
  return paneEscapes.has(commandID)
}

/** Whether the command fires over a focused terminal pane on the binding alone. */
export function commandPiercesPane(commandID: string, mac = typeof navigator !== 'undefined' && /Mac/i.test(navigator.userAgent)): boolean {
  const policy = panePierces.get(commandID)
  return policy === true || (policy === 'non-mac' && !mac)
}

export function defaultCombosFor(command: BindableCommand, mac: boolean): string[] {
  return mac ? command.defaultCombos : (command.ctrlDefaultCombos ?? command.defaultCombos)
}

// The namespace a launcher's bindable command id lives in — `launcher.lazygit`
// for the action `lazygit`. It is what a user writes in settings.yaml, so it is
// as much a part of the config contract as the action id itself.
const LAUNCHER_PREFIX = 'launcher.'

export function launcherCommandID(actionID: string): string {
  return LAUNCHER_PREFIX + actionID
}

/** The action id behind a launcher command, or null for any other command. */
export function launcherActionID(commandID: string): string | null {
  return commandID.startsWith(LAUNCHER_PREFIX) ? commandID.slice(LAUNCHER_PREFIX.length) : null
}

const launcherCommands = ref<BindableCommand[]>([])

/**
 * Replaces the launcher commands, which change whenever actions.yml does.
 * Called by useLaunchers; everything else reads `commands`.
 */
export function setLauncherCommands(next: BindableCommand[]): void {
  launcherCommands.value = next
}

/**
 * Every bindable command: the static catalog above followed by the configured
 * launchers. Launchers come last so a combo they share with a built-in resolves
 * to the built-in — a config file must not be able to take `mod+k` away from
 * the palette.
 */
export const commands: ComputedRef<BindableCommand[]> = computed(
  () => [...commandCatalog, ...launcherCommands.value],
)

/**
 * Every bindable command by id, built from `commands` (not `commandCatalog`)
 * so launcher commands resolve too.
 */
export const commandById: ComputedRef<Map<string, BindableCommand>> = computed(
  () => new Map(commands.value.map((command) => [command.id, command])),
)
