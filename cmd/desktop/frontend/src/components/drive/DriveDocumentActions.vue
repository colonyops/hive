<script setup lang="ts">
import { Dialogs } from '@wailsio/runtime'
import { computed, ref } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import IconDownload from '~icons/lucide/download'
import IconEllipsis from '~icons/lucide/ellipsis'
import IconMaximize from '~icons/lucide/maximize-2'
import IconMinimize from '~icons/lucide/minimize-2'
import IconShield from '~icons/lucide/shield'
import DriveEntryMenu from './DriveEntryMenu.vue'
import AppTooltip from '../ui/AppTooltip.vue'
import IconButton from '../ui/IconButton.vue'
import { useClipboard } from '../../composables/useClipboard'
import { useToasts } from '../../composables/useToasts'
import { errorText } from '../../lib/appError'
import { createLogger } from '../../lib/log'
import {
  Export,
  Markdown,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/driveservice'
import type { EntryWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  space: string
  entry: EntryWire
  size?: 'md' | 'lg'
  /** Derives `-sandbox`, `-frame-expand`, `-copy`, `-download`, `-more`, and the entry menu's ids (DriveEntryMenu). */
  testid: string
}>()
/** Whether an html document fills the reader. */
const htmlExpanded = defineModel<boolean>('htmlExpanded', { default: true })
const emit = defineEmits<{ deleted: [path: string]; moved: [from: string, to: string] }>()

const log = createLogger('drive-document-actions')
const { showToast } = useToasts()

function fail(err: unknown, fallback: string): void {
  log.warn(fallback, { space: props.space, path: props.entry.path, err })
  showToast(errorText(err, fallback), { severity: 'error' })
}

// Markdown and canvas documents have a markdown form; html and plain files
// do not.
const exportable = computed(() => props.entry.kind === 'markdown' || props.entry.kind === 'canvas')
const fileName = computed(() => {
  const base = props.entry.name
  return base.replace(/\.canvas\.json$/, '').replace(/\.md$/, '') + '.md'
})

// Copy is fetch-first: the Go side renders the markdown so copy and save can
// never disagree, and a failure before SetText is still a failed copy.
const { copy, setStatus: setCopyStatus, status: copyStatus } = useClipboard()
async function copyDocument(): Promise<void> {
  try {
    await copy(await Markdown(props.space, props.entry.path))
  } catch (err) {
    setCopyStatus('error')
    fail(err, `Could not copy ${props.entry.path}.`)
    return
  }
  if (copyStatus.value === 'error') showToast('Could not copy to the clipboard', { severity: 'error' })
}

// Status mechanics only: the same auto-resetting affordance, driving the
// save button instead of a clipboard.
const { setStatus: setSaveStatus, status: saveStatus } = useClipboard()
async function downloadDocument(): Promise<void> {
  const { space } = props
  const { path } = props.entry
  try {
    const dest = await Dialogs.SaveFile({ Filename: fileName.value, Title: 'Export document' })
    if (!dest) return
    await Export(space, path, dest)
    setSaveStatus('success')
  } catch (err) {
    setSaveStatus('error')
    fail(err, `Could not export ${path}.`)
  }
}

const entryMenu = ref<{
  open: (entry: EntryWire, anchor: HTMLElement, toggle?: HTMLElement | null) => void
} | null>(null)
function openMenu(event: MouseEvent): void {
  const button = event.currentTarget as HTMLElement
  // The header row, so the menu takes a menu's width rather than the button's.
  entryMenu.value?.open(props.entry, button.parentElement ?? button, button)
}
</script>

<template>
  <template v-if="entry.kind === 'html'">
    <AppTooltip
      text="Sandboxed document. Scripts run inside the frame; it cannot call Hive or send requests to localhost."
    >
      <span class="flex size-6 items-center justify-center text-text-4" :data-testid="`${testid}-sandbox`">
        <IconShield class="size-3.5" aria-label="Sandboxed document" />
      </span>
    </AppTooltip>
    <IconButton
      :label="htmlExpanded ? 'Collapse document' : 'Expand to fill the reader'"
      :icon="htmlExpanded ? IconMinimize : IconMaximize"
      :size="size"
      :aria-pressed="htmlExpanded"
      :data-testid="`${testid}-frame-expand`"
      @click="htmlExpanded = !htmlExpanded"
    />
  </template>
  <template v-if="exportable">
    <IconButton
      :label="copyStatus === 'success' ? 'Copied' : 'Copy as Markdown'"
      :icon="copyStatus === 'success' ? IconCheck : IconCopy"
      :size="size"
      :class="{ '!text-severity-error': copyStatus === 'error' }"
      :data-testid="`${testid}-copy`"
      @click="copyDocument"
    />
    <IconButton
      :label="saveStatus === 'success' ? 'Saved' : 'Save as Markdown…'"
      :icon="saveStatus === 'success' ? IconCheck : IconDownload"
      :size="size"
      :class="{ '!text-severity-error': saveStatus === 'error' }"
      :data-testid="`${testid}-download`"
      @click="downloadDocument"
    />
  </template>
  <IconButton
    label="More actions"
    :icon="IconEllipsis"
    :size="size"
    :data-testid="`${testid}-more`"
    @click="openMenu"
  />
  <DriveEntryMenu
    ref="entryMenu"
    :space="space"
    :testid="testid"
    @moved="(from, to) => emit('moved', from, to)"
    @deleted="emit('deleted', $event)"
  />
</template>
