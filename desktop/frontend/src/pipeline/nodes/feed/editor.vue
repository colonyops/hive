<script setup lang="ts">
import { TextField } from '../../fields'
import type { Config } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

function update(feed: string) {
  emit('update:config', { ...props.config, feed })
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <TextField
      label="Feed"
      :model-value="config.feed"
      placeholder="team-review"
      hint="Id of a feed declared in profiles/*.yml — arriving messages upsert into it as unread."
      monospace
      testid="feed-node-editor-feed"
      @update:model-value="update"
    />
  </div>
</template>
