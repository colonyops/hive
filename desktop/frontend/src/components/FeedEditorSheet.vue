<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import IconAlertTriangle from '~icons/lucide/alert-triangle'
import IconCheck from '~icons/lucide/check'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconClipboardCopy from '~icons/lucide/clipboard-copy'
import IconMenu from '~icons/lucide/menu'
import IconPlus from '~icons/lucide/plus'
import IconRss from '~icons/lucide/rss'
import IconX from '~icons/lucide/x'
import { feedEntryYaml } from '../lib/feedYaml'
import { tokenizeYaml } from '../lib/yamlHighlight'
import type { ConfigInfo, FeedDef, FilterDef, SourceDef } from '../types/feed'

const props = defineProps<{
  /** Non-null selects edit mode; the definition arrives via initialDef. */
  feedId: string | null
  /** Edit-mode prefill (FeedDefFor result); null while loading or in create mode. */
  initialDef: FeedDef | null
  sources: SourceDef[]
  config: ConfigInfo | null
  busy: boolean
  error: string | null
  sourceBusy: boolean
  sourceError: string | null
}>()

const emit = defineEmits<{
  close: []
  save: [def: FeedDef]
  delete: [feedId: string]
  'create-source': [def: SourceDef]
  'copy-prompt': []
  'copy-path': []
}>()

const isEdit = computed(() => props.feedId !== null)

// ── Form state ───────────────────────────────────────────────────────────────

const name = ref('')
const checkedSources = ref<string[]>([])
// Glob groups are textareas, one pattern per line. Globs may contain commas
// via brace expansion ("acme/{a,b}"), so lines are never comma-split.
const reposText = ref('')
const excludeReposText = ref('')
const authorsText = ref('')
const excludeAuthorsText = ref('')
const labelsText = ref('')
const excludeLabelsText = ref('')
const types = ref<string[]>([])
const reasons = ref<string[]>([])

const reasonsOpen = ref(false)

function sourceSelected(id: string): boolean {
  return checkedSources.value.includes(id)
}

function typeChecked(value: string): boolean {
  return types.value.includes(value)
}

// The six glob groups render identically; keep the descriptors in one place.
// `model` holds the Ref itself (arrays don't unwrap), bound via .value.
const globGroups = [
  { key: 'repos', label: 'Repos', model: reposText, testid: 'feed-editor-repos', placeholder: 'colonyops/*' },
  { key: 'exclude_repos', label: 'Exclude repos', model: excludeReposText, testid: 'feed-editor-exclude-repos', placeholder: 'colonyops/sandbox' },
  { key: 'authors', label: 'Authors', model: authorsText, testid: 'feed-editor-authors', placeholder: 'hay-kot' },
  { key: 'exclude_authors', label: 'Exclude authors', model: excludeAuthorsText, testid: 'feed-editor-exclude-authors', placeholder: '*[bot]' },
  { key: 'labels', label: 'Labels', model: labelsText, testid: 'feed-editor-labels', placeholder: 'area/*' },
  { key: 'exclude_labels', label: 'Exclude labels', model: excludeLabelsText, testid: 'feed-editor-exclude-labels', placeholder: 'wontfix' },
]

const nameRef = ref<HTMLInputElement | null>(null)

// The full reasons vocabulary GitHub delivers (see internal/desktop/feed/filters.go).
const allReasons = [
  'approval_requested', 'assign', 'author', 'ci_activity', 'comment', 'invitation', 'manual',
  'member_feature_requested', 'mention', 'review_requested', 'security_advisory_credit',
  'security_alert', 'state_change', 'subscribed', 'team_mention',
]

function parseLines(text: string): string[] {
  return text.split('\n').map((line) => line.trim()).filter((line) => line.length > 0)
}

const filters = computed<FilterDef>(() => {
  const out: FilterDef = {}
  const groups: Array<[keyof FilterDef, string[]]> = [
    ['repos', parseLines(reposText.value)],
    ['exclude_repos', parseLines(excludeReposText.value)],
    ['authors', parseLines(authorsText.value)],
    ['exclude_authors', parseLines(excludeAuthorsText.value)],
    ['labels', parseLines(labelsText.value)],
    ['exclude_labels', parseLines(excludeLabelsText.value)],
    ['types', types.value],
    ['reasons', reasons.value],
  ]
  for (const [key, values] of groups) {
    if (values.length > 0) out[key] = [...values]
  }
  return out
})

