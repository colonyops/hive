<script setup lang="ts">
// Create/edit editor for an agent workspace's manifest, in the app's
// DrawerSheet editor shell (the ActionEditor pattern). It writes the fields
// it shows — name, command, the mcps and skills lists, and the schedules
// list (plus the directory name at creation); hand-written comments in the
// YAML survive the write untouched. Both capability lists work the same way:
// a shared library
// on disk declares what exists (mcps.yaml, skills.yml) and the toggle is this
// workspace's own. The skills list names packages, not individual skills
// (ADR skill-packages-are-the-unit-a-workspace-enables). Deleting the workspace also lives here — the editor
// is the workspace's whole management surface, so its sidebar row needs no
// menu. Delete follows FolderEditModal.vue's shape: a quiet footer action
// that expands into an InlineConfirm over a dimmed, inert form.
import { computed, h, markRaw, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { Component } from 'vue'
import IconArrowLeft from '~icons/lucide/arrow-left'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconExternalLink from '~icons/lucide/external-link'
import IconFolderCog from '~icons/lucide/folder-cog'
import IconFolderOpen from '~icons/lucide/folder-open'
import IconPencil from '~icons/lucide/pencil'
import IconPlay from '~icons/lucide/play'
import IconPlus from '~icons/lucide/plus'
import IconTrash2 from '~icons/lucide/trash-2'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import IconX from '~icons/lucide/x'
import AgentIcon, { agentHasIcon } from './AgentIcon.vue'
import AppSelect, { type AppSelectOption } from './AppSelect.vue'
import AppSwitch from './AppSwitch.vue'
import BaseBadge from './BaseBadge.vue'
import BaseButton from './BaseButton.vue'
import DrawerSheet from './DrawerSheet.vue'
import InlineConfirm from './InlineConfirm.vue'
import SettingsError from './settings/SettingsError.vue'
import SettingsSection from './settings/SettingsSection.vue'
import { CodeField, FieldRow, SelectField, TextField, TextareaField, type SelectOption } from '../pipeline/fields'
import { useAgentSchedules } from '../composables/useAgentSchedules'
import { useAgentWorkspaces } from '../composables/useAgentWorkspaces'
import { timeLabel } from '../lib/activityPresentation'
import { relativeAge, relativeTimeLabel } from '../lib/age'
import {
  buildCron, clock, dayAbbreviation, defaultShape, describe, pad, parseCron, WEEK_ORDER, type ScheduleShape,
} from '../lib/scheduleShape'
import type {
  AgentSchedule, AgentScheduleRun, AgentWorkspace, ScheduleEdit, SkillPackageMember, WorkspaceEditRequest,
} from '../lib/agentWorkspacesClient'

const props = defineProps<{
  /** The workspace being edited, or null to create one. */
  workspace?: AgentWorkspace | null
  busy?: boolean
  error?: string | null
}>()
const emit = defineEmits<{
  close: []
  save: [request: WorkspaceEditRequest]
  delete: [dir: string]
}>()

const {
  root, editor, mcpCatalogue, skillPackages, skillNames, skillPackagesProblem, presets,
  reloadMCPCatalogue, importMCPServers, removeMCPServer,
  reloadSkillPackages, revealSkillPackages, revealSharedSkills,
  openWorkspaceInEditor, revealWorkspace,
} = useAgentWorkspaces()

const creating = computed(() => !props.workspace)

// The confirm strip names the folder it is about to delete, in full: the
// directory name alone reads as a label in the app, not as a path on disk.
const deletedPath = computed(() => (root.value ? `${root.value}/${props.workspace?.dir ?? ''}` : props.workspace?.dir ?? ''))

// A manifest the loader could not read leaves every list on this form empty,
// and Save reconciles the file to what the form holds, so it would rewrite
// mcps:, skills: and schedules: to nothing. The Go side refuses such an update
// (KindInvalid); the form refuses it first, and says what to fix instead.
const manifestProblem = computed(() => (creating.value ? '' : props.workspace?.problem ?? ''))

const dir = ref(props.workspace?.dir ?? '')
const name = ref(props.workspace?.name ?? '')
const command = ref(props.workspace?.command ?? '')
const selectedMCPs = ref<string[]>([...(props.workspace?.mcps ?? [])])
const selectedSkills = ref<string[]>([...(props.workspace?.skills ?? [])])

const CUSTOM = '__custom__'

// Without this the derived selection below snaps back to a suggestion the
// moment the typed command matches one again.
const editingCommand = ref(false)

const selection = computed<string>({
  get: () => (editingCommand.value ? CUSTOM : presets.value.find((p) => p.command === command.value)?.id ?? CUSTOM),
  set(id) {
    if (id === CUSTOM) {
      editingCommand.value = true
      return
    }
    const preset = presets.value.find((p) => p.id === id)
    if (!preset) return
    command.value = preset.command
    editingCommand.value = false
  },
})

// AppSelect takes a bare component, with no props to pass an agent through.
// Memoized: a fresh component identity on every render remounts every row.
const agentIcons = new Map<string, Component>()
function agentIcon(agent: string): Component | undefined {
  if (!agentHasIcon(agent)) return undefined
  let icon = agentIcons.get(agent)
  if (!icon) {
    icon = markRaw(() => h(AgentIcon, { id: agent }))
    agentIcons.set(agent, icon)
  }
  return icon
}

// A hive-seeded preset's label is its profile key, which is already the agent
// when the profile just runs that CLI.
const presetOptions = computed<AppSelectOption[]>(() => [
  ...presets.value.map((preset) => ({
    value: preset.id,
    label: preset.label === preset.agent ? preset.agent : `${preset.agent} · ${preset.label}`,
    hint: preset.command,
    icon: agentIcon(preset.agent),
  })),
  { value: CUSTOM, label: 'Custom', hint: 'Write the invocation yourself', icon: markRaw(IconPencil) },
])

// A watch, not an initializer: presets arrive with the area's overview, which
// can land after this sheet is mounted.
watch(presets, (rows) => {
  if (!creating.value || command.value || !rows.length) return
  command.value = rows[0].command
  // A default the form fills in for itself is not an edit the user has to be
  // asked about on the way out.
  baseline.value = formState()
}, { immediate: true })

// DANGEROUS_FLAGS mirrors agentws's own list. It is duplicated rather than
// served because it only drives a warning: a flag missing here shows no
// banner, which is the same "not recognized" the Go side means, and the
// manifest's own danger flag still labels the saved row.
const DANGEROUS_FLAGS = [
  '--dangerously-skip-permissions',
  '--dangerously-bypass-approvals-and-sandbox',
  '--yolo',
  '--full-auto',
]
const commandIsDangerous = computed(() => DANGEROUS_FLAGS.some((flag) => command.value.includes(flag)))

// Kept in step with agentws.LaunchData.
const TEMPLATE_FIELDS = [
  { field: '{{ .Dir }}', means: 'the workspace directory on disk' },
  { field: '{{ .MCPConfig }}', means: 'the MCP config Hive generates for this workspace' },
  { field: '{{ .SessionID }}', means: 'the id Hive minted for this chat' },
  { field: '{{ .Resume }}', means: 'true when reopening a chat, false on a new one' },
  { field: '{{ .Prompt }}', means: 'an optional opening message for a schedule or prompted launch; pass it through shq' },
  { field: 'shq', means: 'quotes a value for the shell — pipe every path through it' },
]

// The settings pages' vocabulary: SettingsSection's boxed card, drawn here so
// a list can end in its own action row; the small outlined button a row
// carries; the quiet icon button beside it; and the accent row a card ends
// with, which the card's hairlines separate from the list above it.
const listCardClass = 'divide-y divide-row overflow-hidden rounded-[11px] border border-card bg-raised'
const inlineButtonClass = 'flex shrink-0 cursor-pointer items-center gap-1.5 rounded-[7px] border border-card px-3 py-1.5 text-[12.5px] font-medium text-text-2 hover:border-strong hover:text-text disabled:cursor-not-allowed disabled:opacity-50'
const iconButtonClass = 'flex size-7 shrink-0 cursor-pointer items-center justify-center rounded-md text-text-3 hover:bg-chip hover:text-text disabled:cursor-not-allowed disabled:opacity-40'
const footerButtonClass = 'flex w-full cursor-pointer items-center gap-2 px-4 py-3 text-left text-[13px] font-medium text-accent hover:bg-chip disabled:cursor-not-allowed disabled:opacity-50'

const confirming = ref(false)

// ── MCP rows ─────────────────────────────────────────────────────────────────
// The catalogue's rows plus any declared id the catalogue no longer resolves,
// so a hand-authored entry that went missing is visible and removable rather
// than silently kept.
interface MCPRow {
  id: string
  title: string
  command: string
  problem: string
  shipped: boolean
  stability: string
  shadows: string
  missing: boolean
}

const mcpRows = computed<MCPRow[]>(() => {
  const rows: MCPRow[] = mcpCatalogue.value.map((e) => ({
    id: e.id, title: e.title || e.id, command: e.command, problem: e.problem,
    shipped: e.shipped, stability: e.stability, shadows: e.shadows, missing: false,
  }))
  const known = new Set(rows.map((r) => r.id))
  for (const id of selectedMCPs.value) {
    if (!known.has(id)) rows.push({ id, title: id, command: '', problem: '', shipped: false, stability: '', shadows: '', missing: true })
  }
  return rows
})

function mcpEnabled(id: string): boolean {
  return selectedMCPs.value.includes(id)
}

function toggleMCP(id: string): void {
  selectedMCPs.value = mcpEnabled(id)
    ? selectedMCPs.value.filter((x) => x !== id)
    : [...selectedMCPs.value, id]
}

const mcpError = ref('')

async function removeServer(id: string): Promise<void> {
  mcpError.value = ''
  try {
    await removeMCPServer(id)
    selectedMCPs.value = selectedMCPs.value.filter((x) => x !== id)
  } catch (failure) {
    mcpError.value = failure instanceof Error ? failure.message : 'The server could not be removed.'
  }
}

// ── Skill package rows ───────────────────────────────────────────────────────
// A workspace enables packages, not skills: skills.yml defines each package as
// glob patterns, and the rows show what those patterns select right now. An
// enabled name skills.yml no longer defines still rows, so it can be switched
// off rather than silently selecting nothing.
interface SkillPackageRow {
  name: string
  title: string
  description: string
  members: SkillPackageMember[]
  missing: boolean
  /** Why the name does not resolve, when it does not. */
  warning: string
}

const skillRows = computed<SkillPackageRow[]>(() => {
  const rows: SkillPackageRow[] = skillPackages.value.map((pkg) => ({
    name: pkg.name, title: pkg.title || pkg.name, description: pkg.description,
    members: pkg.members, missing: false, warning: '',
  }))
  const known = new Set(rows.map((r) => r.name))
  const bySlug = new Map(skillNames.value.map((skill) => [skill.slug, skill]))
  for (const name of selectedSkills.value) {
    if (known.has(name)) continue
    rows.push({ name, title: name, description: '', members: [], missing: true, warning: missingSkillWarning(name, bySlug.get(name)?.selectedBy) })
  }
  return rows
})

// A name skills.yml does not define is a package that never existed *or* a
// skill slug from a manifest written before packages were the enablement unit
// (#307). Only the second has a fix on screen, and saying "not defined in
// skills.yml" for both hides it.
function missingSkillWarning(name: string, selectedBy: string[] | undefined): string {
  if (!selectedBy) return 'not defined in skills.yml — an enabled package without a definition brings nothing'
  if (!selectedBy.length) return 'a skill, not a package — no package selects it yet, so define one in skills.yml'
  const packages = selectedBy.map((pkg) => `"${pkg}"`).join(' or ')
  return `a skill, not a package — the ${packages} package selects it, so enable that and switch this off`
}

function skillEnabled(name: string): boolean {
  return selectedSkills.value.includes(name)
}

function toggleSkill(name: string): void {
  selectedSkills.value = skillEnabled(name)
    ? selectedSkills.value.filter((x) => x !== name)
    : [...selectedSkills.value, name]
}

// A package's members are worth seeing before enabling it — it is the whole
// authority the package grants — but not worth the height by default, so the
// list expands on demand.
const expandedSkillPackages = ref<string[]>([])

function skillPackageExpanded(name: string): boolean {
  return expandedSkillPackages.value.includes(name)
}

function toggleSkillPackageExpanded(name: string): void {
  expandedSkillPackages.value = skillPackageExpanded(name)
    ? expandedSkillPackages.value.filter((x) => x !== name)
    : [...expandedSkillPackages.value, name]
}

const skillError = ref('')

async function openSkillPackages(): Promise<void> {
  skillError.value = ''
  try {
    await revealSkillPackages()
  } catch (failure) {
    skillError.value = failure instanceof Error ? failure.message : 'skills.yml could not be opened.'
  }
}

async function openSharedSkills(): Promise<void> {
  skillError.value = ''
  try {
    await revealSharedSkills()
  } catch (failure) {
    skillError.value = failure instanceof Error ? failure.message : 'The skills folder could not be opened.'
  }
}

// ── Schedules ────────────────────────────────────────────────────────────────
// A schedule is a manifest key like the lists above, so it is edited here and
// written by the same Save: the request carries the whole list and the Go side
// reconciles `schedules:` to it. The list is rows; one schedule at a time is
// edited on a page that takes the sheet over, working on a draft that Done
// copies back to its row and Cancel drops. Cron is still what the manifest
// stores, but the page never asks for one: the draft edits a ScheduleShape and
// compiles it (lib/scheduleShape.ts), with the cron box reserved for
// expressions no shape can state. Only Run now, the preview and the run
// history reach the network from here; everything else is form state until
// Save.

/** Long enough that a keystroke does not cost a round trip, short enough to feel live. */
const PREVIEW_DEBOUNCE_MS = 300
/** How many of the preview's occurrences the page shows. */
const PREVIEW_SHOWN = 3
const DAY_MS = 24 * 60 * 60 * 1000
/** Under a minute out, a countdown reads as noise; the row says it is up instead. */
const IMMINENT_MS = 60 * 1000
const QUARTER_MINUTES = [0, 15, 30, 45]

interface ScheduleCard {
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

/** The page's working copy of a row. Done writes it back; Cancel drops it. */
interface ScheduleDraft {
  /** The row being edited, or null for a schedule that is not in the list yet. */
  key: string | null
  id: string
  name: string
  shape: ScheduleShape
  prompt: string
  disabled: boolean
  onMissed: AgentSchedule['onMissed']
  saved: boolean
  /** Whether the minute picker is showing its free number input. */
  minuteFree: boolean
  cronError: string
  promptError: string
  next: number[]
  /** Whether a preview has answered; before that the page has nothing to say about next runs. */
  previewed: boolean
  /** Set by the first Done, after which the problem line follows the fields. */
  tried: boolean
  removing: boolean
  history: AgentScheduleRun[]
  historyLoaded: boolean
  historyError: string
}

const { runs: listScheduleRuns, runNow, preview } = useAgentSchedules()

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

const scheduleCards = ref<ScheduleCard[]>((props.workspace?.schedules ?? []).map(cardFrom))
const scheduleDraft = ref<ScheduleDraft | null>(null)

// ── Unsaved work ─────────────────────────────────────────────────────────────
// What Save would write, in one comparable shape. That is also what an edit
// has to mean here: a schedule whose next run time moved while the sheet was
// open changed the card, not the manifest, and must not make the form dirty.
function formState(): string {
  return JSON.stringify({
    dir: dir.value.trim(),
    name: name.value.trim(),
    command: command.value.trim(),
    mcps: [...selectedMCPs.value].sort(),
    skills: [...selectedSkills.value].sort(),
    schedules: scheduleEdits(),
  })
}

const baseline = ref(formState())
const dirty = computed(() => formState() !== baseline.value)

const REPEAT_OPTIONS: SelectOption[] = [
  { value: 'hourly', label: 'Hourly' },
  { value: 'daily', label: 'Daily' },
  { value: 'weekly', label: 'Weekly' },
  { value: 'monthly', label: 'Monthly' },
  { value: 'custom', label: 'Custom' },
]

const MINUTE_OPTIONS: SelectOption[] = [
  ...QUARTER_MINUTES.map((minute) => ({ value: String(minute), label: `:${pad(minute)}` })),
  { value: 'other', label: 'Other minute…' },
]

const MONTH_DAY_OPTIONS: SelectOption[] = Array.from({ length: 31 }, (_, index) => ({
  value: String(index + 1),
  label: String(index + 1),
}))

const ON_MISSED_OPTIONS: SelectOption[] = [
  { value: 'run', label: 'Run once when the app is back' },
  { value: 'skip', label: 'Skip' },
]

const PROMPT_VARIABLES = [
  '{{ .Now }}',
  '{{ .ScheduledFor }}',
  '{{ .LastRun }}',
  '{{ .Reason }}',
  '{{ .Schedule.Name }}',
  '{{ .Workspace.Name }}',
  '{{ date "2006-01-02" .Now }}',
]
const promptHint = `Go template. Available: ${PROMPT_VARIABLES.join(', ')}`

function scheduleLabel(card: ScheduleCard): string {
  return card.name || card.id
}

// ── The page ────────────────────────────────────────────────────────────────
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

const sheet = ref<InstanceType<typeof DrawerSheet> | null>(null)
const draftNameField = ref<{ focus: () => void } | null>(null)
let draftOpener: HTMLElement | null = null
// The page replaces the sheet's body, and the list it replaces is the sheet's
// last section: without this the page opens scrolled to its bottom, and Done
// returns to the top of the form rather than to the row it just wrote.
let listScrollTop = 0

function openScheduleDraft(card: ScheduleCard | null, event?: Event): void {
  draftOpener = event?.currentTarget instanceof HTMLElement ? event.currentTarget : null
  listScrollTop = sheet.value?.body?.scrollTop ?? 0
  scheduleDraft.value = draftFrom(card)
  queuePreview()
  if (card?.saved) void loadHistory()
  void nextTick(() => {
    if (sheet.value?.body) sheet.value.body.scrollTop = 0
    draftNameField.value?.focus()
  })
}

function closeScheduleDraft(): void {
  clearTimeout(previewTimer)
  previewToken++
  scheduleDraft.value = null
  void nextTick(() => {
    if (sheet.value?.body) sheet.value.body.scrollTop = listScrollTop
    draftOpener?.focus()
    draftOpener = null
  })
}

// A new schedule's id follows its name so the manifest reads as prose; a saved
// one is fixed, because the id is what the run history and the scheduler's
// cursor are keyed by. That is also why a name is required to add one and
// optional to keep one: the id is the only thing that has to exist.
function draftId(draft: ScheduleDraft): string {
  if (draft.saved) return draft.id
  return draft.name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')
}

const headerTitle = computed(() => {
  const draft = scheduleDraft.value
  if (draft) return draft.key === null ? 'New schedule' : 'Edit schedule'
  return creating.value ? 'New workspace' : 'Edit workspace'
})

const headerSubtitle = computed(() => {
  const draft = scheduleDraft.value
  if (draft) return draftId(draft) || `Starts a chat in ${name.value.trim() || 'this workspace'} on its own timetable`
  return creating.value ? 'A directory an agent works in' : props.workspace!.dir
})

/** What stops this draft being kept, in the order a user would fix it. */
function draftProblem(draft: ScheduleDraft): string {
  const id = draftId(draft)
  if (!draft.saved) {
    if (!draft.name.trim()) return 'A name is required.'
    if (!id) return 'The name needs a letter or a digit to build an id from.'
  }
  // The Go side upserts by id, so two rows on one id would silently drop a
  // schedule; the collision is the user's to resolve before the manifest is written.
  if (scheduleCards.value.some((card) => card.key !== draft.key && card.id === id)) {
    return `Another schedule already uses the id "${id}".`
  }
  if (draft.shape.kind === 'weekly' && !draft.shape.days.length) return 'Pick at least one day.'
  if (draft.shape.kind === 'custom' && !draft.shape.cron.trim()) return 'A cron expression is required.'
  if (!draft.prompt.trim()) return 'A prompt is required.'
  return ''
}

// The cron box shows its own error; a structured shape has no box, so its
// error, should the Go side ever report one, is said here instead.
const draftIssue = computed(() => {
  const draft = scheduleDraft.value
  if (!draft?.tried) return ''
  return draftProblem(draft) || (draft.shape.kind === 'custom' ? '' : draft.cronError)
})

function keepScheduleDraft(): void {
  const draft = scheduleDraft.value
  if (!draft) return
  draft.tried = true
  if (draftProblem(draft) || draft.cronError || draft.promptError) return
  const fields = {
    id: draftId(draft),
    name: draft.name.trim(),
    shape: draft.shape,
    prompt: draft.prompt,
    disabled: draft.disabled,
    onMissed: draft.onMissed,
  }
  const card = scheduleCards.value.find((row) => row.key === draft.key)
  if (card) {
    const retimed = buildCron(draft.shape) !== buildCron(card.shape)
    Object.assign(card, fields, { edited: card.edited || (card.saved && retimed) })
  } else {
    scheduleCards.value = [...scheduleCards.value, {
      key: `card-${++cardSeq}`, ...fields, saved: false, edited: false,
      nextRunAt: null, lastRun: null, running: false, actionError: '',
    }]
  }
  closeScheduleDraft()
}

function removeScheduleDraft(): void {
  const draft = scheduleDraft.value
  if (!draft) return
  scheduleCards.value = scheduleCards.value.filter((card) => card.key !== draft.key)
  closeScheduleDraft()
}

// ── The page's shape controls ───────────────────────────────────────────────
function shapeClock(shape: ScheduleShape): { hour: number; minute: number } {
  if (shape.kind === 'custom') return { hour: 9, minute: 0 }
  if (shape.kind === 'hourly') return { hour: 9, minute: shape.minute }
  return { hour: shape.hour, minute: shape.minute }
}

function setRepeat(kind: string): void {
  const draft = scheduleDraft.value
  if (!draft || kind === draft.shape.kind) return
  const { hour, minute } = shapeClock(draft.shape)
  switch (kind) {
    case 'hourly':
      draft.shape = { kind: 'hourly', minute }
      draft.minuteFree = !QUARTER_MINUTES.includes(minute)
      break
    case 'daily': draft.shape = { kind: 'daily', hour, minute }; break
    case 'weekly': draft.shape = { kind: 'weekly', days: [5], hour, minute }; break
    case 'monthly': draft.shape = { kind: 'monthly', day: 1, hour, minute }; break
    // Switching to Custom carries the compiled expression over, so the box
    // opens on what the picker was already saying rather than empty.
    default: draft.shape = { kind: 'custom', cron: buildCron(draft.shape) }
  }
  queuePreview()
}

function dayPicked(day: number): boolean {
  const shape = scheduleDraft.value?.shape
  return shape?.kind === 'weekly' && shape.days.includes(day)
}

function toggleDay(day: number): void {
  const draft = scheduleDraft.value
  if (draft?.shape.kind !== 'weekly') return
  const days = dayPicked(day)
    ? draft.shape.days.filter((picked) => picked !== day)
    : [...draft.shape.days, day].sort((a, b) => a - b)
  draft.shape = { ...draft.shape, days }
  // No days compiles to a four-field expression the Go side rejects, and the
  // page already says a day is missing; asking would only trade that for a
  // cron error about the wrong thing.
  if (days.length) queuePreview()
}

function onTimeInput(event: Event): void {
  const draft = scheduleDraft.value
  const match = /^(\d{1,2}):(\d{2})/.exec((event.target as HTMLInputElement).value)
  if (!draft || !match || draft.shape.kind === 'custom' || draft.shape.kind === 'hourly') return
  draft.shape = { ...draft.shape, hour: Number(match[1]), minute: Number(match[2]) }
  queuePreview()
}

const minuteSelection = computed(() => {
  const draft = scheduleDraft.value
  if (draft?.shape.kind !== 'hourly') return '0'
  return draft.minuteFree ? 'other' : String(draft.shape.minute)
})

function setMinuteSelection(value: string): void {
  const draft = scheduleDraft.value
  if (!draft) return
  if (value === 'other') {
    draft.minuteFree = true
    return
  }
  draft.minuteFree = false
  setMinute(Number(value))
}

function setMinute(minute: number): void {
  const draft = scheduleDraft.value
  if (draft?.shape.kind !== 'hourly' || !Number.isFinite(minute)) return
  draft.shape = { kind: 'hourly', minute: Math.min(59, Math.max(0, Math.trunc(minute))) }
  queuePreview()
}

function onMinuteInput(event: Event): void {
  setMinute(Number((event.target as HTMLInputElement).value))
}

function setMonthDay(value: string): void {
  const draft = scheduleDraft.value
  if (draft?.shape.kind !== 'monthly') return
  draft.shape = { ...draft.shape, day: Number(value) }
  queuePreview()
}

function setCron(cron: string): void {
  const draft = scheduleDraft.value
  if (!draft) return
  draft.shape = { kind: 'custom', cron }
  queuePreview()
}

function setPrompt(prompt: string): void {
  const draft = scheduleDraft.value
  if (!draft) return
  draft.prompt = prompt
  queuePreview()
}

// ── Preview ─────────────────────────────────────────────────────────────────
// The Go side is the only thing that knows whether a cron and a template are
// valid, so the page debounces the question and renders the answer beside the
// field that raised it. The token drops an answer to a question an older draft
// asked, including one the page has since closed.
let previewTimer: ReturnType<typeof setTimeout> | undefined
let previewToken = 0

function queuePreview(): void {
  // A workspace being created has nothing on disk to preview against; the
  // structured shapes compile to valid cron without asking.
  if (!props.workspace) return
  clearTimeout(previewTimer)
  previewTimer = setTimeout(() => { void runPreview() }, PREVIEW_DEBOUNCE_MS)
}

async function runPreview(): Promise<void> {
  const draft = scheduleDraft.value
  if (!draft) return
  const token = ++previewToken
  try {
    const result = await preview({
      workspace: props.workspace!.dir,
      cron: buildCron(draft.shape),
      prompt: draft.prompt,
    })
    if (token !== previewToken) return
    draft.next = result.next.slice(0, PREVIEW_SHOWN)
    draft.cronError = result.cronError
    draft.promptError = result.promptError
  } catch {
    // A preview that could not be asked for is not itself a reason to block
    // the draft: the Go side validates the manifest again on the way in. The
    // errors go with the occurrences, since they answered an older cron and
    // prompt and would otherwise block Done for good.
    if (token !== previewToken) return
    draft.next = []
    draft.cronError = ''
    draft.promptError = ''
  }
  draft.previewed = true
}

const nextRunsLine = computed(() => {
  const draft = scheduleDraft.value
  if (!draft?.previewed || draft.cronError) return ''
  return draft.next.length ? `Next: ${draft.next.map(occurrence).join(' · ')}` : 'No upcoming runs.'
})

onBeforeUnmount(() => clearTimeout(previewTimer))

// ── Run now, and what a row says about its runs ─────────────────────────────
async function runScheduleNow(card: ScheduleCard): Promise<void> {
  if (!props.workspace || !card.saved || card.running) return
  card.running = true
  card.actionError = ''
  try {
    // The run the Go side answers with is, by construction, the schedule's
    // newest, which is what lastRun shows. nextRunAt is untouched: a manual
    // run never consumes the window.
    card.lastRun = await runNow(props.workspace.dir, card.id)
  } catch (failure) {
    card.actionError = failure instanceof Error ? failure.message : 'The schedule could not be run.'
  } finally {
    card.running = false
  }
}

async function loadHistory(): Promise<void> {
  const draft = scheduleDraft.value
  if (!props.workspace || !draft?.saved) return
  draft.historyError = ''
  try {
    draft.history = await listScheduleRuns(props.workspace.dir, draft.id)
    draft.historyLoaded = true
  } catch (failure) {
    draft.historyError = failure instanceof Error ? failure.message : 'The run history could not be read.'
  }
}

/** Empty while the timetable is not the scheduler's yet, which the row's unsaved chip already says. */
function nextRunLabel(card: ScheduleCard): string {
  if (card.disabled) return 'paused'
  if (!card.saved || card.edited) return ''
  if (card.nextRunAt === null) return 'not scheduled'
  const now = Date.now()
  const delta = card.nextRunAt - now
  if (delta <= IMMINENT_MS) return 'due now'
  // relativeAge measures backwards from its second argument, so the two are
  // swapped here to get the same terse form ("2h") for a time still to come.
  if (delta < DAY_MS) return `next in ${relativeAge(now, card.nextRunAt)}`
  return `next ${occurrence(card.nextRunAt)}`
}

function occurrence(at: number): string {
  return new Date(at).toLocaleString([], {
    weekday: 'short', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false,
  })
}

const REASON_LABELS: Record<string, string> = { due: 'on schedule', catch_up: 'catch-up', manual: 'by hand' }

function reasonLabel(run: AgentScheduleRun): string {
  return REASON_LABELS[run.reason] ?? run.reason
}

// A run at its scheduled time is the ordinary case, so only the other reasons are said.
function lastRunLabel(run: AgentScheduleRun): string {
  const reason = run.reason === 'due' ? '' : ` · ${reasonLabel(run)}`
  return `Last run ${run.status} ${relativeTimeLabel(run.startedAt)}${reason}`
}

// The launcher rows' dot vocabulary: a launch reads as success, a failure as
// an error, and everything else stays neutral chrome.
function statusDot(status: string): string {
  if (status === 'failed') return 'bg-severity-error'
  if (status === 'launched') return 'bg-severity-success'
  return 'bg-text-4'
}

// A history spanning days needs the day; a run from today does not.
function runStamp(at: number): string {
  const when = new Date(at)
  if (when.toDateString() === new Date().toDateString()) return timeLabel(at)
  return `${when.toLocaleDateString([], { month: 'short', day: 'numeric' })} ${timeLabel(at)}`
}

function scheduleEdits(): ScheduleEdit[] {
  return scheduleCards.value.map((card) => ({
    id: card.id,
    name: card.name,
    cron: buildCron(card.shape),
    prompt: card.prompt,
    disabled: card.disabled,
    onMissed: card.onMissed,
  }))
}

// ── Paste-in JSON import ─────────────────────────────────────────────────────
const importOpen = ref(false)
const importText = ref('')
const importBusy = ref(false)
const importPlaceholder = '{"mcpServers": {"my-server": {"command": "npx", "args": ["-y", "…"]}}}'

function formatImportJSON(): void {
  mcpError.value = ''
  try {
    importText.value = JSON.stringify(JSON.parse(importText.value), null, 2)
  } catch (failure) {
    mcpError.value = failure instanceof Error ? `Not valid JSON: ${failure.message}` : 'Not valid JSON.'
  }
}

async function submitImport(): Promise<void> {
  if (!importText.value.trim() || importBusy.value) return
  importBusy.value = true
  mcpError.value = ''
  try {
    const added = await importMCPServers(importText.value)
    for (const id of added) {
      if (!mcpEnabled(id)) selectedMCPs.value = [...selectedMCPs.value, id]
    }
    importText.value = ''
    importOpen.value = false
  } catch (failure) {
    mcpError.value = failure instanceof Error ? failure.message : 'The pasted configuration could not be imported.'
  } finally {
    importBusy.value = false
  }
}

// ── Open the directory outside the app ───────────────────────────────────────
const actionError = ref('')

async function openInEditor(): Promise<void> {
  actionError.value = ''
  try {
    await openWorkspaceInEditor(props.workspace!.dir)
  } catch (failure) {
    actionError.value = failure instanceof Error ? failure.message : 'The editor could not be opened.'
  }
}

async function reveal(): Promise<void> {
  actionError.value = ''
  try {
    await revealWorkspace(props.workspace!.dir)
  } catch (failure) {
    actionError.value = failure instanceof Error ? failure.message : 'The directory could not be opened.'
  }
}

const valid = computed(() =>
  !!name.value.trim() && !!command.value.trim() && (!creating.value || !!dir.value.trim()) && !manifestProblem.value)

function submit(): void {
  if (props.busy || confirming.value || scheduleDraft.value || !valid.value) return
  emit('save', {
    dir: creating.value ? dir.value.trim() : props.workspace!.dir,
    name: name.value.trim(),
    command: command.value.trim(),
    mcps: selectedMCPs.value,
    skills: selectedSkills.value,
    schedules: scheduleEdits(),
  })
}

const discarding = ref(false)

// Escape and the backdrop step back one surface: off the schedule page while
// one is open, and out of the sheet otherwise.
function cancel(): void {
  if (props.busy || confirming.value || discarding.value) return
  if (scheduleDraft.value) closeScheduleDraft()
  else closeSheet()
}

// The one way out, so every gesture that leaves — Escape, the backdrop, the
// header's X, the footer's Cancel — asks before it throws an edit away. The
// question is a footer strip rather than a second dialog: a modal over the
// sheet doubles the backdrop and both would answer the same Escape, which is
// what InlineConfirm exists to avoid.
function closeSheet(): void {
  if (props.busy || confirming.value) return
  if (dirty.value) discarding.value = true
  else emit('close')
}

const nameInput = ref<{ focus: () => void } | null>(null)
const dirInput = ref<HTMLInputElement | null>(null)
onMounted(async () => {
  void reloadMCPCatalogue()
  void reloadSkillPackages()
  await nextTick()
  ;(creating.value ? dirInput.value : nameInput.value)?.focus()
})
</script>

<template>
  <DrawerSheet
    ref="sheet"
    :ariaLabel="creating ? 'New workspace' : 'Edit workspace'"
    testid="agent-workspace-editor"
    :default-size="520"
    :close-on-escape="!confirming && !discarding && !scheduleDraft?.removing"
    :close-on-backdrop="!confirming && !discarding"
    @close="cancel"
  >
    <template #header>
      <div class="flex items-center gap-3">
        <button
          v-if="scheduleDraft"
          type="button"
          class="flex size-[38px] shrink-0 cursor-pointer items-center justify-center rounded-[10px] border border-card text-text-2 hover:border-strong hover:text-text"
          aria-label="Back to the workspace"
          data-testid="agent-workspace-editor-schedule-back"
          @click="closeScheduleDraft"
        ><IconArrowLeft class="size-[18px]" /></button>
        <span v-else class="flex size-[38px] items-center justify-center rounded-[10px] bg-accent text-accent-contrast"><IconFolderCog class="size-[18px]" /></span>
        <div class="min-w-0 flex-1">
          <div class="text-[15px] font-semibold tracking-[-.01em]">{{ headerTitle }}</div>
          <div class="truncate font-mono text-[12px] text-text-3">{{ headerSubtitle }}</div>
        </div>
        <button class="text-text-3 hover:text-text disabled:opacity-50" aria-label="Close" data-testid="agent-workspace-editor-close" :disabled="busy || confirming || discarding" @click="closeSheet"><IconX class="size-4" /></button>
      </div>
    </template>

    <!-- The schedule page takes the body over, the way the confirm strip takes
         the footer: one surface at a time, so nothing stacks and Escape has one
         thing to answer. -->
    <div
      v-if="scheduleDraft"
      class="flex flex-col gap-4 transition-opacity"
      :class="{ 'pointer-events-none opacity-45': scheduleDraft.removing }"
      data-testid="agent-workspace-editor-schedule-page"
    >
      <TextField
        ref="draftNameField"
        v-model="scheduleDraft.name"
        label="Name"
        :placeholder="scheduleDraft.saved ? scheduleDraft.id : 'Weekly product summary'"
        :hint="scheduleDraft.saved ? 'Optional. The id is what the schedule is called when this is empty.' : 'The id in agent-workspace.yaml follows the name.'"
        :disabled="busy"
        testid="agent-workspace-editor-schedule-name"
      />

      <div class="grid grid-cols-2 gap-3">
        <SelectField
          :model-value="scheduleDraft.shape.kind"
          label="Repeat"
          :options="REPEAT_OPTIONS"
          :disabled="busy"
          testid="agent-workspace-editor-schedule-repeat"
          @update:model-value="setRepeat"
        />
        <div v-if="scheduleDraft.shape.kind === 'hourly'" class="flex flex-col gap-1.5">
          <SelectField
            :model-value="minuteSelection"
            label="Minute"
            :options="MINUTE_OPTIONS"
            :disabled="busy"
            testid="agent-workspace-editor-schedule-minute"
            @update:model-value="setMinuteSelection"
          />
          <input
            v-if="scheduleDraft.minuteFree"
            type="number"
            min="0"
            max="59"
            :value="scheduleDraft.shape.minute"
            :disabled="busy"
            aria-label="Minute past the hour"
            class="w-full rounded-lg border border-strong bg-app px-3 py-2.5 text-[13.5px] text-text outline-none focus:border-accent"
            data-testid="agent-workspace-editor-schedule-minute-free"
            @input="onMinuteInput"
          >
        </div>
        <div v-else-if="scheduleDraft.shape.kind !== 'custom'">
          <div class="mb-1.5 text-[12.5px] text-text-2">Time</div>
          <input
            type="time"
            step="60"
            :value="clock(scheduleDraft.shape.hour, scheduleDraft.shape.minute)"
            :disabled="busy"
            aria-label="Time"
            class="w-full rounded-lg border border-strong bg-app px-3 py-2.5 text-[13.5px] text-text outline-none focus:border-accent"
            data-testid="agent-workspace-editor-schedule-time"
            @input="onTimeInput"
          >
        </div>
      </div>

      <div v-if="scheduleDraft.shape.kind === 'weekly'">
        <div class="mb-1.5 text-[12.5px] text-text-2">Days</div>
        <div class="flex flex-wrap gap-1">
          <button
            v-for="day in WEEK_ORDER"
            :key="day"
            type="button"
            class="cursor-pointer rounded-[7px] border px-2.5 py-1 text-[11.5px]"
            :class="dayPicked(day) ? 'border-accent bg-accent text-accent-contrast' : 'border-card text-text-2 hover:text-text'"
            :aria-pressed="dayPicked(day)"
            :disabled="busy"
            :data-testid="`agent-workspace-editor-schedule-day-${day}`"
            @click="toggleDay(day)"
          >{{ dayAbbreviation(day) }}</button>
        </div>
      </div>

      <SelectField
        v-if="scheduleDraft.shape.kind === 'monthly'"
        :model-value="String(scheduleDraft.shape.day)"
        label="Day of the month"
        :options="MONTH_DAY_OPTIONS"
        :disabled="busy"
        hint="A day past the 28th is skipped in months that are shorter."
        testid="agent-workspace-editor-schedule-month-day"
        @update:model-value="setMonthDay"
      />

      <TextField
        v-if="scheduleDraft.shape.kind === 'custom'"
        :model-value="scheduleDraft.shape.cron"
        label="Cron"
        monospace
        placeholder="0 9 * * 5"
        :disabled="busy"
        :error="scheduleDraft.cronError"
        hint="Five fields in local time, or @hourly / @daily / @weekly / @monthly."
        testid="agent-workspace-editor-schedule-cron"
        @update:model-value="setCron"
      />

      <!-- Always on screen once a preview can answer: the answer lands 300ms
           after a keystroke, and a line that appears then pushes the fields
           below it around. -->
      <p v-if="workspace" class="-mt-2 min-h-5 text-xs leading-relaxed text-text-4" data-testid="agent-workspace-editor-schedule-next-runs">{{ nextRunsLine }}</p>

      <SelectField
        v-model="scheduleDraft.onMissed"
        label="When missed"
        :options="ON_MISSED_OPTIONS"
        :disabled="busy"
        hint="What happens when the app was closed at the scheduled time."
        testid="agent-workspace-editor-schedule-on-missed"
      />

      <TextareaField
        :model-value="scheduleDraft.prompt"
        label="Prompt"
        :rows="6"
        monospace
        placeholder="Summarize what changed since the last run."
        :error="scheduleDraft.promptError"
        :hint="promptHint"
        testid="agent-workspace-editor-schedule-prompt"
        @update:model-value="setPrompt"
      />

      <SettingsError v-if="draftIssue" :message="draftIssue" testid="agent-workspace-editor-schedule-problem" />

      <div v-if="scheduleDraft.saved" class="flex flex-col gap-1.5">
        <span class="text-xs text-text-3">Recent runs</span>
        <p v-if="scheduleDraft.historyError" class="text-xs text-severity-error">{{ scheduleDraft.historyError }}</p>
        <p v-else-if="!scheduleDraft.historyLoaded" class="text-xs text-text-4">Loading…</p>
        <p v-else-if="!scheduleDraft.history.length" class="text-xs text-text-4">No runs yet.</p>
        <div v-else class="flex flex-col divide-y divide-row rounded-lg border border-strong bg-raised" data-testid="agent-workspace-editor-schedule-history">
          <div v-for="run in scheduleDraft.history" :key="run.id" class="flex items-start gap-2.5 px-3 py-2">
            <span class="mt-px shrink-0 font-mono text-[10.5px] text-text-4" :title="new Date(run.startedAt).toLocaleString()">{{ runStamp(run.startedAt) }}</span>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-1.5 text-[11.5px] text-text-2">
                <span class="size-1.5 shrink-0 rounded-full" :class="statusDot(run.status)" />
                <span>{{ run.status }} · {{ reasonLabel(run) }}</span>
                <span v-if="run.missed > 0" class="font-mono text-[10px] text-text-4">{{ run.missed }} missed</span>
              </div>
              <p v-if="run.error" class="mt-0.5 text-[10.5px] leading-relaxed text-severity-error">{{ run.error }}</p>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- While a delete is pending the form recedes: dimmed and inert, so the
         two states can't be misread for each other (FolderEditModal's rule). -->
    <div v-else class="flex flex-col gap-7 transition-opacity" :class="{ 'pointer-events-none opacity-45': confirming || discarding }">
      <!-- Above the open/reveal actions on purpose: the fix is in the file,
           and those two buttons are what reach it. -->
      <div
        v-if="manifestProblem"
        class="flex flex-col gap-1 rounded-md border border-severity-error-border bg-severity-error-tint px-3 py-2 text-xs leading-relaxed text-severity-error"
        data-testid="agent-workspace-editor-problem"
      >
        <p>{{ manifestProblem }}</p>
        <p>Fix agent-workspace.yaml before saving from here. This form could not read the file, so saving would rewrite its lists as empty. The buttons below open it.</p>
      </div>

      <div v-if="!creating" class="flex flex-col gap-1.5">
        <div class="flex items-center gap-2">
          <button
            v-if="editor.command"
            type="button"
            :class="inlineButtonClass"
            :disabled="busy"
            data-testid="agent-workspace-editor-open-editor"
            @click="openInEditor"
          ><IconExternalLink class="size-3.5" />Open in {{ editor.title }}</button>
          <button
            type="button"
            :class="inlineButtonClass"
            :disabled="busy"
            data-testid="agent-workspace-editor-reveal"
            @click="reveal"
          ><IconFolderOpen class="size-3.5" />Show in Finder</button>
        </div>
        <span v-if="!editor.command" class="text-xs text-text-4">Pick a default editor in Settings › General to open this directory in it.</span>
        <p v-if="actionError" class="text-xs text-severity-error" data-testid="agent-workspace-editor-action-error">{{ actionError }}</p>
      </div>

      <div class="flex flex-col gap-3">
        <FieldRow v-if="creating" label="Directory name" hint="A new directory under the workspace root, seeded with an AGENTS.md to shape.">
          <input
            ref="dirInput"
            v-model="dir"
            type="text"
            placeholder="my-project"
            autocapitalize="off"
            autocorrect="off"
            spellcheck="false"
            aria-label="Directory name"
            :disabled="busy"
            class="w-full rounded-lg border border-strong bg-app px-3 py-2.5 font-mono text-[13.5px] text-text outline-none placeholder:text-text-4 focus:border-accent disabled:opacity-60"
            data-testid="agent-workspace-editor-dir"
            @keydown.enter="submit"
          >
        </FieldRow>
        <TextField
          ref="nameInput"
          v-model="name"
          label="Name"
          :disabled="busy"
          testid="agent-workspace-editor-name"
          @keydown.enter="submit"
        />
      </div>

      <SettingsSection
        title="Command"
        description="What a chat in this workspace launches. A suggestion fills the field; Custom edits the template itself."
        testid="agent-workspace-editor-command"
      >
        <div class="flex flex-col gap-1.5">
          <AppSelect
            v-model="selection"
            :options="presetOptions"
            searchable
            search-placeholder="Search commands…"
            aria-label="Command"
            testid="agent-workspace-editor-command-preset"
            :disabled="busy"
          />
          <p
            v-if="selection !== CUSTOM"
            class="truncate font-mono text-[11px] text-text-4"
            :title="command"
            data-testid="agent-workspace-editor-command-preview"
          >{{ command }}</p>
          <template v-else>
            <textarea
              v-model="command"
              rows="4"
              spellcheck="false"
              :disabled="busy"
              placeholder="claude --session-id {{ .SessionID }}"
              class="w-full resize-y rounded-lg border bg-app px-3 py-2.5 font-mono text-[12px] leading-relaxed text-text outline-none focus:border-accent"
              :class="commandIsDangerous ? 'border-severity-warning' : 'border-strong'"
              data-testid="agent-workspace-editor-command-input"
            />
            <div class="flex flex-col gap-1 rounded-lg border border-card px-3 py-2.5" data-testid="agent-workspace-editor-command-fields">
              <p class="text-[11px] leading-relaxed text-text-3">Hive renders this as a Go template each time a chat starts, and fills in:</p>
              <dl class="flex flex-col gap-1">
                <div v-for="entry in TEMPLATE_FIELDS" :key="entry.field" class="flex flex-wrap items-baseline gap-x-2">
                  <dt class="shrink-0 font-mono text-[11px] text-text-2">{{ entry.field }}</dt>
                  <dd class="min-w-0 flex-1 text-[11px] leading-relaxed text-text-4">{{ entry.means }}</dd>
                </div>
              </dl>
            </div>
          </template>
          <p
            v-if="commandIsDangerous"
            class="flex items-start gap-1.5 text-[11.5px] leading-relaxed text-severity-warning"
            data-testid="agent-workspace-editor-command-danger"
          >
            <IconTriangleAlert class="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
            <span>This command bypasses the agent's permission prompts. Unattended, it can take any action your user account can, including through every enabled MCP server.</span>
          </p>
        </div>
      </SettingsSection>

      <SettingsSection
        title="MCP servers"
        description="The shared library in mcps.yaml; each switch is this workspace's own."
        testid="agent-workspace-editor-mcps"
      >
        <div :class="listCardClass">
          <div
            v-for="row in mcpRows"
            :key="row.id"
            class="flex items-start gap-3 px-4 py-3.5"
          >
            <AppSwitch
              class="mt-0.5"
              :model-value="mcpEnabled(row.id)"
              :aria-label="`Enable ${row.title}`"
              :disabled="busy"
              :testid="`agent-workspace-editor-mcp-${row.id}`"
              @update:model-value="toggleMCP(row.id)"
            />
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="truncate text-[13.5px] font-semibold" :class="mcpEnabled(row.id) ? 'text-text' : 'text-text-2'">{{ row.title }}</span>
                <BaseBadge
                  :tone="row.shipped ? 'neutral' : 'accent'"
                  variant="pill"
                  class="shrink-0 px-2 py-0.5 text-[10.5px] font-semibold uppercase"
                >{{ row.shipped ? row.stability : 'custom' }}</BaseBadge>
                <span v-if="row.shadows" class="shrink-0 text-[10.5px] text-severity-warning">replaces shipped</span>
              </div>
              <div v-if="row.command" class="mt-1 truncate font-mono text-[11.5px] text-text-4" :title="row.command">{{ row.command }}</div>
              <div v-if="row.problem" class="mt-1 text-[11.5px] text-severity-warning">{{ row.problem }}</div>
              <div v-if="row.missing" class="mt-1 text-[11.5px] text-severity-warning">not in the catalogue — enabled ids without an entry are skipped at launch</div>
            </div>
            <button
              v-if="!row.shipped && !row.missing"
              type="button"
              :class="[iconButtonClass, 'hover:text-severity-error']"
              :title="`Remove ${row.title} from mcps.yaml (every workspace loses it)`"
              :aria-label="`Remove ${row.title}`"
              :disabled="busy"
              :data-testid="`agent-workspace-editor-mcp-remove-${row.id}`"
              @click="removeServer(row.id)"
            ><IconTrash2 class="size-[15px]" /></button>
          </div>
          <p v-if="!mcpRows.length" class="px-4 py-3.5 text-xs leading-relaxed text-text-3">No servers in mcps.yaml yet.</p>
          <div v-if="importOpen" class="flex flex-col gap-2 px-4 py-3.5">
            <CodeField
              v-model="importText"
              :rows="8"
              :placeholder="importPlaceholder"
              testid="agent-workspace-editor-mcp-import-text"
            />
            <div class="flex items-center gap-2">
              <BaseButton size="sm" :busy="importBusy" :disabled="!importText.trim()" data-testid="agent-workspace-editor-mcp-import-submit" @click="submitImport">Add servers</BaseButton>
              <BaseButton
                variant="secondary"
                size="sm"
                :disabled="importBusy || !importText.trim()"
                data-testid="agent-workspace-editor-mcp-import-format"
                @click="formatImportJSON"
              >Format JSON</BaseButton>
              <BaseButton variant="secondary" size="sm" :disabled="importBusy" @click="importOpen = false">Cancel</BaseButton>
            </div>
            <span class="text-xs text-text-4">Pasted servers land in mcps.yaml — the library every workspace picks from — and switch on here.</span>
          </div>
          <button
            v-else
            type="button"
            :class="footerButtonClass"
            :disabled="busy"
            data-testid="agent-workspace-editor-mcp-import"
            @click="importOpen = true"
          ><IconPlus class="size-3.5" />Add servers from JSON…</button>
        </div>
        <p v-if="mcpError" class="text-xs text-severity-error" data-testid="agent-workspace-editor-mcp-error">{{ mcpError }}</p>
      </SettingsSection>

      <SettingsSection
        title="Skill packages"
        description="A package is glob patterns over skill names in skills.yml. Names come from the skills Hive ships and the SKILL.md files under .shared/skills."
        testid="agent-workspace-editor-skills"
      >
        <div :class="listCardClass">
          <div v-for="row in skillRows" :key="row.name">
            <div class="flex items-start gap-3 px-4 py-3.5">
              <AppSwitch
                class="mt-0.5"
                :model-value="skillEnabled(row.name)"
                :aria-label="`Enable ${row.title}`"
                :disabled="busy"
                :testid="`agent-workspace-editor-skill-${row.name}`"
                @update:model-value="toggleSkill(row.name)"
              />
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2">
                  <span class="truncate text-[13.5px] font-semibold" :class="skillEnabled(row.name) ? 'text-text' : 'text-text-2'">{{ row.title }}</span>
                  <button
                    v-if="!row.missing"
                    type="button"
                    class="flex shrink-0 cursor-pointer items-center gap-1 font-mono text-[11px] text-text-4 hover:text-text-2"
                    :aria-expanded="skillPackageExpanded(row.name)"
                    :data-testid="`agent-workspace-editor-skill-members-${row.name}`"
                    @click="toggleSkillPackageExpanded(row.name)"
                  >
                    {{ row.members.length }} {{ row.members.length === 1 ? 'skill' : 'skills' }}
                    <IconChevronDown class="size-3 transition-transform" :class="{ '-rotate-90': !skillPackageExpanded(row.name) }" />
                  </button>
                </div>
                <div v-if="row.description" class="mt-1 text-[12px] leading-relaxed text-text-3">{{ row.description }}</div>
                <div v-if="!row.missing && !row.members.length" class="mt-1 text-[11.5px] text-severity-warning">matches no skill — check its patterns in skills.yml</div>
                <div v-if="row.warning" class="mt-1 text-[11.5px] text-severity-warning">{{ row.warning }}</div>
              </div>
            </div>
            <ul v-if="skillPackageExpanded(row.name) && row.members.length" class="flex flex-col gap-1 border-t border-row pb-3 pl-[58px] pr-4 pt-2.5">
              <li v-for="member in row.members" :key="member.slug" class="flex items-center gap-2">
                <span class="truncate font-mono text-[11.5px] text-text-3">{{ member.slug }}</span>
                <BaseBadge tone="muted" variant="pill" class="shrink-0 px-2 py-0.5 text-[10.5px] font-medium">{{ member.shipped ? 'shipped' : 'custom' }}</BaseBadge>
              </li>
            </ul>
          </div>
          <p v-if="!skillRows.length" class="px-4 py-3.5 text-xs leading-relaxed text-text-3">No packages are defined yet.</p>
          <div class="flex divide-x divide-row">
            <button
              type="button"
              :class="footerButtonClass"
              :disabled="busy"
              data-testid="agent-workspace-editor-skills-packages"
              @click="openSkillPackages"
            ><IconPencil class="size-3.5" />Edit skills.yml…</button>
            <button
              type="button"
              :class="footerButtonClass"
              :disabled="busy"
              data-testid="agent-workspace-editor-skills-shared"
              @click="openSharedSkills"
            ><IconFolderOpen class="size-3.5" />Open the skills folder…</button>
          </div>
        </div>
        <p v-if="skillPackagesProblem" class="text-xs text-severity-error" data-testid="agent-workspace-editor-skills-problem">{{ skillPackagesProblem }}</p>
        <p v-if="skillError" class="text-xs text-severity-error" data-testid="agent-workspace-editor-skill-error">{{ skillError }}</p>
      </SettingsSection>

      <!-- Schedules are manifest state like the lists above, so they are part
           of this form and travel with its Save. -->
      <SettingsSection
        title="Schedules"
        description="A schedule starts a chat in this workspace on its own timetable. Its prompt is a Go template, so one schedule can ask for everything since the last run."
        testid="agent-workspace-editor-schedules"
      >
        <div :class="listCardClass">
          <div
            v-for="(card, index) in scheduleCards"
            :key="card.key"
            class="flex items-start gap-3 px-4 py-3.5"
            :data-testid="`agent-workspace-editor-schedule-${index}`"
          >
            <AppSwitch
              class="mt-0.5"
              :model-value="!card.disabled"
              :aria-label="card.disabled ? 'Enable this schedule' : 'Pause this schedule'"
              :disabled="busy"
              :testid="`agent-workspace-editor-schedule-${index}-enabled`"
              @update:model-value="(enabled) => (card.disabled = !enabled)"
            />
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="truncate text-[13.5px] font-semibold" :class="card.disabled ? 'text-text-2' : 'text-text'">{{ scheduleLabel(card) }}</span>
                <BaseBadge
                  v-if="!card.saved || card.edited"
                  tone="muted"
                  variant="pill"
                  class="shrink-0 px-2 py-0.5 text-[10.5px] font-medium"
                  :data-testid="`agent-workspace-editor-schedule-${index}-unsaved`"
                >unsaved</BaseBadge>
              </div>
              <div class="mt-1 truncate text-[12px] text-text-3">
                <span :data-testid="`agent-workspace-editor-schedule-${index}-summary`">{{ describe(card.shape) }}</span>
                <template v-if="nextRunLabel(card)">
                  <span aria-hidden="true"> · </span>
                  <span :data-testid="`agent-workspace-editor-schedule-${index}-next`">{{ nextRunLabel(card) }}</span>
                </template>
              </div>
              <div
                v-if="card.lastRun"
                class="mt-1 flex items-center gap-1.5 text-[11.5px] text-text-4"
                :data-testid="`agent-workspace-editor-schedule-${index}-last-run`"
              >
                <span class="size-1.5 shrink-0 rounded-full" :class="statusDot(card.lastRun.status)" />
                <span class="truncate">{{ lastRunLabel(card.lastRun) }}</span>
              </div>
              <p v-if="card.lastRun?.error" class="mt-1 text-[11.5px] leading-relaxed text-severity-warning">{{ card.lastRun.error }}</p>
              <p
                v-if="card.actionError"
                class="mt-1 text-[11.5px] leading-relaxed text-severity-error"
                :data-testid="`agent-workspace-editor-schedule-${index}-action-error`"
              >{{ card.actionError }}</p>
            </div>
            <div class="flex shrink-0 items-center gap-1">
              <button
                v-if="card.saved"
                type="button"
                :class="iconButtonClass"
                title="Run now"
                aria-label="Run now"
                :disabled="busy || card.running"
                :data-testid="`agent-workspace-editor-schedule-${index}-run`"
                @click="runScheduleNow(card)"
              ><IconPlay class="size-[15px]" /></button>
              <button
                type="button"
                :class="iconButtonClass"
                title="Edit this schedule"
                aria-label="Edit this schedule"
                :disabled="busy"
                :data-testid="`agent-workspace-editor-schedule-${index}-edit`"
                @click="openScheduleDraft(card, $event)"
              ><IconChevronRight class="size-4" /></button>
            </div>
          </div>
          <p v-if="!scheduleCards.length" class="px-4 py-3.5 text-xs leading-relaxed text-text-3">No schedules yet.</p>
          <button
            type="button"
            :class="footerButtonClass"
            :disabled="busy"
            data-testid="agent-workspace-editor-schedule-add"
            @click="openScheduleDraft(null, $event)"
          ><IconPlus class="size-3.5" />Add schedule</button>
        </div>
      </SettingsSection>

      <p v-if="!creating" class="border-t border-border pt-4 text-xs leading-relaxed text-text-4">
        Saving rewrites these fields in agent-workspace.yaml and re-syncs the workspace's
        generated files. Comments and anything else in the file stay as written — edit the
        file for those.
      </p>
      <SettingsError v-if="error && !confirming" :message="error" testid="agent-workspace-editor-error" />
    </div>

    <template #footer>
      <!-- The strip escapes the footer's own padding so it reads as the
           sheet's bottom edge, the way InlineConfirm is designed to sit. -->
      <InlineConfirm
        v-if="scheduleDraft?.removing"
        class="-mx-[18px] -my-[13px]"
        title="Remove this schedule?"
        description="It leaves agent-workspace.yaml when you save the workspace."
        confirm-label="Remove"
        testid="agent-workspace-editor-schedule-remove-confirm"
        @confirm="removeScheduleDraft"
        @cancel="scheduleDraft.removing = false"
      />
      <div v-else-if="scheduleDraft" class="flex items-center gap-2.5">
        <button
          v-if="scheduleDraft.key !== null"
          type="button"
          class="cursor-pointer text-[12.5px] text-text-3 hover:text-severity-error disabled:opacity-50"
          :disabled="busy"
          data-testid="agent-workspace-editor-schedule-remove"
          @click="scheduleDraft.removing = true"
        >Remove schedule</button>
        <div class="flex-1" />
        <BaseButton variant="secondary" size="sm" :disabled="busy" data-testid="agent-workspace-editor-schedule-cancel" @click="closeScheduleDraft">Cancel</BaseButton>
        <BaseButton size="sm" :disabled="busy" data-testid="agent-workspace-editor-schedule-keep" @click="keepScheduleDraft">
          {{ scheduleDraft.key === null ? 'Add schedule' : 'Done' }}
        </BaseButton>
      </div>
      <InlineConfirm
        v-else-if="confirming"
        class="-mx-[18px] -my-[13px]"
        title="Delete this workspace?"
        :description="`Every live chat in ${workspace!.name || workspace!.dir} closes, its chat history is removed, and ${deletedPath} is deleted from disk with everything in it — AGENTS.md, docs, canvases, and anything else written there. This cannot be undone.`"
        confirm-label="Delete"
        :busy="busy"
        :error="error"
        testid="agent-workspace-editor-delete-confirm"
        @confirm="emit('delete', workspace!.dir)"
        @cancel="confirming = false"
      />
      <InlineConfirm
        v-else-if="discarding"
        class="-mx-[18px] -my-[13px]"
        title="Discard your changes?"
        description="Nothing here is written until you save. Closing now leaves agent-workspace.yaml exactly as it was."
        confirm-label="Discard"
        cancel-label="Keep editing"
        testid="agent-workspace-editor-discard-confirm"
        @confirm="emit('close')"
        @cancel="discarding = false"
      />
      <div v-else class="flex items-center gap-2.5">
        <button
          v-if="!creating"
          type="button"
          class="cursor-pointer text-[12.5px] text-text-3 hover:text-severity-error disabled:opacity-50"
          :disabled="busy"
          data-testid="agent-workspace-editor-delete"
          @click="confirming = true"
        >Delete workspace</button>
        <div class="flex-1" />
        <BaseButton variant="secondary" size="sm" :busy="busy" data-testid="agent-workspace-editor-cancel" @click="cancel">Cancel</BaseButton>
        <BaseButton size="sm" :busy="busy" :disabled="!valid" data-testid="agent-workspace-editor-save" @click="submit">
          {{ creating ? 'Create workspace' : 'Save' }}
        </BaseButton>
      </div>
    </template>
  </DrawerSheet>
</template>
