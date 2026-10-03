<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import IconGripVertical from '~icons/lucide/grip-vertical'
import IconPlus from '~icons/lucide/plus'
import InlineError from './ui/InlineError.vue'
import SettingsHeading from './settings/SettingsHeading.vue'
import SettingsPage from './settings/SettingsPage.vue'
import BaseBadge from './ui/BaseBadge.vue'
import BaseButton from './ui/BaseButton.vue'
import { appIcon } from './AppIcon.vue'
import ConfigItemCard from './settings/ConfigItemCard.vue'
import ActionEditor from './ActionEditor.vue'
import ConfirmationHost from './ui/ConfirmationHost.vue'
import EmptyState from './ui/EmptyState.vue'
import { useConfirmation } from '../composables/useConfirmation'
import { actionTypeMeta } from '../lib/actionPresentation'
import { moveId, type OrderDropTarget } from '../lib/listOrder'
import { useActionsSettings, type EditableAction } from '../composables/useActionsSettings'
import { SessionLaunchWorkspaces } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type { SessionLaunchWorkspace } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'

const props = withDefaults(defineProps<{ knownTypes?: string[] }>(), { knownTypes: () => [] })
const { actions, loading, error, create, update, remove, reorder } = useActionsSettings()
// What the editor autocompletes and validates against: live feed-item kinds
// (passed down from the app) unioned with types already configured on actions,
// deduped case-insensitively with the first-seen casing kept as canonical.
const editorTypes = computed(() => {
  const canonical = new Map<string, string>()
  for (const type of props.knownTypes)
    if (type && !canonical.has(type.toLowerCase())) canonical.set(type.toLowerCase(), type)
  for (const action of actions.value)
    for (const type of action.appliesTo ?? [])
      if (type && !canonical.has(type.toLowerCase())) canonical.set(type.toLowerCase(), type)
  return [...canonical.values()].sort((a, b) => a.localeCompare(b))
})
const editing = ref<EditableAction | null>(null)
const workspaces = ref<SessionLaunchWorkspace[]>([])
onMounted(async () => {
  try {
    workspaces.value = (await SessionLaunchWorkspaces()) ?? []
  } catch {
    workspaces.value = []
  }
})
const editorTrigger = ref<HTMLElement | null>(null)
const saving = ref(false)
const confirmation = useConfirmation()
const isNew = computed(() => !editing.value || !actions.value.some((action) => action.id === editing.value?.id))
function blank(): EditableAction {
  return {
    id: '',
    label: '',
    type: 'launch-session',
    showInDetail: true,
    targets: ['item'],
    appliesTo: [],
    launch: { promptTemplate: '', repoTemplate: '' },
  }
}
function setEditorTrigger(event: MouseEvent): void {
  editorTrigger.value = event.currentTarget instanceof HTMLElement ? event.currentTarget : null
}
function createNew(event: MouseEvent): void {
  setEditorTrigger(event)
  editing.value = blank()
}
function edit(action: EditableAction, event: MouseEvent): void {
  setEditorTrigger(event)
  editing.value = JSON.parse(JSON.stringify(action)) as EditableAction
}
async function save(): Promise<void> {
  if (!editing.value || saving.value) return
  saving.value = true
  try {
    const saved = isNew.value ? await create(editing.value) : await update(editing.value.id, editing.value)
    if (saved) editing.value = null
  } finally {
    saving.value = false
  }
}
function requestDelete(action: EditableAction): void {
  confirmation.request({
    title: 'Delete action',
    description: `Delete ${action.label}? Existing flows or active commands can block this action.`,
    confirmLabel: 'Delete action',
    onConfirm: async () => {
      if (!(await remove(action.id))) throw new Error(error.value || 'Could not delete action.')
    },
  })
}

// ── drag-and-drop ─────────────────────────────────────────────────────────
// The catalog list order is what the detail pane and item menu render, so a
// drop rewrites actions.yml's sequence. Native HTML5 DnD like the sidebar: the
// dragged id lives in a ref (dataTransfer can't be read during dragover) and
// the hovered edge drives the insertion indicator. The MIME payload is set so
// the drag carries data (some engines refuse to start an empty one) and is
// identifiable as this list's; the drop handlers read the ref, not the payload.
const ACTION_DRAG_MIME = 'application/x-hive-action'
const dragId = ref<string | null>(null)
const dropTarget = ref<OrderDropTarget | null>(null)

