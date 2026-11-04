<script setup lang="ts">
// Every changed file's diff in one scroll. CodeView owns the scroller and keeps
// only the lines near the viewport in the DOM (ADR the-changes-view-renders-through-pierre-diffs).
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useEventListener, useResizeObserver } from '@vueuse/core'
import {
  CodeView,
  type CodeViewDiffItem,
  type CodeViewOptions,
  type FileDiffLoadedFiles,
  type FileDiffMetadata,
  type LineAnnotation,
  type PostRenderPhase,
  type SelectedLineRange,
} from '@pierre/diffs'
import { useHiveUiPlatform } from '@hive/ui/platform'
import AgentCommentCard from './AgentCommentCard.vue'
import DiffCommentCard from './DiffCommentCard.vue'
import DiffFileHeader from './DiffFileHeader.vue'
import { usePerf } from '../../composables/usePerf'
import { useToasts } from '../../composables/useToasts'
import { useScrollGestureSpan } from '../../composables/useScrollGestureSpan'
import type { DiffEntry } from '../../composables/useFileDiffs'
import { useSurfaceScale } from '../../stores/useSurfaceScale'
import { diffThemeName, diffWorkerPool } from '../../lib/diffHighlight'
import { errorText } from '../../lib/appError'
import { hunksMatch, wholeFileFromPatch } from '../../lib/diffContext'
import { gitFileName } from '../../lib/gitTree'
import { diffMetrics, diffHasText, emptyFileDiff, hunkOffsets, parseFileDiff, type DiffMode } from '../../lib/diffItems'
import type { AgentComment, AnchoredAgentComment } from '../../lib/agentReview'
import type { DiffSearchMatch, DiffSide } from '../../lib/diffReview'
import { clearSearchPaint, paintSearch, SEARCH_HIGHLIGHT_CSS } from '../../lib/diffSearchPaint'
import { diffTextSelection, type DiffLineSelection } from '../../lib/diffSelection'
import type { ChangesComment } from '../../stores/useChangesReview'
import type {
  ChangedFile,
  SessionFileDiff,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  /** Which read this stream shows: a session and a target. Held diffs are cached under it. */
  scope: string
  files: ChangedFile[]
  entries: ReadonlyMap<string, DiffEntry>
  openPaths: ReadonlySet<string>
  viewedPaths: ReadonlySet<string>
  current: string | null
  mode: DiffMode
  comments: ReadonlyMap<string, readonly ChangesComment[]>
  isOutdated: (comment: ChangesComment) => boolean
  searchQuery: string
  activeMatch: DiffSearchMatch | null
  /** Reads a file's diff with the whole file as context, for the lines between hunks. */
  loadWholeFile: (path: string) => Promise<SessionFileDiff>
  /** Agents' comments by path, each on the row it is drawn under. */
  agentComments?: ReadonlyMap<string, readonly AnchoredAgentComment[]>
  isAgentStale?: (comment: AgentComment) => boolean
  /** The agent comment the walkthrough is on. */
  focusedAgent?: string | null
}>()
const emit = defineEmits<{
  'update:current': [path: string]
  fold: [file: ChangedFile]
  'toggle-viewed': [file: ChangedFile]
  'open-editor': []
  'comment-file': [file: ChangedFile, rect: DOMRect]
  /** The user selected lines to comment on. `text` is set when they selected code rather than line numbers. */
  'select-lines': [selection: DiffLineSelection]
  'update-comment': [comment: ChangesComment, text: string]
  'remove-comment': [comment: ChangesComment]
  'pick-comment': [comment: ChangesComment]
  'dismiss-agent': [comment: AgentComment]
  'pick-agent': [comment: AgentComment]
  synced: []
}>()

// The separator's height is the only row the library's own stylesheet sizes
// past what its variables reach. No background of its own: the app's shows through.
//
// A file's horizontal scrollbar takes its own height below the last row, and
// CodeView lays files out from itemMetrics without measuring them. Left to the
// library, that height is whatever the platform's scrollbar measures, so each
// mounted file overflowed its slot and the last one lost its bottom rows. Fixing
// it here lets SCROLLBAR_PX stand in itemMetrics as the file's bottom padding.
const SCROLLBAR_PX = 8
const unsafeCss = (separatorPx: number): string => `
  :host { --diffs-bg: var(--color-app); --diffs-scrollbar-gutter-override: ${SCROLLBAR_PX}px; }
  [data-separator=line-info-basic] { height: ${separatorPx}px; }
  ${SEARCH_HIGHLIGHT_CSS}
`
// Lines a click on a separator reveals, as a review tool does: enough to read
// the code around a hunk without burying it.
const EXPANSION_LINES = 20

