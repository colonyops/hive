<script setup lang="ts">
import { Window } from '@wailsio/runtime'
import { ref } from 'vue'
import IconPin from '~icons/lucide/pin'
import IconPinOff from '~icons/lucide/pin-off'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import { createLogger } from '../lib/log'

defineProps<{ title: string }>()

const log = createLogger('pop-out')

// macOS draws its traffic lights over the left of this bar. The height matches
// InvisibleTitleBarHeight in hiddenInsetChrome, as the main TitleBar's does.
const isMac = navigator.userAgent.includes('Mac')

const pinned = ref(false)
function togglePinned(): void {
  pinned.value = !pinned.value
  Window.SetAlwaysOnTop(pinned.value).catch((error: unknown) => {
    log.debug('always on top is unavailable outside Wails', error)
  })
}

// The hidden-inset title bar loses the native double-click-to-zoom.
function onDblclick(event: MouseEvent): void {
  if ((event.target as HTMLElement).closest('button')) return
  Window.ToggleMaximise().catch((error: unknown) => {
    log.debug('window maximise is unavailable outside Wails', error)
  })
}
</script>

<template>
  <header
    class="flex shrink-0 items-center gap-2 border-b border-border bg-raised pr-2"
    :class="isMac ? 'h-[42px] pl-[84px]' : 'h-10 pl-3'"
    style="--wails-draggable: drag"
    data-testid="popout-title-bar"
    @dblclick="onDblclick"
  >
    <span class="min-w-0 flex-1 truncate text-small font-semibold text-text-2" data-testid="popout-title">{{
      title
    }}</span>
    <div class="flex shrink-0 items-center gap-1" style="--wails-draggable: no-drag">
      <slot />
      <IconButton
        :label="pinned ? 'Stop keeping on top' : 'Keep on top'"
        :icon="pinned ? IconPinOff : IconPin"
        :active="pinned"
        :aria-pressed="pinned"
        data-testid="popout-pin"
        @click="togglePinned"
      />
    </div>
  </header>
</template>
