<script setup lang="ts">
// The active view under the Code sidebar's toolbar: its name, the filter it
// applies as chips, and Save / Reset once the chips drift from the saved view.
// Absent on All sessions with no filter, so an unfiltered tree carries no chrome.
// Modeled on Linear's view header, cut down for a narrow sidebar.
import { computed, proxyRefs, ref } from 'vue'
import IconLayers from '~icons/lucide/layers'
import IconX from '~icons/lucide/x'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import { ALL_SESSIONS_VIEW_ID, describeCondition } from '../../lib/sessionViews'
import { useCodeViews } from '../../stores/useCodeViews'
import { useTerminalModeContext } from './terminalModeContext'

const { view, viewMenus } = useTerminalModeContext()
const views = proxyRefs(useCodeViews())

const shown = computed(() => views.activeViewId !== ALL_SESSIONS_VIEW_ID || views.draft.conditions.length > 0)
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
  <div v-if="shown" class="shrink-0 border-b border-border" data-testid="code-view-bar">
    <div class="flex h-7 items-center gap-1.5 pr-1 pl-3">
      <IconLayers class="size-3 shrink-0 text-accent" />
      <span class="min-w-0 truncate text-caption text-text-2" data-testid="code-view-name">{{ viewLabel }}</span>
      <span v-if="views.dirty" class="text-caption text-accent" title="Unsaved filter changes">•</span>
      <span class="flex-1" />
      <span
        v-if="view.viewHiddenCount"
        class="shrink-0 font-mono text-caption text-text-4"
        data-testid="code-view-hidden"
        :title="`${view.viewHiddenCount} sessions hidden by this view`"
        >{{ view.viewHiddenCount }} hidden</span
      >
      <IconButton label="Show all sessions" :icon="IconX" size="sm" data-testid="code-view-clear" @click="clear" />
    </div>

    <div
      v-if="views.draft.conditions.length || views.dirty"
      class="flex flex-wrap items-center gap-1 px-2 pb-2"
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
        Match {{ views.draft.match }}
      </button>
      <div v-for="(condition, index) in views.draft.conditions" :key="index" ref="chipAnchors" class="relative flex">
        <span class="chip" data-testid="code-view-chip">
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
            <IconX class="size-3" />
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
      <div v-if="views.dirty" class="ml-auto flex shrink-0 items-center gap-2">
        <button type="button" class="bar-action" data-testid="code-view-reset" @click="views.reset()">Reset</button>
        <button
          v-if="!views.activeIsBuiltIn"
          type="button"
          class="bar-action text-accent"
          data-testid="code-view-save"
          @click="views.save()"
        >
          Save
        </button>
        <button
          v-else
          type="button"
          class="bar-action text-accent"
          data-testid="code-view-save-new"
          @click="viewMenus.dialog = { kind: 'save-as' }"
        >
          Save view
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chip {
  display: inline-flex;
  max-width: 100%;
  align-items: center;
  gap: 0.25rem;
  border: 1px solid var(--color-border);
  border-radius: 999px;
  background: var(--color-chip);
  padding: 0.0625rem 0.375rem 0.0625rem 0.5rem;
  font-size: var(--text-caption);
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
.bar-action {
  cursor: pointer;
  font-size: var(--text-caption);
  color: var(--color-text-3);
}
.bar-action:hover {
  color: var(--color-text);
}
</style>
