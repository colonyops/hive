<script setup lang="ts">
import { Dialogs } from '@wailsio/runtime'
import { computed } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import IconDownload from '~icons/lucide/download'
import IconTrash2 from '~icons/lucide/trash-2'
import ConfirmationHost from '../ui/ConfirmationHost.vue'
import IconButton from '../ui/IconButton.vue'
import { useClipboard } from '../../composables/useClipboard'
import { useConfirmation } from '../../composables/useConfirmation'
import { useDrive } from '../../stores/useDrive'
import {
  Delete,
  Export,
  Markdown,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/driveservice'

const props = defineProps<{
  space: string
  path: string
  kind: string
  size?: 'md' | 'lg'
  /** Derives `-copy`, `-download`, and `-delete`. */
  testid: string
}>()
const emit = defineEmits<{ deleted: [path: string] }>()

const { reload } = useDrive()

// Markdown and canvas documents have a markdown form; html and plain files
// do not.
const exportable = computed(() => props.kind === 'markdown' || props.kind === 'canvas')
const fileName = computed(() => {
  const base = props.path.split('/').pop() ?? props.path
  return base.replace(/\.canvas\.json$/, '').replace(/\.md$/, '') + '.md'
})

// Copy is fetch-first: the Go side renders the markdown so copy and save can
// never disagree, and a failure before SetText is still a failed copy.
const { copy, setStatus: setCopyStatus, status: copyStatus } = useClipboard()
async function copyDocument(): Promise<void> {
  try {
    await copy(await Markdown(props.space, props.path))
  } catch {
    setCopyStatus('error')
  }
}

// Status mechanics only: the same auto-resetting affordance, driving the
// save button instead of a clipboard.
const { setStatus: setSaveStatus, status: saveStatus } = useClipboard()
async function downloadDocument(): Promise<void> {
  const { space, path } = props
  try {
    const dest = await Dialogs.SaveFile({ Filename: fileName.value, Title: 'Export document' })
    if (!dest) return
    await Export(space, path, dest)
    setSaveStatus('success')
  } catch {
    setSaveStatus('error')
  }
}

const deletion = useConfirmation()
function requestDelete(): void {
  const { space, path } = props
  deletion.request({
    title: 'Delete document',
    description: `Delete ${path}? The file is removed for every chat and session that shares this space. This cannot be undone.`,
    confirmLabel: 'Delete document',
    testid: `${props.testid}-delete-confirmation`,
    onConfirm: async () => {
      await Delete(space, path)
      emit('deleted', path)
      await reload(space)
    },
  })
}
</script>

<template>
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
    label="Delete document"
    :icon="IconTrash2"
    :size="size"
    tone="danger"
    :data-testid="`${testid}-delete`"
    @click="requestDelete"
  />
  <ConfirmationHost :confirmation="deletion" />
</template>
