<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useIntervalFn, useStorage } from '@vueuse/core'
import IconCheck from '~icons/lucide/check'
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
import { useResizablePanel } from '../composables/useResizablePanel'
import type { MenuEntry } from '../types/menu'
import DiagnosticsAgent from './DiagnosticsAgent.vue'
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
import TextInput from './ui/TextInput.vue'

interface Investigation {
  command: string
  dir: string
}

const snapshot = shallowRef<DiagnosticsSnapshot | null>(null)
const level = ref('')
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
const actionMenuOpen = ref(false)
const copiedEntryID = ref('')
const clipboard = useClipboard()
const panelLayout = useStorage<'side-by-side' | 'stacked'>('hive.diagnostics.layout', 'stacked')
const stacked = computed(() => panelLayout.value === 'stacked')
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
  if (fileSources.value.length && fileSources.value.every((item) => item.error)) {
    warnings.push(`Hive log unavailable: ${fileSources.value[0]?.error}`)
  }
  if (jobSource.value?.error) warnings.push(`Job history unavailable: ${jobSource.value.error}`)
  return warnings
})
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
      level: level.value,
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
      level: level.value,
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
    level: level.value,
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
  const pinAfterRead = follow.value && atTail.value
  loading.value = true
  try {
    const result = await Read(currentQuery())
    if (disposed || seq !== generation) return
    snapshot.value = result
    error.value = ''
    if (pinAfterRead) {
      await nextTick()
      list.value?.scrollTo({ top: list.value.scrollHeight })
    }
  } catch (failure) {
    if (!disposed && seq === generation) error.value = String(failure)
  } finally {
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

function time(value: string): string {
  return value ? new Date(value).toLocaleString() : 'No timestamp'
}

function shortTime(value: string): string {
  if (!value) return '--:--:--'
  const date = new Date(value)
  return date.toLocaleTimeString([], { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function sourceLabel(id: string): string {
  if (id === 'desktop') return 'DESKTOP'
  if (id === 'cli') return 'CLI'
  if (id === 'jobs') return 'JOB'
  return id.toUpperCase()
}

function levelLabel(value: string): string {
  return { error: 'ERR', warn: 'WRN', info: 'INF', debug: 'DBG', unknown: 'UNK' }[value] ?? value.toUpperCase()
}

function sortedFields(entry: DiagnosticEntry): [string, string][] {
  return Object.entries(entry.fields ?? {})
    .flatMap(([key, value]) => (typeof value === 'string' ? ([[key, value]] as [string, string][]) : []))
    .sort(([a], [b]) => a.localeCompare(b))
}

function onEntriesScroll(): void {
  if (!list.value) return
  atTail.value = list.value.scrollHeight - list.value.scrollTop - list.value.clientHeight < 48
}

watch([level, search, period, since, until, showHTTP2xx], () => {
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
    <header class="flex h-11 shrink-0 items-center gap-2 px-4" data-testid="diagnostics-toolbar">
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
      <div
        v-if="investigation"
        class="flex items-center rounded-md border border-card bg-pane p-0.5"
        role="group"
        aria-label="Investigation panel layout"
      >
        <IconButton
          label="Show panels side by side"
          :icon="IconColumns2"
          size="md"
          :active="!stacked"
          data-testid="diagnostics-layout-side-by-side"
          @click="panelLayout = 'side-by-side'"
        />
        <IconButton
          label="Stack panels"
          :icon="IconRows2"
          size="md"
          :active="stacked"
          data-testid="diagnostics-layout-stacked"
          @click="panelLayout = 'stacked'"
        />
      </div>
    </header>

    <div
      v-if="notice"
      class="mx-2 mb-2 shrink-0 rounded-lg border border-row bg-pane px-3 py-1.5 text-caption text-text-3"
      role="status"
    >
      {{ notice }}
      <BaseButton v-if="notice.startsWith('Saved')" variant="ghost" size="xs" @click="reveal('exports')">
        Reveal exports
      </BaseButton>
    </div>
    <InlineError v-if="error" :message="error" class="mx-2 mb-2 shrink-0" testid="diagnostics-error" />
    <InlineError
      v-for="warning in sourceWarnings"
      :key="warning"
      :message="warning"
      class="mx-2 mb-2 shrink-0"
      data-testid="diagnostics-source-warning"
    />

    <section
      v-if="!investigation"
      class="mx-2 mb-2 shrink-0 rounded-xl border border-row bg-pane p-3"
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

    <div class="flex min-h-0 flex-1 gap-2 p-2 pt-0" :class="investigation && stacked ? 'flex-col' : 'flex-row'">
      <section
        v-if="investigation"
        class="relative flex min-h-0 min-w-0 shrink-0 flex-col overflow-hidden rounded-xl border border-strong bg-pane"
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
        class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-xl border border-strong bg-pane"
        aria-labelledby="diagnostics-logs-title"
        :aria-busy="loading || undefined"
        data-testid="diagnostics-logs-panel"
      >
        <header class="flex h-9 shrink-0 items-center gap-2 border-b border-row bg-raised px-3">
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

        <div class="flex shrink-0 items-center gap-2 border-b border-row px-2 py-1.5" aria-label="Log filters">
          <SearchField
            v-model="search"
            class="min-w-48 flex-1"
            placeholder="Search logs…"
            aria-label="Search logs"
            testid="diagnostics-search"
          />
          <AppSelect
            v-model="level"
            aria-label="Severity"
            size="sm"
            testid="diagnostics-severity-filter"
            :options="[
              { value: '', label: 'All levels' },
              { value: 'error', label: 'Error' },
              { value: 'warn', label: 'Warning' },
              { value: 'info', label: 'Info' },
              { value: 'debug', label: 'Debug' },
              { value: 'unknown', label: 'Unknown' },
            ]"
          />
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
          class="min-h-0 flex-1 overflow-auto bg-app"
          aria-label="Diagnostic entries"
          data-testid="diagnostics-entries"
          @scroll.passive="onEntriesScroll"
        >
          <EmptyState
            v-if="!loading && !rows.length"
            message="No entries match these filters. Try a wider time range."
            data-testid="diagnostics-empty"
          />
          <details
            v-for="entry in rows"
            :id="entry.id"
            :key="`${entry.id}:${entry.time}`"
            class="group border-b border-l-2 border-row font-mono text-caption"
            :class="{
              'border-l-severity-error': entry.level === 'error',
              'border-l-severity-warning': entry.level === 'warn',
              'border-l-severity-success': entry.level === 'info',
              'border-l-text-4': entry.level === 'debug' || entry.level === 'unknown',
            }"
            data-testid="diagnostics-entry"
            :data-entry-id="entry.id"
          >
            <summary class="flex cursor-pointer list-none items-start gap-2 px-2 py-1.5 hover:bg-hover">
              <time class="w-17 shrink-0 text-text-4" :title="time(entry.time)">{{ shortTime(entry.time) }}</time>
              <span class="w-13 shrink-0 text-text-3">{{ sourceLabel(entry.source) }}</span>
              <span
                class="w-8 shrink-0 font-semibold"
                :class="{
                  'text-severity-error': entry.level === 'error',
                  'text-severity-warning': entry.level === 'warn',
                  'text-text-3': entry.level !== 'error' && entry.level !== 'warn',
                }"
              >
                {{ levelLabel(entry.level) }}
              </span>
              <span
                class="min-w-0 flex-1 text-text-2"
                :class="wrapMessages ? 'whitespace-pre-wrap break-words' : 'truncate'"
              >
                {{ entry.message }}
              </span>
            </summary>
            <div class="border-t border-row bg-raised px-3 py-2.5" data-testid="diagnostics-entry-raw">
              <div class="mb-2 flex flex-wrap items-center gap-2">
                <a class="text-accent hover:underline" :href="`#${entry.id}`">{{ entry.id }}</a>
                <BaseBadge v-if="entry.truncated" tone="accent" variant="pill" class="px-2 py-0.5 text-micro">
                  Entry truncated
                </BaseBadge>
                <IconButton
                  class="ml-auto"
                  :label="copiedEntryID === entry.id ? 'Entry copied' : 'Copy entry'"
                  :icon="copiedEntryID === entry.id ? IconCheck : IconCopy"
                  :active="copiedEntryID === entry.id"
                  size="md"
                  data-testid="diagnostics-entry-copy"
                  :data-entry-id="entry.id"
                  @click="copyEntry(entry)"
                />
                <IconButton
                  label="Investigate this entry"
                  :icon="IconSparkles"
                  size="md"
                  :disabled="!!working || !!investigation"
                  data-testid="diagnostics-entry-investigate"
                  :data-entry-id="entry.id"
                  @click="action('investigate', entry.id)"
                />
              </div>
              <dl
                v-if="sortedFields(entry).length"
                class="mb-2 grid grid-cols-[max-content_minmax(0,1fr)] gap-x-3 gap-y-0.5"
              >
                <template v-for="[key, value] in sortedFields(entry)" :key="key">
                  <dt class="text-text-4">{{ key }}</dt>
                  <dd class="break-all text-text-2">{{ value }}</dd>
                </template>
              </dl>
              <pre class="whitespace-pre-wrap break-words text-text-3">{{ entry.raw }}</pre>
            </div>
          </details>
        </div>
      </section>
    </div>
  </main>
</template>
