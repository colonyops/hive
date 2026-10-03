<script setup lang="ts">
// One schedule, on the page that takes the workspace sheet over. Cron is what
// the manifest stores, but the page never asks for one: it edits a
// ScheduleShape (lib/scheduleShape.ts), with the cron box reserved for
// expressions no shape can state.
import { computed, ref } from 'vue'
import FormField from '../ui/FormField.vue'
import InlineError from '../ui/InlineError.vue'
import TextInput from '../ui/TextInput.vue'
import { SelectField, TextareaField, TextField, type SelectOption } from '../../pipeline/fields'
import { useAutofocus } from '../../composables/useAutofocus'
import { QUARTER_MINUTES, type ScheduleDraft } from '../../composables/useScheduleDraft'
import { buildCron, clock, dayAbbreviation, pad, WEEK_ORDER, type ScheduleShape } from '../../lib/scheduleShape'
import { occurrence, reasonLabel, runStamp, statusDot } from '../../lib/scheduleRuns'

defineProps<{
  busy?: boolean
  /** What blocks Done, once the user has tried it. */
  issue: string
  /** False while the workspace is being created: there is nothing on disk to preview against. */
  previewable: boolean
}>()
const draft = defineModel<ScheduleDraft>('draft', { required: true })

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

const nameField = ref<{ focus: () => void } | null>(null)
useAutofocus(nameField)

function shapeClock(shape: ScheduleShape): { hour: number; minute: number } {
  if (shape.kind === 'custom') return { hour: 9, minute: 0 }
  if (shape.kind === 'hourly') return { hour: 9, minute: shape.minute }
  return { hour: shape.hour, minute: shape.minute }
}

function setRepeat(kind: string): void {
  const shape = draft.value.shape
  if (kind === shape.kind) return
  const { hour, minute } = shapeClock(shape)
  switch (kind) {
    case 'hourly':
      draft.value.shape = { kind: 'hourly', minute }
      draft.value.minuteFree = !QUARTER_MINUTES.includes(minute)
      break
    case 'daily':
      draft.value.shape = { kind: 'daily', hour, minute }
      break
    case 'weekly':
      draft.value.shape = { kind: 'weekly', days: [5], hour, minute }
      break
    case 'monthly':
      draft.value.shape = { kind: 'monthly', day: 1, hour, minute }
      break
    // Switching to Custom carries the compiled expression over, so the box
    // opens on what the picker was already saying rather than empty.
    default:
      draft.value.shape = { kind: 'custom', cron: buildCron(shape) }
  }
}

function dayPicked(day: number): boolean {
  const shape = draft.value.shape
  return shape.kind === 'weekly' && shape.days.includes(day)
}

function toggleDay(day: number): void {
  const shape = draft.value.shape
  if (shape.kind !== 'weekly') return
  const days = dayPicked(day)
    ? shape.days.filter((picked) => picked !== day)
    : [...shape.days, day].sort((a, b) => a - b)
  draft.value.shape = { ...shape, days }
}

function onTimeInput(value: string): void {
  const shape = draft.value.shape
  const match = /^(\d{1,2}):(\d{2})/.exec(value)
  if (!match || shape.kind === 'custom' || shape.kind === 'hourly') return
  draft.value.shape = { ...shape, hour: Number(match[1]), minute: Number(match[2]) }
}

const minuteSelection = computed(() => {
  const { shape, minuteFree } = draft.value
  if (shape.kind !== 'hourly') return '0'
  return minuteFree ? 'other' : String(shape.minute)
})

function setMinuteSelection(value: string): void {
  draft.value.minuteFree = value === 'other'
  if (value !== 'other') setMinute(value)
}

function setMinute(value: string): void {
  const minute = Number(value)
  if (draft.value.shape.kind !== 'hourly' || !Number.isFinite(minute)) return
  draft.value.shape = { kind: 'hourly', minute: Math.min(59, Math.max(0, Math.trunc(minute))) }
}

function setMonthDay(value: string): void {
  const shape = draft.value.shape
  if (shape.kind !== 'monthly') return
  draft.value.shape = { ...shape, day: Number(value) }
}

const nextRunsLine = computed(() => {
  const { previewed, cronError, next } = draft.value
  if (!previewed || cronError) return ''
  return next.length ? `Next: ${next.map(occurrence).join(' · ')}` : 'No upcoming runs.'
})
</script>

