<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { useEscapeToClose } from '../composables/useEscapeToClose'
import { useFocusTrap } from '../composables/useFocusTrap'
import { useRegisterOpenModal } from '../composables/useOpenModalCount'
import { useResizablePanel } from '../composables/useResizablePanel'
import { useReturnFocus } from '../composables/useReturnFocus'
import PanelResizeHandle from './PanelResizeHandle.vue'

const props = withDefaults(
  defineProps<{
    ariaLabel: string
    testid?: string
    backdropTestid?: string
    /** Fixed width in px — opts out of the default resizable behavior. */
    width?: number
    /** Resize persistence key; defaults to `hive.panel.<testid>`. */
    storageKey?: string
    defaultSize?: number
    min?: number
    max?: number
    closeOnEscape?: boolean
    closeOnBackdrop?: boolean
    trapFocus?: boolean
    bodyClass?: string
    /** Where focus goes when the sheet closes; defaults to whatever held it when it opened. */
    returnFocusTo?: HTMLElement | null
  }>(),
  {
    defaultSize: 440,
    min: 360,
    max: 760,
    closeOnEscape: true,
    closeOnBackdrop: true,
    trapFocus: true,
  },
)

const emit = defineEmits<{ close: [] }>()
const sheetRef = ref<HTMLElement | null>(null)
const bodyRef = ref<HTMLElement | null>(null)
// A composable can only be created during setup, so the sheet picks fixed or
// resizable once. No caller switches between the two on a mounted sheet.
/* eslint-disable vue/no-setup-props-reactivity-loss */
const resizePanel =
  props.width === undefined
    ? useResizablePanel({
        storageKey: props.storageKey ?? `hive.panel.${props.testid ?? 'drawer'}`,
        defaultSize: props.defaultSize,
        min: props.min,
        max: props.max,
        edge: 'left',
      })
    : null
/* eslint-enable vue/no-setup-props-reactivity-loss */
const panelWidth = computed(() => resizePanel?.size.value ?? props.width)

function close(): void {
  emit('close')
}

function onBackdropClick(): void {
  if (props.closeOnBackdrop) close()
}

useEscapeToClose(close, { enabled: () => props.closeOnEscape })

const { onKeydown: trapFocus } = useFocusTrap(sheetRef, { enabled: () => props.trapFocus })
useReturnFocus(() => props.returnFocusTo)
useRegisterOpenModal()

// A field inside may have claimed focus already (useAutofocus also waits a
// tick, and children mount first). Otherwise focus stays on the page behind
// the sheet, where keys would still reach it.
onMounted(async () => {
  await nextTick()
  if (!sheetRef.value?.contains(document.activeElement)) sheetRef.value?.focus()
})

function startResize(event: PointerEvent): void {
  resizePanel?.startResize(event)
}

function stepResize(deltaPx: number): void {
  resizePanel?.step(deltaPx)
}

/** The scrolling body, for a host that swaps its content and has to manage the scroll position. */
defineExpose({ body: bodyRef })
</script>

<template>
  <Teleport to="body">
    <div
      class="fixed inset-0 z-40 bg-backdrop"
      :data-testid="backdropTestid ?? (testid ? `${testid}-backdrop` : undefined)"
      @click="onBackdropClick"
    />
    <aside
      ref="sheetRef"
      class="fixed inset-y-0 right-0 z-40 flex max-w-full flex-col overflow-hidden border-l border-strong bg-pane text-text shadow-[-30px_0_60px_-20px_rgba(0,0,0,.5)] outline-none"
      :style="{ width: panelWidth ? `${panelWidth}px` : undefined }"
      role="dialog"
      :aria-label="ariaLabel"
      aria-modal="true"
      tabindex="-1"
      :data-testid="testid"
      @keydown="trapFocus"
    >
      <PanelResizeHandle
        v-if="resizePanel"
        edge="left"
        :name="testid ?? 'drawer'"
        :start="startResize"
        :step="stepResize"
      />
      <header v-if="$slots.header" class="shrink-0 border-b border-row bg-pane px-[18px] py-[15px]">
        <slot name="header" />
      </header>
      <div ref="bodyRef" :class="['hive-scroll min-h-0 flex-1 overflow-y-auto px-[18px] py-[15px]', bodyClass]">
        <slot />
      </div>
      <footer v-if="$slots.footer" class="shrink-0 border-t border-row bg-raised px-[18px] py-[13px]">
        <slot name="footer" />
      </footer>
    </aside>
  </Teleport>
</template>
