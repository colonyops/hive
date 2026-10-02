<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import IconGripVertical from '~icons/lucide/grip-vertical'
import IconPlus from '~icons/lucide/plus'
import IconTrash2 from '~icons/lucide/trash-2'
import SettingsError from './settings/SettingsError.vue'
import SettingsHeading from './settings/SettingsHeading.vue'
import SettingsPage from './settings/SettingsPage.vue'
import BaseBadge from './BaseBadge.vue'
import BaseButton from './BaseButton.vue'
import BaseCard from './BaseCard.vue'
import BaseIconBadge from './BaseIconBadge.vue'
import AppIcon from './AppIcon.vue'
import ActionEditor from './ActionEditor.vue'
import ConfirmationDialog from './ConfirmationDialog.vue'
import EmptyState from './settings/EmptyState.vue'
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

    <SettingsError v-if="error && !editing" :message="error" testid="actions-error" />
    <p v-if="loading" class="text-xs text-text-4">Loading actions…</p>

    <div v-else class="flex flex-col gap-3">
      <BaseCard
        v-for="action in actions"
        :key="action.id"
        :padded="false"
        class="action-row flex-wrap items-start gap-3 rounded-[11px] border border-card bg-raised px-4 py-3.5 transition-colors hover:border-strong @[600px]/pane:flex-nowrap @[600px]/pane:items-center @[600px]/pane:gap-4"
        :class="dropClass(action.id)"
        :data-testid="`action-row-${action.id}`"
        draggable="true"
        @dragstart="onDragStart($event, action.id)"
        @dragover.prevent="onDragOver($event, action.id)"
        @drop.prevent="onDrop"
        @dragend="onDragEnd"
      >
        <template #icon>
          <span class="drag-grip" aria-hidden="true" :data-testid="`action-grip-${action.id}`"
            ><IconGripVertical class="size-[15px]"
          /></span>
          <BaseIconBadge :size="38" rounded="rounded-[10px]" class="border border-accent/35 bg-accent-tint text-accent">
            <AppIcon :name="actionTypeMeta(action.type).icon" class="size-[17px]" />
          </BaseIconBadge>
        </template>
        <div class="min-w-0 flex-1">
          <div class="truncate text-[15px] font-semibold tracking-[-.01em] text-text">{{ action.label }}</div>
          <div class="mt-1.5 flex flex-wrap items-center gap-1.5">
            <BaseBadge class="border border-row !bg-app px-[7px] py-0.5 font-mono text-[11px]">{{
              action.id
            }}</BaseBadge>
            <BaseBadge class="px-2 py-0.5 text-[11px] !text-text-2">{{ actionTypeMeta(action.type).label }}</BaseBadge>
            <BaseBadge class="px-2 py-0.5 text-[11px]">
              <span
                class="size-1.5 rounded-full"
                :class="action.showInDetail ? 'bg-severity-success' : 'bg-text-4'"
              />{{ action.showInDetail ? 'Shown in detail' : 'Flow-only' }}
            </BaseBadge>
          </div>
        </div>
        <template #actions>
          <div class="flex w-full items-center justify-end gap-2 @[600px]/pane:w-auto @[600px]/pane:shrink-0">
            <button
              class="rounded-[7px] border border-card px-3.5 py-1.5 text-[12.5px] text-text-2 hover:border-strong hover:text-text"
              @click="edit(action, $event)"
            >
              Edit
            </button>
            <button
              class="flex size-[34px] items-center justify-center rounded-[7px] border border-card text-text-3 hover:border-severity-error-border hover:text-severity-error"
              aria-label="Delete"
              @click="requestDelete(action)"
            >
              <IconTrash2 class="size-[15px]" />
            </button>
          </div>
        </template>
      </BaseCard>

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
      :action="editing"
      :is-new="isNew"
      :busy="saving"
      :error="error"
      :known-types="editorTypes"
      :workspaces="workspaces"
      :return-focus-to="editorTrigger"
      @save="save"
      @cancel="editing = null"
    />
    <ConfirmationDialog
      v-if="confirmation.open.value && confirmation.options.value"
      :title="confirmation.options.value.title"
      :description="confirmation.options.value.description"
      :confirm-label="confirmation.options.value.confirmLabel"
      :busy="confirmation.busy.value"
      :error="confirmation.error.value"
      @confirm="confirmation.confirm"
      @cancel="confirmation.cancel"
    />
  </SettingsPage>
</template>

<style scoped>
/* The grip is the affordance; the whole row is the drag source, so a drag can
   start anywhere on it (its buttons still take their own clicks). */
.drag-grip {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 12px;
  margin: 0 -6px;
  color: var(--color-text-4);
  cursor: grab;
  opacity: 0;
  transition: opacity 0.12s ease;
}
.action-row:hover .drag-grip,
.action-row.dragging .drag-grip {
  opacity: 1;
}
.action-row.dragging {
  opacity: 0.45;
}

/* The insertion line floats in the gap between cards rather than lighting up a
   card's own border, which reads as an edit to that card and sits badly against
   the 11px corners. */
.action-row {
  position: relative;
}
.action-row::before,
.action-row::after {
  content: '';
  position: absolute;
  left: 8px;
  right: 8px;
  height: 2px;
  border-radius: 1px;
  background: var(--color-accent);
  opacity: 0;
  pointer-events: none;
}
.action-row::before {
  top: -7px;
}
.action-row::after {
  bottom: -7px;
}
.action-row.drop-before::before,
.action-row.drop-after::after {
  opacity: 1;
}
</style>
