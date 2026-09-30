<script setup lang="ts">
// One setting: what it is on the left, the control that changes it on the
// right. Rows own their padding so a boxed SettingsSection can stack them into
// a single card with hairlines between.
//
// The control slot gets no label of its own — the row is the label. Pass
// `aria-label` to the control so it stays named for screen readers.
defineProps<{
  label: string
  hint?: string
  testid?: string
}>()
</script>

<template>
  <div
    class="flex flex-col gap-3 px-4 py-3.5 @[600px]/pane:flex-row @[600px]/pane:items-center @[600px]/pane:gap-6"
    :data-testid="testid"
  >
    <div class="min-w-0 flex-1">
      <div class="text-[13.5px] font-semibold text-text">{{ label }}</div>
      <p v-if="hint" class="mt-1 text-[12px] leading-relaxed text-text-3">{{ hint }}</p>
    </div>
    <div class="shrink-0 @[600px]/pane:flex @[600px]/pane:justify-end"><slot /></div>
  </div>
</template>
