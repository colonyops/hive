<script setup lang="ts">
import { computed, ref } from 'vue'
import IconPlus from '~icons/lucide/plus'
import IconTrash2 from '~icons/lucide/trash-2'
import BaseButton from './BaseButton.vue'
import BaseCard from './BaseCard.vue'
import BaseIconBadge from './BaseIconBadge.vue'
import AppIcon from './AppIcon.vue'
import ActionEditor from './ActionEditor.vue'
import ConfirmationDialog from './ConfirmationDialog.vue'
import { useConfirmation } from '../composables/useConfirmation'
import { actionTypeMeta } from '../lib/actionPresentation'
import { useActionsSettings, type EditableAction } from '../composables/useActionsSettings'

const props = withDefaults(defineProps<{ knownTypes?: string[] }>(), { knownTypes: () => [] })
const { actions, loading, error, create, update, remove } = useActionsSettings()
// What the editor autocompletes and validates against: live feed-item kinds
// (passed down from the app) unioned with types already configured on actions,
// deduped case-insensitively with the first-seen casing kept as canonical.
const editorTypes = computed(() => {
  const canonical = new Map<string, string>()
  for (const type of props.knownTypes) if (type && !canonical.has(type.toLowerCase())) canonical.set(type.toLowerCase(), type)
  for (const action of actions.value) for (const type of action.appliesTo ?? []) if (type && !canonical.has(type.toLowerCase())) canonical.set(type.toLowerCase(), type)
  return [...canonical.values()].sort((a, b) => a.localeCompare(b))
})
const editing = ref<EditableAction | null>(null)
const editorTrigger = ref<HTMLElement | null>(null)
const saving = ref(false)
const confirmation = useConfirmation()
const isNew = computed(() => !editing.value || !actions.value.some((action) => action.id === editing.value?.id))
function blank(): EditableAction { return { id: '', label: '', type: 'launch-session', showInDetail: true, appliesTo: [], launch: { promptTemplate: '', repoTemplate: '' } } }
function setEditorTrigger(event: MouseEvent): void { editorTrigger.value = event.currentTarget instanceof HTMLElement ? event.currentTarget : null }
function createNew(event: MouseEvent): void { setEditorTrigger(event); editing.value = blank() }
function edit(action: EditableAction, event: MouseEvent): void { setEditorTrigger(event); editing.value = JSON.parse(JSON.stringify(action)) as EditableAction }
async function save(): Promise<void> { if (!editing.value || saving.value) return; saving.value = true; try { const saved = isNew.value ? await create(editing.value) : await update(editing.value.id, editing.value); if (saved) editing.value = null } finally { saving.value = false } }
function requestDelete(action: EditableAction): void { confirmation.request({ title: 'Delete action', description: `Delete ${action.label}? Existing flows or active commands can block this action.`, confirmLabel: 'Delete action', onConfirm: async () => { if (!await remove(action.id)) throw new Error(error.value || 'Could not delete action.') } }) }
</script>

<template>
  <div class="mx-auto max-w-[720px]" data-testid="actions-settings">
    <div class="mb-5 flex items-start gap-4">
      <div class="flex-1">
        <h2 class="text-[15px] font-semibold text-text">Actions</h2>
        <p class="mt-1 text-xs leading-relaxed text-text-3">Detail visibility controls only manual feed-item buttons. Flow nodes can still target any action.</p>
      </div>
      <BaseButton
        size="sm"
        class="shrink-0"
        data-testid="action-create"
        @click="createNew"
      ><template #icon><IconPlus class="size-3.5" :stroke-width="2.4" /></template>New action</BaseButton>
    </div>

    <p v-if="error && !editing" class="mb-3 rounded border border-severity-error bg-severity-error-tint px-3 py-2 text-xs text-severity-error" data-testid="actions-error">{{ error }}</p>
    <p v-if="loading" class="text-xs text-text-4">Loading actions…</p>

    <div v-else class="flex flex-col gap-3">
      <BaseCard
        v-for="action in actions"
        :key="action.id"
        :padded="false"
        class="gap-4 rounded-[11px] border border-card bg-raised px-4 py-3.5 transition-colors hover:border-strong"
        :data-testid="`action-row-${action.id}`"
      >
        <template #icon>
          <BaseIconBadge :size="38" rounded="rounded-[10px]" class="border border-[rgba(245,158,11,0.35)] bg-[rgba(245,158,11,0.13)] text-accent">
            <AppIcon :name="actionTypeMeta(action.type).icon" class="size-[17px]" />
          </BaseIconBadge>
        </template>
        <div class="min-w-0 flex-1">
          <div class="truncate text-[15px] font-semibold tracking-[-.01em] text-text">{{ action.label }}</div>
          <div class="mt-1.5 flex flex-wrap items-center gap-1.5">
            <span class="rounded-[5px] border border-row bg-app px-[7px] py-0.5 font-mono text-[11px] text-text-3">{{ action.id }}</span>
            <span class="rounded-[5px] bg-chip px-2 py-0.5 text-[11px] text-text-2">{{ actionTypeMeta(action.type).label }}</span>
            <span class="inline-flex items-center gap-1.5 rounded-[5px] bg-chip px-2 py-0.5 text-[11px] text-text-3">
              <span class="size-1.5 rounded-full" :class="action.showInDetail ? 'bg-severity-success' : 'bg-text-4'" />{{ action.showInDetail ? 'Shown in detail' : 'Flow-only' }}
            </span>
          </div>
        </div>
        <template #actions>
          <div class="flex shrink-0 items-center gap-2">
            <button class="rounded-[7px] border border-card px-3.5 py-1.5 text-[12.5px] text-text-2 hover:border-strong hover:text-text" @click="edit(action, $event)">Edit</button>
            <button class="flex size-[34px] items-center justify-center rounded-[7px] border border-card text-text-3 hover:border-severity-error-border hover:text-severity-error" aria-label="Delete" @click="requestDelete(action)"><IconTrash2 class="size-[15px]" /></button>
          </div>
        </template>
      </BaseCard>

      <p v-if="!actions.length" class="py-8 text-center text-xs text-text-4">No actions configured.</p>
      <div v-else class="mt-1 flex items-center gap-1.5 font-mono text-[11.5px] text-text-4" data-testid="actions-source">Synced from .hive/actions.yml · {{ actions.length }} {{ actions.length === 1 ? 'action' : 'actions' }}</div>
    </div>

    <ActionEditor v-if="editing" :action="editing" :is-new="isNew" :busy="saving" :error="error" :known-types="editorTypes" :return-focus-to="editorTrigger" @save="save" @cancel="editing = null" />
    <ConfirmationDialog v-if="confirmation.open.value && confirmation.options.value" :title="confirmation.options.value.title" :description="confirmation.options.value.description" :confirm-label="confirmation.options.value.confirmLabel" :busy="confirmation.busy.value" :error="confirmation.error.value" @confirm="confirmation.confirm" @cancel="confirmation.cancel" />
  </div>
</template>