watch(() => props.initialDef, (def) => {
  if (!def) return
  name.value = def.name
  checkedSources.value = [...(def.sources ?? [])]
  const f = def.filters ?? {}
  reposText.value = (f.repos ?? []).join('\n')
  excludeReposText.value = (f.exclude_repos ?? []).join('\n')
  authorsText.value = (f.authors ?? []).join('\n')
  excludeAuthorsText.value = (f.exclude_authors ?? []).join('\n')
  labelsText.value = (f.labels ?? []).join('\n')
  excludeLabelsText.value = (f.exclude_labels ?? []).join('\n')
  types.value = [...(f.types ?? [])]
  reasons.value = [...(f.reasons ?? [])]
  if (reasons.value.length > 0) reasonsOpen.value = true
}, { immediate: true })

function buildDef(): FeedDef {
  return {
    id: props.feedId ?? '',
    name: name.value.trim(),
    sources: [...checkedSources.value],
    // Empty filters stay {}: FeedDef.filters is non-optional on the wire.
    filters: filters.value,
  }
}

const canSave = computed(() =>
  name.value.trim().length > 0 &&
  checkedSources.value.length > 0 &&
  !(isEdit.value && !props.initialDef))

function submit() {
  if (props.busy || !canSave.value) return
  emit('save', buildDef())
}

// ── Delete (edit mode only) — two-step inline confirm, no modal. The parent
// owns the actual delete call; this component only reports intent. ─────────

const deleteConfirming = ref(false)

function confirmDelete() {
  if (!props.feedId) return
  emit('delete', props.feedId)
  deleteConfirming.value = false
}

// ── YAML preview ─────────────────────────────────────────────────────────────

const previewLines = computed(() => tokenizeYaml(feedEntryYaml(buildPreviewDef())))

function buildPreviewDef(): FeedDef {
  const def = buildDef()
  return { ...def, name: name.value.trim() || 'New feed' }
}

// ── Inline source creation ───────────────────────────────────────────────────

const newSourceOpen = ref(false)
const sourceId = ref('')
const sourceKind = ref<'search' | 'notifications'>('search')
const sourceQuery = ref('')
const sourceLimit = ref('')

const canAddSource = computed(() =>
  sourceId.value.trim().length > 0 &&
  (sourceKind.value === 'notifications' || sourceQuery.value.trim().length > 0))

// Set while an inline CreateSource is in flight so the sources watcher below
// knows the next new id is ours to auto-check.
const awaitingSource = ref(false)

function addSource() {
  if (props.sourceBusy || !canAddSource.value) return
  const def: SourceDef = { id: sourceId.value.trim(), kind: sourceKind.value }
  if (sourceKind.value === 'search') def.query = sourceQuery.value.trim()
  const limit = Number.parseInt(sourceLimit.value, 10)
  if (Number.isFinite(limit) && limit > 0) def.limit = limit
  awaitingSource.value = true
  emit('create-source', def)
}

// The parent appends the created source (with its backend-assigned id) to
// the sources prop; auto-check it and fold the expander away.
watch(() => props.sources, (next, prev) => {
  if (!awaitingSource.value) return
  const prevIds = new Set((prev ?? []).map((source) => source.id))
  const added = next.filter((source) => !prevIds.has(source.id))
  if (added.length === 0) return
  for (const source of added) {
    if (!checkedSources.value.includes(source.id)) checkedSources.value.push(source.id)
  }
  awaitingSource.value = false
  newSourceOpen.value = false
  sourceId.value = ''
  sourceQuery.value = ''
  sourceLimit.value = ''
  sourceKind.value = 'search'
})

watch(() => props.sourceError, (err) => {
  if (err) awaitingSource.value = false
})

// ── Lifecycle ────────────────────────────────────────────────────────────────

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

