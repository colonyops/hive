<script setup lang="ts">
// sources.relay has no runtime.ts: the fetch and the parse run in Go, on the
// poll tick. Backend access goes through the `client` prop (defaulting to the
// generated Wails bindings) so tests inject fakes instead of mocking modules.
import { computed, onMounted, ref, watch } from 'vue'
import {
  CreateSource,
  ListSources,
  RecentDeliveries,
  RotateSource,
} from '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/relayservice'
import type {
  RecentDelivery,
  SourceView,
} from '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/sources/relay/models'
import BaseBadge from '../../../components/ui/BaseBadge.vue'
import BaseButton from '../../../components/ui/BaseButton.vue'
import CopyButton from '../../../components/ui/CopyButton.vue'
import EmptyState from '../../../components/ui/EmptyState.vue'
import FormField from '../../../components/ui/FormField.vue'
import IconButton from '../../../components/ui/IconButton.vue'
import InlineError from '../../../components/ui/InlineError.vue'
import { defaultWebhookSourceIcon, feedIconComponent, feedIconOptions } from '../../../lib/feedIcons'
import {
  CredentialField,
  IntervalField,
  MarkImageField,
  SelectField,
  TextField,
  type MarkImageClient,
  type SelectOption,
} from '../../fields'
import IconPlus from '~icons/lucide/plus'
import IconX from '~icons/lucide/x'
import { DIALECTS, MAP_POINTERS, type Config, type Dialect, type RelayMap } from './config'

interface RelayBackendClient {
  listSources(account: string): Promise<SourceView[]>
  createSource(account: string, name: string, secret: string): Promise<SourceView>
  rotateSource(account: string, id: string): Promise<string>
  recentDeliveries(account: string, sourceId: string, limit: number): Promise<RecentDelivery[]>
}

/** The editor's backend seam: the relay's reads and writes plus the mark
 *  picker's, which it forwards to MarkImageField. */
export interface RelayEditorClient extends RelayBackendClient, MarkImageClient {}

const props = defineProps<{
  config: Config
  errors?: string[]
  client?: RelayEditorClient
}>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

const client: RelayBackendClient = props.client ?? {
  async listSources(account) {
    return (await ListSources(account)) ?? []
  },
  async createSource(account, name, secret) {
    return await CreateSource(account, name, secret)
  },
  async rotateSource(account, id) {
    return await RotateSource(account, id)
  },
  async recentDeliveries(account, sourceId, limit) {
    return (await RecentDeliveries(account, sourceId, limit)) ?? []
  },
}

const RECENT_LIMIT = 5

function update(patch: Partial<Config>) {
  emit('update:config', { ...props.config, ...patch })
}

const account = computed(() => /^relay\/(.+)$/.exec(props.config.credential ?? '')?.[1] ?? '')

const error = ref<string | null>(null)

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// ── Sources ────────────────────────────────────────────────────────────────

const sources = ref<SourceView[]>([])
const sourcesLoaded = ref(false)

async function loadSources(): Promise<void> {
  sourcesLoaded.value = false
  sources.value = []
  if (!account.value) return
  try {
    sources.value = await client.listSources(account.value)
    sourcesLoaded.value = true
  } catch (err) {
    error.value = message(err)
  }
}

const sourceOptions = computed<SelectOption[]>(() => {
  const options = sources.value.map((s) => ({ value: s.id, label: s.name }))
  // Dropping a since-deleted source would silently rewrite the node's config
  // on the next edit, so it stays selectable and says why.
  const current = props.config.source
  if (current && sourcesLoaded.value && !sources.value.some((s) => s.id === current)) {
    options.unshift({ value: current, label: `${current} (not found)` })
  }
  return options
})

const selectedSource = computed(() => sources.value.find((s) => s.id === props.config.source))

const sourceHint = computed(() => {
  if (!account.value) return 'Pick an account first.'
  if (sourcesLoaded.value && sources.value.length === 0) return 'This account has no sources yet. Create one below.'
  return 'The relay source this node reads.'
})

const newName = ref('')
const newSecret = ref('')
const busy = ref(false)

