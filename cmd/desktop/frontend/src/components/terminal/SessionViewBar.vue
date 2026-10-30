<script setup lang="ts">
// The active filter under the Code sidebar's toolbar, as chips. Saving,
// resetting, and the hidden count live in the filter dropdown and the view's
// name in the list menu; absent when nothing is filtered, so an unfiltered tree
// carries no chrome.
import { computed, proxyRefs, ref } from 'vue'
import IconX from '~icons/lucide/x'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import { ALL_SESSIONS_VIEW_ID, conditionQualifier, describeCondition } from '../../lib/sessionViews'
import { useCodeViews } from '../../stores/useCodeViews'
import { useTerminalModeContext } from './terminalModeContext'

const { viewMenus } = useTerminalModeContext()
const views = proxyRefs(useCodeViews())

const shown = computed(() => views.draft.conditions.length > 0)

const openChip = ref(-1)
const chipAnchors = ref<HTMLElement[]>([])

// The open menu ignores clicks inside its chip, remove button included, so it
// never closes itself here; keep openChip on the condition it was opened for.
// An empty filter is All sessions; left on its view it would read as an unsaved change.
function removeChip(index: number): void {
  if (views.draft.conditions.length === 1) return clear()
  if (openChip.value === index) openChip.value = -1
  else if (openChip.value > index) openChip.value--
  views.removeCondition(index)
}

function clear(): void {
  openChip.value = -1
  views.select(ALL_SESSIONS_VIEW_ID)
}
</script>

<template>
  <div
    v-if="shown"
    class="flex shrink-0 flex-wrap items-center gap-x-1 gap-y-1.5 border-b border-border px-2 py-1.5"
    data-testid="code-view-bar"
  >
    <template v-for="(condition, index) in views.draft.conditions" :key="index">
      <button
        v-if="index > 0"
        type="button"
        class="joiner"
        :title="
          views.draft.match === 'all'
            ? 'Sessions match every filter. Click to match any.'
            : 'Sessions match any filter. Click to match every one.'
        "
        data-testid="code-view-match"
        @click="views.setMatch(views.draft.match === 'all' ? 'any' : 'all')"
      >
        {{ views.draft.match === 'all' ? 'and' : 'or' }}
      </button>
      <div ref="chipAnchors" class="relative flex min-w-0 max-w-full">
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
            {{ conditionQualifier(condition, viewMenus.repoName) }}
          </button>
          <button
            type="button"
            class="chip-remove"
            :aria-label="`Remove filter ${index + 1}`"
            data-testid="code-view-chip-remove"
            @click="removeChip(index)"
          >
            <IconX class="size-2.5" />
          </button>
        </span>
        <AppMenu
          v-if="openChip === index"
          align="start"
          :entries="viewMenus.chipEntries(condition)"
          :ignore="[chipAnchors[index] ?? null]"
          testid="code-view-chip-menu"
          width="12rem"
          @close="openChip = -1"
          @select="(id) => viewMenus.onChipSelect(index, id)"
        />
      </div>
    </template>
    <IconButton
      label="Show all sessions"
      :icon="IconX"
      size="sm"
      class="ml-auto"
      data-testid="code-view-clear"
      @click="clear"
    />
  </div>
</template>

<style scoped>
.chip {
  display: inline-flex;
  max-width: 100%;
  height: 20px;
  align-items: center;
  gap: 3px;
  border-radius: var(--radius-sm);
  background: var(--color-chip);
  padding: 0 4px 0 6px;
  font-size: 11px;
  line-height: 1;
  color: var(--color-text-2);
}
.joiner {
  cursor: pointer;
  padding: 0 1px;
  font-size: 11px;
  color: var(--color-text-4);
}
.joiner:hover {
  color: var(--color-text);
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
