<script setup lang="ts">
import { computed } from 'vue'
import DocumentReader from '@hive/ui/components/drive/DocumentReader.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { linkTarget } from '@hive/ui/lib/document'
import { useCanvasSettings } from '../../stores/useCanvasSettings'
import { toReaderDocument } from '../../lib/drive'
import type { DocumentWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  document: DocumentWire | null
  /** Every document path in the space, which a relative link can name. */
  paths: ReadonlySet<string>
  loading: boolean
  error: string | null
  /** Derives `-document`, `-markdown`, `-block-<id>` (with `-html` and `-link`), `-frame`, `-file`, `-error`, and `-empty`. */
  testid: string
}>()
const emit = defineEmits<{ 'open-url': [url: string]; open: [path: string] }>()

const { fontSizePx, lineHeight } = useCanvasSettings()
const readerDocument = computed(() => (props.document ? toReaderDocument(props.document) : null))
const bareStyle = computed(() => ({ fontSize: `${fontSizePx.value}px`, lineHeight: String(lineHeight.value) }))

function follow(href: string): void {
  const target = linkTarget(href, props.document?.entry.path ?? '', props.paths)
  if (target?.kind === 'url') emit('open-url', target.url)
  else if (target?.kind === 'document') emit('open', target.path)
}
</script>

<template>
  <DocumentReader
    v-if="readerDocument"
    :document="readerDocument"
    :font-size-px="fontSizePx"
    :line-height="lineHeight"
    :testid="testid"
    :error="error"
    :loading="loading"
    @open-link="follow"
  />
  <div v-else class="flex min-h-0 flex-col" :style="bareStyle" :data-testid="`${testid}-document`">
    <InlineError v-if="error" :testid="`${testid}-error`" variant="line" class="leading-relaxed" :message="error" />
    <EmptyState
      v-else-if="document"
      variant="inline"
      message="Hive shows markdown, html, and canvas documents. Open this file in another app to read it."
      :data-testid="`${testid}-file`"
    />
    <EmptyState v-else-if="!loading" variant="inline" message="Nothing here yet." :data-testid="`${testid}-empty`" />
  </div>
</template>
