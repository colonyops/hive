<script setup lang="ts">
// Renames, reorders, and deletes the saved Code views. The list menu only
// switches between them; this is where they are kept tidy.
import { computed, proxyRefs, ref } from 'vue'
import IconGripVertical from '~icons/lucide/grip-vertical'
import IconLayers from '~icons/lucide/layers'
import IconTrash2 from '~icons/lucide/trash-2'
import DrawerSheet from '@hive/ui/components/ui/DrawerSheet.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import InlineConfirm from '@hive/ui/components/ui/InlineConfirm.vue'
import TextInput from '@hive/ui/components/ui/TextInput.vue'
import SettingsSection from '../settings/SettingsSection.vue'
import { dropClass, dropEdge, useDragReorder } from '../../composables/useDragReorder'
import { moveId, type OrderDropTarget } from '../../lib/listOrder'
import { conditionQualifier, type SavedSessionView } from '../../lib/sessionViews'
import { useCodeViews } from '../../stores/useCodeViews'
import { useTerminalModeContext } from './terminalModeContext'

const emit = defineEmits<{ close: [] }>()

const { viewMenus } = useTerminalModeContext()
const views = proxyRefs(useCodeViews())

const confirmingId = ref('')
const confirming = computed(() => views.views.find((view) => view.id === confirmingId.value))

function summary(view: SavedSessionView): string {
  if (!view.filter.conditions.length) return 'No filter'
  const joiner = view.filter.match === 'any' ? ' OR ' : ' '
  return view.filter.conditions.map((c) => conditionQualifier(c, viewMenus.repoName)).join(joiner)
}

// Renames commit on blur or Enter, not per keystroke: each commit rewrites settings.yaml.
function commitName(view: SavedSessionView, event: Event): void {
  const input = event.target as HTMLInputElement
  const name = input.value.trim()
  if (!name) input.value = view.name
  else if (name !== view.name) views.rename(view.id, name)
}

function remove(): void {
  views.remove(confirmingId.value)
  confirmingId.value = ''
}

const drag = useDragReorder<string>({
  mime: 'application/x-hive-code-view',
  onDrop(dragged, target: OrderDropTarget) {
    const next = moveId(
      views.views.map((v) => v.id),
      dragged,
      target,
    )
    if (next) views.reorder(next)
  },
})
</script>

<template>
  <DrawerSheet
    title="Saved views"
    subtitle="Code sidebar"
    :icon="IconLayers"
    closable
    :width="380"
    testid="code-views-drawer"
    @close="emit('close')"
  >
    <SettingsSection title="Views" description="Drag to set the menu order." boxed>
      <div
        v-for="view in views.views"
        :key="view.id"
        class="view-row relative flex items-start gap-2 py-3 pr-2 pl-1.5"
        :class="[dropClass(drag.target.value, view.id), { dragging: drag.dragging.value === view.id }]"
        data-testid="code-views-drawer-row"
        :data-view="view.id"
        @dragover.prevent="drag.over($event, { id: view.id, edge: dropEdge($event) })"
        @drop.prevent="drag.drop()"
      >
        <span
          class="grip mt-1.5"
          draggable="true"
          :aria-label="`Drag to reorder ${view.name}`"
          data-testid="code-views-drawer-grip"
          @dragstart="drag.start($event, view.id)"
          @dragend="drag.end()"
        >
          <IconGripVertical class="size-3.5" />
        </span>
        <div class="min-w-0 flex-1">
          <TextInput
            :model-value="view.name"
            size="sm"
            :aria-label="`Name of ${view.name}`"
            data-testid="code-views-drawer-name"
            @blur="commitName(view, $event)"
            @keydown.enter.prevent="($event.target as HTMLInputElement).blur()"
          />
          <div class="mt-1 truncate px-0.5 font-mono text-caption text-text-4" :title="summary(view)">
            {{ summary(view) }}
          </div>
        </div>
        <IconButton
          :label="`Delete ${view.name}`"
          :icon="IconTrash2"
          size="lg"
          tone="danger"
          data-testid="code-views-drawer-delete"
          @click="confirmingId = view.id"
        />
      </div>
      <EmptyState
        v-if="!views.views.length"
        variant="inline"
        class="px-4 py-3.5"
        message="No saved views. Add a filter from the sidebar, then save it from the bar."
        data-testid="code-views-drawer-empty"
      />
    </SettingsSection>

    <template v-if="confirming" #footer>
      <!-- The strip escapes the footer's own padding so it reads as the sheet's bottom edge. -->
      <InlineConfirm
        class="-mx-[18px] -my-[13px]"
        :title="`Delete “${confirming.name}”?`"
        description="It leaves settings.yaml. Sessions are not touched."
        testid="code-views-drawer-confirm"
        @confirm="remove"
        @cancel="confirmingId = ''"
      />
    </template>
  </DrawerSheet>
</template>

<style scoped>
.view-row.dragging {
  opacity: 0.45;
}
/* The insertion line sits on the hairline between rows, as ConfigItemCard's does between cards. */
.view-row::before,
.view-row::after {
  content: '';
  position: absolute;
  left: 8px;
  right: 8px;
  height: 2px;
  border-radius: var(--radius-xs);
  background: var(--color-accent);
  opacity: 0;
  pointer-events: none;
}
.view-row::before {
  top: -1px;
}
.view-row::after {
  bottom: -1px;
}
.view-row.drop-before::before,
.view-row.drop-after::after {
  opacity: 1;
}
.grip {
  display: inline-flex;
  cursor: grab;
  color: var(--color-text-4);
}
.grip:hover {
  color: var(--color-text-2);
}
</style>
