<script setup lang="ts">
// github-source has no runtime.ts (it's a reference node — the source itself
// runs in Go). The editor is deliberately a free-text ref rather than a
// SelectField: NodeEditorDrawer's contract only passes {config, errors}, not
// the live list of profiles/*.yml sources, so there is nothing to populate
// options from yet (a canvas-level enhancement for Part B/Phase 7, not a
// blocker for this node's own editor contract).
import { TextField } from '../../fields'
import type { Config } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

function update(source: string) {
  emit('update:config', { ...props.config, source })
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <TextField
      label="Source"
      :model-value="config.source"
      placeholder="team-prs"
      hint="Id of a github-search or github-notifications source in profiles/*.yml."
      monospace
      testid="github-source-editor-source"
      @update:model-value="update"
    />
  </div>
</template>
