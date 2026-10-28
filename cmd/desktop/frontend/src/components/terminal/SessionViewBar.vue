<script setup lang="ts">
// The active view under the Code sidebar's toolbar: its name, the filter it
// applies as chips, and Save / Reset once the chips drift from the saved view.
// Absent on All sessions with no filter, so an unfiltered tree carries no chrome.
// Modeled on Linear's view header, cut down for a narrow sidebar.
import { computed, proxyRefs, ref } from 'vue'
import IconEyeOff from '~icons/lucide/eye-off'
import IconLayers from '~icons/lucide/layers'
import IconSave from '~icons/lucide/save'
import IconUndo from '~icons/lucide/undo-2'
import IconX from '~icons/lucide/x'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import { ALL_SESSIONS_VIEW_ID, describeCondition } from '../../lib/sessionViews'
import { useCodeViews } from '../../stores/useCodeViews'
import { useTerminalModeContext } from './terminalModeContext'

const { view, viewMenus } = useTerminalModeContext()
const views = proxyRefs(useCodeViews())

const shown = computed(() => views.activeViewId !== ALL_SESSIONS_VIEW_ID || views.draft.conditions.length > 0)
const stacked = computed(() => views.draft.conditions.length > 1)
const viewLabel = computed(() => views.activeView?.name ?? 'All sessions')

const openChip = ref(-1)
const chipAnchors = ref<HTMLElement[]>([])

function onChipSelect(index: number, id: string): void {
  if (viewMenus.onChipSelect(index, id)) openChip.value = -1
}

function clear(): void {
  openChip.value = -1
  views.select(ALL_SESSIONS_VIEW_ID)
}
</script>

<template>
  <!-- One line for a single chip. With more, the chips drop to a full-width row of their own, since beside the name and the controls each one would truncate to nothing. -->
  <div
    v-if="shown"
    class="flex shrink-0 flex-wrap items-center gap-x-1.5 gap-y-1 border-b border-border py-1 pr-1 pl-2.5"
    data-testid="code-view-bar"
  >
    <span
      class="flex h-5 max-w-[40%] shrink-0 items-center gap-1"
      :title="views.dirty ? 'Unsaved filter changes' : undefined"
    >
      <IconLayers class="size-3 shrink-0 text-accent" />
      <span class="truncate text-caption text-text-2" data-testid="code-view-name">{{ viewLabel }}</span>
      <span v-if="views.dirty" class="text-caption text-accent">•</span>
    </span>
    <div
      class="flex min-w-0 flex-1 flex-wrap items-center gap-1"
      :class="{ 'order-last basis-full': stacked }"
      data-testid="code-view-chips"
    >
      <button
        v-if="views.draft.conditions.length > 1"
        type="button"
        class="chip text-text-3"
        :title="views.draft.match === 'all' ? 'Rows must match every filter' : 'Rows may match any filter'"
        data-testid="code-view-match"
        @click="views.setMatch(views.draft.match === 'all' ? 'any' : 'all')"
      >
        {{ views.draft.match }}
      </button>
      <div
        v-for="(condition, index) in views.draft.conditions"
        :key="index"
        ref="chipAnchors"
        class="relative flex min-w-0 max-w-full"
      >
        <span
          class="chip min-w-0"
          data-testid="code-view-chip"
          :title="describeCondition(condition, viewMenus.repoName)"
        >
          <button
            type="button"
            class="min-w-0 cursor-pointer truncate"
            @click="openChip = openChip === index ? -1 : index"
          >
            {{ describeCondition(condition, viewMenus.repoName) }}
          </button>
          <button
            type="button"
            class="chip-remove"
            :aria-label="`Remove filter ${index + 1}`"
            data-testid="code-view-chip-remove"
            @click="views.removeCondition(index)"
          >
            <IconX class="size-2.5" />
          </button>
        </span>
        <AppMenu
          v-if="openChip === index"
          align="start"
          :entries="viewMenus.chipEntries(condition)"
          :ignore="[chipAnchors[index] ?? null]"
          width="12rem"
          @close="openChip = -1"
          @select="(id) => onChipSelect(index, id)"
        />
      </div>
    </div>
    <span class="ml-auto flex h-5 shrink-0 items-center gap-1">
      <template v-if="views.dirty">
        <IconButton
          label="Reset to the saved view"
          :icon="IconUndo"
          size="sm"
          data-testid="code-view-reset"
          @click="views.reset()"
        />
        <IconButton
          v-if="!views.activeIsBuiltIn"
          label="Save changes to this view"
          :icon="IconSave"
          size="sm"
          class="text-accent"
          data-testid="code-view-save"
          @click="views.save()"
        />
        <IconButton
          v-else
          label="Save as a new view…"
          :icon="IconSave"
          size="sm"
          class="text-accent"
          data-testid="code-view-save-new"
          @click="viewMenus.dialog = { kind: 'save-as' }"
        />
      </template>
      <span
        v-if="view.viewHiddenCount"
        class="flex items-center gap-0.5 font-mono text-caption text-text-4"
        data-testid="code-view-hidden"
        :title="`${view.viewHiddenCount} ${view.viewHiddenCount === 1 ? 'session' : 'sessions'} hidden by this view`"
      >
        <IconEyeOff class="size-3" />{{ view.viewHiddenCount }}
      </span>
      <IconButton label="Show all sessions" :icon="IconX" size="sm" data-testid="code-view-clear" @click="clear" />
    </span>
  </div>
</template>

<style scoped>
.chip {
  display: inline-flex;
  max-width: 100%;
  height: 18px;
  align-items: center;
  gap: 3px;
  border-radius: var(--radius-sm);
  background: var(--color-chip);
  padding: 0 4px 0 6px;
  font-size: 11px;
  line-height: 1;
  color: var(--color-text-2);
}
.chip-remove {
  display: inline-flex;
  cursor: pointer;
  color: var(--color-text-4);
}
.chip-remove:hover {
  color: var(--color-text);
}
</style>
