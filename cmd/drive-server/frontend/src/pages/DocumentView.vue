<script setup lang="ts">
import DocumentReader from '@hive/ui/components/drive/DocumentReader.vue'
import AppSelect from '@hive/ui/components/ui/AppSelect.vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import ConfirmationHost from '@hive/ui/components/ui/ConfirmationHost.vue'
import CopyButton from '@hive/ui/components/ui/CopyButton.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { useConfirmation } from '@hive/ui/composables/useConfirmation'
import { linkTarget, type ReaderDocument } from '@hive/ui/lib/document'
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, docUrl } from '../api'
import { useMe } from '../lib/me'
import { useReaderPrefs } from '../lib/readerPrefs'
import { formatTime } from '../lib/format'
import { documentRoute, type SpaceRef } from '../lib/space'
import type { DriveDocument, Share, ShareExpiry } from '../types'

const props = defineProps<{ space: SpaceRef; path: string }>()

const router = useRouter()
const me = useMe()
const confirmation = useConfirmation()
const { prefs, resize } = useReaderPrefs()

const expiryOptions = [
  { value: '', label: 'No expiry' },
  { value: '1d', label: 'Expires in 1 day' },
  { value: '7d', label: 'Expires in 7 days' },
  { value: '30d', label: 'Expires in 30 days' },
]

const doc = ref<DriveDocument | null>(null)
const share = ref<Share | null>(null)
const shareExpiry = ref<ShareExpiry>('')
const sharing = ref(false)
const shareError = ref<string | null>(null)
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
    share.value = found.shared ?? null
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

async function createShare(): Promise<void> {
  sharing.value = true
  shareError.value = null
  try {
    share.value = await api.share(props.space.apiPrefix, props.path, shareExpiry.value)
  } catch (cause) {
    shareError.value = cause instanceof Error ? cause.message : 'Could not share the document.'
  } finally {
    sharing.value = false
  }
}

async function removeShare(): Promise<void> {
  sharing.value = true
  shareError.value = null
  try {
    await api.unshare(props.space.apiPrefix, props.path)
    share.value = null
  } catch (cause) {
    shareError.value = cause instanceof Error ? cause.message : 'Could not stop sharing the document.'
  } finally {
    sharing.value = false
  }
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
      <section class="mb-4 flex flex-wrap items-center gap-2" data-testid="share-panel">
        <template v-if="share">
          <a :href="share.url" class="min-w-0 truncate text-small text-accent" data-testid="share-url">{{
            share.url
          }}</a>
          <span class="text-small text-text-3">
            {{ share.expires_at ? `expires ${formatTime(share.expires_at)}` : 'no expiry' }}
          </span>
          <CopyButton :text="share.url" label="Copy link" />
          <BaseButton variant="secondary" size="xs" :busy="sharing" data-testid="unshare" @click="removeShare">
            Unshare
          </BaseButton>
        </template>
        <template v-else>
          <AppSelect v-model="shareExpiry" :options="expiryOptions" size="sm" aria-label="Share expiry" />
          <BaseButton variant="secondary" size="xs" :busy="sharing" data-testid="share" @click="createShare">
            Share
          </BaseButton>
        </template>
        <InlineError v-if="shareError" variant="line" :message="shareError" testid="share-error" />
      </section>
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
