<script setup lang="ts">
// Optional fields are stored only when set, so a flow file stays free of keys
// the author never touched.
import { TextField, TextareaField } from '../../fields'
import type { Config } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

// validate() phrases each message from its field's key, so the message is
// routed back to the field it names.
function fieldError(key: keyof Config): string | undefined {
  return props.errors?.find((message) => message.startsWith(`${key} `))
}

function set<K extends keyof Config>(key: K, value: Config[K]) {
  emit('update:config', { ...props.config, [key]: value })
}
</script>

<template>
  <div class="flex flex-col gap-4 text-[13px] leading-relaxed" data-testid="launch-session-node-editor">
    <p class="text-text-2">
      Each item arriving here starts a Hive coding session, once per item. The item's detail view links the session.
    </p>

    <TextField
      label="Repository"
      :model-value="config.repo"
      placeholder="https://github.com/{{ .Payload.repo }}.git"
      hint="Go template rendered over the item. A remote URL or a configured repository."
      monospace
      :error="fieldError('repo')"
      testid="launch-session-node-editor-repo"
      @update:model-value="set('repo', $event)"
    />

    <TextField
      label="Agent"
      :model-value="config.agent ?? ''"
      placeholder="Default agent"
      hint="A Hive agent profile, such as claude. Leave empty for the default."
      monospace
      testid="launch-session-node-editor-agent"
      @update:model-value="set('agent', $event || undefined)"
    />

    <TextField
      label="Session name"
      :model-value="config.sessionName ?? ''"
      placeholder="review-{{ .Payload.num }}"
      hint="Go template, turned into a valid session name. Leave empty to derive one from the node and the item."
      monospace
      testid="launch-session-node-editor-session-name"
      @update:model-value="set('sessionName', $event || undefined)"
    />

    <TextareaField
      label="Prompt"
      :model-value="config.prompt"
      placeholder="Review pull request #{{ .Payload.num }}"
      hint="Go template rendered over the item. Fence outside content with untrustedStart and untrustedEnd."
      monospace
      :error="fieldError('prompt')"
      testid="launch-session-node-editor-prompt"
      @update:model-value="set('prompt', $event)"
    />
  </div>
</template>
