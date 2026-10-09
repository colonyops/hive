<script setup lang="ts">
import { onClickOutside } from '@vueuse/core'
import { computed, nextTick, ref, watch } from 'vue'
import IconClock3 from '~icons/lucide/clock-3'
import { useAnchoredPopover } from '../composables/useAnchoredPopover'
import { useEscapeToClose } from '../composables/useEscapeToClose'
import AppDateTimePicker from './ui/AppDateTimePicker.vue'
import BaseButton from './ui/BaseButton.vue'
import InlineError from './ui/InlineError.vue'

const period = defineModel<string>('period', { required: true })
const since = defineModel<string>('since', { required: true })
const until = defineModel<string>('until', { required: true })
const open = ref(false)
const anchor = ref<HTMLElement | null>(null)
const trigger = ref<{ focus: () => void } | null>(null)
const popover = ref<HTMLElement | null>(null)
const draftSince = ref('')
const draftUntil = ref('')
const rangeError = ref('')
const { style, measure } = useAnchoredPopover(anchor, popover, open)

const label = computed(() => {
  if (since.value || until.value) return 'Custom range'
  return (
    { '15': 'Last 15 minutes', '60': 'Last hour', '1440': 'Last day', all: 'Retained history' }[period.value] ??
    'Time range'
  )
})

function close(): void {
  open.value = false
  trigger.value?.focus()
}

async function toggle(): Promise<void> {
  open.value = !open.value
  if (!open.value) return
  draftSince.value = since.value
  draftUntil.value = until.value
  rangeError.value = ''
  await nextTick()
  measure()
  await nextTick()
  measure()
}

function choose(value: string): void {
  period.value = value
  since.value = ''
  until.value = ''
  close()
}

function apply(): void {
  const start = draftSince.value ? new Date(draftSince.value) : null
  const end = draftUntil.value ? new Date(draftUntil.value) : null
  if (!start && !end) {
    rangeError.value = 'Set a start or end time.'
    return
  }
  if ((start && Number.isNaN(start.getTime())) || (end && Number.isNaN(end.getTime()))) {
    rangeError.value = 'Enter a valid date and time.'
    return
  }
  if (start && end && start > end) {
    rangeError.value = 'From must be before Through.'
    return
  }
  since.value = draftSince.value
  until.value = draftUntil.value
  close()
}

watch([draftSince, draftUntil], () => {
  rangeError.value = ''
})
onClickOutside(
  popover,
  (event) => {
    const target = event.target
    if (target instanceof Element && target.closest('[data-app-date-time-popover]')) return
    close()
  },
  { ignore: [anchor] },
)
useEscapeToClose(close, { enabled: open })
</script>

<template>
  <span ref="anchor">
    <BaseButton
      ref="trigger"
      variant="secondary"
      size="xs"
      data-testid="diagnostics-time-range-toggle"
      :aria-expanded="open"
      aria-haspopup="dialog"
      @click="toggle"
    >
      <template #icon><IconClock3 class="size-3" /></template>
      {{ label }}
    </BaseButton>
  </span>

  <Teleport to="body">
    <section
      v-if="open"
      ref="popover"
      class="fixed z-40 w-80 overflow-auto rounded-xl border border-strong bg-pane p-3 shadow-popover"
      :style="style"
      role="dialog"
      aria-label="Choose diagnostic time range"
      data-testid="diagnostics-time-range"
    >
      <h3 class="mb-2 font-medium text-text">Time range</h3>
      <div class="grid grid-cols-2 gap-1.5">
        <BaseButton variant="secondary" size="xs" @click="choose('15')">Last 15 minutes</BaseButton>
        <BaseButton variant="secondary" size="xs" @click="choose('60')">Last hour</BaseButton>
        <BaseButton variant="secondary" size="xs" @click="choose('1440')">Last day</BaseButton>
        <BaseButton variant="secondary" size="xs" @click="choose('all')">Retained history</BaseButton>
      </div>
      <div class="my-3 border-t border-row" />
      <div class="space-y-2">
        <AppDateTimePicker v-model="draftSince" label="From" testid="diagnostics-time-from" />
        <AppDateTimePicker v-model="draftUntil" label="Through" testid="diagnostics-time-through" />
        <InlineError v-if="rangeError" :message="rangeError" variant="line" testid="diagnostics-time-error" />
      </div>
      <div class="mt-3 flex justify-end gap-2">
        <BaseButton variant="ghost" size="xs" @click="close">Cancel</BaseButton>
        <BaseButton size="xs" data-testid="diagnostics-time-apply" @click="apply">Apply range</BaseButton>
      </div>
    </section>
  </Teleport>
</template>
