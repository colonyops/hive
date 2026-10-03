import { computed, nextTick, onScopeDispose, ref, watch } from 'vue'
import { useAgentSchedules } from './useAgentSchedules'
import { buildCron, defaultShape, parseCron, type ScheduleShape } from '../lib/scheduleShape'
import { seedRef } from '../lib/seedRef'
import type { AgentSchedule, AgentScheduleRun, ScheduleEdit } from '../lib/agentWorkspacesClient'

// A schedule is a manifest key like the workspace's other lists, so the
// editor's Save writes the whole list and the Go side reconciles `schedules:`
// to it. One schedule at a time is edited on a page that takes the sheet over,
// working on a draft that keep() copies back to its card and close() drops.
// Only Run now, the preview and the run history reach the network; everything
// else is form state until Save.

/** Long enough that a keystroke does not cost a round trip, short enough to feel live. */
const PREVIEW_DEBOUNCE_MS = 300
const PREVIEW_SHOWN = 3
export const QUARTER_MINUTES = [0, 15, 30, 45]

export interface ScheduleCard {
  /** Identity for v-for and the draft, stable while the id still follows the name. */
  key: string
  /** Fixed once saved: the run history and the scheduler's cursor are keyed by it. */
  id: string
  name: string
  shape: ScheduleShape
  prompt: string
  disabled: boolean
  onMissed: AgentSchedule['onMissed']
  /** False until a Save writes it, which is what Run now and the history need. */
  saved: boolean
  /** A saved timetable the page changed: the next run the server reported no longer holds. */
  edited: boolean
  nextRunAt: number | null
  lastRun: AgentScheduleRun | null
  running: boolean
  actionError: string
}

export interface ScheduleDraft extends Pick<
  ScheduleCard,
  'id' | 'name' | 'shape' | 'prompt' | 'disabled' | 'onMissed' | 'saved'
> {
  /** The card being edited, or null for a schedule that is not in the list yet. */
  key: string | null
  /** Whether the minute picker is showing its free number input. */
  minuteFree: boolean
  cronError: string
  promptError: string
  next: number[]
  /** Whether a preview has answered; before that the page has nothing to say about next runs. */
  previewed: boolean
  /** Set by the first keep(), after which the problem line follows the fields. */
  tried: boolean
  removing: boolean
  history: AgentScheduleRun[]
  historyLoaded: boolean
  historyError: string
}

let cardSeq = 0

function cardFrom(schedule: AgentSchedule): ScheduleCard {
  return {
    key: `card-${++cardSeq}`,
    id: schedule.id,
    // The manifest's own name, blank included: a hand-authored entry that
    // never named itself must not come back with `name: <id>` written into it.
    name: schedule.name,
    shape: parseCron(schedule.cron),
    prompt: schedule.prompt,
    disabled: schedule.disabled,
    onMissed: schedule.onMissed || 'run',
    saved: true,
    edited: false,
    nextRunAt: schedule.nextRunAt,
    lastRun: schedule.lastRun,
    running: false,
    actionError: '',
  }
}

function copyShape(shape: ScheduleShape): ScheduleShape {
  return shape.kind === 'weekly' ? { ...shape, days: [...shape.days] } : { ...shape }
}

function draftFrom(card: ScheduleCard | null): ScheduleDraft {
  const shape = card ? copyShape(card.shape) : defaultShape()
  return {
    key: card?.key ?? null,
    id: card?.id ?? '',
    name: card?.name ?? '',
    shape,
    prompt: card?.prompt ?? '',
    disabled: card?.disabled ?? false,
    onMissed: card?.onMissed ?? 'run',
    saved: card?.saved ?? false,
    minuteFree: shape.kind === 'hourly' && !QUARTER_MINUTES.includes(shape.minute),
    cronError: '',
    promptError: '',
    next: [],
    previewed: false,
    tried: false,
    removing: false,
    history: [],
    historyLoaded: false,
    historyError: '',
  }
}

