<script setup lang="ts">
// A document with its comments column. For markdown the two scroll together,
// so a thread's card stays level with the text it is about. Both document
// pages use it; each keeps its own header.
import DocumentReader from '@hive/ui/components/drive/DocumentReader.vue'
import { frontmatterLineCount, type ReaderDocument } from '@hive/ui/lib/document'
import { computed, ref } from 'vue'
import type { RouteLocationRaw } from 'vue-router'
import type { DocumentCollab } from '../../lib/collab'
import type { Anchor } from '../../types'
import CommentRail from './CommentRail.vue'
import DocumentOverlay from './DocumentOverlay.vue'

const props = defineProps<{
  document: ReaderDocument
  htmlSrc?: string
  /** Paths of the space's documents, so a link to one opens it in place. */
  paths?: ReadonlySet<string>
  fontSizePx: number
  lineHeight: number
  collab: DocumentCollab
  signIn: RouteLocationRaw | null
  /** Classes for the box around the markdown reader, which sets its width. */
  readerClass?: string
  documentClass?: string
}>()
const emit = defineEmits<{ 'open-link': [href: string] }>()
const railOpen = defineModel<boolean>('railOpen', { default: false })

const isHtml = computed(() => props.document.entry.kind === 'html')
// Lines count the front matter, the way the server numbers them.
const sourceLines = computed(() =>
  isHtml.value ? undefined : { offset: frontmatterLineCount(props.document.markdown ?? '') },
)

// What DocumentReader exposes. Spelled out because ESLint cannot resolve a
// .vue component's instance type.
const reader = ref<{ root: HTMLElement | null; body: HTMLElement | null } | null>(null)
const root = computed(() => reader.value?.root ?? null)
const body = computed(() => reader.value?.body ?? null)

const activeId = ref<string | null>(null)
const hoveredId = ref<string | null>(null)

function select(id: string): void {
  activeId.value = id
  railOpen.value = true
}

async function createThread(anchor: Anchor, text: string): Promise<string | null> {
  const error = await props.collab.createThread(anchor, text)
  if (!error) railOpen.value = true
  return error
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col" :class="isHtml ? '' : 'overflow-y-auto'" data-testid="collab-scroll">
    <!-- A flex row stretches its items only to its own height. Growing with the
         document lets the comments column run to the end of a long one. -->
    <div class="flex" :class="isHtml ? 'min-h-0 flex-1' : 'shrink-0 grow'">
      <div class="min-w-0 flex-1" :class="isHtml ? 'relative overflow-hidden' : ''">
        <div class="relative" :class="isHtml ? 'h-full' : readerClass">
          <DocumentReader
            ref="reader"
            :class="documentClass"
            :document="document"
            :font-size-px="fontSizePx"
            :line-height="lineHeight"
            :html-src="htmlSrc"
            :paths="paths"
            :source-lines="sourceLines"
            :body-class="{ 'cursor-pointer': !!hoveredId }"
            @open-link="emit('open-link', $event)"
          >
            <DocumentOverlay
              v-if="!isHtml"
              :root="root"
              :body="body"
              :threads="collab.threads.value"
              :viewers="collab.viewers.value"
              :active-id="activeId"
              :hovered-id="hoveredId"
              :can-comment="!!collab.viewer.value?.can_comment"
              :submit="createThread"
              @selection="collab.setSelection"
              @select="select"
              @hover="hoveredId = $event"
            />
          </DocumentReader>
        </div>
      </div>
      <div
        v-if="railOpen"
        class="relative flex w-96 shrink-0 flex-col border-l border-row max-md:w-80"
        :class="isHtml ? 'overflow-y-auto' : ''"
      >
        <CommentRail
          :threads="collab.threads.value"
          :viewer="collab.viewer.value"
          :collab="collab"
          :active-id="activeId"
          :hovered-id="hoveredId"
          :align-to="isHtml ? null : root"
          :sign-in="signIn"
          :error="collab.error.value"
          @select="activeId = $event"
          @hover="hoveredId = $event"
          @close="railOpen = false"
        />
      </div>
    </div>
  </div>
</template>
