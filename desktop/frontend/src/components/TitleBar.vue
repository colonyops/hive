<script setup lang="ts">
// profileName is empty during onboarding: the bar shows just the wordmark.
// unreadCount drives the "N new" tally next to the polling indicator.
// flowsActive adds a "/ Flows" breadcrumb crumb and turns the profile crumb
// into a button that exits back to the feed view (the fix for being "stuck"
// in the flows canvas).
defineProps<{ profileName?: string; unreadCount?: number; flowsActive?: boolean }>()
const emit = defineEmits<{ 'exit-flows': [] }>()

// macOS draws its native traffic lights over the top-left of this bar
// (hidden-inset chrome configured in desktop/main.go). Pad past them and keep
// the height in sync with InvisibleTitleBarHeight (42) in main.go so the
// buttons stay vertically centered.
const isMac = navigator.userAgent.includes('Mac')
</script>

<template>
  <header
    class="flex shrink-0 items-center gap-3 border-b border-border bg-raised px-3.5"
    :class="isMac ? 'h-[42px] pl-[84px]' : 'h-10'"
    style="--wails-draggable: drag"
  >
    <span class="font-mono text-[12.5px] font-semibold">hive</span>
    <template v-if="profileName">
      <span class="text-[13px] text-text-3">/</span>
      <!-- In flows mode the profile crumb is a button back to the feed. -->
      <button
        v-if="flowsActive"
        class="cursor-pointer text-[13px] text-text-2 hover:text-text"
        style="--wails-draggable: no-drag"
        data-testid="breadcrumb-profile-name"
        @click="emit('exit-flows')"
      >{{ profileName }}</button>
      <span v-else class="text-[13px] text-text-2" data-testid="breadcrumb-profile-name">{{ profileName }}</span>
      <template v-if="flowsActive">
        <span class="text-[13px] text-text-3">/</span>
        <span class="text-[13px] text-accent" data-testid="breadcrumb-flows">Flows</span>
      </template>
    </template>
    <div class="flex-1" />
    <div v-if="profileName && !flowsActive" class="flex items-center gap-[7px] font-mono text-[11.5px] text-text-2" data-testid="polling-indicator">
      <span class="size-[7px] rounded-full bg-accent [animation:hivePulse_2.4s_ease-in-out_infinite]" />
      <span>polling github<template v-if="unreadCount"> · <span class="text-accent">{{ unreadCount }} new</span></template></span>
    </div>
  </header>
</template>
