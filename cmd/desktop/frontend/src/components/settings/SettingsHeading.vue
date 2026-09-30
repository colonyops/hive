<script setup lang="ts">
// The one place a settings label's type is decided. Two levels, and they must
// stay visually distinct or nesting stops reading:
//
//   section — top level in a page ("Terminal typography"), with its
//             description inline on the same baseline.
//   group   — a labelled block inside a section ("Feeds", "Agents"). Smaller,
//             dimmer, and ruled to the right so it reads as subordinate.
//
// Titles are passed in natural case; the caps are styling, so a label stays
// readable to a screen reader and searchable in source.
withDefaults(defineProps<{
  title: string
  /** Inline prose at section level; a right-aligned note at group level. */
  description?: string
  level?: 'section' | 'group'
  /** Group level only: the hairline that carries the eye to the note. */
  rule?: boolean
}>(), {
  level: 'section',
  rule: true,
})
</script>

<template>
  <div v-if="level === 'section'" class="flex flex-wrap items-start gap-x-4 gap-y-2">
    <div class="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2.5 gap-y-1">
      <h2 class="text-xs font-semibold uppercase tracking-[.1em] text-text-2">{{ title }}</h2>
      <p v-if="description" class="text-xs leading-relaxed text-text-3">{{ description }}</p>
    </div>
    <!-- Header-right controls (New…, Sync) belong to the heading so every list
         page places them identically instead of wrapping its own flex row. -->
    <div v-if="$slots.actions" class="flex shrink-0 items-center gap-3"><slot name="actions" /></div>
  </div>

  <div v-else class="flex items-baseline gap-2.5">
    <h3 class="text-[10.5px] font-semibold uppercase tracking-[.12em] text-text-3">{{ title }}</h3>
    <div v-if="rule" class="h-px flex-1 self-center bg-border" />
    <p v-if="description" class="text-[11.5px] text-text-4">{{ description }}</p>
  </div>
</template>
