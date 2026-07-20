<script setup lang="ts">
import { computed, ref } from 'vue'
import ActionEditor from './ActionEditor.vue'
import ConfirmationDialog from './ConfirmationDialog.vue'
import { useConfirmation } from '../composables/useConfirmation'
import { useActionsSettings, type EditableAction } from '../composables/useActionsSettings'

const { actions, loading, error, create, update, remove } = useActionsSettings()
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
    <div class="mb-5 flex items-start gap-4"><div class="flex-1"><h2 class="text-[15px] font-semibold text-text">Actions</h2><p class="mt-1 text-xs leading-relaxed text-text-3">Detail visibility controls only manual feed-item buttons. Flow nodes can still target actions.</p></div><button class="rounded border border-strong px-3 py-1.5 text-xs text-text hover:text-accent" data-testid="action-create" @click="createNew">New action</button></div>
    <p v-if="error && !editing" class="mb-3 rounded border border-severity-error bg-severity-error-tint px-3 py-2 text-xs text-severity-error" data-testid="actions-error">{{ error }}</p>
    <p v-if="loading" class="text-xs text-text-4">Loading actions…</p>
    <div v-else class="flex flex-col gap-2"><article v-for="action in actions" :key="action.id" class="flex items-center gap-3 rounded border border-border bg-raised px-3 py-2.5" :data-testid="`action-row-${action.id}`"><div class="min-w-0 flex-1"><div class="text-sm font-medium text-text">{{ action.label }}</div><div class="font-mono text-[11px] text-text-4">{{ action.id }} · {{ action.type }} · {{ action.showInDetail ? 'shown in detail' : 'flow-only' }}</div></div><button class="text-xs text-text-2 hover:text-text" @click="edit(action, $event)">Edit</button><button class="text-xs text-severity-error" @click="requestDelete(action)">Delete</button></article><p v-if="!actions.length" class="py-8 text-center text-xs text-text-4">No actions configured.</p></div>
    <ActionEditor v-if="editing" :action="editing" :is-new="isNew" :busy="saving" :error="error" :return-focus-to="editorTrigger" @save="save" @cancel="editing = null" />
    <ConfirmationDialog v-if="confirmation.open.value && confirmation.options.value" :title="confirmation.options.value.title" :description="confirmation.options.value.description" :confirm-label="confirmation.options.value.confirmLabel" :busy="confirmation.busy.value" :error="confirmation.error.value" @confirm="confirmation.confirm" @cancel="confirmation.cancel" />
  </div>
</template>
