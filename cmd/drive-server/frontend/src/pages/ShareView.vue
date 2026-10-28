<script setup lang="ts">
import DocumentReader from '@hive/ui/components/drive/DocumentReader.vue'
import AppTooltip from '@hive/ui/components/ui/AppTooltip.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import type { ReaderDocument } from '@hive/ui/lib/document'
import { computed, ref } from 'vue'
import IconShield from '~icons/lucide/shield'
import HiveMark from '../components/HiveMark.vue'
import { api, ApiError } from '../api'
import { useLoad } from '../lib/load'
import { useReaderPrefs } from '../lib/readerPrefs'

const props = defineProps<{ slug: string }>()

const userContentNotice = 'This is user content. Hive does not review it.'

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
const isHtml = computed(() => shared.value?.document.kind === 'html')
const htmlSrc = computed(() => (isHtml.value ? `/s/${encodeURIComponent(props.slug)}/raw` : undefined))

// The frame stays hidden until its document paints, so the reader never sees
// the empty white frame first.
const frameLoaded = ref(false)
</script>

<template>
  <div class="bg-app" :class="isHtml ? 'flex h-dvh flex-col' : 'min-h-screen'">
    <header class="flex h-12 shrink-0 items-center gap-3 border-b border-row bg-sidebar px-4">
      <RouterLink
        to="/"
        class="-m-1 flex shrink-0 rounded-md p-1 hover:bg-chip"
        title="Hive Drive home"
        data-testid="share-home"
      >
        <HiveMark :size="20" />
      </RouterLink>
      <h1
        class="min-w-0 flex-1 truncate text-title font-semibold text-text"
        :title="shared?.document.path"
        data-testid="share-title"
      >
        {{ shared?.document.title || 'Hive Drive' }}
      </h1>
      <div v-if="shared" class="flex min-w-0 items-center gap-1.5 text-small text-text-3">
        <span class="truncate" data-testid="share-notice">Shared by {{ shared.owner_email }}</span>
        <AppTooltip :text="userContentNotice">
          <IconShield class="size-3.5 shrink-0 text-text-4" :aria-label="userContentNotice" />
        </AppTooltip>
      </div>
    </header>
    <EmptyState
      v-if="missing"
      variant="boxed"
      class="m-4"
      message="This link does not exist, has expired, or was removed."
      data-testid="share-missing"
    />
    <InlineError v-else-if="error" class="m-4" :message="error" testid="share-error" />
    <Spinner v-else-if="!shared || !readerDocument" class="m-4" />
    <!-- load does not bubble, but it does pass the capture phase on its way to the frame. -->
    <main v-else-if="isHtml" class="relative min-h-0 flex-1" @load.capture="frameLoaded = true">
      <Spinner v-if="!frameLoaded" class="absolute inset-0 m-auto" />
      <DocumentReader
        class="transition-opacity duration-150"
        :class="frameLoaded ? 'opacity-100' : 'opacity-0'"
        :document="readerDocument"
        :font-size-px="prefs.fontSizePx"
        :line-height="prefs.lineHeight"
        :html-src="htmlSrc"
      />
    </main>
    <main v-else class="mx-auto max-w-4xl px-4 py-8">
      <DocumentReader :document="readerDocument" :font-size-px="prefs.fontSizePx" :line-height="prefs.lineHeight" />
    </main>
  </div>
</template>
