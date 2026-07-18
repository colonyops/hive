<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import type { ConfigValidationError } from '../lib/configErrors'

const props = defineProps<{
  path: string
  errors: ConfigValidationError[]
}>()
const emit = defineEmits<{ retry: []; dismiss: []; 'copy-path': []; 'copy-errors': [] }>()

// This overlay can appear (config:updated firing with an invalid config)
// on top of another sheet that also has an unconditional window Escape
// listener (FeedEditorSheet, ConfigSheet, DeleteProfileModal, NewProfileModal
// all register one directly on window, uncoordinated). A plain bubble-phase
// listener here would fire alongside theirs on the same Escape press,
// closing the sheet underneath too and losing its unsaved form state.
// Registering with { capture: true } makes this listener run first, in the
// capture phase, before the event ever reaches the target or bubbles back up
// to those other window listeners; stopPropagation() there halts the walk
// before it gets that far, so only the topmost (this overlay) reacts.
function onKeydown(e: KeyboardEvent) {
  if (e.key !== 'Escape') return
  e.stopPropagation()
  emit('dismiss')
}

onMounted(() => window.addEventListener('keydown', onKeydown, { capture: true }))
onUnmounted(() => window.removeEventListener('keydown', onKeydown, { capture: true }))
</script>

<template>
  <Teleport to="body">
    <!-- Full-app block (design spec "6b"): no click-outside-to-dismiss — the
         "Dismiss" button is the explicit, intentional way out. Config keeps
         serving the last-good data behind this, so dismissing is safe. -->
    <div class="fixed inset-0 z-[60] flex items-center justify-center bg-scrim-strong p-10" data-testid="config-error-overlay">
      <div
        class="flex w-[600px] flex-col overflow-hidden rounded-[14px] border border-card bg-pane text-text shadow-[0_40px_90px_-20px_rgba(0,0,0,.8)]"
        role="alertdialog"
        aria-label="Feeds config error"
        aria-modal="true"
      >
        <header class="flex gap-4 border-b border-row px-7 pb-[22px] pt-[26px]">
          <span class="flex size-[42px] shrink-0 items-center justify-center rounded-[11px] bg-severity-error-tint text-severity-error">
            <IconTriangleAlert class="size-[22px]" />
          </span>
          <div class="min-w-0 flex-1">
            <div class="text-[18px] font-semibold tracking-[-.01em]" data-testid="config-error-title">Couldn't load your feeds config</div>
            <p class="mt-1 text-[13px] leading-relaxed text-text-2" data-testid="config-error-subtitle">
              {{ errors.length }} problem{{ errors.length === 1 ? '' : 's' }} found — the last valid version stays active until this is fixed.
            </p>
            <p class="mt-2 truncate font-mono text-[11.5px] text-text-3" data-testid="config-error-path">{{ path || '…' }}</p>
          </div>
        </header>

        <div class="px-7 pt-4">
          <div class="mb-1.5 flex items-baseline justify-between gap-3">
            <span class="font-mono text-[11px] tracking-[.12em] text-text-3" data-testid="config-error-eyebrow">
              VALIDATION DETAIL · {{ errors.length }} PROBLEM{{ errors.length === 1 ? '' : 'S' }}
            </span>
            <button class="cursor-pointer text-[11.5px] text-text-2 hover:text-text" data-testid="config-error-copy" @click="emit('copy-errors')">Copy</button>
          </div>
          <div class="hive-scroll max-h-[150px] overflow-y-auto rounded-[9px] border border-row bg-app px-3.5 py-[13px] font-mono text-xs leading-[1.7]" data-testid="config-error-list">
            <div v-for="(error, i) in errors" :key="i" :class="{ 'mt-1.5': i > 0 }" data-testid="config-error-entry">
              <span v-if="error.line !== null" class="text-text-3">line {{ error.line }}&nbsp;&nbsp;</span><span class="text-severity-error">{{ error.message }}</span>
            </div>
            <div v-if="errors.length === 0" class="text-text-3">No further detail was provided.</div>
          </div>
        </div>

        <div class="flex items-center gap-2.5 px-7 pb-6 pt-5">
          <button class="cursor-pointer rounded-lg bg-accent px-[18px] py-2.5 text-[13.5px] font-semibold text-accent-contrast hover:brightness-110" data-testid="config-error-reload" @click="emit('retry')">Reload</button>
          <button class="cursor-pointer rounded-lg border border-card bg-sidebar px-4 py-2.5 text-[13.5px] text-text hover:border-strong" data-testid="config-error-copy-path" @click="emit('copy-path')">Copy config path</button>
          <div class="flex-1" />
          <button class="cursor-pointer text-[13px] text-text-2 hover:text-text" data-testid="config-error-dismiss" @click="emit('dismiss')">Dismiss</button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
