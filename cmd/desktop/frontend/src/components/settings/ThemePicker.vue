<script setup lang="ts">
// Each card carries its own `data-theme`, so the palette rules in main.css
// re-resolve inside it and the swatch paints in the theme it offers — no
// second copy of the colours in TypeScript to drift from the stylesheet.
// The selected ring sits on the outer button, outside that scope, so it stays
// in the *current* theme's accent rather than the one being previewed.
import { themeLabels, themes, type Theme } from '../../composables/useTheme'

defineProps<{
  modelValue: Theme
}>()
const emit = defineEmits<{ 'update:modelValue': [value: Theme] }>()
</script>

<template>
  <div class="grid grid-cols-2 gap-2.5 @[440px]/pane:grid-cols-3 @[680px]/pane:grid-cols-4" role="radiogroup" aria-label="Theme">
    <button
      v-for="theme in themes"
      :key="theme"
      type="button"
      role="radio"
      class="flex cursor-pointer flex-col gap-2 rounded-[10px] border p-2 text-left transition-colors"
      :class="modelValue === theme ? 'border-accent bg-accent-tint' : 'border-card bg-raised hover:border-strong'"
      :aria-checked="modelValue === theme"
      :data-testid="`settings-theme-${theme}`"
      @click="emit('update:modelValue', theme)"
    >
      <span :data-theme="theme" class="flex h-[52px] w-full overflow-hidden rounded-md border border-border">
        <span class="flex w-1/4 flex-col gap-1 border-r border-border bg-sidebar p-1.5">
          <span class="h-1 w-full rounded-full bg-text-4" />
          <span class="h-1 w-2/3 rounded-full bg-text-4" />
        </span>
        <span class="flex flex-1 flex-col gap-1 bg-app p-1.5">
          <span class="h-1.5 w-3/4 rounded-full bg-text-2" />
          <span class="h-1 w-1/2 rounded-full bg-text-4" />
          <span class="mt-auto flex gap-1">
            <span class="size-2 rounded-full bg-accent" />
            <span class="size-2 rounded-full bg-kind-pr" />
            <span class="size-2 rounded-full bg-kind-issue" />
          </span>
        </span>
      </span>
      <span class="truncate text-[12px]" :class="modelValue === theme ? 'font-medium text-text' : 'text-text-2'">{{ themeLabels[theme] }}</span>
    </button>
  </div>
</template>
