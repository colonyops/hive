<script setup lang="ts">
// The saved-view row under the Code sidebar's toolbar: which view is on, the
// filter it applies as chips, and Save / Reset once the chips drift from the
// saved view. Modeled on Linear's view header, cut down for a narrow sidebar.
import { computed, proxyRefs, ref, shallowRef } from 'vue'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconLayers from '~icons/lucide/layers'
import IconListFilter from '~icons/lucide/list-filter'
import IconPencil from '~icons/lucide/pencil'
import IconPlus from '~icons/lucide/plus'
import IconTrash from '~icons/lucide/trash-2'
import IconX from '~icons/lucide/x'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import RenameDialog from '@hive/ui/components/ui/RenameDialog.vue'
import type { MenuEntry } from '@hive/ui/types/menu'
import { ALL_SESSIONS_VIEW_ID, describeCondition, type SessionCondition } from '../../lib/sessionViews'
import { useCodeViews } from '../../stores/useCodeViews'
import { useTerminalModeContext } from './terminalModeContext'

const { tree, view } = useTerminalModeContext()
const views = proxyRefs(useCodeViews())

const AGE_PRESETS = [1, 3, 7, 14, 30]

const knownTags = computed(() => {
  const tags = new Set<string>()
  for (const row of tree.activeSessions) for (const tag of row.tags ?? []) tags.add(tag)
  return [...tags].sort((a, b) => a.localeCompare(b))
})
const knownRepos = computed(() => tree.sessionGroups.filter((group) => group.kind === 'repo'))
function repoName(identity: string): string {
  return knownRepos.value.find((group) => group.key === identity)?.name ?? identity
}

const viewLabel = computed(() => views.activeView?.name ?? 'All sessions')

// ── Menus ───────────────────────────────────────────────────────────────
const openMenu = ref<'' | 'views' | 'add' | `chip:${number}`>('')
const viewsAnchor = shallowRef<HTMLElement | null>(null)
const addAnchor = shallowRef<HTMLElement | null>(null)
const chipAnchors = ref<HTMLElement[]>([])

const viewsEntries = computed<MenuEntry[]>(() => [
  { kind: 'label', text: 'Views' },
  {
    kind: 'action',
    id: `view:${ALL_SESSIONS_VIEW_ID}`,
    label: 'All sessions',
    checked: views.activeViewId === ALL_SESSIONS_VIEW_ID,
  },
  ...views.views.map<MenuEntry>((saved) => ({
    kind: 'action',
    id: `view:${saved.id}`,
    label: saved.name,
    checked: views.activeViewId === saved.id,
    testid: 'code-view-option',
  })),
  { kind: 'separator' },
  { kind: 'action', id: 'save-as', label: 'Save as new view…', icon: IconPlus, testid: 'code-view-save-as' },
  ...(views.activeView
    ? ([
        { kind: 'action', id: 'rename', label: 'Rename view…', icon: IconPencil },
        { kind: 'action', id: 'delete', label: 'Delete view', icon: IconTrash, testid: 'code-view-delete' },
      ] satisfies MenuEntry[])
    : []),
])

function onViewsSelect(id: string): void {
  openMenu.value = ''
  if (id.startsWith('view:')) views.select(id.slice('view:'.length))
  else if (id === 'save-as') dialog.value = { kind: 'save-as' }
  else if (id === 'rename') dialog.value = { kind: 'rename' }
  else if (id === 'delete' && views.activeView) views.remove(views.activeView.id)
}

const addEntries = computed<MenuEntry[]>(() => [
  {
    kind: 'submenu',
    id: 'tag',
    label: 'Tag',
    search: { label: 'Filter tags' },
    entries: knownTags.value.length
      ? knownTags.value.map<MenuEntry>((tag) => ({ kind: 'action', id: `tag:${tag}`, label: tag }))
      : [{ kind: 'action', id: 'noop', label: 'No tagged sessions', disabled: true }],
  },
  {
    kind: 'submenu',
    id: 'repo',
    label: 'Repository',
    search: { label: 'Filter repositories' },
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
    entries: [
      { kind: 'action', id: 'status:running', label: 'Running' },
      { kind: 'action', id: 'status:stopped', label: 'Stopped' },
    ],
  },
  {
    kind: 'submenu',
    id: 'age',
    label: 'Age',
    entries: AGE_PRESETS.map<MenuEntry>((days) => ({
      kind: 'action',
      id: `age:${days}`,
      label: `Older than ${days}d`,
    })),
  },
  { kind: 'action', id: 'text', label: 'Text…' },
])

