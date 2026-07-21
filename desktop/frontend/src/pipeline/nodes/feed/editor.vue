<script setup lang="ts">
// A feed node's identity is still its node id — the fields here are purely
// cosmetic sidebar presentation: the glyph shown in the tree and a hover
// tooltip that explains the feed's context (handy for LLM-generated feeds).
import { computed } from 'vue'
import { SelectField } from '../../fields'
import { defaultFeedIcon, feedIconComponent, feedIconOptions } from '../../../lib/feedIcons'
import { descriptionMaxLen, type Config } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

const iconOptions = feedIconOptions.map((o) => ({ value: o.value, label: o.label }))
const iconValue = computed(() => props.config.icon || defaultFeedIcon)
const previewIcon = computed(() => feedIconComponent(props.config.icon))

function setIcon(value: string) {
  emit('update:config', { ...props.config, icon: value || undefined })
}

function setDescription(e: Event) {
  const value = (e.target as HTMLTextAreaElement).value
  emit('update:config', { ...props.config, description: value || undefined })
}
</script>

<template>
  <div class="flex flex-col gap-4 text-[13px] leading-relaxed" data-testid="feed-node-editor">
    <p class="text-text-2">
      Messages arriving here upsert into this feed as unread items. The feed
      appears in the sidebar under <span class="font-medium text-text">FEEDS</span>,
      named after this node.
    </p>

    <div class="flex items-end gap-2.5">
      <span
        class="flex size-[38px] shrink-0 items-center justify-center rounded-lg border border-strong bg-app text-text-2"
        data-testid="feed-icon-preview"
      >
        <component :is="previewIcon" class="size-4" />
      </span>
      <SelectField
        class="flex-1"
        label="Sidebar icon"
        :model-value="iconValue"
        :options="iconOptions"
        hint="Shown next to the feed in the sidebar tree."
        testid="feed-editor-icon"
        @update:model-value="setIcon"
      />
    </div>

    <div>
      <div class="mb-1.5 text-[12px] text-text-2">Description</div>
      <textarea
        :value="config.description ?? ''"
        rows="3"
        :maxlength="descriptionMaxLen"
        placeholder="Optional context shown as a tooltip when hovering the feed — useful for explaining generated feeds."
        class="w-full resize-y rounded-lg border border-strong bg-app px-[11px] py-[9px] text-[13px] text-text outline-none placeholder:text-text-4 focus:border-accent"
        data-testid="feed-editor-description"
        @input="setDescription"
      />
    </div>
  </div>
</template>
