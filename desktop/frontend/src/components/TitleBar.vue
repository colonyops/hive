<script setup lang="ts">
import IconArrowLeft from '~icons/lucide/arrow-left'
import IconArrowRight from '~icons/lucide/arrow-right'
import IconTriangleAlert from '~icons/lucide/triangle-alert'

// profileName is empty during onboarding: the bar shows just the wordmark.
// flowsActive adds a "/ Flows" breadcrumb crumb and turns the profile crumb
// into a button that exits back to the feed view (the fix for being "stuck"
// in the flows canvas).
// errorCount (8d) is the count of the active flow's nodes whose last run
// failed — sourced app-wide from useFlowsSession, so the chip renders and
// deep-links even with the flows canvas closed.
defineProps<{
  profileName?: string
  flowsActive?: boolean
  errorCount?: number
  canGoBack?: boolean
  canGoForward?: boolean
}>()
const emit = defineEmits<{
  back: []
  forward: []
  'exit-flows': []
  'open-error-node': []
}>()

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
    <nav class="flex items-center gap-0.5" aria-label="Page history" style="--wails-draggable: no-drag">
      <button
        type="button"
        class="flex size-6 items-center justify-center rounded text-text-3 enabled:cursor-pointer enabled:hover:bg-chip enabled:hover:text-text disabled:opacity-30"
        :disabled="!canGoBack"
        aria-label="Go back"
        data-testid="titlebar-back"
        @click="emit('back')"
      ><IconArrowLeft class="size-3.5" /></button>
      <button
        type="button"
        class="flex size-6 items-center justify-center rounded text-text-3 enabled:cursor-pointer enabled:hover:bg-chip enabled:hover:text-text disabled:opacity-30"
        :disabled="!canGoForward"
        aria-label="Go forward"
        data-testid="titlebar-forward"
        @click="emit('forward')"
      ><IconArrowRight class="size-3.5" /></button>
    </nav>
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
    <button
      v-if="errorCount && errorCount > 0"
      class="flex shrink-0 cursor-pointer items-center gap-1.5 rounded-md border border-severity-error-border bg-severity-error-tint px-2 py-1 text-[11.5px] font-semibold text-severity-error hover:opacity-85"
      style="--wails-draggable: no-drag"
      data-testid="titlebar-error-chip"
      @click="emit('open-error-node')"
    ><IconTriangleAlert class="size-3" />{{ errorCount }} error<template v-if="errorCount !== 1">s</template></button>
    <div v-if="profileName && !flowsActive" class="flex items-center gap-[7px] font-mono text-[11.5px] text-text-2" data-testid="polling-indicator">
      <span class="size-[7px] rounded-full bg-accent [animation:hivePulse_2.4s_ease-in-out_infinite]" />
      <span>polling github</span>
    </div>
  </header>
</template>