function onAddSelect(id: string): void {
  openMenu.value = ''
  const [field, value] = splitId(id)
  if (field === 'tag') views.addCondition({ field: 'tag', op: 'any', values: [value] })
  else if (field === 'repo') views.addCondition({ field: 'repo', op: 'any', values: [value] })
  else if (field === 'status') views.addCondition({ field: 'status', op: 'is', value: value as 'running' | 'stopped' })
  else if (field === 'age') views.addCondition({ field: 'age', op: 'older', days: Number(value) })
  else if (field === 'text') dialog.value = { kind: 'text', index: -1 }
}

function splitId(id: string): [string, string] {
  const at = id.indexOf(':')
  return at < 0 ? [id, ''] : [id.slice(0, at), id.slice(at + 1)]
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
    case 'text':
      return [
        { kind: 'action', id: 'op:contains', label: 'Contains', checked: condition.op === 'contains' },
        { kind: 'action', id: 'op:excludes', label: 'Excludes', checked: condition.op === 'excludes' },
        { kind: 'separator' },
        { kind: 'action', id: 'edit', label: 'Edit text…', icon: IconPencil },
      ]
    default:
      return [{ kind: 'action', id: 'noop', label: 'Not supported in this version', disabled: true }]
  }
}

// Multi-value toggles keep the menu open so several tags can be picked in one go.
function onChipSelect(index: number, id: string): void {
  const condition = views.draft.conditions[index]
  const [kind, value] = splitId(id)
  if (kind === 'edit') {
    openMenu.value = ''
    dialog.value = { kind: 'text', index }
    return
  }
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
      return
    }
    case 'status':
      views.updateCondition(index, { ...condition, value: value as 'running' | 'stopped' })
      return
    case 'age':
      views.updateCondition(index, { ...condition, days: Number(value) })
      return
    case 'text':
      return
  }
}

// ── Dialogs ─────────────────────────────────────────────────────────────
const dialog = ref<{ kind: 'save-as' } | { kind: 'rename' } | { kind: 'text'; index: number } | null>(null)

const textDialogSeed = computed(() => {
  const current = dialog.value
  if (current?.kind !== 'text' || current.index < 0) return ''
  const condition = views.draft.conditions[current.index]
  return condition?.field === 'text' ? condition.value : ''
})

function onDialogSave(name: string): void {
  const current = dialog.value
  dialog.value = null
  if (!current) return
  if (current.kind === 'save-as') views.saveAs(name)
  else if (current.kind === 'rename' && views.activeView) views.rename(views.activeView.id, name)
  else if (current.kind === 'text') {
    if (current.index < 0) views.addCondition({ field: 'text', op: 'contains', value: name })
    else {
      const condition = views.draft.conditions[current.index]
      if (condition.field === 'text') views.updateCondition(current.index, { ...condition, value: name })
    }
  }
}
</script>

