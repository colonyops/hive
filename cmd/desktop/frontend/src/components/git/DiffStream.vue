<script setup lang="ts">
// Every changed file's diff in one scroll. CodeView owns the scroller and keeps
// only the lines near the viewport in the DOM (ADR the-changes-view-renders-through-pierre-diffs).
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useEventListener } from '@vueuse/core'
import {
  CodeView,
  type CodeViewDiffItem,
  type CodeViewOptions,
  type LineAnnotation,
  type PostRenderPhase,
  type SelectedLineRange,
} from '@pierre/diffs'
import { useHiveUiPlatform } from '@hive/ui/platform'
import DiffCommentCard from './DiffCommentCard.vue'
import DiffFileHeader from './DiffFileHeader.vue'
import type { DiffEntry } from '../../composables/useFileDiffs'
import { diffThemeName, diffWorkerPool } from '../../lib/diffHighlight'
import {
  DIFF_METRICS,
  diffHasText,
  emptyFileDiff,
  hunkOffsets,
  parseFileDiff,
  type DiffMode,
} from '../../lib/diffItems'
import type { DiffSearchMatch, DiffSide } from '../../lib/diffReview'
import { clearSearchPaint, paintSearch, SEARCH_HIGHLIGHT_CSS } from '../../lib/diffSearchPaint'
import { diffTextSelection, type DiffLineSelection } from '../../lib/diffSelection'
import type { ChangesComment } from '../../stores/useChangesReview'
import type { ChangedFile } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  /** Which read this stream shows: a session and a base. Held diffs are cached under it. */
  scope: string
  files: ChangedFile[]
  entries: ReadonlyMap<string, DiffEntry>
  openPaths: ReadonlySet<string>
  viewedPaths: ReadonlySet<string>
  current: string | null
  mode: DiffMode
  /** The comments to draw under their lines, by file. */
  comments: ReadonlyMap<string, readonly ChangesComment[]>
  isOutdated: (comment: ChangesComment) => boolean
  searchQuery: string
  activeMatch: DiffSearchMatch | null
}>()
const emit = defineEmits<{
  'update:current': [path: string]
  fold: [file: ChangedFile]
  'toggle-viewed': [file: ChangedFile]
  'open-editor': []
  /** The user selected lines to comment on. `text` is set when they selected code rather than line numbers. */
  'select-lines': [selection: DiffLineSelection]
  'update-comment': [comment: ChangesComment, text: string]
  'remove-comment': [comment: ChangesComment]
  'pick-comment': [comment: ChangesComment]
}>()

// The separator's height is the only row the library's own stylesheet sizes
// past what its variables reach. No background of its own: the app's shows through.
//
// WebKit scrolls composited layers off the main thread, and the code's horizontal
// scrollers are such layers. A header sharing a layer with what lies beneath it
// lets them paint over it mid-scroll, though its z-index says otherwise, so it
// gets a layer of its own.
const UNSAFE_CSS = `
  :host { --diffs-bg: var(--color-app); }
  [data-diffs-header][data-sticky] { will-change: transform; }
  [data-separator=metadata] { height: ${DIFF_METRICS.separatorPx}px; }
  ${SEARCH_HIGHLIGHT_CSS}
`

// A saved comment's row, found by id across CodeView remounting it.
interface CommentAnnotation {
  commentId: string
}

const root = ref<HTMLElement | null>(null)
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

