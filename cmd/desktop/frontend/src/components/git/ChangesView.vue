<script setup lang="ts">
// Every changed file's diff in one scroll, with a file rail beside it and the
// review comments that go to the session's agent. Nothing here writes to the
// checkout: every read goes through the session API.
import { computed, nextTick, onMounted, ref, shallowRef, toRef, watch } from 'vue'
import { useEventListener, useStorage } from '@vueuse/core'
import IconChevronsDownUp from '~icons/lucide/chevrons-down-up'
import IconChevronsUpDown from '~icons/lucide/chevrons-up-down'
import IconColumns2 from '~icons/lucide/columns-2'
import IconEye from '~icons/lucide/eye'
import IconEyeOff from '~icons/lucide/eye-off'
import IconMessageSquare from '~icons/lucide/message-square'
import IconSearch from '~icons/lucide/search'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconMessageSquareQuote from '~icons/lucide/message-square-quote'
import IconSend from '~icons/lucide/send'
import IconGitBranch from '~icons/lucide/git-branch'
import IconGitPullRequest from '~icons/lucide/git-pull-request'
import IconList from '~icons/lucide/list'
import IconListTree from '~icons/lucide/list-tree'
import IconPanelLeft from '~icons/lucide/panel-left'
import IconRows2 from '~icons/lucide/rows-2'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import type { MenuEntry } from '@hive/ui/types/menu'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import PanelResizeHandle from '@hive/ui/components/ui/PanelResizeHandle.vue'
import ViewHeader from '@hive/ui/components/ui/ViewHeader.vue'
import { useOpenModalCount } from '@hive/ui/composables/useOpenModalCount'
import { useResizablePanel } from '@hive/ui/composables/useResizablePanel'
import ChangesLoading from './ChangesLoading.vue'
import ChangesOverview from './ChangesOverview.vue'
import ChangesReviewPanel from './ChangesReviewPanel.vue'
import ChangesTargetPicker from './ChangesTargetPicker.vue'
import ChangesWalkBar from './ChangesWalkBar.vue'
import DiffSearchBar from './DiffSearchBar.vue'
import DiffStream from './DiffStream.vue'
import GitTree from './GitTree.vue'
import DriveCommentPopover from '../drive/DriveCommentPopover.vue'
import SendToAgentDialog from '../drive/SendToAgentDialog.vue'
import SendNoteDialog from './SendNoteDialog.vue'
import { entryRows, useDiffSearch } from '../../composables/useDiffSearch'
import { useCopyReview } from '../../composables/useCopyReview'
import { formatCombo } from '../../composables/useKeybindings'
import { fileVersion, useFileDiffs } from '../../composables/useFileDiffs'
import { targetKey, useSessionBranches, useSessionChanges, type DiffBase } from '../../composables/useSessionChanges'
import { useToasts } from '../../composables/useToasts'
import {
  agentComments,
  anchorAgentComment,
  isAgentCommentStale,
  agentFeedback,
  walkOrder,
  type AgentComment,
  type AnchoredAgentComment,
} from '../../lib/agentReview'
import { errorText } from '../../lib/appError'
import { createLogger } from '../../lib/log'
import { readOrder, type DiffMode } from '../../lib/diffItems'
import { branchName } from '../../lib/diffTarget'
import { commentLabel, diffSpan, hasDiffLine, type DiffSearchMatch, type DiffSpan } from '../../lib/diffReview'
import type { DiffLineSelection } from '../../lib/diffSelection'
import { gitFileName, gitRows, type GitLayout } from '../../lib/gitTree'
import { isEditableTarget } from '../../lib/isEditableTarget'
import { useAgentReviews } from '../../stores/useAgentReviews'
import { useChangesReview, type ChangesComment, type ChangesCommentKind } from '../../stores/useChangesReview'
import { useChangesViewed } from '../../stores/useChangesViewed'
import {
  OpenSessionInEditor,
  SessionFileContext,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type { ChangedFile } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import type { DiffReviewWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import type { AgentTarget } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

const props = withDefaults(
  defineProps<{
    sessionId: string
    /** A tab is mounted while another shows, and its bare keys must not fire from there. */
    keysActive?: boolean
  }>(),
  { keysActive: true },
)
const openModalCount = useOpenModalCount()

const sessionId = toRef(props, 'sessionId')
const {
  size: railWidth,
  startResize: startRailResize,
  step: stepRail,
} = useResizablePanel({ storageKey: 'hive.panel.changes.files', defaultSize: 280, min: 180, max: 520, edge: 'right' })
const base = useStorage<DiffBase>('hive.git.base', 'branch')
// A commit stays picked only while the view is open: the next opening shows the latest.
const commit = ref('')
// Per session, because a branch one repository has means nothing in another.
const againstBySession = useStorage<Record<string, string>>('hive.git.against', {})
const against = computed({
  get: () => againstBySession.value[props.sessionId] ?? '',
  set: (next: string) => {
    againstBySession.value = { ...againstBySession.value, [props.sessionId]: next }
  },
})
const target = computed(() => ({ base: base.value, against: against.value, commit: commit.value }))
const layout = useStorage<GitLayout>('hive.git.layout', 'tree')
const mode = useStorage<DiffMode>('hive.git.diffMode', 'unified')
// One file at a time: a pick in the rail turns it on, and j and k still walk every file.
const isolate = useStorage('hive.git.isolate', false)

// Held diffs set the scrollbar's length and take memory, so the read takes this
// many open files nearest the one on screen. Scrolling on reads further.
const READ_WINDOW = 200

const { changes, error, pending } = useSessionChanges(sessionId, target)
const branches = useSessionBranches(sessionId)
// The tree lists folders first, so the stream follows the rail's order rather
// than git's: j and k then walk the rail row by row. While a new target's answer
// is on its way, the last target's files would be read against the new one.
const files = computed<ChangedFile[]>(() =>
  pending.value
    ? []
    : gitRows(changes.value?.files ?? [], layout.value, () => true).flatMap((row) =>
        row.kind === 'file' ? [row.file] : [],
      ),
)

const shownBase = computed<DiffBase>(() => (changes.value?.base as DiffBase | undefined) || base.value)
const additions = computed(() => files.value.reduce((sum, file) => sum + file.additions, 0))
const deletions = computed(() => files.value.reduce((sum, file) => sum + file.deletions, 0))
const EMPTY_MESSAGES: Record<Exclude<DiffBase, 'branch'>, string> = {
  uncommitted: 'No uncommitted changes',
  staged: 'Nothing is staged',
  unstaged: 'No unstaged changes',
  commit: 'This commit changes no files',
}
const emptyMessage = computed(() =>
  shownBase.value === 'branch'
    ? `No changes against ${changes.value?.against ? branchName(changes.value.against) : (changes.value?.defaultBranch ?? 'the default branch')}`
    : EMPTY_MESSAGES[shownBase.value],
)

function pickTarget(nextBase: DiffBase, nextCommit: string): void {
  base.value = nextBase
  commit.value = nextCommit
}

// Stepping back from any other comparison starts at the latest commit.
function stepCommit(delta: 1 | -1): void {
  if (shownBase.value !== 'commit') {
    if (delta > 0) pickTarget('commit', '')
    return
  }
  const commits = changes.value?.commits ?? []
  const index = commits.findIndex((candidate) => candidate.id === changes.value?.commit)
  const next = index >= 0 ? commits[index + delta] : undefined
  if (next) commit.value = next.id
}

const toasts = useToasts()
const log = createLogger('changes')
const viewedStore = useChangesViewed()
const isViewed = (file: ChangedFile) => viewedStore.isViewed(props.sessionId, file.path, fileVersion(file))
const viewedPaths = computed(() => new Set(files.value.filter(isViewed).map((file) => file.path)))

// An override wins until the file's viewed mark changes. With none, a viewed
// file is folded, so a later edit that clears the mark opens it again. A file
// shown on its own opens, or picking a viewed one would show only its header.
const foldOverrides = ref(new Map<string, boolean>())
const isOpen = (file: ChangedFile) =>
  foldOverrides.value.get(file.path) ?? ((isolate.value && file.path === current.value) || !isViewed(file))

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
  holds: (path: string) => boolean
  stepHunk: (delta: 1 | -1) => boolean
  scrollToLine: (path: string, line: number, side: DiffSpan['startSide']) => void
  selectLines: (path: string, range: DiffLineSelection['range']) => void
  clearSelection: () => void
} | null>(null)
const scope = computed(() => `${props.sessionId}:${targetKey(target.value)}`)
const current = ref<string | null>(null)
// A picked comment can sit in another comparison or a folded file, whose diff is
// not read yet. CodeView drops a scroll to a line it cannot place, so this waits.
const pendingReveal = shallowRef<ChangesComment | null>(null)
const openPaths = computed(() => new Set(files.value.filter(isOpen).map((file) => file.path)))
const wanted = computed(() => {
  const center = files.value.findIndex((file) => file.path === current.value)
  const open = readOrder(files.value, center).filter((file) => openPaths.value.has(file.path))
  return open.slice(0, READ_WINDOW).map((file) => file.path)
})
const { entries, answered } = useFileDiffs(sessionId, target, files, wanted)
const streamFiles = computed(() =>
  isolate.value ? files.value.filter((file) => file.path === current.value) : files.value,
)
// The file tree and the diffs appear together. Alone, the tree lands first and the
// diffs one read later. A failed read ends the wait, because its error is what to show.
const loading = computed(
  () => !error.value && (pending.value || !changes.value || (openPaths.value.size > 0 && !answered.value)),
)

// Arrow keys and Page Down scroll what has focus.
onMounted(() => stream.value?.focus())
watch(
  () => files.value.map((file) => file.path).join('\n'),
  () => {
    if (!current.value || !files.value.some((file) => file.path === current.value)) {
      current.value = files.value[0]?.path ?? null
    }
  },
)

watch([sessionId, () => targetKey(target.value)], () => {
  foldOverrides.value = new Map()
})
watch(sessionId, () => {
  commit.value = ''
  pendingReveal.value = null
})

function jumpTo(path: string): void {
  overviewOpen.value = false
  pendingReveal.value = null
  current.value = path
  stream.value?.scrollToFile(path)
}

// A second pick of the file shown alone goes back to every file.
function pickFile(path: string): void {
  if (isolate.value && current.value === path && !overviewOpen.value) {
    toggleIsolate()
    return
  }
  isolate.value = true
  jumpTo(path)
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
// The Overview takes the diff's place, as a pull request's page sits above its files.
const overviewOpen = ref(false)
watch(sessionId, () => (overviewOpen.value = false))
const copyReview = useCopyReview()
const comments = computed(() => review.comments(props.sessionId))
// Comments already sit under their lines, so the panel waits to be asked for.
const reviewOpen = ref(false)
const filesOpen = useStorage('hive.panel.changes.files.open', true)
const hideViewed = useStorage('hive.git.hideViewed', false)
const railFiles = computed(() => (hideViewed.value ? files.value.filter((file) => !isViewed(file)) : files.value))
const tree = ref<{ anyFolderOpen: boolean; setAllFolders: (open: boolean) => void } | null>(null)
// Send carries the user's comments and every agent comment they did not dismiss.
const sendCount = computed(() => comments.value.length + pendingAgent.value.length)
const canSend = computed(() => sendCount.value > 0)
const sendLabel = computed(() =>
  sendCount.value === 1 ? 'Send 1 comment to the agent' : `Send ${sendCount.value} comments to the agent`,
)

// Only comments whose line is in the diff on screen get a row. The rest stay in
// the panel: the comparison differs, or the agent has moved the line away.
const shownComments = computed(() => {
  const byPath = new Map<string, ChangesComment[]>()
  for (const comment of comments.value) {
    if (comment.kind || !inShownComparison(comment)) continue
    if (!hasDiffLine(entryRows(entries.value.get(comment.path)), comment.endLine, comment.endSide)) continue
    byPath.set(comment.path, [...(byPath.get(comment.path) ?? []), comment])
  }
  return byPath
})
const commentCounts = computed(() => {
  const counts = new Map<string, number>()
  for (const { path } of comments.value) if (path) counts.set(path, (counts.get(path) ?? 0) + 1)
  return counts
})

function inShownComparison(comment: ChangesComment): boolean {
  if (comment.base !== shownBase.value) return false
  if (comment.base === 'branch') return (comment.against ?? '') === (changes.value?.against ?? '')
  return comment.base !== 'commit' || comment.commit === changes.value?.commit
}

// Stamps belong to a comparison, so only the comment's own can say it moved.
function isOutdated(comment: ChangesComment): boolean {
  if (comment.kind === 'review' || pending.value || !inShownComparison(comment)) return false
  const file = files.value.find((candidate) => candidate.path === comment.path)
  return !file || fileVersion(file) !== comment.version
}

// The comparison and version are the diff's when the selection was made: the view
// can move to another session or comparison, or the file can change, during typing.
interface CommentDraft {
  kind?: ChangesCommentKind
  sessionId: string
  comparison: Pick<ChangesComment, 'base' | 'against' | 'commit' | 'staged' | 'version'>
  path: string
  span: DiffSpan
  rect: DiffLineSelection['rect']
  text: string
}
const draft = ref<CommentDraft | null>(null)

function startComment(selection: DiffLineSelection): void {
  const span = diffSpan(entryRows(entries.value.get(selection.path)), selection.range)
  const file = files.value.find((candidate) => candidate.path === selection.path)
  if (!span || !file) return
  stream.value?.selectLines(selection.path, selection.range)
  draft.value = {
    sessionId: props.sessionId,
    comparison: currentComparison(file),
    path: selection.path,
    span: selection.text ? { ...span, quote: selection.text } : span,
    rect: selection.rect,
    text: draft.value?.text ?? '',
  }
}

// A file or review comment has no rows to stand on.
const NO_LINES: Omit<DiffSpan, 'quote'> = { startSide: 'additions', startLine: 0, endSide: 'additions', endLine: 0 }

function currentComparison(file: ChangedFile | undefined): CommentDraft['comparison'] {
  return {
    base: shownBase.value,
    against: shownBase.value === 'branch' ? changes.value?.against : undefined,
    commit: shownBase.value === 'commit' ? changes.value?.commit : undefined,
    staged: shownBase.value === 'staged' || undefined,
    version: file ? fileVersion(file) : '',
  }
}

function startFileComment(file: ChangedFile, rect: CommentDraft['rect']): void {
  draft.value = {
    kind: 'file',
    sessionId: props.sessionId,
    comparison: currentComparison(file),
    path: file.path,
    span: { ...NO_LINES, quote: '' },
    rect,
    text: '',
  }
}

const draftRange = computed(() => {
  const held = draft.value
  if (!held) return null
  if (held.kind) {
    return { startLine: 0, endLine: 0, context: held.kind === 'file' ? held.path : 'All the changes' }
  }
  return {
    startLine: held.span.startLine,
    endLine: held.span.endLine,
    context: held.span.quote
      .split('\n')[0]
      .replace(/^[-+ ]/, '')
      .trim(),
  }
})

const draftLabel = computed(() => {
  const held = draft.value
  if (!held) return ''
  const label = commentLabel({ ...held.span, kind: held.kind })
  return held.path ? `${gitFileName(held.path)} ${label}` : label
})

function endDraft(): void {
  draft.value = null
  stream.value?.clearSelection()
  document.getSelection()?.removeAllRanges()
}

function saveDraft(text: string): void {
  const held = draft.value
  if (!held) return
  review.add(held.sessionId, {
    ...held.span,
    ...held.comparison,
    ...(held.kind ? { kind: held.kind } : {}),
    path: held.path,
    text: text.trim(),
  })
  endDraft()
}

function pickComment(comment: ChangesComment): void {
  if (comment.kind === 'review') {
    overviewOpen.value = true
    return
  }
  overviewOpen.value = false
  if (!inShownComparison(comment)) {
    base.value = comment.base
    if (comment.base === 'branch') against.value = comment.against ?? ''
    commit.value = comment.commit ?? ''
  }
  pendingReveal.value = comment
}

function settleReveal(): void {
  const comment = pendingReveal.value
  if (!comment || pending.value) return
  if (!changes.value) {
    if (error.value) pendingReveal.value = null
    return
  }
  const file = files.value.find((candidate) => candidate.path === comment.path)
  // The panel keeps a comment whose file has left this comparison, or whose
  // comparison no longer resolves, and there is nowhere to take the user.
  if (!file || !inShownComparison(comment)) {
    pendingReveal.value = null
    return
  }
  current.value = file.path
  if (!isOpen(file)) {
    foldOverrides.value.set(file.path, true)
    return
  }
  const entry = entries.value.get(file.path)
  if (!entry || !stream.value?.holds(file.path)) return
  pendingReveal.value = null
  if (comment.kind || !hasDiffLine(entryRows(entry), comment.endLine, comment.endSide)) {
    stream.value.scrollToFile(file.path)
    return
  }
  stream.value.scrollToLine(file.path, comment.endLine, comment.endSide)
  stream.value.selectLines(file.path, {
    start: comment.startLine,
    side: comment.startSide,
    end: comment.endLine,
    endSide: comment.endSide,
  })
}
watch([pendingReveal, pending, files, entries, openPaths], settleReveal, { flush: 'post' })

// Agents' reviews of the branch diff, from hive.db. Their rows are numbered in
// the branch comparison against the default base, so they are drawn only there.
const agentStore = useAgentReviews()
watch(sessionId, (id, _previous, onCleanup) => onCleanup(agentStore.watch(id)), { immediate: true })
const agentReviews = computed<readonly DiffReviewWire[]>(() => agentStore.reviews(props.sessionId))
const inAgentComparison = computed(() => shownBase.value === 'branch' && !changes.value?.against)
const pendingAgent = computed(() =>
  walkOrder(
    agentComments(agentReviews.value),
    files.value.map((file) => file.path),
  ),
)
const shownAgentComments = computed(() => {
  const byPath = new Map<string, AnchoredAgentComment[]>()
  if (!inAgentComparison.value) return byPath
  for (const comment of pendingAgent.value) {
    const anchored = anchorAgentComment(comment, entryRows(entries.value.get(comment.path)))
    if (anchored) byPath.set(comment.path, [...(byPath.get(comment.path) ?? []), anchored])
  }
  return byPath
})

function isAgentStale(comment: AgentComment): boolean {
  const entry = entries.value.get(comment.path)
  return inAgentComparison.value && !!entry && isAgentCommentStale(comment, entryRows(entry))
}

const walkId = ref<string | null>(null)
const walking = computed(() => walkId.value !== null)
const walkIndex = computed(() => pendingAgent.value.findIndex((comment) => comment.id === walkId.value))
const walkComment = computed(() => pendingAgent.value[walkIndex.value] ?? null)
// The comment a jump is waiting to scroll to, until its file's diff is read.
const pendingAgentReveal = shallowRef<AgentComment | null>(null)

watch(sessionId, () => stopWalk())
// Another process can dismiss the comment the walkthrough is on; it then ends rather than jump.
watch(walkComment, (comment) => {
  if (walkId.value && !comment) stopWalk()
})

function showAgentComparison(): void {
  if (inAgentComparison.value) return
  base.value = 'branch'
  against.value = ''
  commit.value = ''
}

function focusAgent(comment: AgentComment): void {
  overviewOpen.value = false
  walkId.value = comment.id
  showAgentComparison()
  pendingAgentReveal.value = comment
}

function settleAgentReveal(): void {
  const comment = pendingAgentReveal.value
  if (!comment || pending.value || !changes.value || !inAgentComparison.value) return
  const file = files.value.find((candidate) => candidate.path === comment.path)
  if (!file) {
    pendingAgentReveal.value = null
    return
  }
  current.value = file.path
  if (!isOpen(file)) {
    foldOverrides.value.set(file.path, true)
    return
  }
  const entry = entries.value.get(file.path)
  if (!entry || !stream.value?.holds(file.path)) return
  pendingAgentReveal.value = null
  const anchored = anchorAgentComment(comment, entryRows(entry))
  if (!anchored) {
    stream.value.scrollToFile(file.path)
    return
  }
  stream.value.scrollToLine(file.path, anchored.line, anchored.side)
}
watch([pendingAgentReveal, pending, files, entries, openPaths], settleAgentReveal, { flush: 'post' })

function startWalk(): void {
  const first = pendingAgent.value[0]
  if (first) focusAgent(first)
}

function stopWalk(): void {
  walkId.value = null
  pendingAgentReveal.value = null
}

function stepWalk(delta: 1 | -1): void {
  const list = pendingAgent.value
  if (!list.length) return
  const from = walkIndex.value < 0 ? 0 : walkIndex.value
  focusAgent(list[(from + delta + list.length) % list.length])
}

// The walkthrough moves to the next comment before the store drops this one, so the
// position holds rather than jumping back to the start.
function afterRemoving(comment: AgentComment): void {
  if (walkId.value !== comment.id) return
  const list = pendingAgent.value
  const index = list.findIndex((candidate) => candidate.id === comment.id)
  const next = list[index + 1] ?? list[index - 1]
  if (next && next.id !== comment.id) focusAgent(next)
  else stopWalk()
}

async function dismissAgent(comment: AgentComment): Promise<void> {
  afterRemoving(comment)
  const failure = await agentStore.dismiss(props.sessionId, comment.id)
  if (failure) toasts.showToast(failure, { severity: 'error' })
}

async function dismissAgentReview(agentReview: DiffReviewWire): Promise<void> {
  if (walkComment.value?.reviewId === agentReview.id) stopWalk()
  const failure = await agentStore.dismissReview(props.sessionId, agentReview.id)
  if (failure) toasts.showToast(failure, { severity: 'error' })
}

// Returns whether the key was the walkthrough's. n and p walk the comments while it
// is on, in place of the hunk keys.
function walkKey(event: KeyboardEvent): boolean {
  if (!walking.value) {
    if (event.key !== 'w' || !pendingAgent.value.length) return false
    startWalk()
    return true
  }
  const comment = walkComment.value
  if (!comment) return false
  switch (event.key) {
    case 'd':
      void dismissAgent(comment)
      return true
    case 'n':
      stepWalk(1)
      return true
    case 'p':
      stepWalk(-1)
      return true
    case 'Escape':
      stopWalk()
      return true
  }
  return false
}

const SEND_KEY = formatCombo('mod+enter')
const picking = ref(false)
const sending = ref(false)
const loadTargets = () => review.targets(props.sessionId)
// The comments stay after a failure. A toast that waits offers them as text too,
// for when the agent cannot take them any time soon.
function offerCopy(message: string): void {
  toasts.showToast(message, {
    severity: 'error',
    duration: 0,
    actions: [
      {
        label: 'Copy comments',
        onClick: () =>
          void copyReview([
            ...comments.value,
            ...pendingAgent.value.map((comment) => ({ ...agentFeedback(comment), id: comment.id })),
          ]),
      },
    ],
  })
}

// A note leads the review as a comment on all the changes. It is never stored:
// it exists for this one send.
function noteComment(note: string): Omit<ChangesComment, 'id'> {
  return { ...NO_LINES, kind: 'review', path: '', quote: '', base: shownBase.value, version: '', text: note }
}

async function deliver(target: AgentTarget): Promise<string | null> {
  const agents = [...pendingAgent.value]
  const note = sendNote.value.trim()
  const extra = [...(note ? [noteComment(note)] : []), ...agents.map(agentFeedback)]
  const failure = await review.send(props.sessionId, target.pane, extra)
  if (failure) {
    offerCopy(failure)
    return failure
  }
  stopWalk()
  sendNote.value = ''
  // The agent has them now, so their rows go, as the user's own comments do.
  for (const comment of agents) {
    const dropped = await agentStore.dismiss(props.sessionId, comment.id)
    if (dropped) log.warn('a sent agent comment stayed in hive.db', { id: comment.id, reason: dropped })
  }
  return null
}

// The view belongs to one session, so its only agent needs no picking. Go refuses
// one on a prompt, which arrives here as the failure. Only several agents need the dialog.
const sendNote = ref('')
const sendMenuOpen = ref(false)
const noteOpen = ref(false)
const sendMenu = ref<HTMLElement | null>(null)
const sendEntries = computed<MenuEntry[]>(() => [
  { kind: 'action', id: 'send', label: 'Send', icon: IconSend, kbd: SEND_KEY, testid: 'changes-send-now' },
  {
    kind: 'action',
    id: 'note',
    label: 'Send with a note…',
    icon: IconMessageSquareQuote,
    kbd: 'C',
    testid: 'changes-send-with-note',
  },
])

function pickSend(id: string): void {
  sendMenuOpen.value = false
  if (id === 'note') noteOpen.value = true
  else void sendReview()
}

function sendWithNote(note: string): void {
  noteOpen.value = false
  sendNote.value = note
  void sendReview()
}

async function sendReview(): Promise<void> {
  if (!canSend.value || sending.value) return
  sending.value = true
  try {
    const targets = await loadTargets()
    if (targets.length > 1) picking.value = true
    else if (!targets.length) offerCopy('No agent is running in this session.')
    else if (!(await deliver(targets[0]))) {
      reviewOpen.value = false
      toasts.showToast(`Sent to ${targets[0].label}`)
    }
  } catch (err) {
    offerCopy(errorText(err, 'Could not list the agents in this session.'))
  } finally {
    sending.value = false
    if (!picking.value) sendNote.value = ''
  }
}

// On the window, because a click on a file header moves focus to the panel,
// which is outside this view's root.
useEventListener(window, 'keydown', (event: KeyboardEvent) => {
  if (openModalCount.value > 0 || !props.keysActive || event.defaultPrevented) return
  if ((event.metaKey || event.ctrlKey) && !event.altKey) {
    if (event.key === 'f') {
      event.preventDefault()
      openSearch()
    } else if (event.key === 'Enter' && !isEditableTarget(event.target)) {
      // A text field's own send key saves what is being typed. Its handler has already cleared
      // the draft by the time the key reaches here, so only the target can tell.
      event.preventDefault()
      void sendReview()
    }
    return
  }
  if (isEditableTarget(event.target) || event.ctrlKey || event.altKey) return
  if (walkKey(event)) {
    event.preventDefault()
    return
  }
  if (event.key === 'Escape' && overviewOpen.value) {
    overviewOpen.value = false
    event.preventDefault()
    return
  }
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
    case 'f':
      toggleIsolate()
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
    case 'c':
      if (file) startFileComment(file, { top: 80, left: window.innerWidth / 2, width: 0, height: 0 })
      break
    case 'C':
      if (canSend.value) noteOpen.value = true
      break
    case 'b':
      filesOpen.value = !filesOpen.value
      break
    case '[':
      stepCommit(1)
      break
    case ']':
      stepCommit(-1)
      break
    default:
      return
  }
  event.preventDefault()
})

function toggleIsolate(): void {
  isolate.value = !isolate.value
  if (current.value) stream.value?.scrollToFile(current.value)
}

function openInEditor(): void {
  OpenSessionInEditor(props.sessionId).catch((err: unknown) =>
    toasts.showToast(errorText(err, 'Could not open the editor.'), { severity: 'error' }),
  )
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-1 flex-col" data-testid="changes-view">
    <ViewHeader class="overflow-x-auto">
      <template #title>
        <IconButton
          :label="filesOpen ? 'Hide files' : 'Show files'"
          :tooltip="`${filesOpen ? 'Hide' : 'Show'} files (b)`"
          :icon="IconPanelLeft"
          size="lg"
          :aria-pressed="filesOpen"
          data-testid="changes-files-toggle"
          @click="filesOpen = !filesOpen"
        />
        <span class="flex min-w-0 items-center gap-1 font-mono text-caption text-text-2">
          <IconGitBranch class="size-3 shrink-0 text-text-4" aria-hidden="true" />
          <span class="truncate" data-testid="changes-branch">{{ changes?.branch }}</span>
        </span>
        <div class="min-w-0 flex-1" />
        <ChangesTargetPicker
          :changes="changes"
          :branches="branches"
          :base="shownBase"
          :commit="commit"
          @pick-target="pickTarget"
          @pick-against="against = $event"
          @step-commit="stepCommit"
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
        <span
          v-if="canSend"
          ref="sendMenu"
          class="flex h-7 shrink-0 items-center overflow-hidden rounded-lg bg-accent-tint text-accent"
        >
          <button
            type="button"
            class="flex h-full cursor-pointer items-center gap-1.5 pr-2 pl-2 font-mono text-caption font-semibold hover:bg-accent/15 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-accent"
            :title="`${sendLabel} (${SEND_KEY})`"
            :aria-label="sendLabel"
            :aria-busy="sending || undefined"
            :disabled="sending"
            data-testid="changes-send"
            @click="sendReview"
          >
            <IconSend class="size-3.5" aria-hidden="true" />
            <span data-testid="changes-send-count">{{ sendCount }}</span>
          </button>
          <span class="h-4 w-px bg-accent/30" aria-hidden="true" />
          <button
            type="button"
            class="flex h-full w-6 cursor-pointer items-center justify-center hover:bg-accent/15 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-accent"
            title="More ways to send"
            aria-label="More ways to send"
            aria-haspopup="menu"
            :aria-expanded="sendMenuOpen"
            data-testid="changes-send-menu"
            @click="sendMenuOpen = !sendMenuOpen"
          >
            <IconChevronDown class="size-3.5 transition-transform" :class="sendMenuOpen ? 'rotate-180' : ''" />
          </button>
          <AppMenu
            v-if="sendMenuOpen"
            :entries="sendEntries"
            :anchor="sendMenu"
            :anchored-width="224"
            :ignore="[sendMenu]"
            testid="changes-send-options"
            @select="pickSend"
            @close="sendMenuOpen = false"
          />
        </span>
      </template>
    </ViewHeader>

    <InlineError v-if="error" class="mx-3 mt-2 shrink-0" :message="error" testid="changes-error" />

    <div class="relative flex min-h-0 flex-1">
      <ChangesLoading v-if="loading" class="absolute inset-0 z-20" />
      <aside
        v-if="filesOpen"
        class="relative flex shrink-0 flex-col border-r border-border"
        :style="{ width: `${railWidth}px` }"
        data-testid="changes-files"
      >
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
            v-if="layout === 'tree'"
            size="sm"
            :label="tree?.anyFolderOpen ? 'Collapse all folders' : 'Expand all folders'"
            :icon="tree?.anyFolderOpen ? IconChevronsDownUp : IconChevronsUpDown"
            data-testid="changes-folders-toggle"
            @click="tree?.setAllFolders(!tree.anyFolderOpen)"
          />
          <IconButton
            size="sm"
            :label="hideViewed ? 'Show viewed files' : 'Hide viewed files'"
            :icon="hideViewed ? IconEyeOff : IconEye"
            :aria-pressed="hideViewed"
            data-testid="changes-hide-viewed"
            @click="hideViewed = !hideViewed"
          />
          <IconButton
            size="sm"
            :label="layout === 'tree' ? 'Show as list' : 'Show as tree'"
            :icon="layout === 'tree' ? IconList : IconListTree"
            data-testid="changes-layout"
            @click="layout = layout === 'tree' ? 'flat' : 'tree'"
          />
        </div>
        <button
          type="button"
          class="flex h-7 shrink-0 cursor-pointer items-center gap-2 border-b border-border px-2.5 text-left text-small"
          :class="overviewOpen ? 'bg-accent-tint text-text' : 'text-text-2 hover:bg-chip'"
          :aria-pressed="overviewOpen"
          data-testid="changes-overview-row"
          @click="overviewOpen = !overviewOpen"
        >
          <IconGitPullRequest class="size-3.5 shrink-0 text-text-3" aria-hidden="true" />
          <span class="min-w-0 flex-1 truncate">Overview</span>
          <span
            v-if="pendingAgent.length"
            class="shrink-0 rounded-full bg-chip px-1.5 font-mono text-micro text-text-2"
            title="Agent comments waiting"
            data-testid="changes-agent-count"
            >{{ pendingAgent.length }}</span
          >
        </button>
        <div class="hive-scroll min-h-0 flex-1 overflow-y-auto">
          <GitTree
            ref="tree"
            :files="railFiles"
            :layout="layout"
            :selected="overviewOpen ? null : current"
            :viewed="viewedPaths"
            :comment-counts="commentCounts"
            @pick="pickFile"
            @open-editor="openInEditor"
          />
          <p v-if="changes?.truncated" class="px-3 pb-3 text-xs text-text-3" data-testid="changes-truncated">
            Showing the first {{ files.length }} files. Open the session in an editor to see the rest.
          </p>
        </div>
      </aside>
      <div class="relative flex min-w-0 flex-1 flex-col">
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
        <div v-if="changes && !pending && !files.length" class="min-h-0 min-w-0 flex-1 bg-app">
          <EmptyState :message="emptyMessage" data-testid="changes-empty" />
        </div>
        <DiffStream
          v-else
          ref="stream"
          :key="scope"
          v-model:current="current"
          class="min-h-0"
          :scope="scope"
          :files="streamFiles"
          :entries="entries"
          :open-paths="openPaths"
          :viewed-paths="viewedPaths"
          :mode="mode"
          :comments="shownComments"
          :is-outdated="isOutdated"
          :search-query="search.debounced.value"
          :active-match="search.active.value"
          :load-whole-file="(path: string) => SessionFileContext(sessionId, target, path)"
          @synced="settleReveal"
          @fold="toggleFold"
          @toggle-viewed="toggleViewed"
          @open-editor="openInEditor"
          @comment-file="(file, rect) => startFileComment(file, rect)"
          @select-lines="startComment"
          @update-comment="(comment, text) => review.update(sessionId, comment.id, text)"
          @remove-comment="(comment) => review.remove(sessionId, comment.id)"
          @pick-comment="pickComment"
          :agent-comments="shownAgentComments"
          :is-agent-stale="isAgentStale"
          :focused-agent="walkId"
          :walk-keys="walking"
          @dismiss-agent="(comment) => void dismissAgent(comment)"
          @pick-agent="focusAgent"
        />
        <ChangesOverview
          v-if="overviewOpen"
          class="absolute inset-0 z-30"
          :reviews="agentReviews"
          :comments="pendingAgent"
          :hidden="!inAgentComparison"
          :is-stale="isAgentStale"
          :branch="changes?.branch ?? ''"
          :base="changes?.against ? branchName(changes.against) : (changes?.defaultBranch ?? 'the default branch')"
          :files="files.length"
          :additions="additions"
          :deletions="deletions"
          @pick="focusAgent"
          @walk="startWalk"
          @dismiss-all="(agentReview) => void dismissAgentReview(agentReview)"
        />
        <div
          v-if="walkComment && inAgentComparison"
          class="pointer-events-none absolute inset-x-0 bottom-3 z-20 flex justify-center"
        >
          <ChangesWalkBar
            :comment="walkComment"
            :index="walkIndex"
            :total="pendingAgent.length"
            @prev="stepWalk(-1)"
            @next="stepWalk(1)"
            @dismiss="void dismissAgent(walkComment)"
            @stop="stopWalk"
          />
        </div>
      </div>
      <ChangesReviewPanel v-if="reviewOpen" :session-id="sessionId" :is-outdated="isOutdated" @pick="pickComment" />
    </div>

    <DriveCommentPopover
      v-if="draft && draftRange"
      v-model:text="draft.text"
      :rect="draft.rect"
      :range="draftRange"
      :label="draftLabel"
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
      @close="((picking = false), (sendNote = ''))"
    />
    <SendNoteDialog
      v-if="noteOpen"
      :count="sendCount"
      :send-key="SEND_KEY"
      @send="sendWithNote"
      @close="noteOpen = false"
    />
  </div>
</template>
