<script setup lang="ts">
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { formatBytes } from '../lib/format'
import { spaceFromKey } from '../lib/space'
import type { SpaceSummary } from '../types'

const spaces = ref<SpaceSummary[] | null>(null)
const error = ref<string | null>(null)

onMounted(async () => {
  try {
    spaces.value = await api.spaces()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Could not load spaces.'
  }
})
</script>

<template>
  <main class="mx-auto max-w-3xl px-4 py-8">
    <h1 class="mb-4 text-heading font-semibold text-text">Spaces</h1>
    <InlineError v-if="error" :message="error" />
    <Spinner v-else-if="!spaces" />
    <EmptyState
      v-else-if="!spaces.length"
      variant="boxed"
      message="No documents yet. Push one with hive drive push."
      data-testid="spaces-empty"
    />
    <ul v-else class="flex flex-col gap-2" data-testid="spaces-list">
      <li v-for="space in spaces" :key="space.space">
        <RouterLink
          :to="spaceFromKey(space.space).routePrefix"
          class="flex items-center gap-3 rounded-lg border border-card bg-raised px-4 py-3 hover:border-strong"
        >
          <span class="flex-1 truncate text-body text-text">{{ spaceFromKey(space.space).title }}</span>
          <BaseBadge tone="muted" class="px-2 py-0.5 text-caption">{{ space.kind }}</BaseBadge>
          <span class="text-small text-text-3">
            {{ space.documents }} {{ space.documents === 1 ? 'document' : 'documents' }},
            {{ formatBytes(space.bytes) }}
          </span>
        </RouterLink>
      </li>
    </ul>
  </main>
</template>
