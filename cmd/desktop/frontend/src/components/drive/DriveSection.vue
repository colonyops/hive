<script setup lang="ts">
// A collapsible pane of the drive sidebar, in the shape of an editor's side
// bar: a small caps heading that stays in view while the list under it
// scrolls, over a body set a step below the sidebar.
import { useStorage } from '@vueuse/core'
import IconChevronRight from '~icons/lucide/chevron-right'

const props = defineProps<{
  title?: string
  /** The local storage key that keeps the fold across launches. */
  storageKey?: string
  /** Only the body, unstyled: the narrow pane lists its tree without panes. */
  bare?: boolean
  /** Derives `-toggle` and `-body`. */
  testid: string
}>()

// A bare section never folds, so its fallback key is never written.
const open = useStorage(() => props.storageKey ?? 'hive.drive.section.open', true)
</script>

<template>
  <div v-if="bare" class="flex flex-1 flex-col" :data-testid="`${testid}-body`">
    <slot />
  </div>
  <section v-else class="drive-section flex flex-col" :data-testid="testid" :data-open="open ? 'true' : 'false'">
    <div class="drive-section-head sticky top-0 z-10 flex h-8 shrink-0 items-center bg-sidebar pr-1">
      <button
        type="button"
        class="flex h-full min-w-0 flex-1 cursor-pointer items-center gap-1 pl-1 text-left text-micro font-semibold uppercase tracking-wide text-text-3 hover:text-text-2"
        :aria-expanded="open"
        :data-testid="`${testid}-toggle`"
        @click="open = !open"
      >
        <IconChevronRight
          class="size-3.5 shrink-0 transition-transform"
          :class="open ? 'rotate-90' : ''"
          aria-hidden="true"
        />
        <span class="truncate">{{ title }}</span>
      </button>
      <!-- Actions show while the pointer or the focus is in the pane, as an editor's do. -->
      <div v-if="open && $slots.actions" class="drive-section-actions flex shrink-0 items-center" @click.stop>
        <slot name="actions" />
      </div>
    </div>
    <div v-show="open" class="drive-section-body flex flex-1 flex-col py-0.5" :data-testid="`${testid}-body`">
      <slot />
    </div>
  </section>
</template>

<style scoped>
.drive-section-head {
  box-shadow: inset 0 1px 0 var(--color-border);
}
/* The search bar's border already runs above the first pane. */
.drive-section:first-child > .drive-section-head {
  box-shadow: none;
}
.drive-section-body {
  background: color-mix(in srgb, var(--color-sidebar), black 7%);
  box-shadow:
    inset 0 1px 0 var(--color-border),
    inset 0 3px 4px -3px color-mix(in srgb, black 35%, transparent);
}
.drive-section-actions {
  opacity: 0;
}
section:hover .drive-section-actions,
section:focus-within .drive-section-actions {
  opacity: 1;
}
</style>
