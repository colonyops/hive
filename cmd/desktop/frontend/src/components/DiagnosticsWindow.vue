<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useIntervalFn, useStorage, useWindowSize } from '@vueuse/core'
import IconArrowUp from '~icons/lucide/arrow-up'
import IconColumns2 from '~icons/lucide/columns-2'
import IconCopy from '~icons/lucide/copy'
import IconDownload from '~icons/lucide/download'
import IconEllipsis from '~icons/lucide/ellipsis'
import IconFolderOpen from '~icons/lucide/folder-open'
import IconList from '~icons/lucide/list'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import IconRows2 from '~icons/lucide/rows-2'
import IconSparkles from '~icons/lucide/sparkles'
import {
  Agents,
  Context,
  Prepare,
  Read,
  Reveal,
  Save,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/diagnosticsservice'
import type { Entry as DiagnosticEntry } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/diagnostics/models'
import type {
  DiagnosticsIncident,
  DiagnosticsQuery,
  DiagnosticsSnapshot,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { useClipboard } from '../composables/useClipboard'
import { usePerf } from '../composables/usePerf'
import { useResizablePanel } from '../composables/useResizablePanel'
import { retainUnchangedDiagnosticEntries } from '../lib/diagnosticsEntries'
import type { MenuEntry } from '../types/menu'
import DiagnosticsAgent from './DiagnosticsAgent.vue'
import DiagnosticsEntry from './DiagnosticsEntry.vue'
import DiagnosticsLevelFilter from './DiagnosticsLevelFilter.vue'
import DiagnosticsTimeRange from './DiagnosticsTimeRange.vue'
import AppMenu from './ui/AppMenu.vue'
import AppSelect from './ui/AppSelect.vue'
import AppSwitch from './ui/AppSwitch.vue'
import BaseBadge from './ui/BaseBadge.vue'
import BaseButton from './ui/BaseButton.vue'
import EmptyState from './ui/EmptyState.vue'
import IconButton from './ui/IconButton.vue'
import InlineError from './ui/InlineError.vue'
import PanelResizeHandle from './ui/PanelResizeHandle.vue'
import SearchField from './ui/SearchField.vue'
import SegmentedControl, { type SegmentedControlOption } from './ui/SegmentedControl.vue'
import TextInput from './ui/TextInput.vue'
import ViewHeader from './ui/ViewHeader.vue'

interface Investigation {
  command: string
  dir: string
}

const NARROW_LAYOUT_WIDTH = 900

const snapshot = shallowRef<DiagnosticsSnapshot | null>(null)
const levels = ref<string[]>([])
const search = ref('')
const period = ref('60')
const since = ref('')
const until = ref('')
const follow = ref(true)
const showHTTP2xx = ref(false)
const wrapMessages = useStorage('hive.diagnostics.wrap', false)
const loading = ref(false)
const error = ref('')
const notice = ref('')
const description = ref('')
const agent = ref('')
const agents = ref<string[]>([])
const defaultAgent = ref('')
const agentError = ref('')
const working = ref<'' | 'copy' | 'save' | 'investigate'>('')
const investigation = ref<Investigation | null>(null)
const list = ref<HTMLElement | null>(null)
const atTail = ref(true)
const awayFromTop = ref(false)
const actionMenuOpen = ref(false)
const copiedEntryID = ref('')
const clipboard = useClipboard()
const perf = usePerf('diagnostics')
const panelLayout = useStorage<'side-by-side' | 'stacked'>('hive.diagnostics.layout', 'stacked')
const viewport = useWindowSize()
const narrowWindow = computed(() => viewport.width.value < NARROW_LAYOUT_WIDTH)
const stacked = computed(() => narrowWindow.value || panelLayout.value === 'stacked')
const horizontalPanel = useResizablePanel({
  storageKey: 'hive.diagnostics.terminal.width',
  defaultSize: 580,
  min: 340,
  max: 900,
  edge: 'right',
})
const verticalPanel = useResizablePanel({
  storageKey: 'hive.diagnostics.terminal.height',
  defaultSize: 300,
  min: 180,
  max: 650,
  edge: 'bottom',
})
let debounce: ReturnType<typeof setTimeout> | undefined
let generation = 0
let disposed = false

const rows = computed(() => snapshot.value?.entries ?? [])
const terminalStyle = computed(() =>
  stacked.value ? { flexBasis: `${verticalPanel.size.value}px` } : { flexBasis: `${horizontalPanel.size.value}px` },
)
const fileSources = computed(() =>
  (snapshot.value?.sources ?? []).filter((item) => item.id === 'desktop' || item.id === 'cli'),
)
const logSource = computed(() => fileSources.value.find((item) => item.path && !item.error) ?? fileSources.value[0])
const jobSource = computed(() => snapshot.value?.sources?.find((item) => item.id === 'jobs'))
const sourceWarnings = computed(() => {
  const warnings: string[] = []
  for (const source of fileSources.value) {
    if (!source.error) continue
    const label = source.id === 'cli' ? 'CLI' : 'Desktop'
    warnings.push(`${label} log unavailable: ${source.error}`)
  }
  if (jobSource.value?.error) warnings.push(`Job history unavailable: ${jobSource.value.error}`)
  return warnings
})
const layoutOptions: SegmentedControlOption<'side-by-side' | 'stacked'>[] = [
  { value: 'side-by-side', label: 'Side by side', title: 'Show panels side by side' },
  { value: 'stacked', label: 'Stacked', title: 'Stack panels' },
]
const layoutIcons = { 'side-by-side': IconColumns2, stacked: IconRows2 }
const actionMenuEntries = computed<MenuEntry[]>(() => [
  { kind: 'label', text: 'Evidence' },
  { kind: 'action', id: 'copy', label: 'Copy context', icon: IconCopy, disabled: !snapshot.value || !!working.value },
  {
    kind: 'action',
    id: 'save',
    label: 'Export snapshot',
    icon: IconDownload,
    disabled: !snapshot.value || !!working.value,
  },
  {
    kind: 'action',
    id: 'reveal-log',
    label: 'Reveal hive.log',
    icon: IconFolderOpen,
    disabled: !logSource.value?.path,
  },
  { kind: 'separator' },
  { kind: 'label', text: 'View' },
  { kind: 'action', id: 'http', label: 'Show successful HTTP requests', checked: showHTTP2xx.value },
  { kind: 'action', id: 'wrap', label: 'Wrap messages', checked: wrapMessages.value },
])

function currentQuery(reference = ''): DiagnosticsQuery {
  if (since.value || until.value) {
    return {
      source: '',
      levels: [...levels.value],
      search: search.value,
      since: since.value ? new Date(since.value).toISOString() : '',
      until: until.value ? new Date(until.value).toISOString() : '',
      reference,
      limit: 500,
      omitRoutine: !showHTTP2xx.value,
    }
  }
  if (period.value === 'all') {
    return {
      source: '',
      levels: [...levels.value],
      search: search.value,
      since: '',
      until: '',
      reference,
      limit: 500,
      omitRoutine: !showHTTP2xx.value,
    }
  }
  const end = new Date()
  return {
    source: '',
    levels: [...levels.value],
    search: search.value,
    since: new Date(end.getTime() - Number(period.value) * 60_000).toISOString(),
    until: end.toISOString(),
    reference,
    limit: 500,
    omitRoutine: !showHTTP2xx.value,
  }
}

async function refresh(): Promise<void> {
  const seq = ++generation
  const initialRead = snapshot.value === null
  const pinAfterRead = !initialRead && follow.value && atTail.value
  const finish = perf.start('logs:refresh', { follow: follow.value })
  let entryCount = 0
  let entriesChanged = false
  let failed = false
  loading.value = true
  try {
    const result = await Read(currentQuery())
    if (disposed || seq !== generation) return
    const entries = retainUnchangedDiagnosticEntries(rows.value, result.entries ?? [])
    entryCount = entries.length
    entriesChanged = entries !== rows.value
    snapshot.value = { ...result, entries }
    error.value = ''
    await nextTick()
    if (pinAfterRead) list.value?.scrollTo({ top: list.value.scrollHeight })
    else if (initialRead) onEntriesScroll()
  } catch (failure) {
    failed = true
    if (!disposed && seq === generation) error.value = String(failure)
  } finally {
    finish({ entryCount, entriesChanged, failed })
    if (!disposed && seq === generation) loading.value = false
  }
}

function incident(reference = ''): DiagnosticsIncident {
  const query = snapshot.value?.query ?? currentQuery()
  return {
    query: { ...query, source: '', reference },
    description: description.value,
    agent: agent.value,
  }
}

async function action(kind: 'copy' | 'save' | 'investigate', reference = ''): Promise<void> {
  if (working.value) return
  working.value = kind
  notice.value = ''
  if (kind === 'investigate') agentError.value = ''
  try {
    if (kind === 'copy') {
      const result = await Context(incident(reference))
      await clipboard.copy(result.text)
      notice.value =
        clipboard.status.value === 'success' ? 'Diagnostic context copied.' : 'Could not copy diagnostic context.'
    } else if (kind === 'save') {
      const result = await Save(incident(reference))
      notice.value = `Saved ${result.path}`
    } else {
      const result = await Prepare(incident(reference))
      investigation.value = { command: result.command, dir: result.dir }
    }
  } catch (failure) {
    if (kind === 'investigate') agentError.value = String(failure)
    else error.value = String(failure)
  } finally {
    working.value = ''
  }
}

async function selectAction(id: string): Promise<void> {
  actionMenuOpen.value = false
  if (id === 'copy' || id === 'save') await action(id)
  else if (id === 'reveal-log') await reveal('desktop')
  else if (id === 'http') showHTTP2xx.value = !showHTTP2xx.value
  else if (id === 'wrap') wrapMessages.value = !wrapMessages.value
}

async function reveal(id: string): Promise<void> {
  try {
    await Reveal(id)
  } catch (failure) {
    error.value = String(failure)
  }
}

async function copyEntry(entry: DiagnosticEntry): Promise<void> {
  await clipboard.copy([entry.time, entry.source, entry.level, entry.raw].filter(Boolean).join(' '))
  if (clipboard.status.value === 'success') copiedEntryID.value = entry.id
  notice.value = clipboard.status.value === 'success' ? 'Entry copied.' : 'Could not copy entry.'
}

function onEntriesScroll(): void {
  if (!list.value) return
  atTail.value = list.value.scrollHeight - list.value.scrollTop - list.value.clientHeight < 48
  awayFromTop.value = list.value.scrollTop > 160
}

function scrollToTop(): void {
  list.value?.scrollTo({ top: 0, behavior: 'smooth' })
}

watch([levels, search, period, since, until, showHTTP2xx], () => {
  clearTimeout(debounce)
  debounce = setTimeout(() => void refresh(), 250)
})
watch(clipboard.status, (status) => {
  if (status === 'idle') copiedEntryID.value = ''
})

onMounted(() => {
  void refresh()
  void Agents()
    .then((result) => {
      agents.value = result.names ?? []
      defaultAgent.value = result.defaultAgent
    })
    .catch((failure: unknown) => {
      agentError.value = String(failure)
    })
})

useIntervalFn(() => {
  if (follow.value && !loading.value && !document.hidden) void refresh()
}, 2000)

onBeforeUnmount(() => {
  disposed = true
  generation++
  clearTimeout(debounce)
})
</script>

<template>
  <main class="flex h-screen min-h-0 flex-col bg-app text-body text-text" data-testid="diagnostics-window">
    <ViewHeader data-testid="diagnostics-toolbar">
      <template #title>
        <h1 class="font-semibold text-text">Diagnostics</h1>
        <BaseBadge tone="muted" variant="pill" class="px-2 py-0.5 font-mono text-micro">
          {{ snapshot?.version || 'Development build' }}
        </BaseBadge>
        <BaseBadge
          :tone="logSource?.error ? 'danger' : logSource?.truncated ? 'accent' : 'success'"
          variant="pill"
          dot
          class="px-2 py-0.5 text-micro"
          data-testid="diagnostics-source-log"
          :title="logSource?.error || logSource?.path || undefined"
        >
          hive.log{{ logSource?.truncated ? ' · recent tail' : '' }}
        </BaseBadge>
        <span class="text-micro text-text-4">Includes recent job outcomes</span>
        <IconButton
          class="ml-auto"
          label="Refresh diagnostics"
          :icon="IconRefreshCw"
          size="lg"
          :busy="loading"
          data-testid="diagnostics-refresh"
          @click="refresh"
        />
        <SegmentedControl
          v-if="investigation && !narrowWindow"
          v-model="panelLayout"
          :options="layoutOptions"
          variant="compact"
          size="sm"
          aria-label="Investigation panel layout"
          testid="diagnostics-layout"
        >
          <template #option="{ option }">
            <component :is="layoutIcons[option.value]" class="size-3.5" />
          </template>
        </SegmentedControl>
      </template>
    </ViewHeader>

    <div
      v-if="notice"
      class="flex shrink-0 items-center gap-2 border-b border-row bg-sidebar px-4 py-2 text-caption text-text-3"
      role="status"
    >
      <span class="min-w-0 truncate">{{ notice }}</span>
      <BaseButton v-if="notice.startsWith('Saved')" variant="ghost" size="xs" @click="reveal('exports')">
        Reveal exports
      </BaseButton>
    </div>
    <InlineError
      v-if="error"
      :message="error"
      variant="line"
      class="shrink-0 border-b border-severity-error-border bg-severity-error-tint px-4 py-2"
      testid="diagnostics-error"
    />
    <InlineError
      v-for="warning in sourceWarnings"
      :key="warning"
      :message="warning"
      variant="line"
      class="shrink-0 border-b border-severity-error-border bg-severity-error-tint px-4 py-2"
      data-testid="diagnostics-source-warning"
    />

    <section
      v-if="!investigation"
      class="shrink-0 border-b border-row bg-sidebar px-4 py-3"
      aria-label="Start an investigation"
    >
      <div class="mb-2.5 flex items-center gap-2">
        <IconSparkles class="size-3.5 shrink-0 text-accent" />
        <div class="min-w-0">
          <h2 class="font-medium text-text">Start an investigation</h2>
          <p class="text-caption text-text-4">
            Give an agent a symptom or question. Hive adds the current log evidence.
          </p>
        </div>
        <AppSelect
          v-model="agent"
          aria-label="Investigation agent"
          size="sm"
          class="ml-auto w-48 shrink-0"
          testid="diagnostics-agent-select"
          :options="[
            { value: '', label: `Default${defaultAgent ? ` · ${defaultAgent}` : ''}` },
            ...agents.map((name) => ({ value: name, label: name })),
          ]"
        />
      </div>
      <div class="flex items-center gap-2">
        <TextInput
          v-model="description"
          maxlength="8000"
          size="sm"
          aria-label="Describe the incident"
          placeholder="Describe what went wrong or ask a question…"
          data-testid="diagnostics-incident-description"
          @keydown.enter="action('investigate')"
        />
        <BaseButton
          size="sm"
          :busy="working === 'investigate'"
          :disabled="!!working"
          data-testid="diagnostics-investigate"
          @click="action('investigate')"
        >
          Investigate
        </BaseButton>
      </div>
      <InlineError
        v-if="agentError"
        :message="`${agentError} Logs remain available.`"
        variant="line"
        class="mt-2 px-1"
        testid="diagnostics-agent-error"
      />
    </section>

    <div class="flex min-h-0 flex-1" :class="investigation && stacked ? 'flex-col' : 'flex-row'">
      <section
        v-if="investigation"
        class="relative flex min-h-0 min-w-0 shrink-0 flex-col overflow-hidden border-row bg-app"
        :class="stacked ? 'border-b' : 'border-r'"
        :style="terminalStyle"
        aria-label="Diagnostic investigation"
        data-testid="diagnostics-terminal-panel"
      >
        <DiagnosticsAgent :command="investigation.command" :dir="investigation.dir" @close="investigation = null" />
        <PanelResizeHandle
          :edge="stacked ? 'bottom' : 'right'"
          name="diagnostics-terminal"
          :start="stacked ? verticalPanel.startResize : horizontalPanel.startResize"
          :step="stacked ? verticalPanel.step : horizontalPanel.step"
        />
      </section>

      <section
        class="relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-app"
        aria-labelledby="diagnostics-logs-title"
        :aria-busy="loading || undefined"
        data-testid="diagnostics-logs-panel"
      >
        <header class="flex h-9 shrink-0 items-center gap-2 border-b border-row bg-canvas-toolbar px-4">
          <IconList class="size-3.5 text-text-3" />
          <h2 id="diagnostics-logs-title" class="font-medium">Logs</h2>
          <BaseBadge
            tone="neutral"
            variant="pill"
            class="px-2 py-0.5 font-mono text-micro"
            data-testid="diagnostics-count"
          >
            {{ rows.length }}
          </BaseBadge>
          <BaseBadge
            v-if="snapshot?.truncated"
            tone="accent"
            variant="pill"
            class="px-2 py-0.5 text-micro"
            data-testid="diagnostics-truncated"
            title="Older entries may not be shown. Narrow the filters or reveal hive.log to inspect the full file."
          >
            Older entries omitted
          </BaseBadge>
        </header>

        <div class="flex shrink-0 items-center gap-2 border-b border-row bg-sidebar px-4 py-2" aria-label="Log filters">
          <SearchField
            v-model="search"
            class="min-w-48 flex-1"
            placeholder="Search logs…"
            aria-label="Search logs"
            testid="diagnostics-search"
          />
          <DiagnosticsLevelFilter v-model="levels" />
          <DiagnosticsTimeRange v-model:period="period" v-model:since="since" v-model:until="until" />
          <AppSwitch v-model="follow" label="Live" size="sm" testid="diagnostics-live-follow" />
          <div class="relative">
            <IconButton
              label="Diagnostics actions and view options"
              :icon="IconEllipsis"
              size="lg"
              :active="actionMenuOpen"
              data-testid="diagnostics-actions"
              @click="actionMenuOpen = !actionMenuOpen"
            />
            <AppMenu
              v-if="actionMenuOpen"
              :entries="actionMenuEntries"
              testid="diagnostics-actions-menu"
              @select="selectAction"
              @close="actionMenuOpen = false"
            />
          </div>
        </div>

        <div
          ref="list"
          class="hive-scroll min-h-0 flex-1 overflow-auto bg-app"
          aria-label="Diagnostic entries"
          data-testid="diagnostics-entries"
          @scroll.passive="onEntriesScroll"
        >
          <EmptyState
            v-if="!loading && !rows.length"
            message="No entries match these filters. Try a wider time range."
            data-testid="diagnostics-empty"
          />
          <DiagnosticsEntry
            v-for="entry in rows"
            :key="`${entry.id}:${entry.time}`"
            v-memo="[entry, wrapMessages, copiedEntryID === entry.id, !!working || !!investigation]"
            :entry="entry"
            :wrap-messages="wrapMessages"
            :copied="copiedEntryID === entry.id"
            :actions-disabled="!!working || !!investigation"
            @copy="copyEntry(entry)"
            @investigate="action('investigate', entry.id)"
          />
        </div>

        <Transition name="tail-pill">
          <BaseButton
            v-if="awayFromTop"
            variant="secondary"
            size="xs"
            class="absolute bottom-4 right-4 z-10 rounded-full bg-raised/95 shadow-popover"
            data-testid="diagnostics-scroll-to-top"
            @click="scrollToTop"
          >
            <template #icon><IconArrowUp class="size-3" /></template>
            Scroll to top
          </BaseButton>
        </Transition>
      </section>
    </div>
  </main>
</template>
