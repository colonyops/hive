<script setup lang="ts">
// Every changed file's diff in one scroll, with a file rail beside it and the
// review comments that go to the session's agent. Nothing here writes to the
// checkout: every read goes through the session API.
import { computed, nextTick, onMounted, ref, toRef, watch } from 'vue'
import { useEventListener, useStorage } from '@vueuse/core'
import IconColumns2 from '~icons/lucide/columns-2'
import IconMessageSquare from '~icons/lucide/message-square'
import IconSearch from '~icons/lucide/search'
import IconSend from '~icons/lucide/send'
import IconGitBranch from '~icons/lucide/git-branch'
import IconList from '~icons/lucide/list'
import IconListTree from '~icons/lucide/list-tree'
import IconRows2 from '~icons/lucide/rows-2'
import IconX from '~icons/lucide/x'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Kbd from '@hive/ui/components/ui/Kbd.vue'
import PanelResizeHandle from '@hive/ui/components/ui/PanelResizeHandle.vue'
import SegmentedControl, { type SegmentedControlOption } from '@hive/ui/components/ui/SegmentedControl.vue'
import ViewHeader from '@hive/ui/components/ui/ViewHeader.vue'
import { useEscapeToClose } from '@hive/ui/composables/useEscapeToClose'
import { useOpenModalCount } from '@hive/ui/composables/useOpenModalCount'
import { useResizablePanel } from '@hive/ui/composables/useResizablePanel'
import ChangesReviewPanel from './ChangesReviewPanel.vue'
import DiffSearchBar from './DiffSearchBar.vue'
import DiffStream from './DiffStream.vue'
import GitTree from './GitTree.vue'
import DriveCommentPopover from '../drive/DriveCommentPopover.vue'
import SendToAgentDialog from '../drive/SendToAgentDialog.vue'
import { entryRows, useDiffSearch } from '../../composables/useDiffSearch'
import { fileVersion, useFileDiffs } from '../../composables/useFileDiffs'
import { useSessionChanges, type DiffBase } from '../../composables/useSessionChanges'
import { useToasts } from '../../composables/useToasts'
import { errorText } from '../../lib/appError'
import { readOrder, type DiffMode } from '../../lib/diffItems'
import { diffSpan, hasDiffLine, spanLabel, type DiffSearchMatch, type DiffSpan } from '../../lib/diffReview'
import type { DiffLineSelection } from '../../lib/diffSelection'
import { gitFileName, gitRows, type GitLayout } from '../../lib/gitTree'
import { isEditableTarget } from '../../lib/isEditableTarget'
import { useChangesReview, type ChangesComment } from '../../stores/useChangesReview'
import { useChangesViewed } from '../../stores/useChangesViewed'
import { OpenSessionInEditor } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type { ChangedFile } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import type { AgentTarget } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

const props = defineProps<{ sessionId: string }>()
const emit = defineEmits<{ close: [] }>()

useEscapeToClose(() => emit('close'))
const openModalCount = useOpenModalCount()

const sessionId = toRef(props, 'sessionId')
const {
  size: railWidth,
  startResize: startRailResize,
  step: stepRail,
} = useResizablePanel({ storageKey: 'hive.panel.changes.files', defaultSize: 280, min: 180, max: 520, edge: 'right' })
const base = useStorage<DiffBase>('hive.git.base', 'branch')
const layout = useStorage<GitLayout>('hive.git.layout', 'tree')
const mode = useStorage<DiffMode>('hive.git.diffMode', 'unified')

// Held diffs set the scrollbar's length and take memory, so the read stops this
// many files from the one on screen. Scrolling on reads further.
const READ_WINDOW = 200

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

const toasts = useToasts()
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

