<script setup lang="ts">
// The active filter under the Code sidebar's toolbar, as chips, with Save /
// Reset once the chips drift from the saved view. The view's name lives in the
// list menu; absent when nothing is filtered, so an unfiltered tree carries no chrome.
import { computed, proxyRefs, ref } from 'vue'
import IconEyeOff from '~icons/lucide/eye-off'
import IconSave from '~icons/lucide/save'
import IconUndo from '~icons/lucide/undo-2'
import IconX from '~icons/lucide/x'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import { ALL_SESSIONS_VIEW_ID, conditionQualifier, describeCondition } from '../../lib/sessionViews'
import { useCodeViews } from '../../stores/useCodeViews'
import { useTerminalModeContext } from './terminalModeContext'

const { view, viewMenus } = useTerminalModeContext()
const views = proxyRefs(useCodeViews())

// Dirty keeps the bar up on a saved view whose last chip was just removed, so Save and Reset stay in reach.
const shown = computed(() => views.draft.conditions.length > 0 || views.dirty)

const openChip = ref(-1)
const chipAnchors = ref<HTMLElement[]>([])

function clear(): void {
  openChip.value = -1
  views.select(ALL_SESSIONS_VIEW_ID)
}
</script>

<template>
  <div
    v-if="shown"
    class="flex shrink-0 items-start gap-1.5 border-b border-border py-1 pr-1 pl-2"
    data-testid="code-view-bar"
  >
    <div class="flex min-w-0 flex-1 flex-wrap items-center gap-1" data-testid="code-view-chips">
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
            {{ conditionQualifier(condition, viewMenus.repoName) }}
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
          @select="(id) => viewMenus.onChipSelect(index, id)"
        />
      </div>
    </div>
    <span class="flex h-5 shrink-0 items-center gap-1">
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
          :label="`Save changes to ${views.activeView?.name}`"
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
          @click="viewMenus.saving = true"
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
