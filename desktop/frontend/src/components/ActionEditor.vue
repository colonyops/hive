<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import IconPlay from '~icons/lucide/play'
import IconX from '~icons/lucide/x'
import BaseButton from './BaseButton.vue'
import AppCheckbox from './AppCheckbox.vue'
import AppSelect from './AppSelect.vue'
import AppliesToField from './AppliesToField.vue'
import DrawerSheet from './DrawerSheet.vue'
import { TextareaField, TextField } from '../pipeline/fields'
import type { EditableAction } from '../composables/useActionsSettings'

const props = withDefaults(defineProps<{ action: EditableAction; isNew: boolean; busy?: boolean; error?: string | null; returnFocusTo?: HTMLElement | null; knownTypes?: string[] }>(), { knownTypes: () => [] })
const emit = defineEmits<{ save: []; cancel: [] }>()
const idRef = ref<{ focus: () => void } | null>(null)
const labelRef = ref<{ focus: () => void } | null>(null)
const appliesField = ref<{ flush: () => void } | null>(null)
const closeRef = ref<HTMLButtonElement | null>(null)
const validationError = ref<string | null>(null)

const typeOptions = [
  { value: 'launch-session', label: 'Launch session' },
  { value: 'shell', label: 'Shell' },
  { value: 'publish-message', label: 'Publish message' },
]
function setType(value: string): void {
  props.action.type = value
  if (value === 'launch-session') { props.action.launch = { promptTemplate: '', repoTemplate: '' }; props.action.shell = undefined; props.action.message = undefined }
  else if (value === 'shell') { props.action.launch = undefined; props.action.shell = { commandTemplate: '' }; props.action.message = undefined }
  else { props.action.launch = undefined; props.action.shell = undefined; props.action.message = { topic: '', messageTemplate: '' } }
}
function envText(): string { return Object.entries(props.action.shell?.env ?? {}).sort(([a], [b]) => a.localeCompare(b)).map(([key, value]) => `${key}=${value ?? ''}`).join('\n') }
function setEnv(text: string): void { if (!props.action.shell) return; const env: Record<string, string> = {}; for (const line of text.split('\n')) { const [key, ...value] = line.split('='); if (key.trim()) env[key.trim()] = value.join('=') }; props.action.shell.env = env }
function save(): void { appliesField.value?.flush(); if (!props.action.id.trim() || !props.action.label.trim()) { validationError.value = 'ID and label are required.'; return }; validationError.value = null; emit('save') }
function cancel(): void { if (!props.busy) emit('cancel') }
let trigger: HTMLElement | null = null
onMounted(async () => {
  trigger = props.returnFocusTo ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null)
  await nextTick()
  if (props.isNew && idRef.value) idRef.value.focus()
  else if (labelRef.value) labelRef.value.focus()
  else closeRef.value?.focus()
})
onUnmounted(() => {
  void nextTick(() => { if (trigger?.isConnected) trigger.focus() })
})
</script>

<template>
  <DrawerSheet
    :ariaLabel="isNew ? 'New action' : 'Edit action'"
    testid="action-editor"
    :default-size="480"
    @close="cancel"
  >
    <template #header>
      <div class="flex items-center gap-3"><span class="flex size-[38px] items-center justify-center rounded-[10px] bg-accent text-accent-contrast"><IconPlay class="size-[18px]" /></span><div class="min-w-0 flex-1"><div class="text-[15px] font-semibold tracking-[-.01em]">{{ isNew ? 'New action' : 'Edit action' }}</div><div class="truncate font-mono text-[12px] text-text-3">{{ isNew ? 'Create a reusable desktop action' : action.id }}</div></div><button ref="closeRef" class="text-text-3 hover:text-text disabled:opacity-50" aria-label="Close" :disabled="busy" @click="cancel"><IconX class="size-4" /></button></div>
    </template>

    <div class="grid gap-3">
      <TextField ref="idRef" v-model="action.id" label="ID" :disabled="!isNew" testid="action-id" />
      <TextField ref="labelRef" v-model="action.label" label="Label" testid="action-label" />
      <div class="text-xs text-text-2">Type<AppSelect :model-value="action.type" :options="typeOptions" testid="action-type" aria-label="Type" class="mt-1" @update:model-value="setType" /></div>
      <AppCheckbox v-model="action.showInDetail" label="Show manual button in detail pane" testid="action-show-in-detail" />
      <AppliesToField ref="appliesField" :model-value="action.appliesTo" :known-types="knownTypes" @update:model-value="action.appliesTo = $event" />
      <template v-if="action.launch">
        <TextareaField v-model="action.launch.promptTemplate" label="Prompt template" :rows="4" monospace testid="action-launch-prompt" />
        <TextField v-model="action.launch.repoTemplate" label="Repository template" testid="action-launch-repo" />
        <TextField v-model="action.launch.agent" label="Agent (optional)" testid="action-launch-agent" />
      </template>
      <template v-if="action.shell">
        <TextareaField v-model="action.shell.commandTemplate" label="Command template" monospace testid="action-shell-command" />
        <TextField v-model="action.shell.cwd" label="Working directory" testid="action-shell-cwd" />
        <TextField v-model="action.shell.timeout" label="Timeout" placeholder="e.g. 30s" testid="action-shell-timeout" />
        <TextareaField :model-value="envText()" label="Environment (KEY=value per line)" monospace testid="action-shell-env" @update:model-value="setEnv" />
      </template>
      <template v-if="action.message">
        <TextareaField v-model="action.message.messageTemplate" label="Message template" monospace testid="action-message-template" />
        <TextField v-model="action.message.topic" label="Topic" testid="action-message-topic" />
      </template>
      <p v-if="validationError || error" class="rounded border border-severity-error bg-severity-error-tint px-3 py-2 text-xs text-severity-error" data-testid="action-editor-error">{{ validationError || error }}</p>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2.5"><BaseButton variant="secondary" size="sm" :busy="busy" @click="cancel">Cancel</BaseButton><BaseButton size="sm" :busy="busy" data-testid="action-save" @click="save">{{ busy ? 'Saving…' : 'Save' }}</BaseButton></div>
    </template>
  </DrawerSheet>
</template>
