<script setup lang="ts">
// The content column every settings page fills. It is left-aligned and capped
// at one width for the whole surface: the pane is already inset by the profile
// rail and the settings nav, so centring the column a second time floated
// pages away from the nav they belong to, and a per-page cap had drifted to
// eight different values.
defineProps<{
  testid?: string
}>()
</script>

<template>
  <div class="flex w-full max-w-[920px] flex-col gap-7" :data-testid="testid">
    <slot />
  </div>
</template>
