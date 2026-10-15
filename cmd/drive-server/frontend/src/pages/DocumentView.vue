<script setup lang="ts">
import DocumentReader from '@hive/ui/components/drive/DocumentReader.vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import ConfirmationHost from '@hive/ui/components/ui/ConfirmationHost.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { useConfirmation } from '@hive/ui/composables/useConfirmation'
import { linkTarget, type ReaderDocument } from '@hive/ui/lib/document'
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, docUrl } from '../api'
import { useMe } from '../lib/me'
import { useReaderPrefs } from '../lib/readerPrefs'
import { documentRoute, type SpaceRef } from '../lib/space'
import type { DriveDocument } from '../types'

const props = defineProps<{ space: SpaceRef; path: string }>()

const router = useRouter()
const me = useMe()
const confirmation = useConfirmation()
const { prefs, resize } = useReaderPrefs()

const doc = ref<DriveDocument | null>(null)
const body = ref('')
const paths = ref<ReadonlySet<string>>(new Set())
const error = ref<string | null>(null)
const loading = ref(true)

onMounted(async () => {
  try {
    const documents = await api.documents(props.space.apiPrefix)
    paths.value = new Set(documents.map((d) => d.path))
    const found = documents.find((d) => d.path === props.path)
    if (!found) {
      error.value = 'This document does not exist.'
      return
    }
    doc.value = found
    if (found.kind === 'markdown') body.value = await api.documentBody(props.space.apiPrefix, props.path)
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Could not load the document.'
  } finally {
    loading.value = false
  }
})

const readerDocument = computed<ReaderDocument | null>(() => {
  if (!doc.value) return null
  const entry = { path: doc.value.path, kind: doc.value.kind, modified: doc.value.updated_at }
  return doc.value.kind === 'markdown' ? { entry, markdown: body.value } : { entry }
})
const htmlSrc = computed(() =>
  doc.value?.kind === 'html' ? `${docUrl(props.space.apiPrefix, props.path)}?raw=1` : undefined,
)

function openLink(href: string): void {
  const target = linkTarget(href, props.path, paths.value)
  if (target?.kind === 'url') window.open(target.url, '_blank', 'noopener,noreferrer')
  else if (target?.kind === 'document') void router.push(documentRoute(props.space, target.path))
}

function requestDelete(): void {
  confirmation.request({
    title: 'Delete this document?',
    description: `${props.path} is deleted from the server. Local copies stay. Any share link stops working.`,
    confirmLabel: 'Delete',
    testid: 'delete-dialog',
    onConfirm: async () => {
      await api.deleteDocument(props.space.apiPrefix, props.path)
      await me?.refresh()
      await router.push(props.space.routePrefix)
    },
  })
}
</script>

<template>
  <main class="mx-auto max-w-4xl px-4 py-8">
    <RouterLink :to="space.routePrefix" class="text-small text-text-3 hover:text-text">{{ space.title }}</RouterLink>
    <InlineError v-if="error" class="mt-3" :message="error" testid="document-load-error" />
    <template v-else-if="doc && readerDocument">
      <div class="mb-4 mt-1 flex flex-wrap items-center gap-3">
        <h1 class="min-w-0 flex-1 break-all text-heading font-semibold text-text">{{ doc.path }}</h1>
        <BaseBadge tone="muted" class="px-2 py-0.5 text-caption">{{ doc.kind }}</BaseBadge>
        <div class="flex items-center gap-1">
          <BaseButton variant="ghost" size="xs" aria-label="Smaller text" @click="resize(-1)">A-</BaseButton>
          <BaseButton variant="ghost" size="xs" aria-label="Larger text" @click="resize(1)">A+</BaseButton>
        </div>
        <BaseButton variant="danger-outline" size="xs" data-testid="delete-document" @click="requestDelete">
          Delete
        </BaseButton>
      </div>
      <div class="relative" :class="doc.kind === 'html' ? 'h-[70vh] min-h-96' : ''">
        <DocumentReader
          :document="readerDocument"
          :font-size-px="prefs.fontSizePx"
          :line-height="prefs.lineHeight"
          :html-src="htmlSrc"
          :loading="loading"
          @open-link="openLink"
        />
      </div>
    </template>
    <ConfirmationHost :confirmation="confirmation" />
  </main>
</template>