// A saved comment's row, found by id across CodeView remounting it.
interface CommentAnnotation {
  commentId: string
  /** An agent's comment, which sits on a band of its own so it stands out while scrolling. */
  agent?: boolean
}

const root = ref<HTMLElement | null>(null)
const perf = usePerf('changes')
useScrollGestureSpan(root, (durationMs, attrs) => perf.record('scroll:gesture', durationMs, { ...attrs }))
const platform = useHiveUiPlatform()
const log = platform.log('changes')
const pool = diffWorkerPool(platform.theme.value, log)
let viewer: CodeView<CommentAnnotation> | null = null
let scrollTop = 0

// Header hosts stay per path: CodeView asks again each time a file remounts, and
// the same element keeps its teleported header's state.
const headerHosts = new Map<string, HTMLElement>()
const mountedHeaders = shallowRef<ReadonlySet<string>>(new Set())
const fileByPath = computed(() => new Map(props.files.map((file) => [file.path, file])))

const pinnedPath = ref<string | null>(null)
const pinnedFile = computed(() => (pinnedPath.value ? fileByPath.value.get(pinnedPath.value) : undefined))
// The next file's header pushes the pinned one out as it arrives, as a sticky header does.
const pinnedPush = ref(0)
// Stops short of the scroller's scrollbar, which must stay draggable.
const pinnedWidth = ref(0)
useResizeObserver(root, () => {
  pinnedWidth.value = root.value?.clientWidth ?? 0
})

function setHeaderMounted(path: string, mounted: boolean): void {
  if (mountedHeaders.value.has(path) === mounted) return
  const next = new Set(mountedHeaders.value)
  if (mounted) next.add(path)
  else next.delete(path)
  mountedHeaders.value = next
}

function headerHost(path: string): HTMLElement {
  let host = headerHosts.get(path)
  if (!host) {
    host = document.createElement('div')
    headerHosts.set(path, host)
  }
  setHeaderMounted(path, true)
  return host
}

// CodeView passes each callback the item it is for. Its overloads leave the
// parameter untyped in a callback written inline.
interface ItemContext {
  item: { id: string }
}

const commentHosts = new Map<string, HTMLElement>()
const mountedComments = shallowRef<ReadonlySet<string>>(new Set())
const commentById = computed(() => new Map([...props.comments.values()].flat().map((comment) => [comment.id, comment])))
const agentById = computed(
  () =>
    new Map(
      [...(props.agentComments?.values() ?? [])].flat().map((anchored) => [anchored.comment.id, anchored.comment]),
    ),
)

function commentHost(id: string, agent = false): HTMLElement {
  let host = commentHosts.get(id)
  if (!host) {
    host = document.createElement('div')
    host.className = agent ? 'border-y border-border bg-pane px-3 py-2' : 'p-2'
    commentHosts.set(id, host)
    mountedComments.value = new Set([...mountedComments.value, id])
  }
  return host
}

// The pointer's last position, where the composer opens: CodeView reports a
// finished selection without one.
let pointer = { x: 0, y: 0 }
useEventListener(root, 'pointerup', (event: PointerEvent) => {
  pointer = { x: event.clientX, y: event.clientY }
})

function onLineSelectionEnd(range: SelectedLineRange | null, context: ItemContext): void {
  if (range) {
    emit('select-lines', {
      path: context.item.id,
      range,
      rect: { top: pointer.y, left: pointer.x, width: 0, height: 0 },
    })
  }
}

function mountedRoots(): { path: string; root: ShadowRoot }[] {
  return (viewer?.getRenderedItems() ?? []).flatMap((item) =>
    item.element.shadowRoot ? [{ path: item.id, root: item.element.shadowRoot }] : [],
  )
}

// A selection in the code is the browser's own, finished on the release. It waits
// a tick so that a click clearing the selection reads as nothing selected.
useEventListener(root, 'mouseup', () => {
  setTimeout(() => {
    const selected = diffTextSelection(document.getSelection(), mountedRoots())
    if (!selected) return
    emit('select-lines', { path: selected.path, range: selected.range, text: selected.text, rect: selected.rect })
  })
})

function selectLines(path: string, range: SelectedLineRange): void {
  viewer?.setSelectedLines({ id: path, range }, { notify: false })
}

function clearSelection(): void {
  viewer?.clearSelectedLines({ notify: false })
}

