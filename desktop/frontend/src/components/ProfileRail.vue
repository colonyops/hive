<script setup lang="ts">
import type { Profile } from '../types/feed'

defineProps<{ profiles: Profile[]; activeProfileId: string }>()
const emit = defineEmits<{ select: [profileId: string] }>()
</script>

<template>
  <aside class="flex w-[58px] shrink-0 flex-col items-center gap-2.5 border-r border-border bg-app py-3">
    <button
      v-for="profile in profiles"
      :key="profile.id"
      :title="profile.name"
      :data-id="profile.id"
      data-testid="profile-tile"
      class="relative flex size-[38px] cursor-pointer items-center justify-center rounded-[10px] border border-card bg-chip font-mono text-sm font-semibold text-text-2 transition-colors hover:bg-hover hover:text-text"
      :class="{ 'text-text': profile.id === activeProfileId }"
      @click="emit('select', profile.id)"
    >
      <span v-if="profile.id === activeProfileId" class="absolute bottom-2 left-[-13px] top-2 w-[3px] rounded-sm bg-accent" />
      {{ profile.letter }}
    </button>
    <button class="flex size-[38px] cursor-default items-center justify-center rounded-[10px] border border-dashed border-card text-xl text-text-4 hover:border-strong hover:text-text-2" aria-label="Add profile">+</button>
    <div class="flex-1" />
    <span class="flex size-[38px] items-center justify-center text-base text-text-4">◔</span>
    <span class="flex size-[30px] items-center justify-center rounded-full bg-accent/15 font-mono text-xs font-semibold text-accent">hy</span>
  </aside>
</template>
