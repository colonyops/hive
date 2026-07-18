<script setup lang="ts">
defineProps<{ profileName: string }>()
const emit = defineEmits<{ hide: [] }>()
</script>

<template>
  <header
    class="flex h-10 shrink-0 items-center gap-3 border-b border-border bg-raised px-3.5"
    style="--wails-draggable: drag"
  >
    <div class="flex items-center gap-[7px]">
      <span v-for="dot in 3" :key="dot" class="size-[11px] rounded-full bg-dot" />
    </div>
    <div class="mx-1 h-4 w-px bg-border" />
    <span class="font-mono text-[12.5px] font-semibold">hive</span>
    <span class="text-[13px] text-text-3">/</span>
    <span class="text-[13px] text-text-2" data-testid="breadcrumb-profile-name">{{ profileName }}</span>
    <div class="flex-1" />
    <div class="flex items-center gap-[7px] font-mono text-[11.5px] text-text-2">
      <span class="size-[7px] rounded-full bg-accent [animation:hivePulse_2.4s_ease-in-out_infinite]" />
      <span>polling github · <span class="text-accent">3 new</span></span>
    </div>
    <div class="mx-1.5 h-4 w-px bg-border" />
    <div class="flex gap-3.5 text-[13px] text-text-4" style="--wails-draggable: no-drag">
      <button class="cursor-default" aria-label="Minimize">—</button>
      <button class="cursor-default" aria-label="Maximize">▢</button>
      <button class="cursor-pointer hover:text-text" aria-label="Hide" @click="emit('hide')">✕</button>
    </div>
  </header>
</template>