// The relay stores only a hash of the ingress token, so the URL exists in
// exactly two answers: create and rotate. It is held per source so picking
// another source does not show a URL that belongs to the previous one.
const revealed = ref<{ sourceId: string; url: string } | null>(null)
const ingressUrl = computed(() => (revealed.value?.sourceId === props.config.source ? revealed.value.url : ''))

async function onCreate(): Promise<void> {
  const name = newName.value.trim()
  if (!account.value || !name) return
  busy.value = true
  error.value = null
  try {
    const created = await client.createSource(account.value, name, newSecret.value.trim())
    sources.value = [...sources.value, created]
    revealed.value = { sourceId: created.id, url: created.ingress_url ?? '' }
    newName.value = ''
    newSecret.value = ''
    update({ source: created.id })
  } catch (err) {
    error.value = message(err)
  } finally {
    busy.value = false
  }
}

async function onRotate(): Promise<void> {
  const id = props.config.source
  if (!account.value || !id) return
  busy.value = true
  error.value = null
  try {
    revealed.value = { sourceId: id, url: await client.rotateSource(account.value, id) }
  } catch (err) {
    error.value = message(err)
  } finally {
    busy.value = false
  }
}

const pasteExample = computed(() => {
  if (!ingressUrl.value) return ''
  const lines = [`curl -X POST '${ingressUrl.value}'`, `  -H 'Content-Type: application/json'`]
  if (selectedSource.value?.has_secret) lines.push(`  -H 'X-Hive-Secret: <your secret>'`)
  lines.push(`  -d '{"id": "build-42", "title": "Build 42 failed", "state": "open"}'`)
  return lines.join(' \\\n')
})

// ── Dialect ────────────────────────────────────────────────────────────────

const dialectOptions: SelectOption[] = DIALECTS.map((d) => ({
  value: d.value,
  label: d.available ? d.label : `${d.label} (later release)`,
}))

// ── Map ────────────────────────────────────────────────────────────────────

const POINTER_LABELS: Record<(typeof MAP_POINTERS)[number], string> = {
  key: 'Key',
  title: 'Title',
  url: 'URL',
  kind: 'Kind',
  state: 'State',
}

interface StateRow {
  from: string
  to: string
}

// Rows are local so a half-typed row (no "from" yet) survives; only rows with
// a "from" reach the config.
// eslint-disable-next-line vue/no-setup-props-reactivity-loss -- seeds the rows once
const stateRows = ref<StateRow[]>(Object.entries(props.config.map?.states ?? {}).map(([from, to]) => ({ from, to })))

function writeMap(patch: Partial<RelayMap>) {
  const merged: RelayMap = { ...(props.config.map ?? {}), ...patch }
  const next: RelayMap = {}
  for (const name of MAP_POINTERS) {
    if (merged[name]) next[name] = merged[name]
  }
  if (merged.states && Object.keys(merged.states).length > 0) next.states = merged.states
  update({ map: Object.keys(next).length > 0 ? next : undefined })
}

function pointerError(name: (typeof MAP_POINTERS)[number]): string | undefined {
  return props.errors?.find((message) => message.startsWith(`map.${name} `))
}

function updatePointer(name: (typeof MAP_POINTERS)[number], value: string) {
  writeMap({ [name]: value || undefined })
}

function writeStates(rows: StateRow[]) {
  stateRows.value = rows
  const states: Record<string, string> = {}
  for (const row of rows) {
    if (row.from) states[row.from] = row.to
  }
  writeMap({ states: Object.keys(states).length > 0 ? states : undefined })
}

function addStateRow() {
  stateRows.value = [...stateRows.value, { from: '', to: '' }]
}

function updateStateRow(index: number, patch: Partial<StateRow>) {
  writeStates(stateRows.value.map((row, i) => (i === index ? { ...row, ...patch } : row)))
}

function removeStateRow(index: number) {
  writeStates(stateRows.value.filter((_, i) => i !== index))
}

// ── Icon and mark ──────────────────────────────────────────────────────────

const iconOptions = feedIconOptions.map((o) => ({ value: o.value, label: o.label, icon: o.component }))

