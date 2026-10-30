<script setup lang="ts">
// Every changed file's diff in one scroll, with a file rail beside it. It is
// read-only: every read goes through the session API, and nothing here writes
// to the checkout.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, toRef, watch } from 'vue'
import { useEventListener, useResizeObserver, useStorage } from '@vueuse/core'
import IconColumns2 from '~icons/lucide/columns-2'
import IconGitBranch from '~icons/lucide/git-branch'
import IconList from '~icons/lucide/list'
import IconListTree from '~icons/lucide/list-tree'
import IconRows2 from '~icons/lucide/rows-2'
import IconX from '~icons/lucide/x'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Kbd from '@hive/ui/components/ui/Kbd.vue'
import SegmentedControl, { type SegmentedControlOption } from '@hive/ui/components/ui/SegmentedControl.vue'
import ViewHeader from '@hive/ui/components/ui/ViewHeader.vue'
import { useEscapeToClose } from '@hive/ui/composables/useEscapeToClose'
import { useOpenModalCount } from '@hive/ui/composables/useOpenModalCount'
import DiffFile from './DiffFile.vue'
import GitTree from './GitTree.vue'
import { fileVersion, useFileDiffs } from '../../composables/useFileDiffs'
import { useSessionChanges, type DiffBase } from '../../composables/useSessionChanges'
import { useToasts } from '../../composables/useToasts'
import { errorText } from '../../lib/appError'
import { gitRows, type GitLayout } from '../../lib/gitTree'
import { isEditableTarget } from '../../lib/isEditableTarget'
import { useChangesViewed } from '../../stores/useChangesViewed'
import { OpenSessionInEditor } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type { ChangedFile } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{ sessionId: string }>()
const emit = defineEmits<{ close: [] }>()

useEscapeToClose(() => emit('close'))
const openModalCount = useOpenModalCount()

const sessionId = toRef(props, 'sessionId')
const base = useStorage<DiffBase>('hive.git.base', 'branch')
const layout = useStorage<GitLayout>('hive.git.layout', 'tree')
const mode = useStorage<'unified' | 'split'>('hive.git.diffMode', 'unified')

const { changes, error } = useSessionChanges(sessionId, base, () => true)
// The tree lists folders first, so the stream follows the rail's order rather
// than git's: j and k then walk the rail row by row.
const files = computed<ChangedFile[]>(() =>
  gitRows(changes.value?.files ?? [], layout.value, () => true).flatMap((row) =>
    row.kind === 'file' ? [row.file] : [],
  ),
)

const shownBase = computed<DiffBase>(() => (changes.value?.base as DiffBase | undefined) || base.value)
const baseOptions = computed<SegmentedControlOption<DiffBase>[]>(() => [
  ...(changes.value?.branchAvailable
    ? [{ value: 'branch' as const, label: 'Branch', title: `Against ${changes.value.defaultBranch}` }]
    : []),
  { value: 'uncommitted', label: 'Uncommitted', title: 'Against HEAD' },
])
const additions = computed(() => files.value.reduce((sum, file) => sum + file.additions, 0))
const deletions = computed(() => files.value.reduce((sum, file) => sum + file.deletions, 0))
const emptyMessage = computed(() =>
  shownBase.value === 'branch'
    ? `No changes against ${changes.value?.defaultBranch ?? 'the default branch'}`
    : 'No uncommitted changes',
)

const viewedStore = useChangesViewed()
const isViewed = (file: ChangedFile) => viewedStore.isViewed(props.sessionId, file.path, fileVersion(file))
const viewedPaths = computed(() => new Set(files.value.filter(isViewed).map((file) => file.path)))

// An override wins until the file's viewed mark changes. With none, a viewed
// file is folded, so a later edit that clears the mark opens it again.
const foldOverrides = ref(new Map<string, boolean>())
const isOpen = (file: ChangedFile) => foldOverrides.value.get(file.path) ?? !isViewed(file)

function toggleFold(file: ChangedFile): void {
  foldOverrides.value.set(file.path, !isOpen(file))
  current.value = file.path
}

function toggleViewed(file: ChangedFile): void {
  viewedStore.setViewed(props.sessionId, file.path, fileVersion(file), !isViewed(file))
  foldOverrides.value.delete(file.path)
}

const scroller = ref<HTMLElement | null>(null)
const current = ref<string | null>(null)
const near = ref(new Set<string>())
const wanted = computed(() => {
  const open = files.value.filter((file) => isOpen(file) && near.value.has(file.path))
  return open.map((file) => file.path)
})
const { entries } = useFileDiffs(sessionId, base, files, wanted)

