<script setup lang="ts">
import { Dialogs } from '@wailsio/runtime'
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import IconDownload from '~icons/lucide/download'
import IconTrash2 from '~icons/lucide/trash-2'
import ConfirmationHost from './ui/ConfirmationHost.vue'
import IconButton from './ui/IconButton.vue'
import { useClipboard } from '../composables/useClipboard'
import { useConfirmation } from '../composables/useConfirmation'
import type { AgentWorkspacesClient } from '../lib/agentWorkspacesClient'

const props = defineProps<{
  workspace: string
  name: string
  client: AgentWorkspacesClient | null
  size?: 'md' | 'lg'
  /** Derives `-copy`, `-download`, and `-delete`. */
  testid: string
}>()
const emit = defineEmits<{ deleted: [name: string] }>()

// Copy is fetch-first (usePrompts' shape): the Go side renders the markdown
// so copy and save can never disagree, and a failure before SetText is still
// a failed copy as far as the user is concerned.
const { copy, setStatus: setCopyStatus, status: copyStatus } = useClipboard()
async function copyCanvas(): Promise<void> {
  if (!props.client) return
  try {
    await copy(await props.client.canvasMarkdown(props.workspace, props.name))
  } catch {
    setCopyStatus('error')
  }
}

// Status mechanics only — the same auto-resetting affordance, driving the
// save button instead of a clipboard.
const { setStatus: setSaveStatus, status: saveStatus } = useClipboard()
async function downloadCanvas(): Promise<void> {
  if (!props.client) return
  const { workspace, name } = props
  try {
    const path = await Dialogs.SaveFile({ Filename: `${name}.md`, Title: 'Export canvas' })
    if (!path) return
    await props.client.exportCanvas(workspace, name, path)
    setSaveStatus('success')
  } catch {
    setSaveStatus('error')
  }
}

const deletion = useConfirmation()
function requestDelete(): void {
  const client = props.client
  if (!client) return
  const { workspace, name } = props
  deletion.request({
    title: 'Delete canvas',
    description: `Delete ${name}? This removes the canvas and all of its blocks. This cannot be undone.`,
    confirmLabel: 'Delete canvas',
    testid: `${props.testid}-delete-confirmation`,
    onConfirm: async () => {
      await client.deleteCanvas(workspace, name)
      emit('deleted', name)
    },
  })
}
</script>

<template>
  <IconButton
    :label="copyStatus === 'success' ? 'Copied' : 'Copy as Markdown'"
    :icon="copyStatus === 'success' ? IconCheck : IconCopy"
    :size="size"
    :class="{ '!text-severity-error': copyStatus === 'error' }"
    :data-testid="`${testid}-copy`"
    @click="copyCanvas"
  />
  <IconButton
    :label="saveStatus === 'success' ? 'Saved' : 'Save as Markdown…'"
    :icon="saveStatus === 'success' ? IconCheck : IconDownload"
    :size="size"
    :class="{ '!text-severity-error': saveStatus === 'error' }"
    :data-testid="`${testid}-download`"
    @click="downloadCanvas"
  />
  <IconButton
    label="Delete canvas"
    :icon="IconTrash2"
    :size="size"
    tone="danger"
    :data-testid="`${testid}-delete`"
    @click="requestDelete"
  />
  <ConfirmationHost :confirmation="deletion" />
</template>