<template>
  <div
    class="flex flex-col gap-4 transition-opacity"
    :class="{ 'pointer-events-none opacity-45': draft.removing }"
    data-testid="agent-workspace-editor-schedule-page"
  >
    <TextField
      ref="nameField"
      v-model="draft.name"
      label="Name"
      :placeholder="draft.saved ? draft.id : 'Weekly product summary'"
      :hint="
        draft.saved
          ? 'Optional. The id is what the schedule is called when this is empty.'
          : 'The id in agent-workspace.yaml follows the name.'
      "
      :disabled="busy"
      testid="agent-workspace-editor-schedule-name"
    />

    <div class="grid grid-cols-2 gap-3">
      <SelectField
        :model-value="draft.shape.kind"
        label="Repeat"
        :options="REPEAT_OPTIONS"
        :disabled="busy"
        testid="agent-workspace-editor-schedule-repeat"
        @update:model-value="setRepeat"
      />
      <div v-if="draft.shape.kind === 'hourly'" class="flex flex-col gap-1.5">
        <SelectField
          :model-value="minuteSelection"
          label="Minute"
          :options="MINUTE_OPTIONS"
          :disabled="busy"
          testid="agent-workspace-editor-schedule-minute"
          @update:model-value="setMinuteSelection"
        />
        <TextInput
          v-if="draft.minuteFree"
          type="number"
          min="0"
          max="59"
          :model-value="String(draft.shape.minute)"
          :disabled="busy"
          aria-label="Minute past the hour"
          data-testid="agent-workspace-editor-schedule-minute-free"
          @update:model-value="setMinute"
        />
      </div>
      <FormField v-else-if="draft.shape.kind !== 'custom'" v-slot="{ id }" label="Time">
        <TextInput
          :id="id"
          type="time"
          step="60"
          :model-value="clock(draft.shape.hour, draft.shape.minute)"
          :disabled="busy"
          data-testid="agent-workspace-editor-schedule-time"
          @update:model-value="onTimeInput"
        />
      </FormField>
    </div>

    <FormField v-if="draft.shape.kind === 'weekly'" label="Days">
      <div class="flex flex-wrap gap-1">
        <button
          v-for="day in WEEK_ORDER"
          :key="day"
          type="button"
          class="cursor-pointer rounded-lg border px-2.5 py-1 text-caption"
          :class="
            dayPicked(day) ? 'border-accent bg-accent text-accent-contrast' : 'border-card text-text-2 hover:text-text'
          "
          :aria-pressed="dayPicked(day)"
          :disabled="busy"
          :data-testid="`agent-workspace-editor-schedule-day-${day}`"
          @click="toggleDay(day)"
        >
          {{ dayAbbreviation(day) }}
        </button>
      </div>
    </FormField>

    <SelectField
      v-if="draft.shape.kind === 'monthly'"
      :model-value="String(draft.shape.day)"
      label="Day of the month"
      :options="MONTH_DAY_OPTIONS"
      :disabled="busy"
      hint="A day past the 28th is skipped in months that are shorter."
      testid="agent-workspace-editor-schedule-month-day"
      @update:model-value="setMonthDay"
    />

    <TextField
      v-if="draft.shape.kind === 'custom'"
      v-model="draft.shape.cron"
      label="Cron"
      monospace
      placeholder="0 9 * * 5"
      :disabled="busy"
      :error="draft.cronError"
      hint="Five fields in local time, or @hourly / @daily / @weekly / @monthly."
      testid="agent-workspace-editor-schedule-cron"
    />

    <!-- Always on screen once a preview can answer: the answer lands 300ms
         after a keystroke, and a line that appears then pushes the fields
         below it around. -->
    <p
      v-if="previewable"
      class="-mt-2 min-h-5 text-xs leading-relaxed text-text-4"
      data-testid="agent-workspace-editor-schedule-next-runs"
    >
      {{ nextRunsLine }}
    </p>

    <SelectField
      v-model="draft.onMissed"
      label="When missed"
      :options="ON_MISSED_OPTIONS"
      :disabled="busy"
      hint="What happens when the app was closed at the scheduled time."
      testid="agent-workspace-editor-schedule-on-missed"
    />

    <TextareaField
      v-model="draft.prompt"
      label="Prompt"
      :rows="6"
      monospace
      placeholder="Summarize what changed since the last run."
      :error="draft.promptError"
      :hint="promptHint"
      testid="agent-workspace-editor-schedule-prompt"
    />

    <InlineError v-if="issue" :message="issue" testid="agent-workspace-editor-schedule-problem" />

    <div v-if="draft.saved" class="flex flex-col gap-1.5">
      <span class="text-xs text-text-3">Recent runs</span>
      <InlineError v-if="draft.historyError" variant="line" :message="draft.historyError" />
      <p v-else-if="!draft.historyLoaded" class="text-xs text-text-4">Loading…</p>
      <p v-else-if="!draft.history.length" class="text-xs text-text-4">No runs yet.</p>
      <div
        v-else
        class="flex flex-col divide-y divide-row rounded-lg border border-strong bg-raised"
        data-testid="agent-workspace-editor-schedule-history"
      >
        <div v-for="run in draft.history" :key="run.id" class="flex items-start gap-2.5 px-3 py-2">
          <span
            class="mt-px shrink-0 font-mono text-micro text-text-4"
            :title="new Date(run.startedAt).toLocaleString()"
            >{{ runStamp(run.startedAt) }}</span
          >
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-1.5 text-caption text-text-2">
              <span class="size-1.5 shrink-0 rounded-full" :class="statusDot(run.status)" />
              <span>{{ run.status }} · {{ reasonLabel(run) }}</span>
              <span v-if="run.missed > 0" class="font-mono text-micro text-text-4">{{ run.missed }} missed</span>
            </div>
            <p v-if="run.error" class="mt-0.5 text-micro leading-relaxed text-severity-error">{{ run.error }}</p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
