<script setup lang="ts">
import { computed, ref } from 'vue'
import ActionEditor from './ActionEditor.vue'
import { useActionsSettings, type EditableAction } from '../composables/useActionsSettings'

const { actions, loading, error, create, update, remove } = useActionsSettings()
const editing = ref<EditableAction | null>(null)
const deleting = ref<string | null>(null)
const isNew = computed(() => !editing.value || !actions.value.some((action) => action.id === editing.value?.id))

function blank(): EditableAction {
  return { id: '', label: '', type: 'launch-session', showInDetail: true, appliesTo: [], launch: { promptTemplate: '', repoTemplate: '' } }
}
function edit(action: EditableAction): void { editing.value = JSON.parse(JSON.stringify(action)) as EditableAction }
async function save(): Promise<void> {
  if (!editing.value) return
  const saved = isNew.value ? await create(editing.value) : await update(editing.value.id, editing.value)
  if (saved) editing.value = null
}
async function confirmDelete(): Promise<void> { if (deleting.value && await remove(deleting.value)) deleting.value = null }
</script>

<template>
  <div class="mx-auto max-w-[720px]" data-testid="actions-settings">
    <div class="mb-5 flex items-start gap-4"><div class="flex-1"><h2 class="text-[15px] font-semibold text-text">Actions</h2><p class="mt-1 text-xs leading-relaxed text-text-3">Detail visibility controls only manual feed-item buttons. Flow nodes can still target actions.</p></div><button class="rounded border border-strong px-3 py-1.5 text-xs text-text hover:text-accent" data-testid="action-create" @click="editing = blank()">New action</button></div>
    <p v-if="error" class="mb-3 rounded border border-severity-error bg-severity-error-tint px-3 py-2 text-xs text-severity-error" data-testid="actions-error">{{ error }}</p>
    <p v-if="loading" class="text-xs text-text-4">Loading actions…</p>
    <div v-else class="flex flex-col gap-2">
      <article v-for="action in actions" :key="action.id" class="flex items-center gap-3 rounded border border-border bg-raised px-3 py-2.5" :data-testid="`action-row-${action.id}`"><div class="min-w-0 flex-1"><div class="text-sm font-medium text-text">{{ action.label }}</div><div class="font-mono text-[11px] text-text-4">{{ action.id }} · {{ action.type }} · {{ action.showInDetail ? 'shown in detail' : 'flow-only' }}</div></div><button class="text-xs text-text-2 hover:text-text" @click="edit(action)">Edit</button><button class="text-xs text-severity-error" @click="deleting = action.id">Delete</button></article>
      <p v-if="!actions.length" class="py-8 text-center text-xs text-text-4">No actions configured.</p>
    </div>
    <ActionEditor v-if="editing" :action="editing" :is-new="isNew" @save="save" @cancel="editing = null" />
    <div v-if="deleting" class="mt-4 rounded border border-severity-error bg-severity-error-tint p-3 text-xs text-text" data-testid="action-delete-confirm">Delete <strong>{{ deleting }}</strong>? Existing flows or active commands will block this action.<div class="mt-3 flex gap-2"><button class="rounded bg-severity-error px-3 py-1.5 text-white" @click="confirmDelete">Delete</button><button class="px-3 py-1.5" @click="deleting = null">Cancel</button></div></div>
  </div>
</template>
