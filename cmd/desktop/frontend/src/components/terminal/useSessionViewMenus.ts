import { computed, ref, type Component } from 'vue'
import IconCirclePlay from '~icons/lucide/circle-play'
import IconLayers from '~icons/lucide/layers'
import IconList from '~icons/lucide/list'
import IconSettings from '~icons/lucide/settings-2'
import type { MenuEntry } from '@hive/ui/types/menu'
import { ALL_SESSIONS_VIEW_ID, BUILT_IN_VIEWS, RUNNING_VIEW_ID, type SessionCondition } from '../../lib/sessionViews'
import { useCodeViews } from '../../stores/useCodeViews'
import type { TerminalTree } from './useTerminalTree'

const AGE_PRESETS = [1, 3, 7, 14, 30]

const VIEW_ICONS: Record<string, Component> = {
  [ALL_SESSIONS_VIEW_ID]: IconList,
  [RUNNING_VIEW_ID]: IconCirclePlay,
}

export type SessionViewMenus = ReturnType<typeof useSessionViewMenus>

function splitId(id: string): [string, string] {
  const at = id.indexOf(':')
  return at < 0 ? [id, ''] : [id.slice(0, at), id.slice(at + 1)]
}

/**
 * The menus that drive the Code sidebar's views: the views section of the list
 * menu, the toolbar's filter dropdown, each chip's editor, and the overlays they
 * open. Built once per Code view because the list menu and the bar open the
 * same overlays.
 */
