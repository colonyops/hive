<script setup lang="ts">
// The label/slot/hint wrapper every settings control below builds on —
// mirrors pipeline/fields/FieldRow.vue's "label above input, hint below"
// layout (kept as a local copy rather than importing across the
// settings/pipeline module boundary: settings is an app-level concern, not
// a flow one — see App.vue's settingsOpen).
defineProps<{
  label?: string
  hint?: string
  testid?: string
}>()
</script>

<template>
  <div>
    <label v-if="label" class="mb-1.5 block text-[12.5px] text-text-2" :for="testid">{{ label }}</label>
    <slot />
    <p v-if="hint" class="mt-1.5 text-xs leading-relaxed text-text-4" :data-testid="testid ? `${testid}-hint` : undefined">{{ hint }}</p>
  </div>
</template>
