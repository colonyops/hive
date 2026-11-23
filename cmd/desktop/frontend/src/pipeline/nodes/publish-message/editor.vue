<script setup lang="ts">
import { TextField, TextareaField } from '../../fields'
import type { Config } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

// validate() prefixes each message with its field's key.
function fieldError(key: keyof Config): string | undefined {
  return props.errors?.find((message) => message.startsWith(`${key} `))
}

function set<K extends keyof Config>(key: K, value: Config[K]) {
  emit('update:config', { ...props.config, [key]: value })
}
</script>

<template>
  <div class="flex flex-col gap-4 text-body leading-relaxed" data-testid="publish-message-node-editor">
    <p class="text-text-2">
      Each item update arriving here publishes a message to a hive topic. Agents and scripts subscribed with hive msg
      receive it.
    </p>

    <TextField
      label="Topic"
      :model-value="config.topic"
      placeholder="reviews"
      hint="A literal topic. No wildcards or templates."
      monospace
      :error="fieldError('topic')"
      testid="publish-message-node-editor-topic"
      @update:model-value="set('topic', $event.trim())"
    />

    <TextareaField
      label="Message"
      :model-value="config.message"
      placeholder="Review requested on {{ .Payload.repo }}#{{ .Payload.num }}"
      hint="Go template rendered over the item."
      monospace
      :error="fieldError('message')"
      testid="publish-message-node-editor-message"
      @update:model-value="set('message', $event)"
    />
  </div>
</template>
