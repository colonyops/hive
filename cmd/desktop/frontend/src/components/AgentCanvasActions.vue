<script setup lang="ts">
// Copy and save for the canvas on screen, as Markdown.
import { Dialogs } from '@wailsio/runtime'
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import IconDownload from '~icons/lucide/download'
import IconButton from './ui/IconButton.vue'
import { useClipboard } from '../composables/useClipboard'
import type { AgentWorkspacesClient } from '../lib/agentWorkspacesClient'

const props = defineProps<{
  workspace: string
  name: string
  client: AgentWorkspacesClient | null
  size?: 'md' | 'lg'
  /** Derives `-copy` and `-download`. */
  testid: string
}>()

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
</template>