export function useSessionViewMenus(tree: Pick<TerminalTree, 'activeSessions' | 'sessionGroups'>) {
  const views = useCodeViews()

  const knownTags = computed(() => {
    const tags = new Set<string>()
    for (const row of tree.activeSessions.value) for (const tag of row.tags ?? []) tags.add(tag)
    return [...tags].sort((a, b) => a.localeCompare(b))
  })
  const knownRepos = computed(() => tree.sessionGroups.value.filter((group) => group.kind === 'repo'))

  function repoName(identity: string): string {
    return knownRepos.value.find((group) => group.key === identity)?.name ?? identity
  }

  /** Whether the dialog that names a new view from the current chips is open. */
  const saving = ref(false)

  /** Whether the drawer that renames, reorders, and deletes saved views is open. */
  const managing = ref(false)

  // Only the views themselves: saving lives on the bar, which is where a
  // change to the chips shows, and the rest lives in the manage drawer.
  const viewEntries = computed<MenuEntry[]>(() => [
    { kind: 'label', text: 'Views' },
    ...[...BUILT_IN_VIEWS, ...views.views.value].map<MenuEntry>((view) => ({
      kind: 'action',
      id: `view:${view.id}`,
      label: view.name,
      icon: VIEW_ICONS[view.id] ?? IconLayers,
      checked: views.activeViewId.value === view.id,
      testid: BUILT_IN_VIEWS.includes(view) ? `code-view-option-${view.id}` : 'code-view-option',
    })),
    ...(views.views.value.length
      ? ([
          { kind: 'action', id: 'view-manage', label: 'Manage views…', icon: IconSettings, testid: 'code-view-manage' },
        ] satisfies MenuEntry[])
      : []),
  ])

  /** Handles a views-section id from the list menu; false for an id it does not own. */
  function onViewSelect(id: string): boolean {
    if (id.startsWith('view:')) views.select(id.slice('view:'.length))
    else if (id === 'view-manage') managing.value = true
    else return false
    return true
  }

  const filterEntries = computed<MenuEntry[]>(() => [
    { kind: 'label', text: 'Filter by' },
    {
      kind: 'submenu',
      id: 'tag',
      label: 'Tag',
      search: { label: 'Filter tags' },
      testid: 'code-filter-tag',
      entries: knownTags.value.length
        ? knownTags.value.map<MenuEntry>((tag) => ({ kind: 'action', id: `tag:${tag}`, label: tag }))
        : [{ kind: 'action', id: 'noop', label: 'No tagged sessions', disabled: true }],
    },
    {
      kind: 'submenu',
      id: 'repo',
      label: 'Repository',
      search: { label: 'Filter repositories' },
      testid: 'code-filter-repo',
      entries: knownRepos.value.map<MenuEntry>((group) => ({
        kind: 'action',
        id: `repo:${group.key}`,
        label: group.name,
      })),
    },
    {
      kind: 'submenu',
      id: 'status',
      label: 'Status',
      testid: 'code-filter-status',
      entries: [
        { kind: 'action', id: 'status:running', label: 'Running' },
        { kind: 'action', id: 'status:stopped', label: 'Stopped' },
      ],
    },
    {
      kind: 'submenu',
      id: 'age',
      label: 'Age',
      testid: 'code-filter-age',
      entries: AGE_PRESETS.map<MenuEntry>((days) => ({
        kind: 'action',
        id: `age:${days}`,
        label: `Older than ${days}d`,
      })),
    },
  ])

  function onFilterSelect(id: string): void {
    const [field, value] = splitId(id)
    if (field === 'tag') views.addCondition({ field: 'tag', op: 'any', values: [value] })
    else if (field === 'repo') views.addCondition({ field: 'repo', op: 'any', values: [value] })
    else if (field === 'status')
      views.addCondition({ field: 'status', op: 'is', value: value as 'running' | 'stopped' })
    else if (field === 'age') views.addCondition({ field: 'age', op: 'older', days: Number(value) })
  }

  function chipEntries(condition: SessionCondition): MenuEntry[] {
    switch (condition.field) {
      case 'tag':
        return [
          { kind: 'action', id: 'op:any', label: 'Is any of', checked: condition.op === 'any' },
          { kind: 'action', id: 'op:all', label: 'Has all of', checked: condition.op === 'all' },
          { kind: 'action', id: 'op:none', label: 'Is none of', checked: condition.op === 'none' },
          { kind: 'separator' },
          ...knownTags.value.map<MenuEntry>((tag) => ({
            kind: 'action',
            id: `val:${tag}`,
            label: tag,
            checked: condition.values.includes(tag),
          })),
        ]
      case 'repo':
        return [
          { kind: 'action', id: 'op:any', label: 'Is', checked: condition.op === 'any' },
          { kind: 'action', id: 'op:none', label: 'Is not', checked: condition.op === 'none' },
          { kind: 'separator' },
          ...knownRepos.value.map<MenuEntry>((group) => ({
            kind: 'action',
            id: `val:${group.key}`,
            label: group.name,
            checked: condition.values.includes(group.key),
          })),
        ]
      case 'status':
        return [
          { kind: 'action', id: 'val:running', label: 'Running', checked: condition.value === 'running' },
          { kind: 'action', id: 'val:stopped', label: 'Stopped', checked: condition.value === 'stopped' },
        ]
      case 'age':
        return [
          { kind: 'action', id: 'op:older', label: 'Older than', checked: condition.op === 'older' },
          { kind: 'action', id: 'op:newer', label: 'Newer than', checked: condition.op === 'newer' },
          { kind: 'separator' },
          ...AGE_PRESETS.map<MenuEntry>((days) => ({
            kind: 'action',
            id: `val:${days}`,
            label: `${days} ${days === 1 ? 'day' : 'days'}`,
            checked: condition.days === days,
          })),
        ]
      default:
        return [{ kind: 'action', id: 'noop', label: 'Not supported in this version', disabled: true }]
    }
  }

  // The chip's menu stays open after a choice, so several values can be picked in one go.
  function onChipSelect(index: number, id: string): void {
    const condition = views.draft.value.conditions[index]
    const [kind, value] = splitId(id)
    if (kind === 'op') {
      views.updateCondition(index, { ...condition, op: value } as SessionCondition)
      return
    }
    switch (condition.field) {
      case 'tag':
      case 'repo': {
        const values = condition.values.includes(value)
          ? condition.values.filter((v) => v !== value)
          : [...condition.values, value]
        views.updateCondition(index, { ...condition, values })
        break
      }
      case 'status':
        views.updateCondition(index, { ...condition, value: value as 'running' | 'stopped' })
        break
      case 'age':
        views.updateCondition(index, { ...condition, days: Number(value) })
        break
    }
  }

  function onSaveAs(name: string): void {
    saving.value = false
    views.saveAs(name)
  }

  return {
    repoName,
    viewEntries,
    onViewSelect,
    filterEntries,
    onFilterSelect,
    chipEntries,
    onChipSelect,
    saving,
    managing,
    onSaveAs,
  }
}