const stream = ref<{
  focus: () => void
  scrollToFile: (path: string) => void
  stepHunk: (delta: 1 | -1) => boolean
  scrollToLine: (path: string, line: number, side: DiffSpan['startSide']) => void
  selectLines: (path: string, range: DiffLineSelection['range']) => void
  clearSelection: () => void
} | null>(null)
const scope = computed(() => `${props.sessionId}:${base.value}`)
const current = ref<string | null>(null)
const openPaths = computed(() => new Set(files.value.filter(isOpen).map((file) => file.path)))
const wanted = computed(() => {
  const center = files.value.findIndex((file) => file.path === current.value)
  const open = readOrder(files.value, center).filter((file) => openPaths.value.has(file.path))
  return open.slice(0, READ_WINDOW).map((file) => file.path)
})
const { entries } = useFileDiffs(sessionId, base, files, wanted)

// Arrow keys and Page Down scroll what has focus. HubOverlay focuses the panel
// one tick after it mounts, which is after this child, so this waits one more.
onMounted(() => {
  void nextTick()
    .then(() => nextTick())
    .then(() => stream.value?.focus())
})
watch(
  () => files.value.map((file) => file.path).join('\n'),
  () => {
    if (!current.value || !files.value.some((file) => file.path === current.value)) {
      current.value = files.value[0]?.path ?? null
    }
  },
)

watch([sessionId, base], () => {
  foldOverrides.value = new Map()
})

function jumpTo(path: string): void {
  current.value = path
  stream.value?.scrollToFile(path)
}

function stepFile(delta: number): void {
  const index = files.value.findIndex((file) => file.path === current.value)
  const next = files.value[index + delta]
  if (next) jumpTo(next.path)
}

function stepHunk(delta: 1 | -1): void {
  if (!stream.value?.stepHunk(delta)) stepFile(delta)
}

// A jump into a folded file unfolds it first, and the stream only learns of that
// after the next tick.
async function revealLine(path: string, line: number, side: DiffSpan['startSide']): Promise<void> {
  const file = files.value.find((candidate) => candidate.path === path)
  if (file && !isOpen(file)) {
    foldOverrides.value.set(path, true)
    await nextTick()
  }
  current.value = path
  stream.value?.scrollToLine(path, line, side)
}

const search = useDiffSearch(files, entries, (match: DiffSearchMatch) => {
  void revealLine(match.path, match.line, match.side)
})
const searchBar = ref<{ focus: () => void } | null>(null)

function openSearch(): void {
  search.open.value = true
  searchBar.value?.focus()
}

function closeSearch(): void {
  search.close()
  stream.value?.focus()
}

const review = useChangesReview()
const comments = computed(() => review.comments(props.sessionId))
const reviewOpen = useStorage('hive.panel.changes.review.open', false)
const canSend = computed(() => comments.value.length > 0)
const sendLabel = computed(() =>
  comments.value.length === 1 ? 'Send 1 comment to the agent' : `Send ${comments.value.length} comments to the agent`,
)

// Only comments whose line is in the diff on screen get a row. The rest stay in
// the panel: the comparison differs, or the agent has moved the line away.
const shownComments = computed(() => {
  const byPath = new Map<string, ChangesComment[]>()
  for (const comment of comments.value) {
    if (comment.base !== shownBase.value) continue
    if (!hasDiffLine(entryRows(entries.value.get(comment.path)), comment.endLine, comment.endSide)) continue
    byPath.set(comment.path, [...(byPath.get(comment.path) ?? []), comment])
  }
  return byPath
})
const commentCounts = computed(() => {
  const counts = new Map<string, number>()
  for (const { path } of comments.value) counts.set(path, (counts.get(path) ?? 0) + 1)
  return counts
})

function isOutdated(comment: ChangesComment): boolean {
  const file = files.value.find((candidate) => candidate.path === comment.path)
  return !file || fileVersion(file) !== comment.version
}

interface CommentDraft {
  path: string
  span: DiffSpan
  rect: DiffLineSelection['rect']
  text: string
}
const draft = ref<CommentDraft | null>(null)