function commentHost(id: string): HTMLElement {
  let host = commentHosts.get(id)
  if (!host) {
    host = document.createElement('div')
    host.className = 'p-2'
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

function viewerOptions(): CodeViewOptions<CommentAnnotation, undefined> {
  const { headerPx, rowPx, separatorPx } = DIFF_METRICS
  return {
    diffStyle: props.mode,
    // A wrapped line would break the fixed row height everything is laid out from.
    overflow: 'scroll',
    hunkSeparators: 'metadata',
    stickyHeaders: true,
    layout: { paddingTop: 0, paddingBottom: 0, gap: 0 },
    itemMetrics: { lineHeight: rowPx, diffHeaderHeight: headerPx, hunkSeparatorHeight: separatorPx, spacing: 0 },
    unsafeCSS: UNSAFE_CSS,
    enableLineSelection: true,
    enableGutterUtility: true,
    lineHoverHighlight: 'number',
    onGutterUtilityClick: () => {},
    onLineSelectionEnd,
    renderCustomHeader: (_fileDiff: unknown, context: ItemContext) => headerHost(context.item.id),
    renderAnnotation: (annotation: LineAnnotation<CommentAnnotation>) => commentHost(annotation.metadata.commentId),
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
  item: CodeViewDiffItem<CommentAnnotation>
}
const built = new Map<string, BuiltItem>()

function itemFor(file: ChangedFile): CodeViewDiffItem<CommentAnnotation> {
  const entry = props.entries.get(file.path)
  const open = props.openPaths.has(file.path)
  const comments = props.comments.get(file.path)
  const held = built.get(file.path)
  if (held && held.entry === entry && held.open === open && held.comments === comments) return held.item
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
    annotations: comments?.map((comment) => ({
      side: comment.endSide,
      lineNumber: comment.endLine,
      metadata: { commentId: comment.id },
    })),
    collapsed: !open || fileDiff.hunks.length === 0,
    version: (held?.item.version ?? 0) + 1,
  }
  built.set(file.path, { entry, open, comments, item })
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
  if (pendingJump && built.has(pendingJump)) {
    const path = pendingJump
    pendingJump = null
    viewer?.scrollTo({ type: 'item', id: path, align: 'start' })
  }
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

function focus(): void {
  root.value?.focus({ preventScroll: true })
}

function scrollToFile(path: string): void {
  if (built.has(path)) viewer?.scrollTo({ type: 'item', id: path, align: 'start' })
  else pendingJump = path
}

/** Moves to the next or previous hunk across the whole list. False when there is none. */
function stepHunk(delta: 1 | -1): boolean {
  if (!viewer) return false
  const { mode } = props
  const files = shown.value
  // Under the sticky header, where a hunk lands.
  const line = scrollTop + DIFF_METRICS.headerPx
  const from = files.findIndex((file) => file.path === fileAt(line)?.path)
  for (let index = from; index >= 0 && index < files.length; index += delta) {
    const file = files[index]
    if (!props.openPaths.has(file.path)) continue
    const top = viewer.getTopForItem(file.path)
    const hunks = built.get(file.path)?.item.fileDiff.hunks
    if (top === undefined || !hunks) continue
    const tops = hunkOffsets(hunks, mode).map((offset) => top + offset)
    const target = delta > 0 ? tops.find((at) => at > line + 2) : tops.findLast((at) => at < line - 2)
    if (target !== undefined) {
      viewer.scrollTo({ type: 'position', position: target - DIFF_METRICS.headerPx })
      return true
    }
  }
  return false
}

function scrollToLine(path: string, line: number, side: DiffSide): void {
  viewer?.scrollTo({ type: 'line', id: path, lineNumber: line, side, align: 'center' })
}

defineExpose({ focus, scrollToFile, stepHunk, scrollToLine, selectLines, clearSelection })

onMounted(() => {
  if (!root.value) return
  const created = new CodeView<CommentAnnotation>(viewerOptions(), pool)
  viewer = created
  created.setup(root.value)
  created.subscribeToScroll((top) => {
    scrollTop = top
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

watch(() => [props.files, props.entries, props.openPaths, props.comments] as const, syncItems, { flush: 'post' })
watch(() => [props.searchQuery, props.activeMatch] as const, queuePaint)
watch(
  () => props.mode,
  () => viewer?.setOptions(viewerOptions()),
  { flush: 'post' },
)
watch(platform.theme, (theme) => {
  pool.setRenderOptions({ theme: diffThemeName(theme) }).catch((err: unknown) => {
    log.warn('could not switch the diff theme', { err })
  })
})
</script>

<template>
  <div
    ref="root"
    class="diff-stream hive-scroll relative min-w-0 flex-1 overflow-y-auto bg-app outline-none"
    tabindex="0"
    aria-label="Diffs"
    data-testid="changes-stream"
  >
    <Teleport v-for="path in mountedHeaders" :key="path" :to="headerHosts.get(path)">
      <DiffFileHeader
        v-if="fileByPath.get(path)"
        :file="fileByPath.get(path)!"
        :entry="entries.get(path)"
        :open="openPaths.has(path)"
        :viewed="viewedPaths.has(path)"
        :current="current === path"
        @fold="emit('fold', fileByPath.get(path)!)"
        @toggle-viewed="emit('toggle-viewed', fileByPath.get(path)!)"
        @open-editor="emit('open-editor')"
      />
    </Teleport>
    <Teleport v-for="id in mountedComments" :key="id" :to="commentHosts.get(id)">
      <DiffCommentCard
        v-if="commentById.get(id)"
        :comment="commentById.get(id)!"
        :outdated="isOutdated(commentById.get(id)!)"
        @update="emit('update-comment', commentById.get(id)!, $event)"
        @remove="emit('remove-comment', commentById.get(id)!)"
        @pick="emit('pick-comment', commentById.get(id)!)"
      />
    </Teleport>
  </div>
</template>

<style scoped>
.diff-stream {
  --diffs-font-family: var(--font-mono);
  --diffs-header-font-family: var(--font-mono);
  --diffs-font-size: 13px;
  --diffs-line-height: 20px;
  --diffs-font-features: 'liga' 0, 'calt' 0;
  --diffs-gap-block: 0px;
  --diffs-addition-color-override: var(--color-severity-success);
  --diffs-deletion-color-override: var(--color-severity-error);
}
</style>
