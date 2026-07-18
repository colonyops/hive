<script setup lang="ts">
defineProps<{ profileName: string }>()

// macOS renders its native traffic lights over the top-left of the frameless
// titlebar (MacTitleBarHiddenInset); pad past them so content doesn't overlap.
const isMac = navigator.userAgent.includes('Mac')
</script>

<template>
  <header
    class="flex h-10 shrink-0 items-center gap-3 border-b border-border bg-raised px-3.5"
    :class="{ 'pl-[84px]': isMac }"
    style="--wails-draggable: drag"
  >
    <span class="font-mono text-[12.5px] font-semibold">hive</span>
    <span class="text-[13px] text-text-3">/</span>
    <span class="text-[13px] text-text-2" data-testid="breadcrumb-profile-name">{{ profileName }}</span>
    <div class="flex-1" />
    <div class="flex items-center gap-[7px] font-mono text-[11.5px] text-text-2">
      <span class="size-[7px] rounded-full bg-accent [animation:hivePulse_2.4s_ease-in-out_infinite]" />
      <span>polling github · <span class="text-accent">3 new</span></span>
    </div>
  </header>
</template>