function startComment(selection: DiffLineSelection): void {
  const span = diffSpan(entryRows(entries.value.get(selection.path)), selection.range)
  if (!span) return
  stream.value?.selectLines(selection.path, selection.range)
  draft.value = {
    path: selection.path,
    span: selection.text ? { ...span, quote: selection.text } : span,
    rect: selection.rect,
    text: draft.value?.text ?? '',
  }
}

const draftRange = computed(() => {
  const span = draft.value?.span
  return span
    ? {
        startLine: span.startLine,
        endLine: span.endLine,
        context: span.quote
          .split('\n')[0]
          .replace(/^[-+ ]/, '')
          .trim(),
      }
    : null
})

function endDraft(): void {
  draft.value = null
  stream.value?.clearSelection()
  document.getSelection()?.removeAllRanges()
}

function saveDraft(text: string): void {
  const held = draft.value
  const file = files.value.find((candidate) => candidate.path === held?.path)
  if (!held || !file) return
  // Only the first comment opens the panel, so one the user closed stays closed while they carry on.
  if (!comments.value.length) reviewOpen.value = true
  review.add(props.sessionId, {
    ...held.span,
    path: held.path,
    base: shownBase.value,
    version: fileVersion(file),
    text: text.trim(),
  })
  endDraft()
}

async function pickComment(comment: ChangesComment): Promise<void> {
  if (comment.base !== shownBase.value) base.value = comment.base
  await revealLine(comment.path, comment.endLine, comment.endSide)
  stream.value?.selectLines(comment.path, {
    start: comment.startLine,
    side: comment.startSide,
    end: comment.endLine,
    endSide: comment.endSide,
  })
}

const picking = ref(false)
const sending = ref(false)
const loadTargets = () => review.targets(props.sessionId)
async function deliver(target: AgentTarget): Promise<string | null> {
  const failure = await review.send(props.sessionId, target.pane)
  if (!failure) reviewOpen.value = false
  return failure
}

// The view belongs to one session, so its only agent needs no picking. Go refuses
// one on a prompt, which arrives here as the failure. Only several agents, or none
// to explain, need the dialog.
async function sendReview(): Promise<void> {
  if (!canSend.value || sending.value) return
  sending.value = true
  try {
    const targets = await loadTargets()
    if (targets.length === 1) {
      const failure = await deliver(targets[0])
      if (failure) toasts.showToast(failure, { severity: 'error' })
      else toasts.showToast(`Sent to ${targets[0].label}`)
    } else {
      picking.value = true
    }
  } catch {
    // The dialog reads the targets again and says why it could not.
    picking.value = true
  } finally {
    sending.value = false
  }
}

