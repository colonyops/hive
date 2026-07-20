<script setup lang="ts">
// A feed node carries no config — the node *is* the feed, and its display
// name (edited in the drawer header) plus its wiring are all there is to set.
// The drawer still mounts this so every node type has a body; it shows what
// the node does rather than any fields.
import type { Config } from './config'

defineProps<{ config: Config; errors?: string[] }>()
defineEmits<{ 'update:config': [config: Config] }>()
</script>

<template>
  <div class="flex flex-col gap-3 text-[13px] leading-relaxed text-text-2" data-testid="feed-node-editor">
    <p>
      Messages arriving here upsert into this feed as unread items. The feed
      appears in the sidebar under <span class="font-medium text-text">FEEDS</span>,
      named after this node.
    </p>
    <p class="text-text-3">
      Rename the feed in the header above, and wire a source or filter into its
      input on the canvas.
    </p>
  </div>
</template>
