<script setup lang="ts">
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { formatBytes, formatTime } from '../lib/format'
import { documentRoute, type SpaceRef } from '../lib/space'
import type { DriveDocument } from '../types'

const props = defineProps<{ space: SpaceRef }>()

const documents = ref<DriveDocument[] | null>(null)
const error = ref<string | null>(null)
const sorted = computed(() => [...(documents.value ?? [])].sort((a, b) => a.path.localeCompare(b.path)))

onMounted(async () => {
  try {
    documents.value = await api.documents(props.space.apiPrefix)
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Could not load documents.'
  }
})
</script>

<template>
  <main class="mx-auto max-w-3xl px-4 py-8">
    <RouterLink :to="{ name: 'spaces' }" class="text-small text-text-3 hover:text-text">Spaces</RouterLink>
    <h1 class="mb-4 mt-1 text-heading font-semibold text-text">{{ space.title }}</h1>
    <InlineError v-if="error" :message="error" />
    <Spinner v-else-if="!documents" />
    <EmptyState v-else-if="!sorted.length" variant="boxed" message="This space has no documents." />
    <ul
      v-else
      class="flex flex-col divide-y divide-border rounded-lg border border-card bg-raised"
      data-testid="documents"
    >
      <li v-for="doc in sorted" :key="doc.id" data-testid="document-row">
        <RouterLink
          :to="documentRoute(space, doc.path)"
          class="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 hover:bg-hover"
        >
          <span class="min-w-0 flex-1 truncate text-body text-text">{{ doc.path }}</span>
          <BaseBadge v-if="doc.shared" tone="accent" class="px-2 py-0.5 text-caption">Shared</BaseBadge>
          <BaseBadge tone="muted" class="px-2 py-0.5 text-caption">{{ doc.kind }}</BaseBadge>
          <span class="text-small text-text-3">{{ formatBytes(doc.size) }}</span>
          <span class="text-small text-text-3">{{ formatTime(doc.updated_at) }}</span>
        </RouterLink>
      </li>
    </ul>
  </main>
</template>
