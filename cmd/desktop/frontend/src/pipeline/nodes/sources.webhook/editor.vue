<script setup lang="ts">
// sources.webhook has no runtime.ts (deliveries are ingested by the Go
// listener). Beyond the config fields the editor surfaces backend-derived
// affordances: the endpoint URL to paste into the sending system, the last
// captured delivery with the non-blocking feed-shape hint, and the copyable
// LLM prompt for authoring a downstream function-node transform. Backend
// access goes through the `client` prop (defaulting to the generated Wails
// bindings) so tests inject fakes instead of mocking module imports.
import { computed, onMounted, ref } from 'vue'
import {
  Capture,
  Info,
} from '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/webhookservice'
import BaseButton from '../../../components/ui/BaseButton.vue'
import { useClipboard } from '../../../composables/useClipboard'
import { defaultWebhookSourceIcon, feedIconComponent, feedIconOptions } from '../../../lib/feedIcons'
import { MarkImageField, SelectField, TextField, type MarkImageClient } from '../../fields'
import IconRefresh from '~icons/lucide/refresh-cw'
import { randomPath, randomSecret } from './config'
import type { Config } from './config'
import { renderPrompt } from '../../../composables/usePrompts'
import EmptyState from '../../../components/ui/EmptyState.vue'
import CopyButton from '../../../components/ui/CopyButton.vue'

export interface WebhookInfoView {
  running: boolean
  port: number
  baseUrl: string
}

export interface WebhookCaptureView {
  receivedAt: number
  body: string
  feedShaped: boolean
  missingFields: string[] | null
}

interface WebhookBackendClient {
  info(): Promise<WebhookInfoView>
  capture(flowId: string, nodeId: string): Promise<WebhookCaptureView>
}

/** The editor's backend seam: this node's own reads plus the mark picker's,
 *  which it forwards to MarkImageField. */
export interface WebhookEditorClient extends WebhookBackendClient, MarkImageClient {}

const props = defineProps<{
  config: Config
  errors?: string[]
  /** Node identity, threaded through NodeEditorDrawer for the capture lookup. */
  flowId?: string
  nodeId?: string
  client?: WebhookEditorClient
}>()

const emit = defineEmits<{ 'update:config': [config: Config] }>()

const client: WebhookBackendClient = props.client ?? {
  async info() {
    return await Info()
  },
  async capture(flowId, nodeId) {
    return await Capture(flowId, nodeId)
  },
}

const info = ref<WebhookInfoView | null>(null)
const capture = ref<WebhookCaptureView | null>(null)

onMounted(async () => {
  try {
    info.value = await client.info()
  } catch {
    /* endpoint URL row simply stays hidden */
  }
  if (!props.flowId || !props.nodeId) return
  try {
    capture.value = await client.capture(props.flowId, props.nodeId)
  } catch {
    /* capture section falls back to "none yet" */
  }
})

function updatePath(path: string) {
  emit('update:config', { ...props.config, path })
}

function updateSecret(secret: string) {
  emit('update:config', { ...props.config, secret: secret || undefined })
}

// Regenerating is unconditional — the field is the undo (the flow only
// persists on save), and a "keep the old one?" prompt for a value nothing
// depends on yet would be friction.
function regeneratePath() {
  updatePath(randomPath())
}

function regenerateSecret() {
  updateSecret(randomSecret())
}

const iconOptions = feedIconOptions.map((o) => ({ value: o.value, label: o.label, icon: o.component }))

function updateIcon(icon: string) {
  emit('update:config', { ...props.config, icon: icon || undefined })
}

// The mark picker's fallback preview, and what items render with no image set.
const iconGlyph = computed(() => feedIconComponent(props.config.icon || defaultWebhookSourceIcon))

function updateImage(image: string | undefined) {
  emit('update:config', { ...props.config, image })
}

const endpointUrl = computed(() => {
  if (!info.value || !props.config.path?.trim()) return ''
  return info.value.baseUrl + props.config.path
})

const hasCapture = computed(() => (capture.value?.receivedAt ?? 0) > 0)
const capturedAt = computed(() => (hasCapture.value ? new Date(capture.value!.receivedAt).toLocaleString() : ''))
const missingFields = computed(() => capture.value?.missingFields ?? [])

const capturePreview = computed(() => {
  if (!hasCapture.value) return ''
  try {
    return JSON.stringify(JSON.parse(capture.value!.body), null, 2)
  } catch {
    return capture.value!.body
  }
})

const { copy: copyPrompt, setStatus: setPromptStatus, copied: promptCopied } = useClipboard({ resetDelay: 2500 })

