<script setup lang="ts">
import { computed, ref, shallowRef, watch } from 'vue'
import { useIntervalFn } from '@vueuse/core'
import { useRemoteCatalog } from '../stores/useRemoteCatalog'
import { useRemoteConnection } from '../stores/useRemoteConnection'
import { useResource, type Resource } from '../stores/useResource'
import type { RemoteInboxItem } from '../lib/remoteInboxClient'
import BaseButton from './ui/BaseButton.vue'
import InlineError from './ui/InlineError.vue'
import EmptyState from './ui/EmptyState.vue'

const catalog = useRemoteCatalog()
const { activeProfile, color } = useRemoteConnection()
const inbox = computed(() => catalog.inboxes.value.find((row) => row.id === catalog.inboxId.value))
const feedId = ref('')
const resource = shallowRef<Resource<RemoteInboxItem[]> | null>(null)
watch(
  () => catalog.inboxId.value,
  () => {
    feedId.value = inbox.value?.feeds[0]?.id ?? ''
  },
  { immediate: true },
)
watch(
  [feedId, catalog.inboxClient],
  ([id, client]) => {
    const profile = catalog.inboxId.value
    resource.value =
      id && client
        ? useResource(() => client.feed(profile, id), { initial: [], errorFallback: 'Could not read remote feed.' })
        : null
    void resource.value?.reload()
  },
  { immediate: true },
)
useIntervalFn(() => void resource.value?.reload(), 10000)
</script>
<template>
  <div class="flex min-h-0 flex-1" data-testid="remote-inbox">
    <aside class="flex w-60 shrink-0 flex-col gap-1 border-r border-border p-2">
      <p class="p-2 text-body font-semibold">{{ inbox?.name }} · Remote {{ activeProfile?.name }}</p>
      <BaseButton
        v-for="row in inbox?.feeds"
        :key="row.id"
        variant="ghost"
        class="justify-start"
        :style="{ borderLeft: `3px solid ${color}` }"
        :data-testid="`remote-feed-${row.id}`"
        @click="feedId = row.id"
        >{{ row.name }}</BaseButton
      >
      <InlineError v-if="inbox?.problem" :message="inbox.problem" />
    </aside>
    <div class="min-w-0 flex-1 overflow-y-auto p-4">
      <p class="mb-3 text-small text-text-3">Remote inbox preview · read only · up to 200 active items</p>
      <InlineError v-if="resource?.error.value" :message="resource.error.value" />
      <article
        v-for="item in resource?.data.value"
        :key="item.id"
        class="border-b border-border p-3"
        data-testid="remote-inbox-item"
      >
        <p class="text-body" :class="{ 'font-semibold': item.unread }">{{ item.title }}</p>
        <p class="text-small text-text-3">{{ item.sourceKind }} · {{ item.sourceState }}</p>
      </article>
      <EmptyState
        v-if="!resource?.data.value.length"
        :message="resource?.loading.value ? 'Loading remote inbox…' : 'No items in this remote feed.'"
      />
    </div>
  </div>
</template>