function onDragStart(event: DragEvent, id: string): void {
  dragId.value = id
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData(ACTION_DRAG_MIME, id)
  }
}
function onDragOver(event: DragEvent, id: string): void {
  if (!dragId.value) return
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  dropTarget.value = { id, edge: event.clientY < rect.top + rect.height / 2 ? 'before' : 'after' }
}
function onDrop(): void {
  const order =
    dragId.value && dropTarget.value
      ? moveId(
          actions.value.map((action) => action.id),
          dragId.value,
          dropTarget.value,
        )
      : null
  onDragEnd()
  if (order) void reorder(order)
}
function onDragEnd(): void {
  dragId.value = null
  dropTarget.value = null
}
function dropClass(id: string): Record<string, boolean> {
  const target = dropTarget.value
  return {
    dragging: dragId.value === id,
    'drop-before': target?.id === id && target.edge === 'before',
    'drop-after': target?.id === id && target.edge === 'after',
  }
}
</script>

<template>
  <SettingsPage testid="actions-settings">
    <SettingsHeading
      title="Actions"
      description="Drag to set the order they appear on an item. Detail visibility controls only manual feed-item buttons; flow nodes can still target any action."
    >
      <template #actions>
        <BaseButton size="sm" data-testid="action-create" @click="createNew">
          <template #icon><IconPlus class="size-3.5" :stroke-width="2.4" /></template>New action
        </BaseButton>
      </template>
    </SettingsHeading>

    <InlineError v-if="error && !editing" :message="error" testid="actions-error" />
    <p v-if="loading" class="text-xs text-text-4">Loading actions…</p>

    <div v-else class="flex flex-col gap-3">
      <ConfigItemCard
        v-for="action in actions"
        :key="action.id"
        :title="action.label"
        :icon="appIcon(actionTypeMeta(action.type).icon)"
        :data-testid="`action-row-${action.id}`"
        :class="dropClass(action.id)"
        draggable="true"
        @dragstart="onDragStart($event, action.id)"
        @dragover.prevent="onDragOver($event, action.id)"
        @drop.prevent="onDrop"
        @dragend="onDragEnd"
        @edit="edit(action, $event)"
        @delete="requestDelete(action)"
      >
        <template #leading>
          <span
            class="-mx-1.5 flex w-3 flex-none cursor-grab items-center justify-center text-text-4 opacity-0 transition-opacity group-hover/item:opacity-100 group-[.dragging]/item:opacity-100"
            aria-hidden="true"
            :data-testid="`action-grip-${action.id}`"
            ><IconGripVertical class="size-[15px]"
          /></span>
        </template>
        <template #badges>
          <BaseBadge class="border border-row !bg-app px-[7px] py-0.5 font-mono text-[11px]">{{ action.id }}</BaseBadge>
          <BaseBadge class="px-2 py-0.5 text-[11px] !text-text-2">{{ actionTypeMeta(action.type).label }}</BaseBadge>
          <BaseBadge class="px-2 py-0.5 text-[11px]">
            <span class="size-1.5 rounded-full" :class="action.showInDetail ? 'bg-severity-success' : 'bg-text-4'" />{{
              action.showInDetail ? 'Shown in detail' : 'Flow-only'
            }}
          </BaseBadge>
        </template>
      </ConfigItemCard>

      <EmptyState v-if="!actions.length" message="No actions configured." />
      <div
        v-else
        class="mt-1 flex items-center gap-1.5 font-mono text-[11.5px] text-text-4"
        data-testid="actions-source"
      >
        Synced from .hive/actions.yml · {{ actions.length }} {{ actions.length === 1 ? 'action' : 'actions' }}
      </div>
    </div>

    <ActionEditor
      v-if="editing"
      v-model:action="editing"
      :is-new="isNew"
      :busy="saving"
      :error="error"
      :known-types="editorTypes"
      :workspaces="workspaces"
      :return-focus-to="editorTrigger"
      @save="save"
      @cancel="editing = null"
    />
    <ConfirmationHost :confirmation="confirmation" />
  </SettingsPage>
</template>
