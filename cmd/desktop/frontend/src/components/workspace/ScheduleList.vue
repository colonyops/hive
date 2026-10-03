<script setup lang="ts">
import IconChevronRight from '~icons/lucide/chevron-right'
import IconPlay from '~icons/lucide/play'
import IconPlus from '~icons/lucide/plus'
import AppSwitch from '../ui/AppSwitch.vue'
import BaseBadge from '../ui/BaseBadge.vue'
import EmptyState from '../ui/EmptyState.vue'
import InlineError from '../ui/InlineError.vue'
import SettingsSection from '../settings/SettingsSection.vue'
import ListFooterButton from './ListFooterButton.vue'
import type { ScheduleCard } from '../../composables/useScheduleDraft'
import { describe } from '../../lib/scheduleShape'
import { lastRunLabel, nextRunLabel, statusDot } from '../../lib/scheduleRuns'

defineProps<{ busy?: boolean }>()
const cards = defineModel<ScheduleCard[]>('cards', { required: true })
const emit = defineEmits<{
  open: [card: ScheduleCard | null, event: Event]
  run: [card: ScheduleCard]
}>()

const iconButtonClass =
  'flex size-7 shrink-0 cursor-pointer items-center justify-center rounded-md text-text-3 hover:bg-chip hover:text-text disabled:cursor-not-allowed disabled:opacity-40'
</script>

<template>
  <SettingsSection
    title="Schedules"
    description="A schedule starts a chat in this workspace on its own timetable. Its prompt is a Go template, so one schedule can ask for everything since the last run."
    boxed
    testid="agent-workspace-editor-schedules"
  >
    <div
      v-for="(card, index) in cards"
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
          <span class="truncate text-body font-semibold" :class="card.disabled ? 'text-text-2' : 'text-text'">{{
            card.name || card.id
          }}</span>
          <BaseBadge
            v-if="!card.saved || card.edited"
            tone="muted"
            variant="pill"
            class="shrink-0 px-2 py-0.5 text-micro font-medium"
            :data-testid="`agent-workspace-editor-schedule-${index}-unsaved`"
            >unsaved</BaseBadge
          >
        </div>
        <div class="mt-1 truncate text-small text-text-3">
          <span :data-testid="`agent-workspace-editor-schedule-${index}-summary`">{{ describe(card.shape) }}</span>
          <template v-if="nextRunLabel(card)">
            <span aria-hidden="true"> · </span>
            <span :data-testid="`agent-workspace-editor-schedule-${index}-next`">{{ nextRunLabel(card) }}</span>
          </template>
        </div>
        <div
          v-if="card.lastRun"
          class="mt-1 flex items-center gap-1.5 text-caption text-text-4"
          :data-testid="`agent-workspace-editor-schedule-${index}-last-run`"
        >
          <span class="size-1.5 shrink-0 rounded-full" :class="statusDot(card.lastRun.status)" />
          <span class="truncate">{{ lastRunLabel(card.lastRun) }}</span>
        </div>
        <p v-if="card.lastRun?.error" class="mt-1 text-caption leading-relaxed text-severity-warning">
          {{ card.lastRun.error }}
        </p>
        <InlineError
          v-if="card.actionError"
          :testid="`agent-workspace-editor-schedule-${index}-action-error`"
          variant="line"
          class="mt-1 leading-relaxed"
          :message="card.actionError"
        />
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
          @click="emit('run', card)"
        >
          <IconPlay class="size-[15px]" />
        </button>
        <button
          type="button"
          :class="iconButtonClass"
          title="Edit this schedule"
          aria-label="Edit this schedule"
          :disabled="busy"
          :data-testid="`agent-workspace-editor-schedule-${index}-edit`"
          @click="emit('open', card, $event)"
        >
          <IconChevronRight class="size-4" />
        </button>
      </div>
    </div>
    <EmptyState v-if="!cards.length" variant="inline" class="px-4 py-3.5" message="No schedules yet." />
    <ListFooterButton
      :disabled="busy"
      data-testid="agent-workspace-editor-schedule-add"
      @click="emit('open', null, $event)"
    >
      <IconPlus class="size-3.5" />Add schedule
    </ListFooterButton>
  </SettingsSection>
</template>