let paintQueued = false
function queuePaint(): void {
  if (paintQueued) return
  paintQueued = true
  requestAnimationFrame(() => {
    paintQueued = false
    paintSearch(mountedRoots(), props.searchQuery, props.activeMatch)
  })
}

const { scales } = useSurfaceScale()
const metrics = computed(() => diffMetrics(scales.code.value, scales.interface.value))
// Comments follow the code scale, as the rows around them do, so + and - size both.
const commentZoom = computed(() => ({ zoom: metrics.value.fontPx / 13 }))
const rowStyle = computed(() => ({
  '--diffs-font-size': `${metrics.value.fontPx}px`,
  '--diffs-line-height': `${metrics.value.rowPx}px`,
}))

const toasts = useToasts()

// CodeView calls this on the first expansion in a file. A file that changed since
// its diff was read fails here rather than mixing two versions; the next poll
// brings a diff of the new version. CodeView only logs a failure, so the toast
// is what tells the user why the click did nothing.
async function loadDiffFiles(fileDiff: FileDiffMetadata): Promise<FileDiffLoadedFiles> {
  const path = fileDiff.name
  let read: SessionFileDiff
  try {
    read = await props.loadWholeFile(path)
  } catch (err) {
    read = { path, state: 'error', text: '', error: errorText(err, 'the read failed') }
  }
  const whole = read.state === 'text' ? wholeFileFromPatch(read.text) : null
  if (!whole || !hunksMatch(fileDiff, whole)) {
    const reason =
      read.state === 'error'
        ? read.error
        : whole
          ? 'the file changed since its diff was read'
          : `the file reads as ${read.state}`
    log.warn('could not expand the diff context', { path, reason })
    toasts.showToast(`Could not show more of ${gitFileName(path)}: ${reason}`, { severity: 'error' })
    throw new Error(`could not read ${path} in full: ${reason}`)
  }
  const key = fileDiff.cacheKey ?? `${props.scope}:${path}`
  return {
    oldFile: { name: fileDiff.prevName ?? path, contents: whole.old, cacheKey: `${key}:old` },
    newFile: { name: path, contents: whole.new, cacheKey: `${key}:new` },
  }
}

function viewerOptions(): CodeViewOptions<CommentAnnotation, undefined> {
  const { headerPx, rowPx, separatorPx } = metrics.value
  return {
    diffStyle: props.mode,
    // A wrapped line would break the fixed row height everything is laid out from.
    overflow: 'scroll',
    hunkSeparators: 'line-info-basic',
    expansionLineCount: EXPANSION_LINES,
    loadDiffFiles,
    // WKWebView paints a sticky header transparent during a fast scroll, so the
    // header stays pinned from outside the scroller instead.
    stickyHeaders: false,
    layout: { paddingTop: 0, paddingBottom: 0, gap: 0 },
    itemMetrics: {
      lineHeight: rowPx,
      diffHeaderHeight: headerPx,
      hunkSeparatorHeight: separatorPx,
      spacing: 0,
      paddingBottom: SCROLLBAR_PX,
    },
    unsafeCSS: unsafeCss(separatorPx),
    enableLineSelection: true,
    enableGutterUtility: true,
    lineHoverHighlight: 'number',
    onGutterUtilityClick: () => {},
    onLineSelectionEnd,
    renderCustomHeader: (_fileDiff: unknown, context: ItemContext) => headerHost(context.item.id),
    renderAnnotation: (annotation: LineAnnotation<CommentAnnotation>) =>
      commentHost(annotation.metadata.commentId, annotation.metadata.agent),
    onPostRender: (_node: unknown, _instance: unknown, phase: PostRenderPhase, context: ItemContext) => {
      if (phase === 'unmount') setHeaderMounted(context.item.id, false)
      queuePaint()
    },
  }
}

interface BuiltItem {
  entry: DiffEntry | undefined
  open: boolean
  comments: readonly ChangesComment[] | undefined
  agents: readonly AnchoredAgentComment[] | undefined
  item: CodeViewDiffItem<CommentAnnotation>
}
const built = new Map<string, BuiltItem>()