// The transform prompt is rendered by the Go prompts service like every other
// prompt in the app; this node supplies the two facts only it has — the
// endpoint path and the last delivery captured on it.
async function onCopyPrompt(): Promise<void> {
  const text = await renderPrompt('webhook-transform', {
    webhookPath: props.config.path ?? '',
    webhookSample: hasCapture.value ? capture.value!.body : '',
  })
  if (text === null) {
    setPromptStatus('error')
    return
  }
  await copyPrompt(text)
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <TextField
      label="Path"
      :model-value="config.path ?? ''"
      placeholder="ci-alerts"
      hint="Served at /hooks/<path> on the local webhook listener."
      monospace
      testid="sources.webhook-editor-path"
      @update:model-value="updatePath"
    >
      <template #trailing>
        <button
          type="button"
          class="field-action"
          title="Generate a new path"
          aria-label="Generate a new path"
          data-testid="sources.webhook-editor-path-generate"
          @click="regeneratePath"
        >
          <IconRefresh class="size-[14px]" />
        </button>
      </template>
    </TextField>
    <TextField
      label="Secret"
      :model-value="config.secret ?? ''"
      placeholder="optional shared secret"
      hint="When set, requests must send this value in the X-Hive-Secret header."
      monospace
      testid="sources.webhook-editor-secret"
      @update:model-value="updateSecret"
    >
      <template #trailing>
        <button
          type="button"
          class="field-action"
          title="Generate a new secret"
          aria-label="Generate a new secret"
          data-testid="sources.webhook-editor-secret-generate"
          @click="regenerateSecret"
        >
          <IconRefresh class="size-[14px]" />
        </button>
      </template>
    </TextField>
    <SelectField
      label="Item icon"
      :model-value="config.icon || defaultWebhookSourceIcon"
      :options="iconOptions"
      searchable
      search-placeholder="Search icons…"
      hint="Shown on this source's items when no image is set."
      testid="sources.webhook-editor-icon"
      @update:model-value="updateIcon"
    />

    <MarkImageField
      :model-value="config.image"
      :glyph="iconGlyph"
      :client="props.client"
      testid="sources.webhook-editor-mark"
      @update:model-value="updateImage"
    />

    <div v-if="endpointUrl">
      <div class="mb-1.5 text-[12px] text-text-2">Endpoint</div>
      <div class="flex items-center gap-2">
        <code
          class="min-w-0 flex-1 truncate rounded-lg border border-strong bg-app px-3 py-2 font-mono text-[12px] text-text-2"
          data-testid="sources.webhook-editor-url"
          >{{ endpointUrl }}</code
        >
        <CopyButton :text="endpointUrl" data-testid="sources.webhook-editor-copy-url" />
      </div>
      <p v-if="info && !info.running" class="mt-1.5 text-[11.5px] text-text-4">
        The listener is not running in this session; the URL applies to a live desktop run.
      </p>
    </div>

    <div>
      <div class="mb-1.5 text-[12px] text-text-2">Last delivery</div>
      <template v-if="hasCapture">
        <div class="mb-1.5 font-mono text-[11px] text-text-4" data-testid="sources.webhook-editor-captured-at">
          {{ capturedAt }}
        </div>
        <div
          v-if="missingFields.length > 0"
          class="mb-2 rounded-lg border border-strong bg-selection px-3 py-2.5 text-[12px] leading-relaxed text-text-2"
          data-testid="sources.webhook-editor-shape-warning"
        >
          Missing canonical item fields: <span class="font-mono">{{ missingFields.join(', ') }}</span> — the item still
          ingests, but renders minimally in feeds (see ADR canonical-item-contract). Add a
          <span class="font-mono">function</span>
          node to reshape it, or copy the LLM prompt below to have one written for you. Adding
          <span class="font-mono">state</span> also enables auto-archive.
        </div>
        <pre
          class="max-h-48 overflow-auto rounded-lg border border-row bg-app px-3 py-2.5 font-mono text-[11.5px] leading-relaxed text-text-2"
          data-testid="sources.webhook-editor-capture"
          >{{ capturePreview }}</pre>
      </template>
      <EmptyState
        v-else
        variant="inline"
        message="Nothing captured yet — POST JSON to the endpoint and reopen this editor to see the payload here."
        data-testid="sources.webhook-editor-no-capture"
      />
    </div>

    <div class="flex items-center gap-2.5">
      <BaseButton
        variant="secondary"
        size="sm"
        class="whitespace-nowrap"
        data-testid="sources.webhook-editor-copy-prompt"
        @click="onCopyPrompt"
      >
        Copy LLM prompt
      </BaseButton>
      <span v-if="promptCopied" class="text-[11.5px] text-text-4">Copied — paste into your coding agent</span>
      <span v-else class="text-[11.5px] text-text-4">Generates a function-node transform for this payload</span>
    </div>
  </div>
</template>

<style scoped>
/* In-field regenerate affordance: sits inside the input's right padding, so
   it reads as part of the field rather than a button beside it. */
.field-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 26px;
  width: 26px;
  cursor: pointer;
  border-radius: 5px;
  color: var(--color-text-4);
}

.field-action:hover {
  background: var(--color-selection);
  color: var(--color-text-2);
}
</style>
