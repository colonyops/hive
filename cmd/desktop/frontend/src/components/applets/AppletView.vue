<script setup lang="ts">
import { useEventListener, useIntervalFn } from '@vueuse/core'
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import ConfirmationHost from '@hive/ui/components/ui/ConfirmationHost.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Kbd from '@hive/ui/components/ui/Kbd.vue'
import SearchField from '@hive/ui/components/ui/SearchField.vue'
import { useConfirmation } from '@hive/ui/composables/useConfirmation'
import { useEscapeToClose } from '@hive/ui/composables/useEscapeToClose'
import {
  Rows,
  Run,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice'
import type {
  Applet,
  AppletAction,
  AppletColumn,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { useToasts } from '../../composables/useToasts'
import { filterAppletRows, reselect, type AppletRow } from '../../lib/applets'
import { isEditableTarget } from '../../lib/isEditableTarget'
import { launcherIconComponent } from '../../lib/launcherIcons'
import { usePopupTerminal } from '../../stores/usePopupTerminal'

// Rendering every row of a five-thousand-line listing is what freezes a webview, and
// nobody reads past the first screen of an unfiltered list.
const MAX_RENDERED_ROWS = 500

const TONE_DOTS: Record<string, string> = {
  neutral: 'bg-text-3',
  success: 'bg-severity-success',
  accent: 'bg-accent',
  muted: 'bg-text-4',
  danger: 'bg-severity-error',
}

const props = defineProps<{ applet: Applet; sessionSlug: string }>()
const emit = defineEmits<{ close: [] }>()

const popupTerminal = usePopupTerminal()
const { showToast } = useToasts()
const confirmation = useConfirmation()

const isPicker = computed(() => props.applet.view === 'picker')
const icon = computed(() => launcherIconComponent(props.applet.icon))

const rows = ref<AppletRow[]>([])
const truncated = ref(false)
const loaded = ref(false)
const loading = ref(false)
const loadError = ref<string | null>(null)
const query = ref('')
const selected = ref(0)
const runningAction = ref<string | null>(null)
const filter = ref<{ focus: () => void } | null>(null)
const list = ref<HTMLElement | null>(null)

const matched = computed(() => filterAppletRows(rows.value, props.applet.columns ?? [], query.value))
const visible = computed(() => matched.value.slice(0, MAX_RENDERED_ROWS))
const selectedRow = computed<AppletRow | undefined>(() => visible.value[selected.value])
const actions = computed(() => props.applet.actions ?? [])

async function load(): Promise<void> {
  // A slow command must not stack a second copy of itself on every refresh tick.
  if (loading.value) return
  loading.value = true
  const previous = selectedRow.value
  try {
    const result = await Rows(props.applet.id, props.sessionSlug)
    rows.value = (result.rows ?? []) as AppletRow[]
    truncated.value = result.truncated
    loadError.value = null
    selected.value = reselect(visible.value, props.applet.columns ?? [], previous, selected.value)
  } catch (cause) {
    // The rows from a moment ago stay up beside the error: a daemon that dropped out
    // is exactly when the last known state is worth seeing.
    loadError.value = cause instanceof Error ? cause.message : 'The data command failed.'
  } finally {
    loading.value = false
    loaded.value = true
  }
}

async function runInBackground(action: AppletAction, row: AppletRow): Promise<void> {
  runningAction.value = action.id
  try {
    const output = await Run(props.applet.id, action.id, props.sessionSlug, row)
    showToast(`${action.label}: done`, { severity: 'success', body: output || undefined })
  } catch (cause) {
    showToast(`${action.label} failed`, {
      severity: 'error',
      body: cause instanceof Error ? cause.message : undefined,
    })
  } finally {
    runningAction.value = null
  }
  if (!isPicker.value) await load()
}

async function execute(action: AppletAction, row: AppletRow): Promise<void> {
  const picker = isPicker.value
  if (action.mode === 'terminal') {
    popupTerminal.launch({
      applet: props.applet.id,
      appletAction: action.id,
      appletRow: row,
      sessionSlug: props.sessionSlug || undefined,
    })
  }
  if (picker) emit('close')
  if (action.mode !== 'terminal') await runInBackground(action, row)
}

function invoke(action: AppletAction | undefined): void {
  const row = selectedRow.value
  if (!action || !row || runningAction.value) return
  if (!action.confirm) {
    void execute(action, row)
    return
  }
  confirmation.request({
    title: action.label,
    description: action.confirm,
    confirmLabel: action.label,
    testid: 'applet-confirm',
    onConfirm: () => {
      void execute(action, row)
    },
  })
}

function move(delta: number): void {
  if (visible.value.length === 0) return
  selected.value = Math.max(0, Math.min(selected.value + delta, visible.value.length - 1))
  void nextTick(() => {
    list.value?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' })
  })
}

function toneDot(column: AppletColumn, value: string | undefined): string | null {
  const tone = value === undefined ? undefined : column.tones?.[value]
  return tone ? (TONE_DOTS[tone] ?? null) : null
}

// On the window because the overlay holds focus on its own panel, and a stacked
// surface (the palette, a confirm, the pop-up terminal) takes these keys first.
useEventListener(window, 'keydown', (event: KeyboardEvent) => {
  const inFilter = event.target instanceof HTMLElement && event.target.dataset.testid === 'applet-filter'
  if (popupTerminal.visible.value || confirmation.options.value) return
  if (isEditableTarget(event.target) && !inFilter) return
  if (event.metaKey || event.ctrlKey || event.altKey) return
  // Enter on a focused button is that button's own click.
  if (event.key === 'Enter' && event.target instanceof HTMLButtonElement) return

  if (event.key === 'ArrowDown') move(1)
  else if (event.key === 'ArrowUp') move(-1)
  else if (event.key === 'Enter') invoke(actions.value[0])
  else if (inFilter) return
  else if (event.key === 'j') move(1)
  else if (event.key === 'k') move(-1)
  else if (event.key === 'g') move(-selected.value)
  else if (event.key === 'G') move(visible.value.length)
  else if (event.key === '/') filter.value?.focus()
  else {
    const action = actions.value.find((candidate) => candidate.key === event.key)
    if (!action) return
    invoke(action)
  }
  event.preventDefault()
})

// A picker sits in a BaseModal, which owns Escape. A table sits in a HubOverlay,
// which leaves it to the view.
useEscapeToClose(() => emit('close'), {
  enabled: () => !isPicker.value && !popupTerminal.visible.value && !confirmation.options.value,
})

useIntervalFn(
  () => {
    void load()
  },
  () => props.applet.refreshMillis,
)

watch(query, () => {
  selected.value = 0
})

// An agent rewriting the file while the applet is open is the normal way to iterate.
watch(
  () => props.applet,
  () => {
    void load()
  },
)

onMounted(() => {
  void load()
  if (isPicker.value) filter.value?.focus()
})
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col" :class="isPicker ? 'h-[60vh]' : ''" data-testid="applet-view">
    <header v-if="!isPicker" class="flex h-12 shrink-0 items-center gap-2.5 border-b border-row pr-3 pl-4">
      <component :is="icon" class="size-4 shrink-0 text-text-3" />
      <h2 class="truncate text-small font-semibold text-text" data-testid="applet-title">{{ applet.title }}</h2>
      <div class="flex-1" />
      <IconButton
        label="Refresh"
        :icon="IconRefreshCw"
        size="md"
        :busy="loading"
        data-testid="applet-refresh"
        @click="load"
      />
      <Kbd variant="boxed">esc</Kbd>
    </header>

    <div class="flex h-10 shrink-0 items-center gap-3 border-b border-row px-4">
      <SearchField
        ref="filter"
        v-model="query"
        variant="bar"
        :placeholder="isPicker ? 'Type to filter…' : 'Filter…'"
        aria-label="Filter rows"
        testid="applet-filter"
      />
      <Kbd v-if="!isPicker && !query" variant="boxed">/</Kbd>
      <span class="shrink-0 text-caption text-text-4 tabular-nums" data-testid="applet-count">
        {{ matched.length }}<template v-if="query"> of {{ rows.length }}</template>
      </span>
    </div>

    <InlineError
      v-if="applet.error"
      class="mx-4 mt-3"
      :message="`${applet.path}: ${applet.error}`"
      testid="applet-file-error"
    />
    <InlineError v-if="loadError" class="mx-4 mt-3" :message="loadError" testid="applet-load-error" />

    <div ref="list" class="min-h-0 flex-1 overflow-auto" role="listbox" :aria-label="applet.title">
      <table v-if="visible.length" class="w-full border-collapse text-small">
        <thead v-if="!isPicker && (applet.columns?.length ?? 0) > 1" class="sticky top-0 z-10 bg-pane">
          <tr>
            <th
              v-for="column in applet.columns"
              :key="column.key"
              class="border-b border-row px-4 py-2 text-left text-caption font-normal whitespace-nowrap text-text-4"
            >
              {{ column.label }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="(row, index) in visible"
            :key="index"
            role="option"
            :aria-selected="index === selected"
            class="cursor-default border-b border-row transition-colors"
            :class="index === selected ? 'bg-hover' : 'hover:bg-row-hover'"
            data-testid="applet-row"
            @click="selected = index"
            @dblclick="invoke(actions[0])"
          >
            <td
              v-for="(column, columnIndex) in applet.columns"
              :key="column.key"
              class="relative max-w-[44ch] truncate px-4 py-2"
              :class="[
                columnIndex === 0 ? 'font-medium text-text' : 'text-text-3',
                columnIndex === 0 && index === selected
                  ? 'before:absolute before:inset-y-1.5 before:left-1 before:w-0.5 before:rounded-full before:bg-accent'
                  : '',
              ]"
            >
              <span v-if="toneDot(column, row[column.key])" class="inline-flex items-center gap-2">
                <span class="size-1.5 shrink-0 rounded-full" :class="toneDot(column, row[column.key])" />
                {{ row[column.key] }}
              </span>
              <template v-else>{{ row[column.key] }}</template>
            </td>
          </tr>
        </tbody>
      </table>
      <EmptyState
        v-else-if="!loadError"
        :message="!loaded ? 'Loading…' : query ? 'Nothing matches that filter.' : 'The data command returned no rows.'"
      />
    </div>

    <footer
      v-if="actions.length || truncated || matched.length > visible.length"
      class="flex min-h-11 shrink-0 items-center gap-3 border-t border-row px-4 py-1.5"
    >
      <span
        v-if="truncated || matched.length > visible.length"
        class="min-w-0 truncate text-caption text-text-4"
        data-testid="applet-truncated"
      >
        Showing {{ visible.length }} of {{ truncated ? `more than ${rows.length}` : matched.length }}. Filter to narrow
        it.
      </span>
      <span v-else-if="selectedRow" class="min-w-0 truncate text-caption text-text-4" data-testid="applet-selection">
        {{ selectedRow[applet.columns?.[0]?.key ?? ''] }}
      </span>
      <div class="ml-auto flex shrink-0 flex-wrap items-center justify-end gap-1">
        <BaseButton
          v-for="(action, index) in actions"
          :key="action.id"
          size="xs"
          variant="ghost"
          :disabled="!selectedRow || runningAction !== null"
          :busy="runningAction === action.id"
          :data-testid="`applet-action-${action.id}`"
          @click="invoke(action)"
        >
          {{ action.label }}
          <Kbd v-if="index === 0" variant="boxed" class="ml-1">↵</Kbd>
          <Kbd v-else-if="action.key && !isPicker" variant="boxed" class="ml-1">{{ action.key }}</Kbd>
        </BaseButton>
      </div>
    </footer>
    <ConfirmationHost :confirmation="confirmation" />
  </div>
</template>
