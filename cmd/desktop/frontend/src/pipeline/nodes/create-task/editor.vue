<script setup lang="ts">
import { ref, watch } from 'vue'
import { TextField, TextareaField } from '../../fields'
import { parseTags, type Config } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

// validate() prefixes each message with its field's key.
function fieldError(key: keyof Config): string | undefined {
  return props.errors?.find((message) => message.startsWith(`${key} `))
}

function set<K extends keyof Config>(key: K, value: Config[K]) {
  emit('update:config', { ...props.config, [key]: value })
}

// Kept apart from config.tags so a trailing comma survives while typing;
// resynced only when the config changes from outside the field.
const tagsText = ref('')
watch(
  () => props.config.tags,
  (tags) => {
    if ((parseTags(tagsText.value) ?? []).join(',') !== (tags ?? []).join(',')) {
      tagsText.value = (tags ?? []).join(', ')
    }
  },
  { immediate: true },
)

function setTags(text: string) {
  tagsText.value = text
  set('tags', parseTags(text))
}

function setOptional(key: 'description' | 'repo' | 'parent', value: string) {
  set(key, value || undefined)
}
</script>

<template>
  <div class="flex flex-col gap-4 text-body leading-relaxed" data-testid="create-task-node-editor">
    <p class="text-text-2">
      Each item arriving here creates a hive task, once per item. Agents pick it up with hive hc next.
    </p>

    <TextField
      label="Title"
      :model-value="config.title"
      placeholder="Triage {{ .Payload.title }}"
      hint="Go template rendered over the item."
      monospace
      :error="fieldError('title')"
      testid="create-task-node-editor-title"
      @update:model-value="set('title', $event)"
    />

    <TextareaField
      label="Description"
      :model-value="config.description ?? ''"
      placeholder="{{ .Payload.url }}"
      hint="Go template. Fence outside content with untrustedStart and untrustedEnd."
      monospace
      testid="create-task-node-editor-description"
      @update:model-value="setOptional('description', $event)"
    />

    <TextField
      label="Repository"
      :model-value="config.repo ?? ''"
      placeholder="{{ .Payload.repo }}"
      hint="Go template that renders owner/repo. Leave empty for a task with no repository."
      monospace
      testid="create-task-node-editor-repo"
      @update:model-value="setOptional('repo', $event.trim())"
    />

    <TextField
      label="Parent"
      :model-value="config.parent ?? ''"
      placeholder="hc-a1b2c3"
      hint="Optional id of an existing epic or task to file the task under."
      monospace
      :error="fieldError('parent')"
      testid="create-task-node-editor-parent"
      @update:model-value="setOptional('parent', $event.trim())"
    />

    <TextField
      label="Tags"
      :model-value="tagsText"
      placeholder="triage, from-flow"
      hint="Optional tags for the task, separated by commas."
      monospace
      :error="fieldError('tags')"
      testid="create-task-node-editor-tags"
      @update:model-value="setTags($event)"
    />
  </div>
</template>
