<script setup lang="ts">
import { TextField } from '../../fields'
import type { Config } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

function update(action: string) {
  emit('update:config', { ...props.config, action })
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <TextField
      label="Action"
      :model-value="config.action"
      placeholder="review-pr"
      hint="Id of an action declared in desktop actions.yml — arriving messages enqueue it as an output_command."
      monospace
      testid="action-node-editor-action"
      @update:model-value="update"
    />
  </div>
</template>
