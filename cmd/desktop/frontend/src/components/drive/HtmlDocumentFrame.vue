<script setup lang="ts">
// An html document runs as written, so it gets an opaque origin: without
// allow-same-origin it cannot read the app's storage or call its bindings,
// and the CSP Go puts first in <head> keeps it off localhost. allow-popups
// is left out until it is verified on WKWebView (ADR
// hive-drive-is-one-document-tree-under-the-context-root-and-a-canvas-is-one-document-kind-in-it).
import { effectScope, onScopeDispose, ref, watch, type EffectScope } from 'vue'
import IconMaximize from '~icons/lucide/maximize-2'
import IconMinimize from '~icons/lucide/minimize-2'
import IconShield from '~icons/lucide/shield'
import IconButton from '../ui/IconButton.vue'
import { useEscapeToClose } from '../../composables/useEscapeToClose'
import { useRegisterOpenModal } from '../../composables/useOpenModalCount'

defineProps<{
  html: string
  /** Derives `-frame`, `-frame-bar`, and `-frame-expand`. */
  testid: string
}>()

// Expanding fills the reader it sits in, the nearest positioned ancestor in
// DrivePage and DrivePane, and leaves the tree and the document header in
// place. It restyles the same element rather than teleporting it: moving an
// iframe in the DOM reloads it, and a demo would lose its state.
const expanded = ref(false)

// While expanded the global keys stop as they do under a modal. Escape
// reaches this only while focus is outside the frame; keys typed inside it
// stay in its own document.
let modalScope: EffectScope | null = null
watch(expanded, (on) => {
  modalScope?.stop()
  modalScope = null
  if (!on) return
  modalScope = effectScope()
  modalScope.run(useRegisterOpenModal)
})
onScopeDispose(() => modalScope?.stop())
useEscapeToClose(() => (expanded.value = false), { enabled: expanded })
</script>

<template>
  <div
    class="flex min-h-0 flex-1 flex-col overflow-hidden border-border"
    :class="expanded ? 'absolute inset-0 z-30 border-0 bg-pane' : 'rounded-lg border'"
  >
    <div
      class="flex shrink-0 items-center gap-1.5 border-b border-border bg-pane py-1 pr-1 pl-3 text-caption text-text-3"
      :data-testid="`${testid}-frame-bar`"
    >
      <IconShield class="size-3.5 shrink-0 text-text-4" aria-hidden="true" />
      <p class="min-w-0 flex-1 truncate">
        Sandboxed document. Scripts run inside the frame; it cannot call Hive or send requests to localhost.
      </p>
      <IconButton
        :label="expanded ? 'Collapse' : 'Expand to fill the reader'"
        :icon="expanded ? IconMinimize : IconMaximize"
        :aria-pressed="expanded"
        :data-testid="`${testid}-frame-expand`"
        @click="expanded = !expanded"
      />
    </div>
    <iframe
      sandbox="allow-scripts"
      :srcdoc="html"
      referrerpolicy="no-referrer"
      title="HTML document"
      class="w-full flex-1 border-0 bg-white"
      :class="expanded ? 'min-h-0' : 'min-h-[480px]'"
      :data-testid="`${testid}-frame`"
    />
  </div>
</template>