function itemFor(file: ChangedFile): CodeViewDiffItem<CommentAnnotation> {
  const entry = props.entries.get(file.path)
  const open = props.openPaths.has(file.path)
  const comments = props.comments.get(file.path)
  const agents = props.agentComments?.get(file.path)
  const held = built.get(file.path)
  if (held && held.entry === entry && held.open === open && held.comments === comments && held.agents === agents) {
    return held.item
  }
  const fileDiff =
    held && held.entry === entry
      ? held.item.fileDiff
      : entry && diffHasText(entry.diff)
        ? parseFileDiff(file, entry.diff.text, `${props.scope}:${file.path}:${entry.version}`)
        : emptyFileDiff(file)
  const item: CodeViewDiffItem<CommentAnnotation> = {
    id: file.path,
    type: 'diff',
    fileDiff,
    annotations: [
      ...(comments ?? []).map((comment) => ({
        side: comment.endSide,
        lineNumber: comment.endLine,
        metadata: { commentId: comment.id },
      })),
      ...(agents ?? []).map((anchored) => ({
        side: anchored.side,
        lineNumber: anchored.line,
        metadata: { commentId: anchored.comment.id, agent: true },
      })),
    ],
    collapsed: !open || fileDiff.hunks.length === 0,
    version: (held?.item.version ?? 0) + 1,
  }
  built.set(file.path, { entry, open, comments, agents, item })
  return item
}

// A file joins the stream once its diff is read, or at once when it stays folded.
// Growing it on the spot would push every file below it down the screen, while a
// file inserted whole leaves what is on screen alone, as CodeView anchors the view.
const shown = shallowRef<ChangedFile[]>([])
let pendingJump: string | null = null

function syncItems(): void {
  for (const path of built.keys()) if (!fileByPath.value.has(path)) built.delete(path)
  shown.value = props.files.filter(
    (file) => built.has(file.path) || !props.openPaths.has(file.path) || props.entries.has(file.path),
  )
  viewer?.setItems(shown.value.map(itemFor))
  updatePinned()
  if (pendingJump && !fileByPath.value.has(pendingJump)) pendingJump = null
  if (pendingJump && built.has(pendingJump)) {
    const path = pendingJump
    pendingJump = null
    viewer?.scrollTo({ type: 'item', id: path, align: 'start' })
  }
  emit('synced')
}

/** The stream shows this file open with the diff read last, so a line in it can be scrolled to. */
function holds(path: string): boolean {
  const held = built.get(path)
  return !!held && held.open && held.entry === props.entries.get(path)
}

function fileAt(top: number): ChangedFile | undefined {
  const files = shown.value
  let low = 0
  let high = files.length - 1
  let found: ChangedFile | undefined = files[0]
  while (low <= high) {
    const middle = (low + high) >> 1
    const itemTop = viewer?.getTopForItem(files[middle].path) ?? 0
    if (itemTop <= top) {
      found = files[middle]
      low = middle + 1
    } else high = middle - 1
  }
  return found
}

function updatePinned(): void {
  const file = fileAt(scrollTop)
  pinnedPath.value = file?.path ?? null
  const next = file && shown.value[shown.value.indexOf(file) + 1]
  const nextTop = next && viewer?.getTopForItem(next.path)
  pinnedPush.value = nextTop === undefined ? 0 : Math.min(0, nextTop - scrollTop - metrics.value.headerPx)
}

function focus(): void {
  root.value?.focus({ preventScroll: true })
}

function scrollToFile(path: string): void {
  if (built.has(path)) {
    pendingJump = null
    viewer?.scrollTo({ type: 'item', id: path, align: 'start' })
  } else pendingJump = path
}

/** Moves to the next or previous hunk across the whole list. False when there is none. */
function stepHunk(delta: 1 | -1): boolean {
  if (!viewer) return false
  const { mode } = props
  const files = shown.value
  // Under the pinned header, where a hunk lands.
  const line = scrollTop + metrics.value.headerPx
  const from = files.findIndex((file) => file.path === fileAt(line)?.path)
  for (let index = from; index >= 0 && index < files.length; index += delta) {
    const file = files[index]
    if (!props.openPaths.has(file.path)) continue
    const top = viewer.getTopForItem(file.path)
    const hunks = built.get(file.path)?.item.fileDiff.hunks
    if (top === undefined || !hunks) continue
    const tops = hunkOffsets(hunks, mode, metrics.value).map((offset) => top + offset)
    const target = delta > 0 ? tops.find((at) => at > line + 2) : tops.findLast((at) => at < line - 2)
    if (target !== undefined) {
      viewer.scrollTo({ type: 'position', position: target - metrics.value.headerPx })
      // The scroll is reported a frame later, and a key pressed before then must
      // step on from here rather than to this hunk again.
      scrollTop = target - metrics.value.headerPx
      return true
    }
  }
  return false
}

