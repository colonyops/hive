<script setup lang="ts">
import DocumentReader from '@hive/ui/components/drive/DocumentReader.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import type { ReaderDocument } from '@hive/ui/lib/document'
import { computed } from 'vue'
import HiveMark from '../components/HiveMark.vue'
import { api, ApiError } from '../api'
import { useLoad } from '../lib/load'
import { useReaderPrefs } from '../lib/readerPrefs'

const props = defineProps<{ slug: string }>()

const { prefs } = useReaderPrefs()
const load = useLoad(() => api.publicDocument(props.slug), 'Could not load the shared document.')
const shared = load.data
const missing = computed(() => load.failure.value instanceof ApiError && load.failure.value.status === 404)
const error = computed(() => (missing.value ? null : load.error.value))

const readerDocument = computed<ReaderDocument | null>(() => {
  const value = shared.value
  if (!value) return null
  const entry = { path: value.document.path, kind: value.document.kind, modified: value.document.updated_at }
  return value.document.kind === 'markdown' ? { entry, markdown: value.body ?? '' } : { entry }
})
const htmlSrc = computed(() =>
  shared.value?.document.kind === 'html' ? `/s/${encodeURIComponent(props.slug)}/raw` : undefined,
)
</script>

<template>
  <div class="min-h-screen bg-app">
    <header class="flex h-12 items-center gap-2 border-b border-row bg-sidebar px-4 text-title font-semibold text-text">
      <HiveMark :size="22" />
      Hive Drive
    </header>
    <main class="mx-auto max-w-4xl px-4 py-8">
      <EmptyState
        v-if="missing"
        variant="boxed"
        message="This link does not exist, has expired, or was removed."
        data-testid="share-missing"
      />
      <InlineError v-else-if="error" :message="error" testid="share-error" />
      <Spinner v-else-if="!shared || !readerDocument" />
      <template v-else>
        <header class="mb-4">
          <h1 class="break-all text-heading font-semibold text-text">{{ shared.document.path }}</h1>
          <p class="mt-1 text-small text-text-3" data-testid="share-notice">
            Shared by {{ shared.owner_email }}. This is user content. Hive does not review it.
          </p>
        </header>
        <div class="relative" :class="shared.document.kind === 'html' ? 'h-[70vh] min-h-96' : ''">
          <DocumentReader
            :document="readerDocument"
            :font-size-px="prefs.fontSizePx"
            :line-height="prefs.lineHeight"
            :html-src="htmlSrc"
          />
        </div>
      </template>
    </main>
  </div>
</template>
