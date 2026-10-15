<script setup lang="ts">
import DocumentReader from '@hive/ui/components/drive/DocumentReader.vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import ConfirmationHost from '@hive/ui/components/ui/ConfirmationHost.vue'
import CopyButton from '@hive/ui/components/ui/CopyButton.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import SegmentedControl from '@hive/ui/components/ui/SegmentedControl.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import { useConfirmation } from '@hive/ui/composables/useConfirmation'
import { linkTarget, type ReaderDocument } from '@hive/ui/lib/document'
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import IconAArrowDown from '~icons/lucide/a-arrow-down'
import IconAArrowUp from '~icons/lucide/a-arrow-up'
import IconArrowLeft from '~icons/lucide/arrow-left'
import IconLink from '~icons/lucide/link'
import IconTrash from '~icons/lucide/trash-2'
import { api, docUrl } from '../api'
import Breadcrumb, { type Crumb } from '../components/Breadcrumb.vue'
import { formatTime } from '../lib/format'
import { useMe } from '../lib/me'
import { useReaderPrefs } from '../lib/readerPrefs'
import { documentRoute, type SpaceRef } from '../lib/space'
import { ancestorDirs, documentLabel } from '../lib/tree'
import type { DriveDocument, Share, ShareExpiry } from '../types'

const props = defineProps<{
  space: SpaceRef
  path: string
  /** The space's documents; null while they load. */
  documents: DriveDocument[] | null
  loadError: string | null
}>()
const emit = defineEmits<{ changed: [] }>()

const router = useRouter()
const me = useMe()
const confirmation = useConfirmation()
const { prefs, resize } = useReaderPrefs()

const expiryOptions: { value: ShareExpiry; label: string; title: string }[] = [
  { value: '', label: 'No expiry', title: 'Link never expires' },
  { value: '1d', label: '1 day', title: 'Link expires in 1 day' },
  { value: '7d', label: '7 days', title: 'Link expires in 7 days' },
  { value: '30d', label: '30 days', title: 'Link expires in 30 days' },
]

const doc = computed(() => props.documents?.find((d) => d.path === props.path) ?? null)
const paths = computed<ReadonlySet<string>>(() => new Set((props.documents ?? []).map((d) => d.path)))
const folders = computed(() => ancestorDirs(props.path))
const folderRoute = computed(() => {
  const parent = folders.value[folders.value.length - 1]
  return parent ? documentRoute(props.space, parent) : props.space.routePrefix
})
const crumbs = computed<Crumb[]>(() => [
  { label: props.space.title, to: props.space.routePrefix },
  ...folders.value.map((dir) => ({ label: dir.slice(dir.lastIndexOf('/') + 1), to: documentRoute(props.space, dir) })),
  { label: doc.value ? documentLabel(doc.value) : props.path, to: documentRoute(props.space, props.path) },
])

const share = ref<Share | null>(null)
const shareExpiry = ref<ShareExpiry>('')
const sharing = ref(false)
const shareError = ref<string | null>(null)
const body = ref('')
const bodyError = ref<string | null>(null)
const loadingBody = ref(false)

// The body loads once, when the document first appears in the list.
let started = false
watch(
  doc,
  async (found) => {
    if (!found || started) return
    started = true
    share.value = found.shared ?? null
    if (found.kind !== 'markdown') return
    loadingBody.value = true
    try {
      body.value = await api.documentBody(props.space.apiPrefix, props.path)
    } catch (cause) {
      bodyError.value = cause instanceof Error ? cause.message : 'Could not load the document.'
    } finally {
      loadingBody.value = false
    }
  },
  { immediate: true },
)

const error = computed(() => {
  if (props.loadError) return props.loadError
  if (props.documents && !doc.value) return 'This document does not exist.'
  return bodyError.value
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
    emit('changed')
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
    emit('changed')
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
      await router.push(folderRoute.value)
      emit('changed')
    },
  })
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <InlineError v-if="error" class="m-4" :message="error" testid="document-load-error" />
    <Spinner v-else-if="!doc || !readerDocument" class="m-4" />
    <template v-else>
      <header class="flex shrink-0 flex-wrap items-center gap-x-4 gap-y-2 border-b border-row px-4 py-2.5">
        <IconButton label="Back to folder" :icon="IconArrowLeft" size="lg" @click="router.push(folderRoute)" />
        <div class="min-w-0 flex-1">
          <h1 class="truncate text-title font-semibold text-text" data-testid="document-title">
            {{ documentLabel(doc) }}
          </h1>
          <Breadcrumb :items="crumbs" />
        </div>
        <div class="flex flex-wrap items-center gap-1.5" data-testid="share-panel">
          <template v-if="share">
            <a
              :href="share.url"
              class="inline-flex h-7 min-w-0 max-w-64 items-center truncate rounded-lg bg-chip px-2.5 text-small text-accent hover:underline"
              data-testid="share-url"
              >{{ share.url }}</a
            >
            <BaseBadge tone="muted" class="px-2 py-0.5 text-caption">
              {{ share.expires_at ? `expires ${formatTime(share.expires_at)}` : 'no expiry' }}
            </BaseBadge>
            <CopyButton :text="share.url" label="Copy link" size="xs" class="h-7" />
            <BaseButton
              variant="secondary"
              size="xs"
              class="h-7"
              :busy="sharing"
              data-testid="unshare"
              @click="removeShare"
            >
              Unshare
            </BaseButton>
          </template>
          <template v-else>
            <SegmentedControl
              v-model="shareExpiry"
              variant="compact"
              size="sm"
              aria-label="Share expiry"
              :options="expiryOptions"
              testid="share-expiry"
            />
            <BaseButton
              variant="secondary"
              size="xs"
              class="h-7"
              :busy="sharing"
              data-testid="share"
              @click="createShare"
            >
              <template #icon><IconLink class="size-3.5" /></template>
              Share
            </BaseButton>
          </template>
          <span class="mx-1 h-4 w-px bg-border" aria-hidden="true" />
          <IconButton label="Smaller text" :icon="IconAArrowDown" size="lg" @click="resize(-1)" />
          <IconButton label="Larger text" :icon="IconAArrowUp" size="lg" @click="resize(1)" />
          <IconButton
            label="Delete document"
            :icon="IconTrash"
            size="lg"
            tone="danger"
            data-testid="delete-document"
            @click="requestDelete"
          />
        </div>
        <InlineError v-if="shareError" variant="line" class="w-full" :message="shareError" testid="share-error" />
      </header>
      <div class="min-h-0 flex-1 overflow-y-auto">
        <div class="relative mx-auto max-w-5xl px-6 py-5" :class="doc.kind === 'html' ? 'h-full' : ''">
          <DocumentReader
            :document="readerDocument"
            :font-size-px="prefs.fontSizePx"
            :line-height="prefs.lineHeight"
            :html-src="htmlSrc"
            :loading="loadingBody"
            @open-link="openLink"
          />
        </div>
      </div>
    </template>
    <ConfirmationHost :confirmation="confirmation" />
  </div>
</template>