<template>
  <div class="shrink-0 border-b border-border" data-testid="code-view-bar">
    <div class="flex h-8 items-center gap-1 px-2">
      <div ref="viewsAnchor" class="relative flex min-w-0">
        <button
          type="button"
          class="view-switch"
          data-testid="code-view-switch"
          aria-haspopup="menu"
          :aria-expanded="openMenu === 'views'"
          @click="openMenu = openMenu === 'views' ? '' : 'views'"
        >
          <IconLayers class="size-3.5 shrink-0 text-text-3" />
          <span class="min-w-0 truncate">{{ viewLabel }}</span>
          <span v-if="views.dirty" class="text-accent" title="Unsaved filter changes">•</span>
          <IconChevronDown class="size-3 shrink-0 text-text-4" />
        </button>
        <AppMenu
          v-if="openMenu === 'views'"
          :entries="viewsEntries"
          :ignore="[viewsAnchor]"
          width="14rem"
          testid="code-view-menu"
          @close="openMenu = ''"
          @select="onViewsSelect"
        />
      </div>
      <span class="flex-1" />
      <span
        v-if="view.viewHiddenCount"
        class="shrink-0 font-mono text-caption text-text-4"
        data-testid="code-view-hidden"
        :title="`${view.viewHiddenCount} sessions hidden by this view`"
        >{{ view.viewHiddenCount }} hidden</span
      >
      <div ref="addAnchor" class="relative flex">
        <IconButton
          label="Add filter"
          :icon="IconListFilter"
          size="sm"
          data-testid="code-view-add-filter"
          aria-haspopup="menu"
          :aria-expanded="openMenu === 'add'"
          @click="openMenu = openMenu === 'add' ? '' : 'add'"
        />
        <AppMenu
          v-if="openMenu === 'add'"
          :entries="addEntries"
          :ignore="[addAnchor]"
          width="12rem"
          testid="code-view-add-menu"
          @close="openMenu = ''"
          @select="onAddSelect"
        />
      </div>
    </div>

    <div
      v-if="views.draft.conditions.length || views.dirty"
      class="flex flex-wrap items-center gap-1 px-2 pb-2"
      data-testid="code-view-chips"
    >
      <button
        v-if="views.draft.conditions.length > 1"
        type="button"
        class="chip text-text-3"
        :title="views.draft.match === 'all' ? 'Rows must match every filter' : 'Rows may match any filter'"
        data-testid="code-view-match"
        @click="views.setMatch(views.draft.match === 'all' ? 'any' : 'all')"
      >
        Match {{ views.draft.match }}
      </button>
      <div v-for="(condition, index) in views.draft.conditions" :key="index" ref="chipAnchors" class="relative flex">
        <span class="chip" data-testid="code-view-chip">
          <button
            type="button"
            class="min-w-0 cursor-pointer truncate"
            @click="openMenu = openMenu === `chip:${index}` ? '' : `chip:${index}`"
          >
            {{ describeCondition(condition, repoName) }}
          </button>
          <button
            type="button"
            class="chip-remove"
            :aria-label="`Remove filter ${index + 1}`"
            data-testid="code-view-chip-remove"
            @click="views.removeCondition(index)"
          >
            <IconX class="size-3" />
          </button>
        </span>
        <AppMenu
          v-if="openMenu === `chip:${index}`"
          :entries="chipEntries(condition)"
          :ignore="[chipAnchors[index] ?? null]"
          width="12rem"
          @close="openMenu = ''"
          @select="(id) => onChipSelect(index, id)"
        />
      </div>
      <template v-if="views.dirty">
        <span class="flex-1" />
        <button type="button" class="bar-action" data-testid="code-view-reset" @click="views.reset()">Reset</button>
        <button
          v-if="views.activeView"
          type="button"
          class="bar-action text-accent"
          data-testid="code-view-save"
          @click="views.save()"
        >
          Save
        </button>
        <button
          v-else
          type="button"
          class="bar-action text-accent"
          data-testid="code-view-save-new"
          @click="dialog = { kind: 'save-as' }"
        >
          Save view
        </button>
      </template>
    </div>

    <RenameDialog
      v-if="dialog?.kind === 'save-as'"
      title="Save view"
      label="Name"
      confirm-label="Save"
      :icon="IconLayers"
      testid="code-view-save-dialog"
      @close="dialog = null"
      @save="onDialogSave"
    />
    <RenameDialog
      v-else-if="dialog?.kind === 'rename'"
      title="Rename view"
      label="Name"
      :name="views.activeView?.name ?? ''"
      testid="code-view-rename-dialog"
      @close="dialog = null"
      @save="onDialogSave"
    />
    <RenameDialog
      v-else-if="dialog?.kind === 'text'"
      title="Filter by text"
      label="Matches name, slug, or tag"
      :name="textDialogSeed"
      :confirm-label="dialog.index < 0 ? 'Add' : 'Update'"
      :icon="IconListFilter"
      testid="code-view-text-dialog"
      @close="dialog = null"
      @save="onDialogSave"
    />
  </div>
</template>

<style scoped>
.view-switch {
  display: flex;
  min-width: 0;
  cursor: pointer;
  align-items: center;
  gap: 0.375rem;
  border-radius: var(--radius-sm);
  padding: 0.125rem 0.375rem;
  font-size: var(--text-caption);
  color: var(--color-text-2);
}
.view-switch:hover {
  background: var(--color-chip);
  color: var(--color-text);
}
.chip {
  display: inline-flex;
  max-width: 100%;
  align-items: center;
  gap: 0.25rem;
  border: 1px solid var(--color-border);
  border-radius: 999px;
  background: var(--color-chip);
  padding: 0.0625rem 0.375rem 0.0625rem 0.5rem;
  font-size: var(--text-caption);
  color: var(--color-text-2);
}
.chip-remove {
  display: inline-flex;
  cursor: pointer;
  color: var(--color-text-4);
}
.chip-remove:hover {
  color: var(--color-text);
}
.bar-action {
  cursor: pointer;
  font-size: var(--text-caption);
  color: var(--color-text-3);
}
.bar-action:hover {
  color: var(--color-text);
}
</style>