// Sections are observed by the path they carry, so the rendered list can change
// under the observer.
const NEAR_MARGIN_PX = 1500
let nearObserver: IntersectionObserver | null = null
const sectionElements = ref<HTMLElement[]>([])
const heights = new WeakMap<Element, number>()

// WebKit has no scroll anchoring, so a file growing above the viewport as its
// diff loads would push the one being read out of view.
useResizeObserver(sectionElements, (records) => {
  const box = scroller.value
  if (!box) return
  const top = box.getBoundingClientRect().top
  for (const record of records) {
    const { height, bottom } = record.target.getBoundingClientRect()
    const before = heights.get(record.target)
    heights.set(record.target, height)
    if (before !== undefined && bottom - (height - before) <= top) box.scrollTop += height - before
  }
})

function sections(): HTMLElement[] {
  return [...(scroller.value?.querySelectorAll<HTMLElement>('section[data-file]') ?? [])]
}

function observeSections(): void {
  sectionElements.value = sections()
  for (const section of sectionElements.value) nearObserver?.observe(section)
}

onMounted(() => {
  if (typeof IntersectionObserver !== 'undefined') {
    nearObserver = new IntersectionObserver(
      (records) => {
        const next = new Set(near.value)
        for (const record of records) {
          const path = (record.target as HTMLElement).dataset.file
          if (!path) continue
          if (record.isIntersecting) next.add(path)
          else next.delete(path)
        }
        near.value = next
      },
      { root: scroller.value, rootMargin: `${NEAR_MARGIN_PX}px 0px` },
    )
  }
  observeSections()
  // Arrow keys and Page Down scroll what has focus. HubOverlay focuses the panel
  // one tick after it mounts, which is after this child, so this waits one more.
  void nextTick()
    .then(() => nextTick())
    .then(() => scroller.value?.focus({ preventScroll: true }))
})
onBeforeUnmount(() => {
  nearObserver?.disconnect()
})
watch(
  () => files.value.map((file) => file.path).join('\n'),
  () => {
    if (!current.value || !files.value.some((file) => file.path === current.value)) {
      current.value = files.value[0]?.path ?? null
    }
    void nextTick(observeSections)
  },
)

watch([sessionId, base], () => {
  near.value = new Set()
  foldOverrides.value = new Map()
  if (scroller.value) scroller.value.scrollTop = 0
})

function jumpTo(path: string): void {
  current.value = path
  const box = scroller.value
  const section = sections().find((node) => node.dataset.file === path)
  if (box && section) box.scrollTop += section.getBoundingClientRect().top - box.getBoundingClientRect().top
}

function onScroll(): void {
  const box = scroller.value
  if (!box) return
  const line = box.getBoundingClientRect().top + 8
  let at: string | undefined
  for (const section of sections()) {
    if (section.getBoundingClientRect().top > line) break
    at = section.dataset.file
  }
  if (at) current.value = at
}

function stepFile(delta: number): void {
  const index = files.value.findIndex((file) => file.path === current.value)
  const next = files.value[index + delta]
  if (next) jumpTo(next.path)
}

function stepHunk(delta: number): void {
  const box = scroller.value
  if (!box) return
  // Below the sticky file header, which covers the top of the scroller.
  const line = box.getBoundingClientRect().top + 34
  const offset = (hunk: Element) => hunk.getBoundingClientRect().top - line
  const hunks = [...box.querySelectorAll('[data-hunk]')]
  const next = delta > 0 ? hunks.find((hunk) => offset(hunk) > 2) : hunks.reverse().find((hunk) => offset(hunk) < -2)
  if (next) box.scrollTop += offset(next)
  else stepFile(delta)
}

// On the window, because a click on a file header moves focus to the panel,
// which is outside this view's root.
useEventListener(window, 'keydown', (event: KeyboardEvent) => {
  if (openModalCount.value > 0 || isEditableTarget(event.target) || event.metaKey || event.ctrlKey || event.altKey)
    return
  const file = files.value.find((candidate) => candidate.path === current.value)
  switch (event.key) {
    case 'j':
      stepFile(1)
      break
    case 'k':
      stepFile(-1)
      break
    case 'n':
      stepHunk(1)
      break
    case 'N':
      stepHunk(-1)
      break
    case 's':
      mode.value = mode.value === 'split' ? 'unified' : 'split'
      break
    case 'v':
      if (file) toggleViewed(file)
      break
    case 'o':
      if (file) toggleFold(file)
      break
    default:
      return
  }
  event.preventDefault()
})

const toasts = useToasts()
function openInEditor(): void {
  OpenSessionInEditor(props.sessionId).catch((err: unknown) =>
    toasts.showToast(errorText(err, 'Could not open the editor.'), { severity: 'error' }),
  )
}

