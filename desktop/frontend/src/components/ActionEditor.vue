<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import IconPlay from '~icons/lucide/play'
import IconX from '~icons/lucide/x'
import BaseButton from './BaseButton.vue'
import AppCheckbox from './AppCheckbox.vue'
import AppSelect from './AppSelect.vue'
import AppliesToField from './AppliesToField.vue'
import DrawerSheet from './DrawerSheet.vue'
import type { EditableAction } from '../composables/useActionsSettings'

const props = withDefaults(defineProps<{ action: EditableAction; isNew: boolean; busy?: boolean; error?: string | null; returnFocusTo?: HTMLElement | null; knownTypes?: string[] }>(), { knownTypes: () => [] })
const emit = defineEmits<{ save: []; cancel: [] }>()
const idRef = ref<HTMLInputElement | null>(null)
const labelRef = ref<HTMLInputElement | null>(null)
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
function setEnv(event: Event): void { if (!props.action.shell) return; const env: Record<string, string> = {}; for (const line of (event.target as HTMLTextAreaElement).value.split('\n')) { const [key, ...value] = line.split('='); if (key.trim()) env[key.trim()] = value.join('=') }; props.action.shell.env = env }
function save(): void { appliesField.value?.flush(); if (!props.action.id.trim() || !props.action.label.trim()) { validationError.value = 'ID and label are required.'; return }; validationError.value = null; emit('save') }
function cancel(): void { if (!props.busy) emit('cancel') }
let trigger: HTMLElement | null = null
onMounted(async () => {
  trigger = props.returnFocusTo ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null)
  await nextTick()
  if (props.isNew && idRef.value && !idRef.value.disabled) idRef.value.focus()
  else if (labelRef.value && !labelRef.value.disabled) labelRef.value.focus()
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
      <label class="text-xs text-text-2">ID<input ref="idRef" v-model="action.id" :disabled="!isNew" data-testid="action-id" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent disabled:opacity-60" /></label>
      <label class="text-xs text-text-2">Label<input ref="labelRef" v-model="action.label" data-testid="action-label" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label>
      <div class="text-xs text-text-2">Type<AppSelect :model-value="action.type" :options="typeOptions" testid="action-type" aria-label="Type" class="mt-1" @update:model-value="setType" /></div>
      <AppCheckbox v-model="action.showInDetail" label="Show manual button in detail pane" testid="action-show-in-detail" />
      <AppliesToField ref="appliesField" :model-value="action.appliesTo" :known-types="knownTypes" @update:model-value="action.appliesTo = $event" />
      <template v-if="action.launch"><label class="text-xs text-text-2">Prompt template<textarea v-model="action.launch.promptTemplate" data-testid="action-launch-prompt" class="mt-1 min-h-[90px] w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label><label class="text-xs text-text-2">Repository template<input v-model="action.launch.repoTemplate" data-testid="action-launch-repo" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label><label class="text-xs text-text-2">Agent (optional)<input v-model="action.launch.agent" data-testid="action-launch-agent" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label></template>
      <template v-if="action.shell"><label class="text-xs text-text-2">Command template<textarea v-model="action.shell.commandTemplate" data-testid="action-shell-command" class="mt-1 min-h-[72px] w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label><label class="text-xs text-text-2">Working directory<input v-model="action.shell.cwd" data-testid="action-shell-cwd" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label><label class="text-xs text-text-2">Timeout<input v-model="action.shell.timeout" data-testid="action-shell-timeout" placeholder="e.g. 30s" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label><label class="text-xs text-text-2">Environment (KEY=value per line)<textarea :value="envText()" data-testid="action-shell-env" class="mt-1 min-h-[72px] w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" @input="setEnv" /></label></template>
      <template v-if="action.message"><label class="text-xs text-text-2">Message template<textarea v-model="action.message.messageTemplate" data-testid="action-message-template" class="mt-1 min-h-[72px] w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label><label class="text-xs text-text-2">Topic<input v-model="action.message.topic" data-testid="action-message-topic" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label></template>
      <p v-if="validationError || error" class="rounded border border-severity-error bg-severity-error-tint px-3 py-2 text-xs text-severity-error" data-testid="action-editor-error">{{ validationError || error }}</p>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2.5"><BaseButton variant="secondary" size="sm" :busy="busy" @click="cancel">Cancel</BaseButton><BaseButton size="sm" :busy="busy" data-testid="action-save" @click="save">{{ busy ? 'Saving…' : 'Save' }}</BaseButton></div>
    </template>
  </DrawerSheet>
</template>
