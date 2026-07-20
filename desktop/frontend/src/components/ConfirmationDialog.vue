<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import IconAlertTriangle from '~icons/lucide/alert-triangle'
import IconX from '~icons/lucide/x'

const props = withDefaults(defineProps<{
  title: string
  description: string
  confirmLabel?: string
  busy?: boolean
  error?: string | null
  testid?: string
  confirmTestid?: string
  cancelTestid?: string
}>(), { confirmLabel: 'Confirm', busy: false, error: null, testid: 'confirmation-dialog', confirmTestid: undefined, cancelTestid: undefined })
const emit = defineEmits<{ confirm: []; cancel: [] }>()
const confirmRef = ref<HTMLButtonElement | null>(null)

function cancel(): void { if (!props.busy) emit('cancel') }
function onKeydown(event: KeyboardEvent): void { if (event.key === 'Escape') cancel() }
onMounted(async () => { window.addEventListener('keydown', onKeydown); await nextTick(); confirmRef.value?.focus() })
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-40 flex items-start justify-center bg-backdrop pt-[24vh]" :data-testid="`${testid}-backdrop`" @click.self="cancel">
      <div class="w-[420px] overflow-hidden rounded-xl border border-strong bg-pane text-text shadow-2xl" role="alertdialog" :aria-label="title" aria-modal="true" :data-testid="testid">
        <header class="flex items-center gap-3 border-b border-row px-5 py-4">
          <span class="flex size-7 items-center justify-center rounded-[7px] bg-severity-error-tint text-severity-error"><IconAlertTriangle class="size-4" /></span>
          <div class="flex-1 text-[15px] font-semibold tracking-[-.01em]">{{ title }}</div>
          <button class="cursor-pointer text-text-3 hover:text-text disabled:cursor-default disabled:opacity-50" aria-label="Close" :disabled="busy" @click="cancel"><IconX class="size-4" /></button>
        </header>
        <div class="flex flex-col gap-3 px-5 py-4"><p class="text-[13px] leading-relaxed text-text-2">{{ description }}</p><p v-if="error" class="rounded border border-severity-error bg-severity-error-tint px-3 py-2 text-xs text-severity-error" :data-testid="`${testid}-error`">{{ error }}</p></div>
        <footer class="flex gap-2.5 border-t border-row bg-raised px-5 py-3.5">
          <button ref="confirmRef" class="flex-1 cursor-pointer rounded-lg bg-severity-error px-4 py-2.5 text-[13.5px] font-semibold text-accent-contrast hover:brightness-110 disabled:cursor-default disabled:opacity-50" :disabled="busy" :data-testid="confirmTestid ?? `${testid}-confirm`" @click="emit('confirm')">{{ busy ? 'Working…' : confirmLabel }}</button>
          <button class="cursor-pointer rounded-lg border border-card px-4 py-2.5 text-[13.5px] text-text-2 hover:text-text disabled:cursor-default disabled:opacity-50" :disabled="busy" :data-testid="cancelTestid ?? `${testid}-cancel`" @click="cancel">Cancel</button>
        </footer>
      </div>
    </div>
  </Teleport>
</template>