onMounted(async () => {
  window.addEventListener('keydown', onKeydown)
  await nextTick()
  nameRef.value?.focus()
})
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-40 bg-backdrop" data-testid="feed-editor-backdrop" @click.self="emit('close')">
      <aside
        class="absolute bottom-0 right-0 top-0 flex w-[620px] flex-col border-l border-strong bg-pane text-text shadow-[-30px_0_80px_-20px_rgba(0,0,0,.7)]"
        role="dialog"
        :aria-label="isEdit ? 'Edit feed' : 'New feed'"
        aria-modal="true"
        data-testid="feed-editor"
      >
        <header class="flex shrink-0 items-center gap-3 border-b border-row bg-pane px-[22px] py-[18px]">
          <span class="flex size-[30px] shrink-0 items-center justify-center rounded-lg bg-accent-tint text-accent"><IconRss class="size-4" /></span>
          <div class="flex-1 text-[16px] font-semibold tracking-[-.01em]" data-testid="feed-editor-title">{{ isEdit ? 'Edit feed' : 'New feed' }}</div>
          <button class="cursor-pointer text-text-3 hover:text-text" aria-label="Close" data-testid="feed-editor-close" @click="emit('close')"><IconX class="size-4.5" /></button>
        </header>

        <div class="hive-scroll flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto px-[22px] py-[22px]">
          <!-- Name -->
          <div>
            <div class="mb-1.5 text-[12.5px] text-text-2">Name</div>
            <input
              ref="nameRef"
              v-model="name"
              type="text"
              placeholder="Team PRs"
              class="w-full rounded-lg border border-strong bg-app px-3 py-2.5 text-[13.5px] text-text outline-none placeholder:text-text-4 focus:border-accent"
              data-testid="feed-editor-name"
              @keydown.enter="submit"
            >
            <p v-if="!isEdit" class="mt-1.5 text-xs leading-relaxed text-text-4">The feed's id is derived from the name on save.</p>
          </div>

          <div class="h-px shrink-0 bg-row" />

          <!-- Sources -->
          <div>
            <div class="mb-[3px] flex items-baseline justify-between gap-2">
              <div class="text-[14.5px] font-semibold tracking-[-.01em] text-text">Sources</div>
              <div class="font-mono text-[11.5px] text-text-4">{{ checkedSources.length }} selected</div>
            </div>
            <p class="mb-3.5 text-[12.5px] leading-relaxed text-text-4">
              Sources are the GitHub API cost, shared across feeds. This feed merges their items — pick at least one.
            </p>
            <div class="flex flex-col gap-2" data-testid="feed-editor-sources">
              <div v-if="sources.length === 0" class="px-2 py-1.5 font-mono text-[11.5px] text-text-4" data-testid="feed-editor-no-sources">
                No sources yet — create one below.
              </div>
              <label
                v-for="source in sources"
                :key="source.id"
                class="flex cursor-pointer gap-3 rounded-[10px] border px-[13px] py-3 transition-colors"
                :class="sourceSelected(source.id) ? 'border-strong bg-raised' : 'border-row bg-app hover:border-card'"
              >
                <span
                  class="relative mt-px flex size-[18px] shrink-0 items-center justify-center rounded-[5px] border-[1.5px]"
                  :class="sourceSelected(source.id) ? 'border-accent bg-accent' : 'border-strong bg-transparent'"
                >
                  <input
                    v-model="checkedSources"
                    type="checkbox"
                    :value="source.id"
                    class="absolute inset-0 size-full cursor-pointer opacity-0"
                    :data-testid="`feed-editor-source-${source.id}`"
                  >
                  <IconCheck v-if="sourceSelected(source.id)" class="size-3 text-accent-contrast" :stroke-width="3" />
                </span>
                <span class="min-w-0 flex-1">
                  <span class="flex items-center gap-2">
                    <span class="font-mono text-[13.5px] font-semibold text-text">{{ source.id }}</span>
                    <span class="rounded bg-chip px-1.5 py-px font-mono text-[10.5px] text-text-3">{{ source.kind }}</span>
                  </span>
                  <span v-if="source.query" class="mt-[5px] block truncate font-mono text-xs leading-relaxed text-text-4">{{ source.query }}</span>
                </span>
              </label>
            </div>

            <!-- Inline source creation -->
            <div class="mt-3 overflow-hidden rounded-[10px] border border-row">
              <button
                class="flex w-full cursor-pointer items-center gap-2 bg-raised px-[13px] py-[11px] text-left"
                :class="{ 'border-b border-row': newSourceOpen }"
                data-testid="feed-editor-new-source-toggle"
                @click="newSourceOpen = !newSourceOpen"
              >
                <IconPlus class="size-3.5 text-accent" />
                <span class="text-[12.5px] font-semibold text-text">New source</span>
                <span class="flex-1" />
                <component :is="newSourceOpen ? IconChevronDown : IconChevronRight" class="size-3 text-text-3" />
              </button>
              <div v-if="newSourceOpen" class="flex flex-col gap-3.5 bg-app p-[13px]" data-testid="feed-editor-new-source">
                <div class="grid grid-cols-[1fr_150px] gap-2.5">
                  <div>
                    <div class="mb-1.5 text-[11.5px] text-text-2">Id</div>
                    <input
                      v-model="sourceId"
                      type="text"
                      placeholder="team-prs"
                      class="w-full rounded-lg border border-card bg-app px-[11px] py-[9px] font-mono text-[13.5px] text-text outline-none placeholder:text-text-4 focus:border-accent"
                      data-testid="feed-editor-source-id"
                    >
                  </div>
                  <div>
                    <div class="mb-1.5 text-[11.5px] text-text-2">Kind</div>
                    <select
                      v-model="sourceKind"
                      class="w-full cursor-pointer rounded-lg border border-card bg-app px-[11px] py-[9px] text-[13.5px] text-text outline-none focus:border-accent"
                      data-testid="feed-editor-source-kind"
                    >
                      <option value="search">search</option>
                      <option value="notifications">notifications</option>
                    </select>
                  </div>
                </div>
                <div v-if="sourceKind === 'search'">
                  <div class="mb-1.5 text-[11.5px] text-text-2">Query</div>
                  <input
                    v-model="sourceQuery"
                    type="text"
                    placeholder="is:open involves:@me archived:false"
                    class="w-full rounded-lg border border-card bg-app px-[11px] py-[9px] font-mono text-[13.5px] text-text outline-none placeholder:text-text-4 focus:border-accent"
                    data-testid="feed-editor-source-query"
                  >
                </div>
                <div class="flex items-end gap-2.5">
                  <div class="w-[120px]">
                    <div class="mb-1.5 text-[11.5px] text-text-2">Limit</div>
                    <input
                      v-model="sourceLimit"
                      type="number"
                      placeholder="50"
                      min="1"
                      class="w-full rounded-lg border border-card bg-app px-[11px] py-[9px] font-mono text-[13.5px] text-text outline-none placeholder:text-text-4 focus:border-accent"
                      data-testid="feed-editor-source-limit"
                    >
                  </div>
                  <button
                    class="cursor-pointer rounded-lg border border-card bg-sidebar px-4 py-[9px] text-[13.5px] text-text hover:border-strong disabled:cursor-default disabled:opacity-50"
                    :disabled="sourceBusy || !canAddSource"
                    data-testid="feed-editor-source-add"
                    @click="addSource"
                  >Add source</button>
                </div>
                <p v-if="sourceError" class="text-xs leading-relaxed text-kind-issue" data-testid="feed-editor-source-error">{{ sourceError }}</p>
              </div>
            </div>
          </div>

          <div class="h-px shrink-0 bg-row" />

          <!-- Filters -->
          <div>
            <div class="mb-[3px] text-[14.5px] font-semibold tracking-[-.01em] text-text">Filters</div>
            <p class="mb-3.5 text-[12.5px] leading-relaxed text-text-4">
              Client-side, no API cost. Groups AND together; values within a group OR; excludes win. One doublestar glob per line.
            </p>
            <div class="flex flex-col gap-3.5" data-testid="feed-editor-filters">
              <div class="grid grid-cols-2 gap-x-3.5 gap-y-3">
                <div v-for="group in globGroups" :key="group.key">
                  <div class="mb-1.5 text-[11.5px] text-text-2">{{ group.label }}</div>
                  <textarea
                    v-model="group.model.value"
                    rows="2"
                    :placeholder="group.placeholder"
                    class="w-full resize-y rounded-lg border border-strong bg-app px-3 py-2.5 font-mono text-[12.5px] leading-relaxed text-text outline-none placeholder:text-text-4 focus:border-accent"
                    :data-testid="group.testid"
                  />
                </div>
              </div>

              <div class="mt-1 flex items-center gap-6 border-t border-row pt-4">
                <div class="text-[12.5px] text-text-2">Types</div>
                <label class="flex cursor-pointer items-center gap-2 text-[13px]" :class="typeChecked('pr') ? 'text-text' : 'text-text-2'">
                  <span
                    class="relative flex size-4 shrink-0 items-center justify-center rounded-[5px] border-[1.5px]"
                    :class="typeChecked('pr') ? 'border-accent bg-accent' : 'border-strong bg-transparent'"
                  >
                    <input v-model="types" type="checkbox" value="pr" class="absolute inset-0 size-full cursor-pointer opacity-0" data-testid="feed-editor-type-pr">
                    <IconCheck v-if="typeChecked('pr')" class="size-2.5 text-accent-contrast" :stroke-width="3.2" />
                  </span>
                  Pull requests
                </label>
                <label class="flex cursor-pointer items-center gap-2 text-[13px]" :class="typeChecked('issue') ? 'text-text' : 'text-text-2'">
                  <span
                    class="relative flex size-4 shrink-0 items-center justify-center rounded-[5px] border-[1.5px]"
                    :class="typeChecked('issue') ? 'border-accent bg-accent' : 'border-strong bg-transparent'"
                  >
                    <input v-model="types" type="checkbox" value="issue" class="absolute inset-0 size-full cursor-pointer opacity-0" data-testid="feed-editor-type-issue">
                    <IconCheck v-if="typeChecked('issue')" class="size-2.5 text-accent-contrast" :stroke-width="3.2" />
                  </span>
                  Issues
                </label>
              </div>

              <div>
                <button
                  class="flex cursor-pointer items-center gap-1.5 text-[12.5px] text-text-2 hover:text-text"
                  data-testid="feed-editor-reasons-toggle"
                  @click="reasonsOpen = !reasonsOpen"
                >
                  <component :is="reasonsOpen ? IconChevronDown : IconChevronRight" class="size-3.5" />
                  Notification reasons<span v-if="!reasonsOpen && reasons.length" class="font-mono text-[11px] text-accent">·&nbsp;{{ reasons.length }}</span>
                </button>
                <div v-if="reasonsOpen" class="mt-2 grid grid-cols-3 gap-x-3 gap-y-1.5" data-testid="feed-editor-reasons">
                  <label
                    v-for="reason in allReasons"
                    :key="reason"
                    class="flex cursor-pointer items-center gap-2 font-mono text-[11.5px] text-text-2"
                  >
                    <input v-model="reasons" type="checkbox" :value="reason" class="accent-accent" :data-testid="`feed-editor-reason-${reason}`">{{ reason }}
                  </label>
                </div>
                <p v-if="reasons.length > 0" class="mt-2 text-xs leading-relaxed text-text-4" data-testid="feed-editor-reasons-hint">
                  Reasons exist only on notification items — with a reasons filter, items known only from search sources are excluded.
                </p>
              </div>
            </div>
          </div>

          <div class="h-px shrink-0 bg-row" />

          <!-- YAML preview -->
          <div>
            <div class="mb-2 font-mono text-[11px] uppercase tracking-[.12em] text-text-4">Yaml preview</div>
            <pre class="hive-scroll max-h-[320px] overflow-y-auto rounded-[9px] border border-row bg-app px-3.5 py-3 font-mono text-xs leading-[1.65]" data-testid="feed-editor-yaml"><code><template v-for="(line, i) in previewLines" :key="i"><span v-for="(token, j) in line" :key="j" :class="{
              'text-code-key': token.kind === 'key',
              'text-code-string': token.kind === 'string',
              'text-code-comment': token.kind === 'comment',
              'text-text-2': token.kind === 'plain',
            }">{{ token.text }}</span>{{ '\n' }}</template></code></pre>
            <p class="mt-1.5 text-xs leading-relaxed text-text-4">
              Written into this profile's <span class="text-text-3">feeds</span> in the config below — hand edits and app edits share one file.
            </p>
          </div>

          <div class="h-px shrink-0 bg-row" />

          <!-- Config file: same as-code affordances as the config sheet -->
          <div>
            <div class="mb-1.5 text-[12.5px] text-text-2">Config file</div>
            <div class="flex gap-2">
              <div class="flex min-w-0 flex-1 items-center gap-2 rounded-lg border border-card bg-app px-3 py-2 font-mono text-[12.5px] text-text-2">
                <IconMenu class="size-3 shrink-0 text-text-3" />
                <span class="truncate" data-testid="feed-editor-path">{{ config?.path ?? '…' }}</span>
              </div>
              <button
                class="cursor-pointer whitespace-nowrap rounded-lg border border-card bg-sidebar px-3.5 py-2 text-[12.5px] text-text hover:border-strong"
                data-testid="feed-editor-copy-path"
                @click="emit('copy-path')"
              >Copy path</button>
              <button
                class="flex cursor-pointer items-center gap-1.5 whitespace-nowrap rounded-lg border border-card bg-sidebar px-3.5 py-2 text-[12.5px] text-text hover:border-strong"
                data-testid="feed-editor-copy-prompt"
                @click="emit('copy-prompt')"
              ><IconClipboardCopy class="size-3.5" />Copy prompt</button>
            </div>
          </div>

          <div v-if="error" class="flex items-start gap-2.5 rounded-lg border border-accent/40 bg-selection px-3 py-2.5" data-testid="feed-editor-error">
            <IconAlertTriangle class="mt-0.5 size-4 shrink-0 text-accent" />
            <div class="text-xs leading-relaxed text-text-2">
              <span class="font-semibold text-accent">Could not save the feed.</span>
              <span class="mt-0.5 block font-mono">{{ error }}</span>
            </div>
          </div>
        </div>

        <footer class="flex shrink-0 items-center gap-2.5 border-t border-row bg-raised px-[22px] py-4">
          <div v-if="isEdit" class="flex items-center gap-2.5">
            <button
              v-if="!deleteConfirming"
              class="cursor-pointer whitespace-nowrap text-[13.5px] text-kind-issue hover:brightness-110"
              data-testid="feed-editor-delete"
              @click="deleteConfirming = true"
            >Delete feed</button>
            <template v-else>
              <span class="whitespace-nowrap text-[13px] text-text-3">Confirm delete?</span>
              <button
                class="cursor-pointer whitespace-nowrap text-[13.5px] font-semibold text-kind-issue hover:brightness-110"
                data-testid="feed-editor-delete-confirm"
                @click="confirmDelete"
              >Delete</button>
              <button
                class="cursor-pointer whitespace-nowrap text-[13.5px] text-text-3 hover:text-text"
                data-testid="feed-editor-delete-cancel"
                @click="deleteConfirming = false"
              >Cancel</button>
            </template>
          </div>
          <button
            class="flex-1 cursor-pointer rounded-lg bg-accent px-4 py-2.5 text-[13.5px] font-semibold text-accent-contrast hover:brightness-110 disabled:cursor-default disabled:opacity-50"
            :disabled="busy || !canSave"
            data-testid="feed-editor-save"
            @click="submit"
          >{{ isEdit ? 'Save changes' : 'Create feed' }}</button>
          <button
            class="cursor-pointer whitespace-nowrap rounded-lg border border-card px-4 py-2.5 text-[13.5px] text-text-2 hover:text-text"
            data-testid="feed-editor-cancel"
            @click="emit('close')"
          >Cancel</button>
        </footer>
      </aside>
    </div>
  </Teleport>
</template>
