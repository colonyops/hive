<script setup lang="ts">
import IconPause from '~icons/lucide/pause'
import IconPlus from '~icons/lucide/plus'
import IconSettings from '~icons/lucide/settings'
import type { Profile } from '../types/feed'

defineProps<{ profiles: Profile[]; activeProfileId: string }>()
const emit = defineEmits<{ select: [profileId: string]; add: []; 'open-settings': [] }>()
</script>

<template>
  <aside class="flex w-[58px] shrink-0 flex-col items-center gap-2.5 border-r border-border bg-raised py-3">
    <button
      v-for="profile in profiles"
      :key="profile.id"
      :title="profile.enabled ? profile.name : `${profile.name} (disabled)`"
      :aria-label="profile.enabled ? profile.name : `${profile.name}, disabled`"
      :data-id="profile.id"
      :data-enabled="profile.enabled"
      data-testid="profile-tile"
      class="relative flex size-[38px] cursor-pointer items-center justify-center rounded-[10px] border border-card bg-chip font-mono text-sm font-semibold text-text-2 transition-colors hover:bg-hover hover:text-text"
      :class="{ 'text-text': profile.id === activeProfileId, 'opacity-55': !profile.enabled }"
      @click="emit('select', profile.id)"
    >
      <span v-if="profile.id === activeProfileId" class="absolute bottom-2 left-[-13px] top-2 w-[3px] rounded-sm bg-accent" />
      {{ profile.letter }}
      <span v-if="!profile.enabled" class="absolute -bottom-1 -right-1 flex size-4 items-center justify-center rounded-full border border-border bg-raised text-text-3" aria-hidden="true"><IconPause class="size-2.5" /></span>
    </button>
    <button class="flex size-[38px] cursor-pointer items-center justify-center rounded-[10px] border border-dashed border-card text-text-4 hover:border-strong hover:text-text-2" aria-label="Add profile" data-testid="profile-add" @click="emit('add')"><IconPlus class="size-4" /></button>
    <div class="flex-1" />
    <button
      type="button"
      class="flex size-[38px] cursor-pointer items-center justify-center rounded-[10px] text-text-3 hover:bg-hover hover:text-text"
      title="Application settings"
      aria-label="Application settings"
      data-testid="application-settings"
      @click="emit('open-settings')"
    ><IconSettings class="size-4" /></button>
  </aside>
</template>
