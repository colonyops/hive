<script setup lang="ts">
// A feed node's identity is still its node id — the fields here are purely
// cosmetic sidebar presentation: the glyph shown in the tree and a hover
// tooltip that explains the feed's context (handy for LLM-generated feeds).
import FormField from '../../../components/ui/FormField.vue'
import TextArea from '../../../components/ui/TextArea.vue'
import { SelectField } from '../../fields'
import { defaultFeedIcon, feedIconOptions } from '../../../lib/feedIcons'
import { descriptionMaxLen, type Config } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

const iconOptions = feedIconOptions.map((o) => ({ value: o.value, label: o.label, icon: o.component }))

function setIcon(value: string) {
  emit('update:config', { ...props.config, icon: value || undefined })
}

function setDescription(value: string) {
  emit('update:config', { ...props.config, description: value || undefined })
}
</script>

<template>
  <div class="flex flex-col gap-4 text-[13px] leading-relaxed" data-testid="feed-node-editor">
    <p class="text-text-2">
      Messages arriving here upsert into this feed as unread items. The feed appears in the sidebar under
      <span class="font-medium text-text">FEEDS</span>, named after this node.
    </p>

    <SelectField
      label="Sidebar icon"
      :model-value="config.icon || defaultFeedIcon"
      :options="iconOptions"
      searchable
      search-placeholder="Search icons…"
      hint="Shown next to the feed in the sidebar tree."
      testid="feed-editor-icon"
      @update:model-value="setIcon"
    />

    <FormField v-slot="{ id }" label="Description">
      <TextArea
        :id="id"
        :model-value="config.description ?? ''"
        size="sm"
        :maxlength="descriptionMaxLen"
        placeholder="Optional context shown as a tooltip when hovering the feed — useful for explaining generated feeds."
        data-testid="feed-editor-description"
        @update:model-value="setDescription"
      />
    </FormField>
  </div>
</template>
