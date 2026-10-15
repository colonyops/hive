<script setup lang="ts">
import DocumentReader from '@hive/ui/components/drive/DocumentReader.vue'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import ConfirmationHost from '@hive/ui/components/ui/ConfirmationHost.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import { useClipboard } from '@hive/ui/composables/useClipboard'
import { useConfirmation } from '@hive/ui/composables/useConfirmation'
import { linkTarget, type ReaderDocument } from '@hive/ui/lib/document'
import type { MenuEntry } from '@hive/ui/types/menu'
import { computed, ref, shallowRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import IconBack from '~icons/material-symbols/arrow-back'
import IconCode from '~icons/material-symbols/code-blocks'
import IconFile from '~icons/material-symbols/description'
import IconAArrowDown from '~icons/material-symbols/text-decrease'
import IconAArrowUp from '~icons/material-symbols/text-increase'
import IconEllipsis from '~icons/material-symbols/more-vert'
import IconLink from '~icons/material-symbols/link'
import IconShare from '~icons/material-symbols/share'
import IconTrash from '~icons/material-symbols/delete'
import { api, docUrl } from '../api'
import Breadcrumb, { type Crumb } from '../components/Breadcrumb.vue'
import SharePopover from '../components/SharePopover.vue'
import { useMe } from '../lib/me'
import { useReaderPrefs } from '../lib/readerPrefs'
import { documentRoute, type SpaceRef } from '../lib/space'
import { useToast } from '../lib/toast'
import { ancestorDirs, documentLabel } from '../lib/tree'
import type { DriveDocument, Share } from '../types'

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
const clipboard = useClipboard()
const toast = useToast()

const doc = computed(() => props.documents?.find((d) => d.path === props.path) ?? null)
const paths = computed<ReadonlySet<string>>(() => new Set((props.documents ?? []).map((d) => d.path)))
const folders = computed(() => ancestorDirs(props.path))
const folderRoute = computed(() => {
  const parent = folders.value[folders.value.length - 1]
  return parent ? documentRoute(props.space, parent) : props.space.routePrefix
})
const folderCrumbs = computed<Crumb[]>(() => [
  { label: props.space.title, to: props.space.routePrefix },
  ...folders.value.map((dir) => ({ label: dir.slice(dir.lastIndexOf('/') + 1), to: documentRoute(props.space, dir) })),
])

const isHtml = computed(() => doc.value?.kind === 'html')
const kindIcon = computed(() => (isHtml.value ? IconCode : IconFile))
const kindTone = computed(() => (isHtml.value ? 'text-node-orange' : 'text-node-blue'))

const share = ref<Share | null>(null)
const shareAnchor = shallowRef<HTMLElement | null>(null)
const moreAnchor = shallowRef<HTMLElement | null>(null)
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

function onShareChanged(next: Share | null): void {
  share.value = next
  emit('changed')
}

async function copyLink(): Promise<void> {
  if (!share.value) return
  await clipboard.copy(share.value.url)
  toast.show('Link copied')
}

const textSizeEntries: MenuEntry[] = [
  { kind: 'action', id: 'larger', label: 'Larger text', icon: IconAArrowUp, testid: 'menu-larger' },
  { kind: 'action', id: 'smaller', label: 'Smaller text', icon: IconAArrowDown, testid: 'menu-smaller' },
  { kind: 'separator' },
]
const deleteEntry: MenuEntry = {
  kind: 'action',
  id: 'delete',
  label: 'Delete document',
  icon: IconTrash,
  testid: 'delete-document',
}
// Text size scales the markdown reader only; an HTML document sets its own type.
const moreEntries = computed<MenuEntry[]>(() => (isHtml.value ? [deleteEntry] : [...textSizeEntries, deleteEntry]))

// The text-size entries leave the menu open so a few steps need one visit.
function onMore(id: string): void {
  if (id === 'larger') resize(1)
  else if (id === 'smaller') resize(-1)
  else {
    moreAnchor.value = null
    requestDelete()
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
      <header class="flex h-16 shrink-0 items-center gap-3 border-b border-row px-6">
        <IconButton
          label="Back to folder"
          :icon="IconBack"
          size="xl"
          data-testid="back"
          @click="router.push(folderRoute)"
        />
        <component :is="kindIcon" class="size-6 shrink-0" :class="kindTone" aria-hidden="true" />
        <div class="min-w-0 flex-1">
          <h1
            class="truncate text-heading leading-snug font-medium text-text"
            :title="documentLabel(doc)"
            data-testid="document-title"
          >
            {{ documentLabel(doc) }}
          </h1>
          <Breadcrumb :items="folderCrumbs" compact />
        </div>
        <div class="flex shrink-0 items-center gap-2" data-testid="share-panel">
          <IconButton
            v-if="share"
            label="Copy share link"
            :icon="IconLink"
            size="xl"
            variant="outline"
            class="size-9!"
            data-testid="copy-link"
            @click="copyLink"
          />
          <BaseButton
            size="sm"
            class="h-9! py-0!"
            data-testid="share"
            aria-label="Share"
            @click="shareAnchor = shareAnchor ? null : ($event.currentTarget as HTMLElement)"
          >
            <template #icon><IconShare class="size-4" /></template>
            <span class="max-sm:hidden">Share</span>
          </BaseButton>
          <div class="relative">
            <IconButton
              label="More actions"
              :icon="IconEllipsis"
              size="xl"
              variant="outline"
              class="size-9!"
              :active="!!moreAnchor"
              data-testid="more-actions"
              @click="moreAnchor = moreAnchor ? null : ($event.currentTarget as HTMLElement)"
            />
            <AppMenu
              v-if="moreAnchor"
              :entries="moreEntries"
              :ignore="[moreAnchor]"
              width="14rem"
              testid="more-menu"
              @select="onMore"
              @close="moreAnchor = null"
            />
          </div>
        </div>
        <SharePopover
          v-if="shareAnchor"
          :anchor="shareAnchor"
          :prefix="space.apiPrefix"
          :path="path"
          :share="share"
          @changed="onShareChanged"
          @close="shareAnchor = null"
        />
      </header>
      <div class="min-h-0 flex-1" :class="isHtml ? 'overflow-hidden' : 'overflow-y-auto'">
        <div class="relative" :class="isHtml ? 'h-full' : 'mx-auto max-w-5xl px-6 py-6'">
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
