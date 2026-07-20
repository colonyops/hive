<script setup lang="ts">
// FieldRow is the label/slot/hint-or-error wrapper every other field-kit
// component builds on, mirroring FeedEditorSheet's "label above input, hint
// or error below" layout. `testid` namespaces the hint/error paragraph's
// data-testid so multiple fields on one page stay distinguishable
// (`${testid}-error` / `${testid}-hint`) — plain "field-row-error" would
// collide across a form with several fields.
defineProps<{
  label?: string
  hint?: string
  error?: string
  testid?: string
}>()
</script>

<template>
  <div>
    <div v-if="label" class="mb-1.5 text-[12.5px] text-text-2">{{ label }}</div>
    <slot />
    <p v-if="error" class="mt-1.5 text-xs leading-relaxed text-kind-issue" :data-testid="testid ? `${testid}-error` : undefined">{{ error }}</p>
    <p v-else-if="hint" class="mt-1.5 text-xs leading-relaxed text-text-4" :data-testid="testid ? `${testid}-hint` : undefined">{{ hint }}</p>
  </div>
</template>
