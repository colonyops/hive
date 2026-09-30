<script setup lang="ts">
import { computed, ref, type Component } from 'vue'
import IconX from '~icons/lucide/x'
import { useEscapeToClose } from '../composables/useEscapeToClose'
import { useFocusTrap } from '../composables/useFocusTrap'
import { useRegisterOpenModal } from '../composables/useOpenModalCount'
import { useReturnFocus } from '../composables/useReturnFocus'

const props = withDefaults(defineProps<{
  title: string
  icon?: Component
  tone?: 'accent' | 'danger'
  width?: number
  ariaRole?: 'dialog' | 'alertdialog'
  busy?: boolean
  closeOnBackdrop?: boolean
  closeOnEscape?: boolean
  testid?: string
}>(), {
  tone: 'accent',
  width: 420,
  ariaRole: 'dialog',
  busy: false,
  closeOnBackdrop: true,
  closeOnEscape: true,
})

const emit = defineEmits<{ close: [] }>()

const dialog = ref<HTMLElement | null>(null)

const badgeClasses = computed(() => props.tone === 'danger'
  ? 'bg-severity-error-tint text-severity-error'
  : 'bg-accent-tint text-accent')

function close(): void {
  emit('close')
}

function onBackdropClick(): void {
  if (props.closeOnBackdrop && !props.busy) close()
}

useEscapeToClose(close, { enabled: () => props.closeOnEscape && !props.busy })
const { onKeydown: trapFocus } = useFocusTrap(dialog)
useReturnFocus()
useRegisterOpenModal()
</script>

<template>
  <Teleport to="body">
    <div
      class="fixed inset-0 z-40 flex items-center justify-center bg-backdrop py-[6vh]"
      :data-testid="testid ? `${testid}-backdrop` : undefined"
      @click.self="onBackdropClick"
    >
      <div
        ref="dialog"
        class="flex max-h-full flex-col overflow-hidden rounded-xl border border-strong bg-pane text-text shadow-2xl"
        :style="{ width: `${width}px` }"
        :role="ariaRole"
        :aria-label="title"
        aria-modal="true"
        :data-testid="testid"
        @keydown="trapFocus"
      >
        <header class="flex shrink-0 items-center gap-3 border-b border-row px-5 py-4">
          <span v-if="icon" :class="['flex size-7 items-center justify-center rounded-[7px]', badgeClasses]"><component :is="icon" class="size-4" /></span>
          <div class="flex-1 text-[15px] font-semibold tracking-[-.01em]">{{ title }}</div>
          <slot name="header-actions" />
          <button
            class="cursor-pointer text-text-3 hover:text-text disabled:cursor-default disabled:opacity-50"
            aria-label="Close"
            :data-testid="testid ? `${testid}-close` : undefined"
            :disabled="busy"
            @click="close"
          ><IconX class="size-4" /></button>
        </header>
        <div class="min-h-0 flex-1 overflow-y-auto">
          <slot />
        </div>
        <footer v-if="$slots.footer" class="flex shrink-0 gap-2.5 border-t border-row bg-raised px-5 py-3.5">
          <slot name="footer" />
        </footer>
      </div>
    </div>
  </Teleport>
</template>
