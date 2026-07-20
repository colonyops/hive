import type { Component } from 'vue'
import IconArrowDown from '~icons/lucide/arrow-down'
import IconArrowUp from '~icons/lucide/arrow-up'
import IconCommand from '~icons/lucide/command'
import IconExternalLink from '~icons/lucide/external-link'
import IconEye from '~icons/lucide/eye'
import IconMinus from '~icons/lucide/minus'
import IconRefreshCw from '~icons/lucide/refresh-cw'

// The single declarative source of truth for *bindable* commands — the stable
// app actions a user can rebind from Settings ▸ Keybindings and that also seed
// the command palette. Dynamic palette entries (switch profile, select feed,
// jump to node) are data, not commands, and are NOT bindable, so they live in
// App.vue's palette registration rather than here.
//
// `context` gates where a bare (modifier-less) binding fires: `feed` commands
// only run when the feed is actually on screen; `global` commands run anywhere.
// `defaultCombos` are canonical combo strings (see useKeybindings.comboFromEvent)
// — an empty array means "bindable, but unbound by default".
export type CommandContext = 'global' | 'feed'

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
  context: CommandContext
  /** Omit from the command palette (still bindable + listed in settings). */
  paletteHidden?: boolean
}

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
    keywords: ['open', 'browser', 'github', 'link'],
    icon: IconExternalLink,
    defaultCombos: ['o', 'enter'],
    context: 'feed',
  },
  {
    id: 'feed.toggle-unread',
    title: 'Toggle unread filter',
    group: 'Feeds',
    keywords: ['unread', 'filter'],
    icon: IconEye,
    defaultCombos: ['u'],
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
    id: 'palette.toggle',
    title: 'Command palette',
    group: 'General',
    keywords: ['search', 'commands', 'palette'],
    icon: IconCommand,
    defaultCombos: ['mod+k'],
    context: 'global',
    paletteHidden: true,
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
