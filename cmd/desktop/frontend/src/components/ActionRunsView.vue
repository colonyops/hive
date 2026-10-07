<script setup lang="ts">
// The action run viewer: recent runs on the left, the selected run's log on
// the right. The log is what the run's executor wrote through its RunLog, so a
// shell action shows its interleaved output and an internal action (a
// session launch, a published message, a notification) shows the steps it
// took. Shown in a HubOverlay, opened from the title bar, the jobs menu, or an
// item's action card.
import { computed, nextTick, onMounted, onScopeDispose, ref, watch } from 'vue'
import { useIntervalFn } from '@vueuse/core'
import IconCheck from '~icons/lucide/check'
import IconCircleAlert from '~icons/lucide/circle-alert'
import IconClock3 from '~icons/lucide/clock-3'
import IconCopy from '~icons/lucide/copy'
import IconExternalLink from '~icons/lucide/external-link'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import IconCircleStop from '~icons/lucide/circle-stop'
import IconX from '~icons/lucide/x'
import {
  List,
  Log,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/actionrunservice'
import { Cancel } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/jobservice'
import { ActionRun } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/pipelineservice'
import type {
  ActionRunLogLine,
  ActionRunSummary,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { useClipboard } from '../composables/useClipboard'
import { useEscapeToClose } from '../composables/useEscapeToClose'
import { useWailsEvent } from '../composables/useWailsEvent'
import { relativeAgo } from '../lib/age'
import { timeLabel } from '../lib/activityPresentation'
import { errorText } from '../lib/appError'
import {
  RUN_FILTERS,
  formatDuration,
  groupByAttempt,
  laneLabel,
  logText,
  matchesRun,
  runDurationMs,
  runIsActive,
  runStatusLabel,
  type RunFilter,
} from '../lib/actionRunPresentation'
import BaseButton from './ui/BaseButton.vue'
import EmptyState from './ui/EmptyState.vue'
import IconButton from './ui/IconButton.vue'
import SearchField from './ui/SearchField.vue'
import SegmentedControl from './ui/SegmentedControl.vue'
import Spinner from './ui/Spinner.vue'
import ViewHeader from './ui/ViewHeader.vue'

const props = defineProps<{ initialRunId?: number | null }>()
const emit = defineEmits<{
  close: []
  'open-item': [commandId: number, actionId: string]
}>()

const LIST_LIMIT = 200
const LOG_PAGE = 2000
const LOG_POLL_MS = 500
const LIST_POLL_MS = 2000

const runs = ref<ActionRunSummary[]>([])
const loaded = ref(false)
const listError = ref<string | null>(null)
const filter = ref<RunFilter>('all')
const search = ref('')
const selectedId = ref<number | null>(null)
const now = ref(Date.now())

const lines = ref<ActionRunLogLine[]>([])
const logLoaded = ref(false)
const logError = ref<string | null>(null)
const fallback = ref<{ stdout: string; stderr: string } | null>(null)
const cancelling = ref(false)
const { copy, copied } = useClipboard()
const logEl = ref<HTMLElement | null>(null)
let logCursor = 0
let logGeneration = 0

const visibleRuns = computed(() => runs.value.filter((run) => matchesRun(run, filter.value, search.value)))
const selected = computed(() => runs.value.find((run) => run.id === selectedId.value) ?? null)
const groups = computed(() => groupByAttempt(lines.value))
const showAttemptHeaders = computed(() => groups.value.length > 1 || (groups.value[0]?.attempt ?? 1) > 1)
const anyActive = computed(() => runs.value.some((run) => runIsActive(run.status)))
const filterOptions = RUN_FILTERS

async function loadRuns(): Promise<void> {
  try {
    runs.value = (await List(0, LIST_LIMIT)) ?? []
    listError.value = null
  } catch (error) {
    listError.value = errorText(error, 'Could not load action runs.')
  } finally {
    loaded.value = true
  }
  if (selectedId.value === null && runs.value.length) selectedId.value = runs.value[0].id
}

function nearBottom(): boolean {
  const el = logEl.value
  return !el || el.scrollHeight - el.scrollTop - el.clientHeight < 48
}

async function scrollToBottom(): Promise<void> {
  await nextTick()
  const el = logEl.value
  if (el) el.scrollTop = el.scrollHeight
}

// Reads every line past the cursor. A newer selection bumps the generation,
// so a read for the previous run never lands in this one's log.
async function readLog(reset: boolean): Promise<void> {
  const id = selectedId.value
  if (id === null) return
  if (reset) {
    logGeneration++
    lines.value = []
    logCursor = 0
    logLoaded.value = false
    logError.value = null
    fallback.value = null
  }
  const generation = logGeneration
  const follow = reset || nearBottom()
  try {
    for (;;) {
      const page = await Log(id, logCursor, LOG_PAGE)
      if (generation !== logGeneration) return
      const next = page.lines ?? []
      if (next.length) lines.value = [...lines.value, ...next]
      logCursor = page.nextAfterId
      if (!page.more) break
    }
    logError.value = null
  } catch (error) {
    if (generation === logGeneration) logError.value = errorText(error, 'Could not load the log.')
    return
  } finally {
    if (generation === logGeneration) logLoaded.value = true
  }
  if (!lines.value.length && selected.value && !runIsActive(selected.value.status)) await loadFallback(id, generation)
  if (follow) await scrollToBottom()
}

// A run from before run logs existed still has its bounded stdout and stderr
// on the command record.
async function loadFallback(id: number, generation: number): Promise<void> {
  try {
    const run = await ActionRun(id)
    if (generation !== logGeneration) return
    fallback.value = run.stdout || run.stderr ? { stdout: run.stdout ?? '', stderr: run.stderr ?? '' } : null
  } catch (error) {
    console.warn('Unable to read action run output', error)
  }
}

function select(id: number): void {
  if (selectedId.value === id) return
  selectedId.value = id
}

function moveSelection(step: number): void {
  const list = visibleRuns.value
  if (!list.length) return
  const index = list.findIndex((run) => run.id === selectedId.value)
  const next = list[Math.min(list.length - 1, Math.max(0, index + step))]
  if (next) select(next.id)
}

function onListKeydown(event: KeyboardEvent): void {
  if (event.key === 'ArrowDown' || event.key === 'j') {
    event.preventDefault()
    moveSelection(1)
  } else if (event.key === 'ArrowUp' || event.key === 'k') {
    event.preventDefault()
    moveSelection(-1)
  }
}

async function cancelSelected(): Promise<void> {
  const run = selected.value
  if (!run || cancelling.value) return
  cancelling.value = true
  try {
    await Cancel(run.id)
  } catch (error) {
    logError.value = errorText(error, 'Could not cancel the run.')
  } finally {
    cancelling.value = false
  }
  await loadRuns()
}

watch(selectedId, () => {
  void readLog(true)
})

watch(
  () => props.initialRunId,
  (id) => {
    if (id) selectedId.value = id
  },
  { immediate: true },
)

// The worker writes the last lines before the terminal status, so one read
// after a run finishes is complete.
watch(
  () => selected.value?.status,
  (status, previous) => {
    if (previous && runIsActive(previous) && status && !runIsActive(status)) void readLog(false)
  },
)

let logTimer: ReturnType<typeof setTimeout> | null = null
let listTimer: ReturnType<typeof setTimeout> | null = null

function scheduleLogPoll(): void {
  logTimer = setTimeout(() => {
    void (async () => {
      if (selected.value && runIsActive(selected.value.status)) await readLog(false)
      scheduleLogPoll()
    })()
  }, LOG_POLL_MS)
}

function scheduleListPoll(): void {
  listTimer = setTimeout(() => {
    void (async () => {
      if (anyActive.value) await loadRuns()
      scheduleListPoll()
    })()
  }, LIST_POLL_MS)
}

useWailsEvent('jobs:updated', () => {
  void loadRuns()
})
useEscapeToClose(() => emit('close'))
useIntervalFn(() => (now.value = Date.now()), 1000)

onMounted(() => {
  void loadRuns().then(() => readLog(true))
  scheduleLogPoll()
  scheduleListPoll()
})

onScopeDispose(() => {
  if (logTimer) clearTimeout(logTimer)
  if (listTimer) clearTimeout(listTimer)
})

function statusIconClass(status: string): string {
  if (status === 'failed') return 'text-severity-error'
  if (status === 'done') return 'text-severity-success'
  if (status === 'running') return 'text-accent'
  return 'text-text-3'
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-1 flex-col" data-testid="action-runs-view">
    <ViewHeader>
      <template #title>
        <span class="text-body font-semibold text-text">Action runs</span>
        <span class="font-mono text-caption text-text-4"
          >{{ runs.length }} {{ runs.length === 1 ? 'run' : 'runs' }}</span
        >
        <div class="flex-1" />
        <IconButton label="Close" :icon="IconX" size="lg" data-testid="action-runs-close" @click="emit('close')" />
      </template>
    </ViewHeader>

    <div class="flex shrink-0 items-center gap-2.5 border-b border-row bg-sidebar px-5 py-2.5">
      <SegmentedControl
        v-model="filter"
        variant="compact"
        :options="filterOptions"
        aria-label="Filter action runs"
        testid="action-runs-filter"
      />
      <SearchField
        v-model="search"
        placeholder="Filter runs…"
        aria-label="Filter action runs"
        testid="action-runs-search"
        class="w-[230px]"
      />
      <div class="flex-1" />
      <BaseButton variant="secondary" size="xs" data-testid="action-runs-refresh" @click="loadRuns">
        <template #icon><IconRefreshCw class="size-3.5" /></template>Refresh
      </BaseButton>
    </div>

    <div
      v-if="listError"
      class="shrink-0 border-b border-severity-error-border bg-severity-error-tint px-5 py-2 text-xs text-severity-error"
      role="alert"
      data-testid="action-runs-error"
    >
      Couldn't load action runs — {{ listError }}
    </div>

    <div class="flex min-h-0 flex-1">
      <!-- runs -->
      <div
        class="hive-scroll w-[360px] shrink-0 overflow-y-auto border-r border-row bg-sidebar"
        tabindex="0"
        role="listbox"
        aria-label="Action runs"
        data-testid="action-runs-list"
        @keydown="onListKeydown"
      >
        <div v-if="!loaded" class="px-5 py-8 text-center font-mono text-xs text-text-4">Loading runs…</div>
        <EmptyState
          v-else-if="!runs.length"
          class="px-6 font-mono"
          message="No action runs yet. Item actions and flow actions show up here when they run."
          data-testid="action-runs-empty"
        />
        <div v-else-if="!visibleRuns.length" class="px-5 py-8 text-center font-mono text-xs text-text-4">
          No runs match this filter.
        </div>
        <button
          v-for="run in visibleRuns"
          :key="run.id"
          type="button"
          role="option"
          :aria-selected="run.id === selectedId"
          class="flex w-full cursor-pointer items-start gap-2.5 border-b border-row px-4 py-2.5 text-left hover:bg-row-hover"
          :class="run.id === selectedId ? 'bg-selection' : ''"
          data-testid="action-run-row"
          :data-run-id="run.id"
          @click="select(run.id)"
        >
          <span class="mt-0.5 flex size-4 shrink-0 items-center justify-center" :class="statusIconClass(run.status)">
            <Spinner v-if="run.status === 'running'" />
            <IconClock3 v-else-if="run.status === 'pending'" class="size-3.5" />
            <IconCheck v-else-if="run.status === 'done'" class="size-3.5" />
            <IconCircleStop v-else-if="run.status === 'cancelled'" class="size-3.5" />
            <IconCircleAlert v-else class="size-3.5" />
          </span>
          <span class="min-w-0 flex-1">
            <span class="block truncate text-small font-medium text-text">{{ run.label }}</span>
            <span class="mt-0.5 block truncate text-micro text-text-3">
              {{ run.itemTitle || run.key || run.actionId }}
            </span>
            <span class="mt-0.5 flex items-center gap-1.5 font-mono text-micro text-text-4">
              <span>#{{ run.id }}</span>
              <span>·</span>
              <span>{{ laneLabel(run.lane) }}</span>
              <template v-if="run.attempts > 1">
                <span>·</span>
                <span>{{ run.attempts }} attempts</span>
              </template>
            </span>
          </span>
          <span class="flex shrink-0 flex-col items-end gap-0.5 font-mono text-micro text-text-4">
            <span>{{ relativeAgo(run.createdAt, now) }}</span>
            <span>{{ formatDuration(runDurationMs(run, now)) }}</span>
          </span>
        </button>
      </div>

      <!-- detail -->
      <section class="flex min-w-0 flex-1 flex-col bg-app" data-testid="action-run-detail">
        <EmptyState
          v-if="!selected"
          class="m-auto px-6 font-mono"
          message="Select a run to read its log."
          data-testid="action-run-none"
        />
        <template v-else>
          <div class="flex shrink-0 items-start gap-3 border-b border-row px-5 py-3">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2.5">
                <span class="truncate text-body font-semibold text-text" data-testid="action-run-title">{{
                  selected.label
                }}</span>
                <span
                  class="flex shrink-0 items-center gap-1 text-caption font-medium"
                  :class="statusIconClass(selected.status)"
                  data-testid="action-run-status"
                >
                  <Spinner v-if="selected.status === 'running'" />
                  <IconClock3 v-else-if="selected.status === 'pending'" class="size-3.5" />
                  <IconCheck v-else-if="selected.status === 'done'" class="size-3.5" />
                  <IconCircleStop v-else-if="selected.status === 'cancelled'" class="size-3.5" />
                  <IconCircleAlert v-else class="size-3.5" />
                  {{ runStatusLabel(selected.status) }}
                </span>
              </div>
              <div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-micro text-text-3">
                <span>#{{ selected.id }}</span>
                <span>{{ selected.actionId }}</span>
                <span v-if="selected.type">{{ selected.type }}</span>
                <span>{{ laneLabel(selected.lane) }}{{ selected.rerun ? ' · rerun' : '' }}</span>
                <span v-if="selected.startedAt">started {{ timeLabel(selected.startedAt) }}</span>
                <span>{{ formatDuration(runDurationMs(selected, now)) }}</span>
                <span v-if="selected.attempts > 1">{{ selected.attempts }} attempts</span>
              </div>
              <div v-if="selected.itemTitle" class="mt-1 truncate text-caption text-text-2">
                {{ selected.itemTitle }}
              </div>
            </div>
            <div class="flex shrink-0 items-center gap-1">
              <IconButton
                v-if="selected.status === 'running'"
                label="Cancel run"
                :icon="IconCircleStop"
                tone="danger"
                :busy="cancelling"
                data-testid="action-run-cancel"
                @click="cancelSelected"
              />
              <IconButton
                v-if="selected.itemId"
                label="Open item"
                :icon="IconExternalLink"
                data-testid="action-run-open-item"
                @click="emit('open-item', selected.id, selected.actionId)"
              />
              <IconButton
                :label="copied ? 'Copied' : 'Copy log'"
                :icon="copied ? IconCheck : IconCopy"
                :disabled="!lines.length"
                data-testid="action-run-copy"
                @click="copy(logText(lines))"
              />
            </div>
          </div>

          <div
            v-if="selected.error && !runIsActive(selected.status)"
            class="shrink-0 border-b border-row px-5 py-2 font-mono text-caption"
            :class="selected.status === 'failed' ? 'bg-severity-error-tint text-severity-error' : 'text-text-3'"
            data-testid="action-run-error"
          >
            {{ selected.error }}
          </div>
          <div
            v-if="logError"
            class="shrink-0 border-b border-severity-error-border bg-severity-error-tint px-5 py-2 text-xs text-severity-error"
            role="alert"
          >
            {{ logError }}
          </div>

          <div
            ref="logEl"
            class="hive-scroll min-h-0 flex-1 overflow-auto py-2 font-mono text-caption leading-[1.55]"
            data-testid="action-run-log"
          >
            <div v-if="!logLoaded" class="px-5 py-6 text-text-4">Loading log…</div>
            <template v-else-if="lines.length">
              <template v-for="group in groups" :key="group.attempt">
                <div
                  v-if="showAttemptHeaders"
                  class="sticky top-0 z-[1] flex items-center gap-3 bg-app px-5 py-1.5 text-micro uppercase tracking-[.12em] text-text-2"
                  data-testid="action-run-attempt"
                >
                  Attempt {{ group.attempt }}
                  <div class="h-px flex-1 bg-row" />
                </div>
                <div
                  v-for="line in group.lines"
                  :key="line.id"
                  class="log-line"
                  :class="`log-${line.stream}`"
                  data-testid="action-run-line"
                  :data-stream="line.stream"
                >
                  <span class="log-n">{{ line.n }}</span>
                  <span class="log-time">{{ timeLabel(line.at) }}</span>
                  <span class="log-text">{{ line.text || ' ' }}</span>
                </div>
              </template>
            </template>
            <template v-else-if="fallback">
              <div class="px-5 pb-2 text-text-4">This run predates run logs. Its recorded output:</div>
              <div v-for="(text, i) in fallback.stdout.split('\n')" :key="`o${i}`" class="log-line log-stdout">
                <span class="log-n">{{ i + 1 }}</span
                ><span class="log-time" /><span class="log-text">{{ text || ' ' }}</span>
              </div>
              <div v-for="(text, i) in fallback.stderr.split('\n')" :key="`e${i}`" class="log-line log-stderr">
                <span class="log-n">{{ i + 1 }}</span
                ><span class="log-time" /><span class="log-text">{{ text || ' ' }}</span>
              </div>
            </template>
            <div v-else-if="runIsActive(selected.status)" class="px-5 py-6 text-text-4">Waiting for output…</div>
            <div v-else class="px-5 py-6 text-text-4" data-testid="action-run-log-empty">This run wrote no output.</div>
          </div>
        </template>
      </section>
    </div>
  </div>
</template>

<style scoped>
.log-line {
  display: grid;
  grid-template-columns: 4.5em 6.5em minmax(0, 1fr);
  padding: 0 20px 0 0;
}
.log-line:hover {
  background: var(--color-row-hover);
}
.log-n {
  padding-right: 12px;
  text-align: right;
  color: var(--color-text-4);
  user-select: none;
}
.log-time {
  color: var(--color-text-4);
  user-select: none;
}
.log-text {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  color: var(--color-text);
}
.log-stderr .log-text {
  color: var(--color-severity-error);
}
.log-system .log-text {
  color: var(--color-accent);
}
</style>
