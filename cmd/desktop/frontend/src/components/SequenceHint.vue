<script setup lang="ts">
import { computed } from 'vue'
import { formatCombo, useKeybindings } from '../composables/useKeybindings'
import { commandById } from '../keybindings/catalog'

const { pendingSequence } = useKeybindings()

const steps = computed(() => pendingSequence.value?.steps.map((step) => formatCombo(step)) ?? [])

// A continuation can name a command a launcher reload removed after the
// pending sequence snapshotted its continuations — skip it rather than
// showing a blank title.
const continuations = computed(() => {
  const pending = pendingSequence.value
  if (!pending) return []
  return pending.continuations.flatMap((continuation) => {
    const command = commandById.value.get(continuation.commandId)
    if (!command) return []
    return [{ step: continuation.step, label: formatCombo(continuation.step), title: command.title }]
  })
})
</script>

<template>
  <div
    v-if="pendingSequence"
    class="pointer-events-none fixed bottom-4 left-1/2 z-40 flex -translate-x-1/2 select-none items-center gap-3 rounded-full border border-card bg-raised px-3 py-1.5 font-mono text-[11px] text-text-3 shadow-lg"
    data-testid="sequence-hint"
  >
    <span class="flex items-center gap-1">
      <kbd
        v-for="(step, i) in steps"
        :key="i"
        class="rounded border border-card bg-card px-1.5 py-0.5 text-text-2"
        data-testid="sequence-hint-step"
      >{{ step }}</kbd>
    </span>
    <span
      v-for="continuation in continuations"
      :key="continuation.step"
      class="flex items-center gap-1.5"
      data-testid="sequence-hint-continuation"
    >
      <kbd
        class="rounded border border-card bg-card px-1.5 py-0.5 text-text-2"
        data-testid="sequence-hint-continuation-key"
      >{{ continuation.label }}</kbd>
      <span data-testid="sequence-hint-continuation-title">{{ continuation.title }}</span>
    </span>
  </div>
</template>
