<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useResizablePanel } from '../composables/useResizablePanel'
import PanelResizeHandle from './PanelResizeHandle.vue'

interface DrawerResizeOptions {
  storageKey: string
  defaultSize: number
  min: number
  max: number
}

const props = withDefaults(defineProps<{
  ariaLabel: string
  testid?: string
  backdropTestid?: string
  width?: number
  resize?: DrawerResizeOptions
  closeOnEscape?: boolean
  closeOnBackdrop?: boolean
  trapFocus?: boolean
  bodyClass?: string
}>(), {
  closeOnEscape: true,
  closeOnBackdrop: true,
  trapFocus: true,
})

const emit = defineEmits<{ close: [] }>()
const sheetRef = ref<HTMLElement | null>(null)
const resizePanel = props.resize
  ? useResizablePanel({ ...props.resize, edge: 'left' })
  : null
const panelWidth = computed(() => resizePanel?.size.value ?? props.width)

function close(): void {
  emit('close')
}

function onBackdropClick(): void {
  if (props.closeOnBackdrop) close()
}

function onKeydown(event: KeyboardEvent): void {
  if (props.closeOnEscape && event.key === 'Escape') close()
}

function focusableElements(): HTMLElement[] {
  return Array.from(sheetRef.value?.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])') ?? [])
}

function trapFocus(event: KeyboardEvent): void {
  if (!props.trapFocus || event.key !== 'Tab') return
  const focusable = focusableElements()
  if (!focusable.length) return
  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

function startResize(event: PointerEvent): void {
  resizePanel?.startResize(event)
}

function stepResize(deltaPx: number): void {
  resizePanel?.step(deltaPx)
}

onMounted(() => {
  if (props.closeOnEscape) window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-40 bg-backdrop" :data-testid="backdropTestid ?? (testid ? `${testid}-backdrop` : undefined)" @click="onBackdropClick" />
    <aside
      ref="sheetRef"
      class="fixed inset-y-0 right-0 z-40 flex max-w-full flex-col overflow-hidden border-l border-strong bg-pane text-text shadow-[-30px_0_60px_-20px_rgba(0,0,0,.5)]"
      :style="{ width: panelWidth ? `${panelWidth}px` : undefined }"
      role="dialog"
      :aria-label="ariaLabel"
      aria-modal="true"
      :data-testid="testid"
      @keydown="trapFocus"
    >
      <PanelResizeHandle v-if="resize" edge="left" :name="testid ?? 'drawer'" :start="startResize" :step="stepResize" />
      <header v-if="$slots.header" class="shrink-0 border-b border-row bg-pane px-[18px] py-[15px]">
        <slot name="header" />
      </header>
      <div :class="['hive-scroll min-h-0 flex-1 overflow-y-auto px-[18px] py-[15px]', bodyClass]">
        <slot />
      </div>
      <footer v-if="$slots.footer" class="shrink-0 border-t border-row bg-raised px-[18px] py-[13px]">
        <slot name="footer" />
      </footer>
    </aside>
  </Teleport>
</template>
