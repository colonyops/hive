<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import IconCircleAlert from '~icons/lucide/circle-alert'
import IconCircleCheck from '~icons/lucide/circle-check'
import IconInfo from '~icons/lucide/info'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import IconX from '~icons/lucide/x'
import IconZap from '~icons/lucide/zap'
import type { ToastActionDef, ToastInstance, ToastSeverity } from '../types/toast'

const props = defineProps<{ toast: ToastInstance }>()
const emit = defineEmits<{ dismiss: [] }>()

const icons: Record<ToastSeverity, unknown> = {
  info: IconInfo,
  success: IconCircleCheck,
  warning: IconTriangleAlert,
  error: IconCircleAlert,
  'auto-action': IconZap,
}

// design spec "6a Toasts": one border/icon/accent/progress-bar color per
// severity, plus the primary inline action (first entry) inheriting it.
const severityStyles: Record<ToastSeverity, { border: string; iconBg: string; iconColor: string; accent: string; bar: string }> = {
  info: { border: 'border-border', iconBg: 'bg-severity-info-tint', iconColor: 'text-severity-info', accent: 'text-severity-info', bar: 'bg-severity-info' },
  success: { border: 'border-severity-success-border', iconBg: 'bg-severity-success-tint', iconColor: 'text-severity-success', accent: 'text-severity-success', bar: 'bg-severity-success' },
  warning: { border: 'border-severity-warning-border', iconBg: 'bg-severity-warning-tint', iconColor: 'text-severity-warning', accent: 'text-severity-warning', bar: 'bg-severity-warning' },
  error: { border: 'border-severity-error-border', iconBg: 'bg-severity-error-tint', iconColor: 'text-severity-error', accent: 'text-severity-error', bar: 'bg-severity-error' },
  'auto-action': { border: 'border-severity-auto-border', iconBg: 'bg-severity-auto-tint', iconColor: 'text-accent', accent: 'text-accent', bar: 'bg-accent' },
}

const icon = computed(() => icons[props.toast.severity])
const style = computed(() => severityStyles[props.toast.severity])

// The auto-dismiss progress bar shrinks from 100% to 0% over the toast's
// duration. Starting the CSS transition a frame after mount (rather than at
// width:100% from the first paint) gives the browser a frame to render the
// full-width state before animating away from it.
const progressStarted = ref(false)
onMounted(() => {
  requestAnimationFrame(() => { progressStarted.value = true })
})

function runAction(action: ToastActionDef) {
  action.onClick()
  emit('dismiss')
}
</script>

<template>
  <div
    class="overflow-hidden rounded-[11px] bg-raised shadow-[0_18px_40px_-14px_rgba(0,0,0,.7)]"
    :class="['border', style.border]"
    data-testid="toast"
    :data-toast-severity="toast.severity"
  >
    <div class="flex gap-3 px-3.5 pb-[13px] pt-3.5">
      <span class="flex size-[26px] shrink-0 items-center justify-center rounded-[7px]" :class="style.iconBg">
        <component :is="icon" class="size-[15px]" :class="style.iconColor" />
      </span>
      <div class="min-w-0 flex-1">
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-[13.5px] font-semibold text-text" data-testid="toast-title">{{ toast.message }}</span>
          <span
            v-if="toast.severity === 'auto-action'"
            class="rounded-[4px] border border-accent/30 bg-accent/13 px-[5px] py-px font-mono text-[9.5px] tracking-[.06em] text-accent"
            data-testid="toast-auto-badge"
          >AUTO</span>
        </div>
        <p v-if="toast.body" class="mt-0.5 text-[12.5px] leading-[1.45] text-text-2" data-testid="toast-body">{{ toast.body }}</p>
        <div v-if="toast.actions.length" class="mt-[9px] flex gap-3.5">
          <button
            v-for="(action, i) in toast.actions"
            :key="action.label"
            class="cursor-pointer text-[12.5px]"
            :class="i === 0 ? [style.accent, 'font-semibold hover:brightness-125'] : 'text-text-3 hover:text-text'"
            data-testid="toast-action"
            @click="runAction(action)"
          >{{ action.label }}</button>
        </div>
      </div>
      <button class="shrink-0 cursor-pointer self-start leading-none text-text-3 hover:text-text" aria-label="Dismiss" data-testid="toast-dismiss" @click="emit('dismiss')">
        <IconX class="size-[15px]" />
      </button>
    </div>
    <div
      class="h-0.5 transition-[width] ease-linear"
      :class="style.bar"
      :style="{ width: progressStarted ? '0%' : '100%', transitionDuration: `${toast.duration}ms` }"
      data-testid="toast-progress"
    />
  </div>
</template>