function scrollToLine(path: string, line: number, side: DiffSide): void {
  viewer?.scrollTo({ type: 'line', id: path, lineNumber: line, side, align: 'center' })
}

defineExpose({ focus, scrollToFile, holds, stepHunk, scrollToLine, selectLines, clearSelection })

onMounted(() => {
  if (!root.value) return
  const created = new CodeView<CommentAnnotation>(viewerOptions(), pool)
  viewer = created
  created.setup(root.value)
  created.subscribeToScroll((top) => {
    scrollTop = top
    updatePinned()
    queuePaint()
    const at = fileAt(top + 8)
    if (!pendingJump && at && at.path !== props.current) emit('update:current', at.path)
  })
  syncItems()
})
onBeforeUnmount(() => {
  clearSearchPaint()
  viewer?.cleanUp()
  viewer = null
})

watch(() => [props.files, props.entries, props.openPaths, props.comments, props.agentComments] as const, syncItems, {
  flush: 'post',
})
watch(() => [props.searchQuery, props.activeMatch] as const, queuePaint)
watch(
  () => [props.mode, metrics.value] as const,
  () => viewer?.setOptions(viewerOptions()),
  { flush: 'post' },
)
// Immediate because the pool outlives this view, and the theme can change while no
// stream is open to hear of it. The pool does nothing for the theme it already has.
watch(
  platform.theme,
  (theme) => {
    pool.setRenderOptions({ theme: diffThemeName(theme) }).catch((err: unknown) => {
      log.warn('could not switch the diff theme', { err })
    })
  },
  { immediate: true },
)
</script>

<template>
  <div class="relative flex min-w-0 flex-1 flex-col overflow-hidden">
    <div
      ref="root"
      class="diff-stream hive-scroll relative min-h-0 min-w-0 flex-1 overflow-y-auto bg-app outline-none"
      :style="rowStyle"
      data-scale-surface="code"
      tabindex="0"
      aria-label="Diffs"
      data-testid="changes-stream"
    />
    <div
      v-if="pinnedFile"
      class="absolute top-0 left-0 z-10"
      :style="{ width: `${pinnedWidth}px`, transform: `translateY(${pinnedPush}px)` }"
    >
      <DiffFileHeader
        :file="pinnedFile"
        :entry="entries.get(pinnedFile.path)"
        :open="openPaths.has(pinnedFile.path)"
        :viewed="viewedPaths.has(pinnedFile.path)"
        test-prefix="changes-pinned"
        @fold="emit('fold', pinnedFile!)"
        @toggle-viewed="emit('toggle-viewed', pinnedFile!)"
        @open-editor="emit('open-editor')"
        @comment="emit('comment-file', pinnedFile!, $event)"
      />
    </div>
    <Teleport v-for="path in mountedHeaders" :key="path" :to="headerHosts.get(path)">
      <DiffFileHeader
        v-if="fileByPath.get(path)"
        :file="fileByPath.get(path)!"
        :entry="entries.get(path)"
        :open="openPaths.has(path)"
        :viewed="viewedPaths.has(path)"
        @fold="emit('fold', fileByPath.get(path)!)"
        @toggle-viewed="emit('toggle-viewed', fileByPath.get(path)!)"
        @open-editor="emit('open-editor')"
        @comment="emit('comment-file', fileByPath.get(path)!, $event)"
      />
    </Teleport>
    <Teleport v-for="id in mountedComments" :key="id" :to="commentHosts.get(id)">
      <div :style="commentZoom">
        <DiffCommentCard
          v-if="commentById.get(id)"
          :comment="commentById.get(id)!"
          :outdated="isOutdated(commentById.get(id)!)"
          @update="emit('update-comment', commentById.get(id)!, $event)"
          @remove="emit('remove-comment', commentById.get(id)!)"
          @pick="emit('pick-comment', commentById.get(id)!)"
        />
        <AgentCommentCard
          v-else-if="agentById.get(id)"
          :comment="agentById.get(id)!"
          :stale="isAgentStale?.(agentById.get(id)!)"
          :focused="focusedAgent === id"
          @dismiss="emit('dismiss-agent', agentById.get(id)!)"
          @pick="emit('pick-agent', agentById.get(id)!)"
        />
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.diff-stream {
  --diffs-font-family: var(--font-mono);
  --diffs-header-font-family: var(--font-mono);
  --diffs-font-features: 'liga' 0, 'calt' 0;
  --diffs-gap-block: 0px;
  --diffs-addition-color-override: var(--color-severity-success);
  --diffs-deletion-color-override: var(--color-severity-error);
}
</style>
