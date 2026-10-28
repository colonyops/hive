<script setup lang="ts">
// Renames, reorders, and deletes the saved Code views. The list menu only
// switches between them; this is where they are kept tidy.
import { proxyRefs, ref } from 'vue'
import IconGripVertical from '~icons/lucide/grip-vertical'
import IconLayers from '~icons/lucide/layers'
import IconTrash from '~icons/lucide/trash-2'
import DrawerSheet from '@hive/ui/components/ui/DrawerSheet.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import InlineConfirm from '@hive/ui/components/ui/InlineConfirm.vue'
import TextInput from '@hive/ui/components/ui/TextInput.vue'
import { dropClass, dropEdge, useDragReorder } from '../../composables/useDragReorder'
import { moveId, type OrderDropTarget } from '../../lib/listOrder'
import { conditionQualifier, type SavedSessionView } from '../../lib/sessionViews'
import { useCodeViews } from '../../stores/useCodeViews'
import { useTerminalModeContext } from './terminalModeContext'

const emit = defineEmits<{ close: [] }>()

const { viewMenus } = useTerminalModeContext()
const views = proxyRefs(useCodeViews())

const confirmingId = ref('')

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

function remove(id: string): void {
  confirmingId.value = ''
  views.remove(id)
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
    subtitle="The Code sidebar's views, in menu order"
    :icon="IconLayers"
    closable
    :width="380"
    testid="code-views-drawer"
    @close="emit('close')"
  >
    <p v-if="!views.views.length" class="px-1 py-2 text-body text-text-3" data-testid="code-views-drawer-empty">
      No saved views. Add a filter from the sidebar, then use Save view on the bar.
    </p>
    <ul v-else class="flex flex-col gap-1">
      <li
        v-for="view in views.views"
        :key="view.id"
        class="view-row"
        :class="[dropClass(drag.target.value, view.id), { dragging: drag.dragging.value === view.id }]"
        data-testid="code-views-drawer-row"
        :data-view="view.id"
        @dragover.prevent="drag.over($event, { id: view.id, edge: dropEdge($event) })"
        @drop.prevent="drag.drop()"
      >
        <div class="flex items-center gap-2">
          <span
            class="grip"
            draggable="true"
            :aria-label="`Drag to reorder ${view.name}`"
            data-testid="code-views-drawer-grip"
            @dragstart="drag.start($event, view.id)"
            @dragend="drag.end()"
          >
            <IconGripVertical class="size-3.5" />
          </span>
          <TextInput
            :model-value="view.name"
            size="sm"
            class="min-w-0 flex-1"
            :aria-label="`Name of ${view.name}`"
            data-testid="code-views-drawer-name"
            @blur="commitName(view, $event)"
            @keydown.enter.prevent="($event.target as HTMLInputElement).blur()"
          />
          <IconButton
            :label="`Delete ${view.name}`"
            :icon="IconTrash"
            size="sm"
            data-testid="code-views-drawer-delete"
            @click="confirmingId = confirmingId === view.id ? '' : view.id"
          />
        </div>
        <p class="truncate pl-6 text-caption text-text-4" :title="summary(view)">{{ summary(view) }}</p>
        <InlineConfirm
          v-if="confirmingId === view.id"
          :title="`Delete “${view.name}”?`"
          description="The view is removed from settings.yaml. Sessions are not touched."
          testid="code-views-drawer-confirm"
          @confirm="remove(view.id)"
          @cancel="confirmingId = ''"
        />
      </li>
    </ul>
  </DrawerSheet>
</template>

<style scoped>
.view-row {
  display: flex;
  flex-direction: column;
  gap: 2px;
  border-radius: var(--radius-md);
  padding: 6px 4px;
  border-top: 2px solid transparent;
  border-bottom: 2px solid transparent;
}
.view-row.dragging {
  opacity: 0.5;
}
.view-row.drop-before {
  border-top-color: var(--color-accent);
}
.view-row.drop-after {
  border-bottom-color: var(--color-accent);
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