// A new schedule's id follows its name so the manifest reads as prose; a saved
// one is fixed, because the id is what the run history and the scheduler's
// cursor are keyed by. That is also why a name is required to add one and
// optional to keep one: the id is the only thing that has to exist.
function draftId(draft: ScheduleDraft): string {
  if (draft.saved) return draft.id
  return draft.name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

/**
 * The schedules half of the workspace editor. `workspace` is the saved
 * directory, or empty while the workspace is being created, which has nothing
 * on disk to preview or run against. `scroller` is the element the list and
 * the page share, so closing the page returns to the row it was opened from.
 */
export function useScheduleDraft(options: {
  schedules: () => AgentSchedule[]
  workspace: () => string
  scroller: () => HTMLElement | null | undefined
}) {
  const { runs, runNow: runSchedule, preview } = useAgentSchedules()
  const cards = seedRef<ScheduleCard[]>(() => options.schedules().map(cardFrom))
  const draft = ref<ScheduleDraft | null>(null)

  let opener: HTMLElement | null = null
  let listScrollTop = 0

  function open(card: ScheduleCard | null, event?: Event): void {
    opener = event?.currentTarget instanceof HTMLElement ? event.currentTarget : null
    const scroller = options.scroller()
    listScrollTop = scroller?.scrollTop ?? 0
    draft.value = draftFrom(card)
    if (card?.saved) void loadHistory()
    void nextTick(() => {
      if (scroller) scroller.scrollTop = 0
    })
  }

  function close(): void {
    clearTimeout(previewTimer)
    previewToken++
    draft.value = null
    void nextTick(() => {
      const scroller = options.scroller()
      if (scroller) scroller.scrollTop = listScrollTop
      opener?.focus()
      opener = null
    })
  }

  /** What stops this draft being kept, in the order a user would fix it. */
  function problem(current: ScheduleDraft): string {
    const id = draftId(current)
    if (!current.saved) {
      if (!current.name.trim()) return 'A name is required.'
      if (!id) return 'The name needs a letter or a digit to build an id from.'
    }
    // The Go side upserts by id, so two rows on one id would silently drop a
    // schedule; the collision is the user's to resolve before the manifest is written.
    if (cards.value.some((card) => card.key !== current.key && card.id === id)) {
      return `Another schedule already uses the id "${id}".`
    }
    if (current.shape.kind === 'weekly' && !current.shape.days.length) return 'Pick at least one day.'
    if (current.shape.kind === 'custom' && !current.shape.cron.trim()) return 'A cron expression is required.'
    if (!current.prompt.trim()) return 'A prompt is required.'
    return ''
  }

  // The cron box shows its own error; a structured shape has no box, so its
  // error, should the Go side ever report one, is said here instead.
  const issue = computed(() => {
    const current = draft.value
    if (!current?.tried) return ''
    return problem(current) || (current.shape.kind === 'custom' ? '' : current.cronError)
  })

  function keep(): void {
    const current = draft.value
    if (!current) return
    current.tried = true
    if (problem(current) || current.cronError || current.promptError) return
    const fields = {
      id: draftId(current),
      name: current.name.trim(),
      shape: current.shape,
      prompt: current.prompt,
      disabled: current.disabled,
      onMissed: current.onMissed,
    }
    const card = cards.value.find((row) => row.key === current.key)
    if (card) {
      const retimed = buildCron(current.shape) !== buildCron(card.shape)
      Object.assign(card, fields, { edited: card.edited || (card.saved && retimed) })
    } else {
      const fresh = cardFrom({ ...fields, cron: '', nextRunAt: null, lastRun: null })
      cards.value = [...cards.value, { ...fresh, shape: fields.shape, saved: false }]
    }
    close()
  }

  function remove(): void {
    const current = draft.value
    if (!current) return
    cards.value = cards.value.filter((card) => card.key !== current.key)
    close()
  }

  // The Go side is the only thing that knows whether a cron and a template are
  // valid, so the page debounces the question. The token drops an answer to a
  // question an older draft asked, including one the page has since closed.
  let previewTimer: ReturnType<typeof setTimeout> | undefined
  let previewToken = 0

  // No days compiles to a four-field expression the Go side rejects, and the
  // page already says a day is missing; asking would only trade that for a
  // cron error about the wrong thing.
  const previewQuestion = computed(() => {
    const current = draft.value
    if (!current || (current.shape.kind === 'weekly' && !current.shape.days.length)) return null
    return { cron: buildCron(current.shape), prompt: current.prompt }
  })

  watch(
    () => (previewQuestion.value ? `${previewQuestion.value.cron}\n${previewQuestion.value.prompt}` : null),
    (question) => {
      if (question === null || !options.workspace()) return
      clearTimeout(previewTimer)
      previewTimer = setTimeout(() => void runPreview(), PREVIEW_DEBOUNCE_MS)
    },
  )

  async function runPreview(): Promise<void> {
    const current = draft.value
    const question = previewQuestion.value
    if (!current || !question) return
    const token = ++previewToken
    try {
      const result = await preview({ workspace: options.workspace(), ...question })
      if (token !== previewToken) return
      current.next = result.next.slice(0, PREVIEW_SHOWN)
      current.cronError = result.cronError
      current.promptError = result.promptError
    } catch {
      // A preview that could not be asked for is not itself a reason to block
      // the draft: the Go side validates the manifest again on the way in. The
      // errors go with the occurrences, since they answered an older cron and
      // prompt and would otherwise block Done for good.
      if (token !== previewToken) return
      current.next = []
      current.cronError = ''
      current.promptError = ''
    }
    current.previewed = true
  }

  onScopeDispose(() => clearTimeout(previewTimer))

  async function runNow(card: ScheduleCard): Promise<void> {
    const workspace = options.workspace()
    if (!workspace || !card.saved || card.running) return
    card.running = true
    card.actionError = ''
    try {
      // The run the Go side answers with is, by construction, the schedule's
      // newest, which is what lastRun shows. nextRunAt is untouched: a manual
      // run never consumes the window.
      card.lastRun = await runSchedule(workspace, card.id)
    } catch (failure) {
      card.actionError = failure instanceof Error ? failure.message : 'The schedule could not be run.'
    } finally {
      card.running = false
    }
  }

  async function loadHistory(): Promise<void> {
    const current = draft.value
    const workspace = options.workspace()
    if (!workspace || !current?.saved) return
    current.historyError = ''
    try {
      current.history = await runs(workspace, current.id)
      current.historyLoaded = true
    } catch (failure) {
      current.historyError = failure instanceof Error ? failure.message : 'The run history could not be read.'
    }
  }

  function edits(): ScheduleEdit[] {
    return cards.value.map((card) => ({
      id: card.id,
      name: card.name,
      cron: buildCron(card.shape),
      prompt: card.prompt,
      disabled: card.disabled,
      onMissed: card.onMissed,
    }))
  }

  return { cards, draft, issue, draftId, open, close, keep, remove, runNow, edits }
}
