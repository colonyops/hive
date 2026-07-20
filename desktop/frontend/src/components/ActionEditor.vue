<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import IconPlay from '~icons/lucide/play'
import IconX from '~icons/lucide/x'
import AppCheckbox from './AppCheckbox.vue'
import PanelResizeHandle from './PanelResizeHandle.vue'
import { useResizablePanel } from '../composables/useResizablePanel'
import type { EditableAction } from '../composables/useActionsSettings'

const props = defineProps<{ action: EditableAction; isNew: boolean; busy?: boolean; error?: string | null; returnFocusTo?: HTMLElement | null }>()
const emit = defineEmits<{ save: []; cancel: [] }>()
const editorRef = ref<HTMLElement | null>(null)
const idRef = ref<HTMLInputElement | null>(null)
const labelRef = ref<HTMLInputElement | null>(null)
const closeRef = ref<HTMLButtonElement | null>(null)
const validationError = ref<string | null>(null)
const { size, startResize, step } = useResizablePanel({ storageKey: 'hive.panel.action-editor', defaultSize: 480, min: 360, max: 760, edge: 'left' })

function setType(): void {
  if (props.action.type === 'launch-session') { props.action.launch = { promptTemplate: '', repoTemplate: '' }; props.action.shell = undefined; props.action.message = undefined }
  else if (props.action.type === 'shell') { props.action.launch = undefined; props.action.shell = { commandTemplate: '' }; props.action.message = undefined }
  else { props.action.launch = undefined; props.action.shell = undefined; props.action.message = { topic: '', messageTemplate: '' } }
}
const appliesTo = computed({ get: () => props.action.appliesTo?.join(', ') ?? '', set: (value: string) => { props.action.appliesTo = value.split(',').map((item) => item.trim()).filter(Boolean) } })
function envText(): string { return Object.entries(props.action.shell?.env ?? {}).sort(([a], [b]) => a.localeCompare(b)).map(([key, value]) => `${key}=${value ?? ''}`).join('\n') }
function setEnv(event: Event): void { if (!props.action.shell) return; const env: Record<string, string> = {}; for (const line of (event.target as HTMLTextAreaElement).value.split('\n')) { const [key, ...value] = line.split('='); if (key.trim()) env[key.trim()] = value.join('=') }; props.action.shell.env = env }
function save(): void { if (!props.action.id.trim() || !props.action.label.trim()) { validationError.value = 'ID and label are required.'; return }; validationError.value = null; emit('save') }
function cancel(): void { if (!props.busy) emit('cancel') }
function onKeydown(event: KeyboardEvent): void { if (event.key === 'Escape') cancel() }
function focusableElements(): HTMLElement[] {
  return Array.from(editorRef.value?.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])') ?? [])
}
function trapFocus(event: KeyboardEvent): void {
  if (event.key !== 'Tab') return
  const focusable = focusableElements()
  if (!focusable.length) return
  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
}
let trigger: HTMLElement | null = null
onMounted(async () => {
  trigger = props.returnFocusTo ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null)
  window.addEventListener('keydown', onKeydown)
  await nextTick()
  if (props.isNew && idRef.value && !idRef.value.disabled) idRef.value.focus()
  else if (labelRef.value && !labelRef.value.disabled) labelRef.value.focus()
  else closeRef.value?.focus()
})
onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
  void nextTick(() => { if (trigger?.isConnected) trigger.focus() })
})
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-40 bg-backdrop" data-testid="action-editor-backdrop" @click="cancel" />
    <aside ref="editorRef" class="fixed inset-y-0 right-0 z-40 flex max-w-full flex-col overflow-hidden border-l border-strong bg-pane text-text shadow-[-30px_0_60px_-20px_rgba(0,0,0,.5)]" :style="{ width: size + 'px' }" role="dialog" :aria-label="isNew ? 'New action' : 'Edit action'" aria-modal="true" data-testid="action-editor" @keydown="trapFocus">
      <PanelResizeHandle edge="left" name="action-editor" :start="startResize" :step="step" />
      <header class="flex shrink-0 items-center gap-3 border-b border-row px-[18px] py-[15px]"><span class="flex size-7 items-center justify-center rounded-[7px] bg-accent-tint text-accent"><IconPlay class="size-4" /></span><div class="min-w-0 flex-1"><div class="text-[14px] font-semibold tracking-[-.01em]">{{ isNew ? 'New action' : 'Edit action' }}</div><div class="truncate font-mono text-[11px] text-text-3">{{ isNew ? 'Create a reusable desktop action' : action.id }}</div></div><button ref="closeRef" class="text-text-3 hover:text-text disabled:opacity-50" aria-label="Close" :disabled="busy" @click="cancel"><IconX class="size-4" /></button></header>
      <div class="hive-scroll min-h-0 flex-1 overflow-y-auto px-[18px] py-[15px]"><div class="grid gap-3">
        <label class="text-xs text-text-2">ID<input ref="idRef" v-model="action.id" :disabled="!isNew" data-testid="action-id" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent disabled:opacity-60" /></label>
        <label class="text-xs text-text-2">Label<input ref="labelRef" v-model="action.label" data-testid="action-label" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label>
        <label class="text-xs text-text-2">Type<select v-model="action.type" data-testid="action-type" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" @change="setType"><option value="launch-session">Launch session</option><option value="shell">Shell</option><option value="publish-message">Publish message</option></select></label>
        <AppCheckbox v-model="action.showInDetail" label="Show manual button in detail pane" testid="action-show-in-detail" />
        <label class="text-xs text-text-2">Applies to (comma-separated)<input v-model="appliesTo" data-testid="action-applies-to" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label>
        <template v-if="action.launch"><label class="text-xs text-text-2">Prompt template<textarea v-model="action.launch.promptTemplate" data-testid="action-launch-prompt" class="mt-1 min-h-[90px] w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label><label class="text-xs text-text-2">Repository template<input v-model="action.launch.repoTemplate" data-testid="action-launch-repo" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label><label class="text-xs text-text-2">Agent (optional)<input v-model="action.launch.agent" data-testid="action-launch-agent" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label></template>
        <template v-if="action.shell"><label class="text-xs text-text-2">Command template<textarea v-model="action.shell.commandTemplate" data-testid="action-shell-command" class="mt-1 min-h-[72px] w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label><label class="text-xs text-text-2">Working directory<input v-model="action.shell.cwd" data-testid="action-shell-cwd" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label><label class="text-xs text-text-2">Timeout<input v-model="action.shell.timeout" data-testid="action-shell-timeout" placeholder="e.g. 30s" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label><label class="text-xs text-text-2">Environment (KEY=value per line)<textarea :value="envText()" data-testid="action-shell-env" class="mt-1 min-h-[72px] w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" @input="setEnv" /></label></template>
        <template v-if="action.message"><label class="text-xs text-text-2">Message template<textarea v-model="action.message.messageTemplate" data-testid="action-message-template" class="mt-1 min-h-[72px] w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label><label class="text-xs text-text-2">Topic<input v-model="action.message.topic" data-testid="action-message-topic" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text outline-none focus:border-accent" /></label></template>
        <p v-if="validationError || error" class="rounded border border-severity-error bg-severity-error-tint px-3 py-2 text-xs text-severity-error" data-testid="action-editor-error">{{ validationError || error }}</p>
      </div></div>
      <footer class="flex shrink-0 justify-end gap-2.5 border-t border-row bg-raised px-[18px] py-[13px]"><button class="rounded-lg border border-card px-[15px] py-2 text-[13px] text-text-2 hover:text-text disabled:opacity-50" :disabled="busy" @click="cancel">Cancel</button><button class="rounded-lg bg-accent px-[18px] py-2 text-[13px] font-semibold text-accent-contrast disabled:opacity-50" :disabled="busy" data-testid="action-save" @click="save">{{ busy ? 'Saving…' : 'Save' }}</button></footer>
    </aside>
  </Teleport>
</template>