const KEY_HINTS = [
  ['j k', 'file'],
  ['n N', 'hunk'],
  ['v', 'viewed'],
  ['o', 'fold'],
  ['s', 'side by side'],
  ['esc', 'close'],
]
</script>

<template>
  <div class="flex h-full min-h-0 flex-1 flex-col" data-testid="changes-view">
    <ViewHeader>
      <template #title>
        <span class="text-body font-semibold text-text">Changes</span>
        <span class="flex min-w-0 items-center gap-1 font-mono text-caption text-text-2">
          <IconGitBranch class="size-3 shrink-0 text-text-4" aria-hidden="true" />
          <span class="truncate" data-testid="changes-branch">{{ changes?.branch }}</span>
        </span>
        <div class="min-w-0 flex-1" />
        <SegmentedControl
          :model-value="shownBase"
          :options="baseOptions"
          variant="compact"
          size="sm"
          aria-label="Compare against"
          testid="changes-base"
          @update:model-value="base = $event"
        />
        <span class="shrink-0 font-mono text-caption" data-testid="changes-total">
          <span class="text-severity-success">+{{ additions }}</span>
          <span class="ml-1.5 text-severity-error">-{{ deletions }}</span>
        </span>
        <IconButton
          :label="mode === 'split' ? 'Show unified' : 'Show side by side'"
          :icon="mode === 'split' ? IconRows2 : IconColumns2"
          size="lg"
          data-testid="changes-mode"
          @click="mode = mode === 'split' ? 'unified' : 'split'"
        />
        <span class="mx-1 h-4 w-px bg-strong" aria-hidden="true" />
        <IconButton label="Close" :icon="IconX" size="lg" data-testid="changes-close" @click="emit('close')" />
      </template>
    </ViewHeader>

    <InlineError v-if="error" class="mx-3 mt-2 shrink-0" :message="error" testid="changes-error" />

    <div class="flex min-h-0 flex-1">
      <aside class="flex w-[clamp(210px,24%,320px)] shrink-0 flex-col border-r border-border">
        <div class="flex shrink-0 items-center gap-2 border-b border-border px-2.5 py-1">
          <span class="h-1 min-w-0 flex-1 overflow-hidden rounded-sm bg-chip">
            <span
              class="block h-full bg-severity-success"
              :style="{ width: files.length ? `${(viewedPaths.size / files.length) * 100}%` : '0' }"
            />
          </span>
          <span class="shrink-0 font-mono text-caption text-text-3" data-testid="changes-progress"
            >{{ viewedPaths.size }}/{{ files.length }} viewed</span
          >
          <IconButton
            size="sm"
            :label="layout === 'tree' ? 'Show as list' : 'Show as tree'"
            :icon="layout === 'tree' ? IconList : IconListTree"
            data-testid="changes-layout"
            @click="layout = layout === 'tree' ? 'flat' : 'tree'"
          />
        </div>
        <div class="hive-scroll min-h-0 flex-1 overflow-y-auto">
          <GitTree
            :files="files"
            :layout="layout"
            :selected="current"
            :viewed="viewedPaths"
            @pick="jumpTo"
            @open-editor="openInEditor"
          />
          <p v-if="changes?.truncated" class="px-3 pb-3 text-xs text-text-3" data-testid="changes-truncated">
            Showing the first {{ files.length }} files. Open the session in an editor to see the rest.
          </p>
        </div>
      </aside>
      <div
        ref="scroller"
        class="hive-scroll relative min-w-0 flex-1 overflow-y-auto bg-app outline-none"
        tabindex="0"
        aria-label="Diffs"
        data-testid="changes-stream"
        @scroll.passive="onScroll"
      >
        <EmptyState v-if="changes && !files.length" :message="emptyMessage" data-testid="changes-empty" />
        <DiffFile
          v-for="file in files"
          :key="file.path"
          :file="file"
          :entry="entries.get(file.path)"
          :open="isOpen(file)"
          :viewed="viewedPaths.has(file.path)"
          :current="current === file.path"
          :near="near.has(file.path)"
          :mode="mode"
          @fold="toggleFold(file)"
          @toggle-viewed="toggleViewed(file)"
          @open-editor="openInEditor"
        />
      </div>
    </div>

    <div
      class="flex shrink-0 flex-wrap items-center gap-x-3.5 gap-y-0.5 border-t border-border px-3 py-1 text-caption text-text-3"
    >
      <span v-for="[key, label] in KEY_HINTS" :key="key" class="flex items-center gap-1">
        <Kbd>{{ key }}</Kbd>
        {{ label }}
      </span>
    </div>
  </div>
</template>
