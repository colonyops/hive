<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { SelectField, TextField, TextareaField } from '../../fields'
import { useAgentWorkspaces } from '../../../stores/useAgentWorkspaces'
import type { Config } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

const { workspaces, workspacesLoaded, reloadWorkspaces } = useAgentWorkspaces()

onMounted(() => {
  if (!workspacesLoaded.value) void reloadWorkspaces()
})

const options = computed(() => {
  const rows = workspaces.value.map((w) => ({ value: w.dir, label: w.name || w.dir }))
  if (props.config.workspace && !rows.some((row) => row.value === props.config.workspace)) {
    rows.unshift({ value: props.config.workspace, label: `${props.config.workspace} (not found)` })
  }
  return rows
})

// validate() prefixes each message with its field's key.
function fieldError(key: keyof Config): string | undefined {
  return props.errors?.find((message) => message.startsWith(`${key} `))
}

function set<K extends keyof Config>(key: K, value: Config[K]) {
  emit('update:config', { ...props.config, [key]: value })
}
</script>

<template>
  <div class="flex flex-col gap-4 text-body leading-relaxed" data-testid="launch-chat-node-editor">
    <p class="text-text-2">
      Each item arriving here opens a chat in an agent workspace, once per item. The prompt is the chat's first message,
      and the item's detail view links the chat.
    </p>

    <SelectField
      v-if="workspaces.length"
      label="Workspace"
      :model-value="config.workspace"
      :options="options"
      placeholder="Choose a workspace"
      searchable
      :error="fieldError('workspace')"
      testid="launch-chat-node-editor-workspace"
      @update:model-value="set('workspace', $event)"
    />
    <TextField
      v-else
      label="Workspace"
      :model-value="config.workspace"
      placeholder="incident-triage"
      hint="The workspace's directory name under the workspace root."
      monospace
      :error="fieldError('workspace')"
      testid="launch-chat-node-editor-workspace"
      @update:model-value="set('workspace', $event.trim())"
    />

    <TextareaField
      label="Prompt"
      :model-value="config.prompt"
      placeholder="Triage {{ .Payload.title }}"
      hint="Go template rendered over the item. Fence outside content with untrustedStart and untrustedEnd."
      monospace
      :error="fieldError('prompt')"
      testid="launch-chat-node-editor-prompt"
      @update:model-value="set('prompt', $event)"
    />
  </div>
</template>
