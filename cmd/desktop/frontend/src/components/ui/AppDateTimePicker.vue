<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { onClickOutside } from '@vueuse/core'
import IconCalendar from '~icons/lucide/calendar-days'
import IconChevronLeft from '~icons/lucide/chevron-left'
import IconChevronRight from '~icons/lucide/chevron-right'
import { useAnchoredPopover } from '../../composables/useAnchoredPopover'
import { useEscapeToClose } from '../../composables/useEscapeToClose'
import BaseButton from './BaseButton.vue'
import FormField from './FormField.vue'
import IconButton from './IconButton.vue'
import TextInput from './TextInput.vue'

const props = defineProps<{
  modelValue: string
  label: string
  testid: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const root = ref<HTMLElement | null>(null)
const calendar = ref<HTMLElement | null>(null)
const trigger = ref<{ focus: () => void } | null>(null)
const open = ref(false)
const { style, measure } = useAnchoredPopover(root, calendar, open)
const viewMonth = ref(startOfMonth(new Date()))
const timeDraft = ref(formatTime(new Date()))

const selected = computed(() => parseLocal(props.modelValue))
const displayValue = computed(() => {
  if (!selected.value) return 'Choose date and time'
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(selected.value)
})
const monthLabel = computed(() =>
  new Intl.DateTimeFormat(undefined, { month: 'long', year: 'numeric' }).format(viewMonth.value),
)
const days = computed(() => {
  const first = startOfMonth(viewMonth.value)
  const cursor = new Date(first)
  cursor.setDate(cursor.getDate() - cursor.getDay())
  return Array.from({ length: 42 }, (_, index) => {
    const date = new Date(cursor)
    date.setDate(cursor.getDate() + index)
    return date
  })
})
watch(
  () => props.modelValue,
  (value) => {
    const date = parseLocal(value)
    if (date) {
      viewMonth.value = startOfMonth(date)
      timeDraft.value = formatTime(date)
    }
  },
  { immediate: true },
)

onClickOutside(root, () => close(false), { ignore: [calendar] })
useEscapeToClose(() => close(true), { enabled: open })

async function toggle(): Promise<void> {
  open.value = !open.value
  if (!open.value) return
  viewMonth.value = startOfMonth(selected.value ?? new Date())
  await nextTick()
  measure()
  await nextTick()
  measure()
}

function close(restoreFocus: boolean): void {
  if (!open.value) return
  open.value = false
  if (restoreFocus) void nextTick(() => trigger.value?.focus())
}

function moveMonth(offset: number): void {
  const next = new Date(viewMonth.value)
  next.setMonth(next.getMonth() + offset)
  viewMonth.value = startOfMonth(next)
}

function chooseDay(day: Date): void {
  const value = selected.value ?? new Date()
  value.setFullYear(day.getFullYear(), day.getMonth(), day.getDate())
  emit('update:modelValue', formatLocal(value))
}

function finish(): void {
  commitTime()
  close(true)
}

function commitTime(): void {
  const match = /^(\d{1,2}):(\d{2})$/.exec(timeDraft.value.trim())
  const nextHour = Number(match?.[1])
  const nextMinute = Number(match?.[2])
  if (!match || nextHour > 23 || nextMinute > 59) {
    timeDraft.value = formatTime(selected.value ?? new Date())
    return
  }
  const value = selected.value ?? new Date()
  value.setHours(nextHour, nextMinute, 0, 0)
  emit('update:modelValue', formatLocal(value))
}

function isSameDay(left: Date | null, right: Date): boolean {
  return (
    !!left &&
    left.getFullYear() === right.getFullYear() &&
    left.getMonth() === right.getMonth() &&
    left.getDate() === right.getDate()
  )
}

function parseLocal(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(value)
  if (!match) return null
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]), Number(match[4]), Number(match[5]))
  if (
    date.getFullYear() !== Number(match[1]) ||
    date.getMonth() !== Number(match[2]) - 1 ||
    date.getDate() !== Number(match[3]) ||
    date.getHours() !== Number(match[4]) ||
    date.getMinutes() !== Number(match[5])
  ) {
    return null
  }
  return date
}

function formatLocal(date: Date): string {
  const part = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}-${part(date.getMonth() + 1)}-${part(date.getDate())}T${part(date.getHours())}:${part(date.getMinutes())}`
}

function formatTime(date: Date): string {
  return `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}

function startOfMonth(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), 1)
}
</script>

<template>
  <div ref="root">
    <FormField v-slot="{ id }" :label="label" :testid="testid">
      <BaseButton
        :id="id"
        ref="trigger"
        variant="secondary"
        size="sm"
        class="w-full justify-start text-left font-normal"
        :aria-expanded="open"
        :data-testid="`${testid}-toggle`"
        @click="toggle"
      >
        <template #icon><IconCalendar class="size-4 text-text-4" /></template>
        {{ displayValue }}
      </BaseButton>
    </FormField>

    <Teleport to="body">
      <div
        v-if="open"
        ref="calendar"
        class="fixed z-50 w-72 overflow-auto rounded-lg border border-card bg-raised p-2 shadow-popover"
        :style="style"
        data-app-date-time-popover
        :data-testid="`${testid}-calendar`"
      >
        <div class="mb-2 flex items-center">
          <IconButton label="Previous month" :icon="IconChevronLeft" size="md" @click="moveMonth(-1)" />
          <div class="flex-1 text-center text-small font-medium text-text">{{ monthLabel }}</div>
          <IconButton label="Next month" :icon="IconChevronRight" size="md" @click="moveMonth(1)" />
        </div>
        <div class="grid grid-cols-7 gap-1 text-center text-micro text-text-4" aria-hidden="true">
          <span v-for="(name, index) in ['S', 'M', 'T', 'W', 'T', 'F', 'S']" :key="index">{{ name }}</span>
        </div>
        <div class="mt-1 grid grid-cols-7 gap-1" role="grid">
          <BaseButton
            v-for="day in days"
            :key="day.toISOString()"
            variant="ghost"
            size="xs"
            class="size-7 !p-0 text-small"
            :class="[
              day.getMonth() === viewMonth.getMonth() ? 'text-text-2' : 'text-text-4',
              isSameDay(selected, day) ? '!bg-accent !text-accent-contrast' : '',
            ]"
            :aria-label="day.toLocaleDateString(undefined, { dateStyle: 'full' })"
            :aria-selected="isSameDay(selected, day)"
            :data-testid="`${testid}-day`"
            @click="chooseDay(day)"
          >
            {{ day.getDate() }}
          </BaseButton>
        </div>
        <div class="mt-2 flex items-center gap-2 border-t border-row pt-2">
          <span class="text-caption text-text-4">Time</span>
          <TextInput
            v-model="timeDraft"
            class="w-24"
            size="sm"
            monospace
            inputmode="numeric"
            maxlength="5"
            placeholder="HH:mm"
            aria-label="Time in 24-hour format"
            :data-testid="`${testid}-time`"
            @blur="commitTime"
            @keydown.enter="commitTime"
          />
          <BaseButton variant="ghost" size="xs" class="ml-auto" @click="finish">Done</BaseButton>
        </div>
      </div>
    </Teleport>
  </div>
</template>
