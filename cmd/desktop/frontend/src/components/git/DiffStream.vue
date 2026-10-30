<script setup lang="ts">
// Every changed file's diff in one scroll. CodeView owns the scroller and keeps
// only the lines near the viewport in the DOM (ADR the-changes-view-renders-through-pierre-diffs).
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { CodeView, type CodeViewDiffItem, type CodeViewOptions, type PostRenderPhase } from '@pierre/diffs'
import { useHiveUiPlatform } from '@hive/ui/platform'
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
}>()
const emit = defineEmits<{
  'update:current': [path: string]
  fold: [file: ChangedFile]
  'toggle-viewed': [file: ChangedFile]
  'open-editor': []
}>()

// The separator's height is the only row the library's own stylesheet sizes
// past what its variables reach. No background of its own: the app's shows through.
const UNSAFE_CSS = `
  :host { --diffs-bg: var(--color-app); }
  [data-separator=metadata] { height: ${DIFF_METRICS.separatorPx}px; }
`

const root = ref<HTMLElement | null>(null)
const platform = useHiveUiPlatform()
const log = platform.log('changes')
const pool = diffWorkerPool(platform.theme.value, log)
let viewer: CodeView | null = null
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

function viewerOptions(): CodeViewOptions<undefined, undefined> {
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
    renderCustomHeader: (_fileDiff: unknown, context: ItemContext) => headerHost(context.item.id),
    onPostRender: (_node: unknown, _instance: unknown, phase: PostRenderPhase, context: ItemContext) => {
      if (phase === 'unmount') setHeaderMounted(context.item.id, false)
    },
  }
}

interface BuiltItem {
  entry: DiffEntry | undefined
  open: boolean
  item: CodeViewDiffItem<undefined>
}
const built = new Map<string, BuiltItem>()

function itemFor(file: ChangedFile): CodeViewDiffItem<undefined> {
  const entry = props.entries.get(file.path)
  const open = props.openPaths.has(file.path)
  const held = built.get(file.path)
  if (held && held.entry === entry && held.open === open) return held.item
  const fileDiff =
    held && held.entry === entry
      ? held.item.fileDiff
      : entry && diffHasText(entry.diff)
        ? parseFileDiff(file, entry.diff.text, `${props.scope}:${file.path}:${entry.version}`)
        : emptyFileDiff(file)
  const item: CodeViewDiffItem<undefined> = {
    id: file.path,
    type: 'diff',
    fileDiff,
    collapsed: !open || fileDiff.hunks.length === 0,
    version: (held?.item.version ?? 0) + 1,
  }
  built.set(file.path, { entry, open, item })
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

defineExpose({ focus, scrollToFile, stepHunk })

onMounted(() => {
  if (!root.value) return
  viewer = new CodeView(viewerOptions(), pool)
  viewer.setup(root.value)
  viewer.subscribeToScroll((top) => {
    scrollTop = top
    const at = fileAt(top + 8)
    if (!pendingJump && at && at.path !== props.current) emit('update:current', at.path)
  })
  syncItems()
})
onBeforeUnmount(() => {
  viewer?.cleanUp()
  viewer = null
})

watch(() => [props.files, props.entries, props.openPaths] as const, syncItems, { flush: 'post' })
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
