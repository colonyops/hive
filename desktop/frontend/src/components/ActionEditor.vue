<script setup lang="ts">
import { computed } from 'vue'
import type { EditableAction } from '../composables/useActionsSettings'

const props = defineProps<{ action: EditableAction; isNew: boolean }>()
const emit = defineEmits<{ save: []; cancel: [] }>()

function setType(): void {
  // The DTO is a discriminated union. Assign every branch in one operation so
  // an edited action cannot accidentally submit mixed executable configs.
  if (props.action.type === 'launch-session') {
    props.action.launch = { promptTemplate: '', repoTemplate: '' }
    props.action.shell = undefined
    props.action.message = undefined
  } else if (props.action.type === 'shell') {
    props.action.launch = undefined
    props.action.shell = { commandTemplate: '' }
    props.action.message = undefined
  } else {
    props.action.launch = undefined
    props.action.shell = undefined
    props.action.message = { topic: '', messageTemplate: '' }
  }
}

const appliesTo = computed({
  get: () => props.action.appliesTo?.join(', ') ?? '',
  set: (value: string) => { props.action.appliesTo = value.split(',').map((item) => item.trim()).filter(Boolean) },
})

function envText(): string {
  return Object.entries(props.action.shell?.env ?? {}).sort(([a], [b]) => a.localeCompare(b)).map(([key, value]) => `${key}=${value ?? ''}`).join('\n')
}

function setEnv(event: Event): void {
  if (!props.action.shell) return
  const env: Record<string, string> = {}
  for (const line of (event.target as HTMLTextAreaElement).value.split('\n')) {
    const [key, ...value] = line.split('=')
    if (key.trim()) env[key.trim()] = value.join('=')
  }
  props.action.shell.env = env
}
</script>

<template>
  <div class="mt-5 rounded-lg border border-strong bg-pane p-4" data-testid="action-editor">
    <h3 class="mb-3 text-sm font-semibold text-text">{{ isNew ? 'New action' : 'Edit action' }}</h3>
    <div class="grid gap-3">
      <label class="text-xs text-text-2">ID<input v-model="action.id" :disabled="!isNew" data-testid="action-id" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text disabled:opacity-60" /></label>
      <label class="text-xs text-text-2">Label<input v-model="action.label" data-testid="action-label" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text" /></label>
      <label class="text-xs text-text-2">Type<select v-model="action.type" data-testid="action-type" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text" @change="setType"><option value="launch-session">Launch session</option><option value="shell">Shell</option><option value="publish-message">Publish message</option></select></label>
      <label class="flex gap-2 text-xs text-text-2"><input v-model="action.showInDetail" type="checkbox" data-testid="action-show-in-detail" /> Show manual button in detail pane</label>
      <label class="text-xs text-text-2">Applies to (comma-separated)<input v-model="appliesTo" data-testid="action-applies-to" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text" /></label>
      <template v-if="action.launch">
        <label class="text-xs text-text-2">Prompt template<textarea v-model="action.launch.promptTemplate" data-testid="action-launch-prompt" class="mt-1 min-h-[90px] w-full rounded border border-border bg-app px-2 py-1.5 text-text" /></label>
        <label class="text-xs text-text-2">Repository template<input v-model="action.launch.repoTemplate" data-testid="action-launch-repo" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text" /></label>
        <label class="text-xs text-text-2">Agent (optional)<input v-model="action.launch.agent" data-testid="action-launch-agent" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text" /></label>
      </template>
      <template v-if="action.shell">
        <label class="text-xs text-text-2">Command template<textarea v-model="action.shell.commandTemplate" data-testid="action-shell-command" class="mt-1 min-h-[72px] w-full rounded border border-border bg-app px-2 py-1.5 text-text" /></label>
        <label class="text-xs text-text-2">Working directory<input v-model="action.shell.cwd" data-testid="action-shell-cwd" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text" /></label>
        <label class="text-xs text-text-2">Timeout<input v-model="action.shell.timeout" data-testid="action-shell-timeout" placeholder="e.g. 30s" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text" /></label>
        <label class="text-xs text-text-2">Environment (KEY=value per line)<textarea :value="envText()" data-testid="action-shell-env" class="mt-1 min-h-[72px] w-full rounded border border-border bg-app px-2 py-1.5 text-text" @input="setEnv" /></label>
      </template>
      <template v-if="action.message"><label class="text-xs text-text-2">Message template<textarea v-model="action.message.messageTemplate" data-testid="action-message-template" class="mt-1 min-h-[72px] w-full rounded border border-border bg-app px-2 py-1.5 text-text" /></label><label class="text-xs text-text-2">Topic<input v-model="action.message.topic" data-testid="action-message-topic" class="mt-1 w-full rounded border border-border bg-app px-2 py-1.5 text-text" /></label></template>
    </div>
    <div class="mt-4 flex justify-end gap-2"><button class="px-3 py-1.5 text-xs text-text-2" @click="emit('cancel')">Cancel</button><button class="rounded bg-accent px-3 py-1.5 text-xs font-medium text-accent-contrast" data-testid="action-save" @click="emit('save')">Save</button></div>
  </div>
</template>