// The mark picker's fallback preview, and what items render with no image set.
const iconGlyph = computed(() => feedIconComponent(props.config.icon || defaultWebhookSourceIcon))

// ── Recent deliveries ──────────────────────────────────────────────────────

const recent = ref<RecentDelivery[]>([])

async function loadRecent(): Promise<void> {
  recent.value = []
  if (!account.value || !props.config.source) return
  try {
    recent.value = await client.recentDeliveries(account.value, props.config.source, RECENT_LIMIT)
  } catch (err) {
    error.value = message(err)
  }
}

function receivedAt(delivery: RecentDelivery): string {
  return new Date(delivery.receivedAt).toLocaleString()
}

function bodyPreview(delivery: RecentDelivery): string {
  try {
    return JSON.stringify(JSON.parse(delivery.body), null, 2)
  } catch {
    return delivery.body
  }
}

onMounted(async () => {
  await loadSources()
  await loadRecent()
})

watch(account, async () => {
  error.value = null
  await loadSources()
  await loadRecent()
})

watch(
  () => props.config.source,
  async () => {
    await loadRecent()
  },
)
</script>

<template>
  <div class="flex flex-col gap-4">
    <CredentialField
      provider="relay"
      :model-value="config.credential"
      testid="sources.relay-editor-credential"
      @update:model-value="(credential: string) => update({ credential })"
    />

    <div class="flex flex-col gap-2">
      <SelectField
        label="Source"
        :model-value="config.source ?? ''"
        :options="sourceOptions"
        :disabled="!account"
        :hint="sourceHint"
        placeholder="Pick a source"
        testid="sources.relay-editor-source"
        @update:model-value="(source: string) => update({ source })"
      />
      <div class="flex items-end gap-2">
        <div class="min-w-0 flex-1">
          <TextField
            label="New source"
            :model-value="newName"
            :disabled="!account"
            placeholder="uptime-kuma"
            testid="sources.relay-editor-new-name"
            @update:model-value="(name: string) => (newName = name)"
          />
        </div>
        <div class="min-w-0 flex-1">
          <TextField
            label="Secret"
            :model-value="newSecret"
            :disabled="!account"
            placeholder="optional"
            monospace
            testid="sources.relay-editor-new-secret"
            @update:model-value="(secret: string) => (newSecret = secret)"
          />
        </div>
        <BaseButton
          variant="secondary"
          size="sm"
          :busy="busy"
          :disabled="!account || !newName.trim()"
          data-testid="sources.relay-editor-create"
          @click="onCreate"
          >Create</BaseButton
        >
      </div>
      <div>
        <BaseButton
          variant="secondary"
          size="xs"
          :disabled="!account || !config.source || busy"
          data-testid="sources.relay-editor-rotate"
          @click="onRotate"
          >Rotate URL</BaseButton
        >
      </div>
    </div>

    <InlineError v-if="error" :message="error" testid="sources.relay-editor-error" />

    <div v-if="ingressUrl">
      <div class="mb-1.5 text-small text-text-2">Ingress URL</div>
      <div class="flex items-center gap-2">
        <code
          class="min-w-0 flex-1 truncate rounded-lg border border-strong bg-app px-3 py-2 font-mono text-small text-text-2"
          data-testid="sources.relay-editor-url"
          >{{ ingressUrl }}</code
        >
        <CopyButton :text="ingressUrl" data-testid="sources.relay-editor-copy-url" />
      </div>
      <p class="mt-1.5 text-caption text-text-4">Shown once. Copy it into the sender now.</p>
      <pre
        class="mt-2 max-h-48 overflow-auto rounded-lg border border-row bg-app px-3 py-2.5 font-mono text-caption leading-relaxed text-text-2"
        data-testid="sources.relay-editor-paste"
        >{{ pasteExample }}</pre>
    </div>

    <SelectField
      label="Format"
      :model-value="config.dialect || 'auto'"
      :options="dialectOptions"
      hint="How a delivery is parsed into items. Auto parses everything as generic today."
      testid="sources.relay-editor-dialect"
      @update:model-value="(dialect: string) => update({ dialect: (dialect || undefined) as Dialect | undefined })"
    />

    <FormField
      label="Map"
      hint="JSON Pointers that overlay the parsed item. raw/body reaches into the request body."
      testid="sources.relay-editor-map"
    >
      <div class="flex flex-col gap-2">
        <TextField
          v-for="name in MAP_POINTERS"
          :key="name"
          :label="POINTER_LABELS[name]"
          :model-value="config.map?.[name] ?? ''"
          :placeholder="`/raw/body/${name}`"
          :error="pointerError(name)"
          monospace
          :testid="`sources.relay-editor-map-${name}`"
          @update:model-value="(value: string) => updatePointer(name, value)"
        />
        <div>
          <div class="mb-1.5 flex items-center justify-between text-small text-text-2">
            <span>State values</span>
            <IconButton
              label="Add a state value"
              :icon="IconPlus"
              data-testid="sources.relay-editor-states-add"
              @click="addStateRow"
            />
          </div>
          <div
            v-for="(row, i) in stateRows"
            :key="i"
            class="mb-1.5 flex items-center gap-2"
            :data-testid="`sources.relay-editor-states-row-${i}`"
          >
            <div class="min-w-0 flex-1">
              <TextField
                :model-value="row.from"
                placeholder="0"
                monospace
                :testid="`sources.relay-editor-states-row-${i}-from`"
                @update:model-value="(from: string) => updateStateRow(i, { from })"
              />
            </div>
            <span class="text-caption text-text-4">to</span>
            <div class="min-w-0 flex-1">
              <TextField
                :model-value="row.to"
                placeholder="down"
                monospace
                :testid="`sources.relay-editor-states-row-${i}-to`"
                @update:model-value="(to: string) => updateStateRow(i, { to })"
              />
            </div>
            <IconButton
              label="Remove this state value"
              :icon="IconX"
              :data-testid="`sources.relay-editor-states-row-${i}-remove`"
              @click="removeStateRow(i)"
            />
          </div>
        </div>
      </div>
    </FormField>

    <IntervalField
      :model-value="config.interval"
      testid="sources.relay-editor-interval"
      @update:model-value="(interval?: string) => update({ interval })"
    />
    <SelectField
      label="Item icon"
      :model-value="config.icon || defaultWebhookSourceIcon"
      :options="iconOptions"
      searchable
      search-placeholder="Search icons…"
      hint="Shown on this source's items when no image is set."
      testid="sources.relay-editor-icon"
      @update:model-value="(icon: string) => update({ icon: icon || undefined })"
    />
    <MarkImageField
      :model-value="config.image"
      :glyph="iconGlyph"
      :client="props.client"
      testid="sources.relay-editor-mark"
      @update:model-value="(image: string | undefined) => update({ image })"
    />

    <div>
      <div class="mb-1.5 text-small text-text-2">Recent deliveries</div>
      <div v-if="recent.length > 0" class="flex flex-col gap-3" data-testid="sources.relay-editor-recent">
        <div v-for="delivery in recent" :key="delivery.id" :data-testid="`sources.relay-editor-recent-${delivery.id}`">
          <div class="mb-1.5 flex items-center gap-2 font-mono text-caption text-text-4">
            <span>{{ receivedAt(delivery) }}</span>
            <span class="text-text-2">{{ delivery.method }}</span>
            <span class="min-w-0 truncate">{{ delivery.path || '/' }}</span>
            <BaseBadge v-if="delivery.dialect" tone="muted" class="px-1.5 py-0.5 text-micro font-semibold">{{
              delivery.dialect
            }}</BaseBadge>
          </div>
          <InlineError v-if="delivery.error" variant="line" class="mb-1.5" :message="delivery.error" />
          <pre
            class="max-h-48 overflow-auto rounded-lg border border-row bg-app px-3 py-2.5 font-mono text-caption leading-relaxed text-text-2"
            >{{ bodyPreview(delivery) }}</pre>
        </div>
      </div>
      <EmptyState
        v-else
        variant="inline"
        message="No deliveries yet. Send a request to the ingress URL and reopen this editor to see it here."
        data-testid="sources.relay-editor-no-recent"
      />
    </div>
  </div>
</template>