// On the window, because a click on a file header moves focus to the panel,
// which is outside this view's root.
useEventListener(window, 'keydown', (event: KeyboardEvent) => {
  if (openModalCount.value > 0) return
  if (event.metaKey && !event.ctrlKey && !event.altKey) {
    if (event.key === 'f') {
      event.preventDefault()
      openSearch()
    } else if (event.key === 'Enter' && !isEditableTarget(event.target)) {
      // A text field's ⌘↵ saves what is being typed. Its handler has already cleared
      // the draft by the time the key reaches here, so only the target can tell.
      event.preventDefault()
      void sendReview()
    }
    return
  }
  if (isEditableTarget(event.target) || event.ctrlKey || event.altKey) return
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
    case '/':
      openSearch()
      break
    case 'r':
      reviewOpen.value = !reviewOpen.value
      break
    default:
      return
  }
  event.preventDefault()
})

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
  ['/', 'search'],
  ['r', 'comments'],
  ['⌘↵', 'send'],
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
        <IconButton
          label="Search the diffs"
          tooltip="Search the diffs (/)"
          :icon="IconSearch"
          size="lg"
          data-testid="changes-search-open"
          @click="openSearch"
        />
        <IconButton
          :label="reviewOpen ? 'Hide comments' : 'Show comments'"
          :tooltip="`${reviewOpen ? 'Hide' : 'Show'} comments (r)`"
          :icon="IconMessageSquare"
          size="lg"
          :aria-pressed="reviewOpen"
          data-testid="changes-review-toggle"
          @click="reviewOpen = !reviewOpen"
        />
        <span v-if="canSend" class="relative flex shrink-0">
          <IconButton
            :label="sendLabel"
            :tooltip="`${sendLabel} (⌘↵)`"
            :icon="IconSend"
            size="lg"
            active
            :busy="sending"
            data-testid="changes-send"
            @click="sendReview"
          />
          <span
            class="pointer-events-none absolute -top-1 -right-1 flex h-3.5 min-w-3.5 items-center justify-center rounded-full bg-accent px-0.5 font-mono text-micro font-semibold text-accent-contrast"
            data-testid="changes-send-count"
            >{{ comments.length }}</span
          >
        </span>
        <span class="mx-1 h-4 w-px bg-strong" aria-hidden="true" />
        <IconButton label="Close" :icon="IconX" size="lg" data-testid="changes-close" @click="emit('close')" />
      </template>
    </ViewHeader>

    <InlineError v-if="error" class="mx-3 mt-2 shrink-0" :message="error" testid="changes-error" />

    <div class="flex min-h-0 flex-1">
      <aside class="relative flex shrink-0 flex-col border-r border-border" :style="{ width: `${railWidth}px` }">
        <PanelResizeHandle edge="right" name="changes-files" :start="startRailResize" :step="stepRail" />
        <!-- As tall as a diff's file header (DIFF_METRICS.headerPx), which sits beside it. -->
        <div class="flex h-8 shrink-0 items-center gap-2 border-b border-border px-2.5">
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
            :comment-counts="commentCounts"
            @pick="jumpTo"
            @open-editor="openInEditor"
          />
          <p v-if="changes?.truncated" class="px-3 pb-3 text-xs text-text-3" data-testid="changes-truncated">
            Showing the first {{ files.length }} files. Open the session in an editor to see the rest.
          </p>
        </div>
      </aside>
      <div class="flex min-w-0 flex-1 flex-col">
        <DiffSearchBar
          v-if="search.open.value"
          ref="searchBar"
          v-model="search.query.value"
          :count="search.matches.value.length"
          :index="search.activeIndex.value"
          :searched="search.searchedFiles.value"
          :total="files.length"
          @step="search.step"
          @close="closeSearch"
        />
        <div v-if="changes && !files.length" class="min-h-0 min-w-0 flex-1 bg-app">
          <EmptyState :message="emptyMessage" data-testid="changes-empty" />
        </div>
        <DiffStream
          v-else
          ref="stream"
          :key="scope"
          v-model:current="current"
          class="min-h-0"
          :scope="scope"
          :files="files"
          :entries="entries"
          :open-paths="openPaths"
          :viewed-paths="viewedPaths"
          :mode="mode"
          :comments="shownComments"
          :is-outdated="isOutdated"
          :search-query="search.debounced.value"
          :active-match="search.active.value"
          @fold="toggleFold"
          @toggle-viewed="toggleViewed"
          @open-editor="openInEditor"
          @select-lines="startComment"
          @update-comment="(comment, text) => review.update(sessionId, comment.id, text)"
          @remove-comment="(comment) => review.remove(sessionId, comment.id)"
          @pick-comment="pickComment"
        />
      </div>
      <ChangesReviewPanel v-if="reviewOpen" :session-id="sessionId" :is-outdated="isOutdated" @pick="pickComment" />
    </div>

    <DriveCommentPopover
      v-if="draft && draftRange"
      v-model:text="draft.text"
      :rect="draft.rect"
      :range="draftRange"
      :label="`${gitFileName(draft.path)} ${spanLabel(draft.span)}`"
      :busy="false"
      :error="null"
      testid="changes"
      @save="saveDraft"
      @cancel="endDraft"
    />
    <SendToAgentDialog
      v-if="picking"
      :load="loadTargets"
      :deliver="deliver"
      testid="changes"
      @close="picking = false"
    />

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
