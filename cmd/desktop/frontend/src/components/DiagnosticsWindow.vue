<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useIntervalFn } from '@vueuse/core'
import { Clipboard } from '@wailsio/runtime'
import {
  Agents,
  Context,
  Prepare,
  Read,
  Reveal,
  Save,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/diagnosticsservice'
import type {
  DiagnosticsSnapshot,
  DiagnosticsQuery,
  DiagnosticsIncident,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import AppSelect from './ui/AppSelect.vue'
import AppCheckbox from './ui/AppCheckbox.vue'

const DiagnosticsAgent = defineAsyncComponent(() => import('./DiagnosticsAgent.vue'))

const snapshot = shallowRef<DiagnosticsSnapshot | null>(null)
const source = ref('')
const level = ref('')
const search = ref('')
const period = ref('60')
const since = ref('')
const until = ref('')
const follow = ref(true)
const noise = ref(false)
const loading = ref(false)
const error = ref('')
const notice = ref('')
const description = ref('')
const agent = ref('')
const agents = ref<string[]>([])
const defaultAgent = ref('')
const agentError = ref('')
const working = ref(false)
const investigation = ref<{ command: string; dir: string } | null>(null)
const list = ref<HTMLElement | null>(null)
let debounce: ReturnType<typeof setTimeout> | undefined
let generation = 0
let disposed = false

function query(): DiagnosticsQuery {
  const end = until.value ? new Date(until.value) : new Date()
  const start = since.value
    ? new Date(since.value)
    : period.value === 'all'
      ? null
      : new Date(end.getTime() - Number(period.value) * 60_000)
  return {
    source: source.value,
    level: level.value,
    search: search.value,
    since: start?.toISOString() ?? '',
    until: end.toISOString(),
    reference: '',
    limit: 500,
    omitRoutine: !noise.value,
  }
}
async function refresh(): Promise<void> {
  const seq = ++generation
  loading.value = true
  try {
    const result = await Read(query())
    if (disposed || seq !== generation) return
    snapshot.value = result
    error.value = ''
    if (follow.value) {
      await nextTick()
      list.value?.scrollTo({ top: list.value.scrollHeight })
    }
  } catch (e) {
    if (!disposed && seq === generation) error.value = String(e)
  } finally {
    if (!disposed && seq === generation) loading.value = false
  }
}
const rows = computed(() =>
  (snapshot.value?.entries ?? []).filter(
    (e) =>
      noise.value ||
      !(e.level === 'info' && /request complete/.test(e.message) && /status[=:"\s]+(?:200|204)/.test(e.raw)),
  ),
)
const hidden = computed(() => (snapshot.value?.entries?.length ?? 0) - rows.value.length)
function incident(): DiagnosticsIncident {
  return { query: snapshot.value?.query ?? query(), description: description.value, agent: agent.value }
}
async function action(kind: 'copy' | 'save' | 'investigate'): Promise<void> {
  working.value = true
  notice.value = ''
  if (kind === 'investigate') agentError.value = ''
  try {
    if (kind === 'copy') {
      const result = await Context(incident())
      await Clipboard.SetText(result.text)
      notice.value = 'Diagnostic context copied.'
    } else if (kind === 'save') {
      const result = await Save(incident())
      notice.value = `Saved ${result.path}`
    } else {
      const result = await Prepare(incident())
      investigation.value = { command: result.command, dir: result.dir }
    }
  } catch (e) {
    if (kind === 'investigate') agentError.value = String(e)
    else error.value = String(e)
  } finally {
    working.value = false
  }
}
async function reveal(id: string): Promise<void> {
  try {
    await Reveal(id)
  } catch (e) {
    error.value = String(e)
  }
}
function clearTimeBounds(): void {
  since.value = ''
  until.value = ''
}
function time(value: string): string {
  return value ? new Date(value).toLocaleString() : 'No timestamp'
}
watch([source, level, search, period, since, until, noise], () => {
  clearTimeout(debounce)
  debounce = setTimeout(() => void refresh(), 250)
})
onMounted(() => {
  void refresh()
  void Agents()
    .then((result) => {
      agents.value = result.names ?? []
      defaultAgent.value = result.defaultAgent
    })
    .catch((e: unknown) => {
      agentError.value = String(e)
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
  <main class="diagnostics">
    <header class="title">
      <div>
        <h1>Diagnostics</h1>
        <p>Logs and job outcomes · {{ snapshot?.version || 'Development build' }}</p>
      </div>
      <button :disabled="loading" @click="refresh">{{ loading ? 'Reading…' : 'Refresh' }}</button>
    </header>
    <section class="sources" aria-label="Evidence sources">
      <div v-for="item in snapshot?.sources" :key="item.id" class="source">
        <strong>{{ item.id === 'cli' ? 'Hive CLI' : item.id === 'desktop' ? 'Hive Desktop' : 'Jobs' }}</strong>
        <span v-if="item.path" :title="item.path">{{ item.path }}</span>
        <span v-if="item.updatedAt">Last written {{ time(item.updatedAt) }}</span>
        <span v-if="item.error" class="error" :title="item.error">{{ item.error }}</span>
        <span v-if="item.truncated">Recent tail only</span>
        <button v-if="item.path" @click="reveal(item.id)">Reveal file</button>
      </div>
    </section>
    <section class="investigate" aria-label="Investigation controls">
      <input
        v-model="description"
        aria-label="Describe the incident"
        maxlength="8000"
        placeholder="What happened? Describe the incident…"
      />
      <AppSelect
        v-model="agent"
        aria-label="Investigation agent"
        size="sm"
        :options="[
          { value: '', label: `Default agent${defaultAgent ? ` (${defaultAgent})` : ''}` },
          ...agents.map((name) => ({ value: name, label: name })),
        ]"
      />
      <button :disabled="working || !!investigation" @click="action('investigate')">Investigate</button>
      <button :disabled="working || !snapshot" @click="action('copy')">Copy context</button>
      <button :disabled="working || !snapshot" @click="action('save')">Export</button>
    </section>
    <p v-if="agentError" class="error banner" role="alert">{{ agentError }} · Logs remain available below.</p>
    <p v-if="notice" class="banner" role="status">
      {{ notice }} <button v-if="notice.startsWith('Saved')" @click="reveal('exports')">Reveal exports</button>
    </p>
    <DiagnosticsAgent
      v-if="investigation"
      :command="investigation.command"
      :dir="investigation.dir"
      @close="investigation = null"
    />
    <section class="filters" aria-label="Log filters">
      <AppSelect
        v-model="source"
        aria-label="Source"
        size="sm"
        :options="[
          { value: '', label: 'All sources' },
          { value: 'desktop', label: 'Desktop' },
          { value: 'cli', label: 'CLI' },
          { value: 'jobs', label: 'Jobs' },
        ]"
      />
      <AppSelect
        v-model="level"
        aria-label="Severity"
        size="sm"
        :options="[
          { value: '', label: 'All severities' },
          { value: 'error', label: 'Error' },
          { value: 'warn', label: 'Warning' },
          { value: 'info', label: 'Info' },
          { value: 'debug', label: 'Debug' },
          { value: 'unknown', label: 'Unknown' },
        ]"
      />
      <input v-model="search" aria-label="Search logs" placeholder="Search messages, IDs, errors…" />
      <AppSelect
        v-model="period"
        aria-label="Time range"
        size="sm"
        :options="[
          { value: '15', label: 'Last 15 minutes' },
          { value: '60', label: 'Last hour' },
          { value: '1440', label: 'Last day' },
          { value: 'all', label: 'Retained history' },
        ]"
      />
      <AppCheckbox v-model="follow" label="Live follow" />
      <AppCheckbox v-model="noise" label="Show HTTP successes" />
    </section>
    <details class="time-range">
      <summary>Custom time range</summary>
      <label>From <input v-model="since" type="datetime-local" /></label
      ><label>Through <input v-model="until" type="datetime-local" /></label
      ><button @click="clearTimeBounds">Reset</button>
    </details>
    <p v-if="error" class="error banner" role="alert">{{ error }}</p>
    <div class="summary">
      {{ rows.length }} entries<span v-if="hidden"> · {{ hidden }} routine HTTP entries hidden</span
      ><span v-if="snapshot?.truncated">
        · Bounded results; older evidence may be omitted. Narrow the filters or reveal the original files.</span
      >
    </div>
    <section ref="list" class="entries" aria-label="Diagnostic entries">
      <p v-if="!loading && !rows.length" class="empty">No entries match these filters. Try a wider time range.</p>
      <details
        v-for="entry in rows"
        :id="entry.id"
        :key="`${entry.id}:${entry.time}`"
        class="entry"
        :class="entry.level"
      >
        <summary>
          <time>{{ time(entry.time) }}</time
          ><span class="badge">{{ entry.source }}</span
          ><span class="level">{{ entry.level }}</span
          ><span class="message">{{ entry.message }}</span>
        </summary>
        <div class="evidence">
          <a :href="`#${entry.id}`">{{ entry.id }}</a
          ><span v-if="entry.truncated"> · Entry truncated</span>
          <pre>{{ entry.raw }}</pre>
        </div>
      </details>
    </section>
  </main>
</template>

<style scoped>
.diagnostics {
  height: 100vh;
  overflow: auto;
  display: flex;
  flex-direction: column;
  background: var(--hv-app);
  color: var(--hv-text);
  font-size: 13px;
}
.title {
  padding: 18px 20px 12px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}
h1 {
  font-size: 22px;
  font-weight: 600;
}
p,
.summary {
  color: var(--hv-text-3);
}
button,
select,
input:not([type='checkbox']) {
  background: var(--hv-raised);
  border: 1px solid var(--hv-border);
  border-radius: 6px;
  padding: 6px 9px;
  color: var(--hv-text);
}
button {
  cursor: pointer;
  white-space: nowrap;
}
button:disabled {
  opacity: 0.45;
  cursor: default;
}
button:hover:not(:disabled) {
  border-color: var(--hv-accent);
}
.sources {
  display: flex;
  gap: 12px;
  padding: 0 20px 12px;
}
.source {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 10px;
  border: 1px solid var(--hv-border);
  border-radius: 7px;
}
.source span {
  font-size: 11px;
  color: var(--hv-text-3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.source button {
  align-self: flex-start;
  margin-top: 4px;
}
.investigate,
.filters {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 20px;
  flex-wrap: wrap;
  border-top: 1px solid var(--hv-border);
}
.investigate > input,
.filters > input {
  flex: 1;
  min-width: 160px;
}
.filters label {
  white-space: nowrap;
}
.time-range {
  padding: 0 20px 8px;
}
.time-range label {
  margin-right: 12px;
}
.time-range summary {
  cursor: pointer;
  color: var(--hv-text-3);
}
.summary {
  padding: 8px 20px;
  border-bottom: 1px solid var(--hv-border);
  font-size: 11px;
}
.entries {
  flex: 1;
  min-height: 80px;
  overflow: auto;
}
.empty {
  padding: 30px;
  text-align: center;
}
.entry {
  border-bottom: 1px solid var(--hv-border);
  font-family: monospace;
  font-size: 12px;
}
.entry summary {
  display: flex;
  align-items: baseline;
  gap: 12px;
  padding: 9px 20px;
  cursor: pointer;
}
.entry summary::before {
  content: '›';
  color: var(--hv-text-3);
}
.entry[open] summary::before {
  transform: rotate(90deg);
}
.entry summary:hover {
  background: var(--hv-hover);
}
time {
  flex-shrink: 0;
  width: 165px;
  color: var(--hv-text-3);
}
.badge {
  width: 50px;
  flex-shrink: 0;
}
.level {
  width: 48px;
  flex-shrink: 0;
  color: var(--hv-text-3);
}
.message {
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}
.entry.error .level,
.error {
  color: var(--hv-severity-error);
}
.entry.warn .level {
  color: var(--hv-severity-warning);
}
.evidence {
  padding: 4px 20px 14px;
}
a {
  color: var(--hv-accent);
}
pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  margin-top: 8px;
}
.banner {
  padding: 8px 20px;
  overflow-wrap: anywhere;
}
</style>
