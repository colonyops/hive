<script setup lang="ts">
// The feed-mark image picker, shared by every source editor whose config
// carries an `image` hash. The bytes are stored before the graph save that
// records the hash, so this field emits a hash and never a file. Backend access
// goes through the `client` prop (defaulting to the generated Wails bindings) so
// tests inject fakes instead of mocking module imports.
import { ref, watch, type Component } from 'vue'
import { MarkImages, SetMarkImage } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/flowsservice'
import BaseButton from '../../components/BaseButton.vue'
import { appErrorMessage } from '../../lib/appError'
import { fileToImageBase64, ImageUploadError, imageUploadAccept } from '../../lib/imageUpload'

export interface MarkImageClient {
  /** Uploads a picked mark image (base64); returns its hash and stored PNG. */
  setMarkImage(data: string): Promise<{ hash: string; image: string }>
  /** Resolves a mark hash to its PNG data URL, or undefined when it has no file. */
  markImage(hash: string): Promise<string | undefined>
}

const props = defineProps<{
  /** The configured mark hash, or undefined when the node has none. */
  modelValue?: string
  /** Preview fallback, and what the item renders when no image is set. */
  glyph: Component
  /** Prefix for this field's test ids: `<testid>-preview`, `-upload`, … */
  testid: string
  client?: MarkImageClient
}>()

const emit = defineEmits<{ 'update:modelValue': [hash: string | undefined] }>()

const client: MarkImageClient = props.client ?? {
  async setMarkImage(data) { return await SetMarkImage(data) },
  async markImage(hash) { return (await MarkImages([hash]))?.[hash] },
}

// previewFor tracks the hash preview was resolved for, to skip re-resolving
// after our own upload.
const input = ref<HTMLInputElement | null>(null)
const preview = ref('')
const previewFor = ref('')
const uploading = ref(false)
const error = ref<string | null>(null)

async function resolvePreview(hash?: string): Promise<void> {
  const h = hash ?? ''
  if (!h) { preview.value = ''; previewFor.value = ''; return }
  if (previewFor.value === h && preview.value) return
  try {
    const url = await client.markImage(h)
    if (props.modelValue === h) { preview.value = url ?? ''; previewFor.value = h }
  } catch {
    if (props.modelValue === h) { preview.value = ''; previewFor.value = h }
  }
}

watch(() => props.modelValue, (hash) => { void resolvePreview(hash) }, { immediate: true })

function pick(): void {
  if (uploading.value) return
  input.value?.click()
}

async function onChange(event: Event): Promise<void> {
  const el = event.target as HTMLInputElement
  const file = el.files?.[0]
  el.value = '' // let the same file be re-picked after an error
  if (!file) return
  error.value = null
  uploading.value = true
  try {
    const view = await client.setMarkImage(await fileToImageBase64(file))
    preview.value = view.image
    previewFor.value = view.hash
    emit('update:modelValue', view.hash)
  } catch (err) {
    error.value = err instanceof ImageUploadError
      ? err.message
      : appErrorMessage(err) || (err instanceof Error && err.message) || 'That image could not be read.'
  } finally {
    uploading.value = false
  }
}

function remove(): void {
  error.value = null
  preview.value = ''
  previewFor.value = ''
  emit('update:modelValue', undefined)
}
</script>

<template>
  <div>
    <div class="mb-1.5 text-[12px] text-text-2">Item image</div>
    <div class="flex items-center gap-3">
      <div
        class="flex size-9 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-card bg-app"
        :data-testid="`${testid}-preview`"
      >
        <img v-if="preview" :src="preview" alt="" class="size-full object-contain">
        <component :is="glyph" v-else class="size-4 text-text-3" />
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <input ref="input" type="file" :accept="imageUploadAccept" class="hidden" :data-testid="`${testid}-input`" @change="onChange">
        <BaseButton
          variant="secondary"
          size="sm"
          :busy="uploading"
          :data-testid="`${testid}-upload`"
          @click="pick"
        >{{ modelValue ? 'Replace image' : 'Upload image' }}</BaseButton>
        <BaseButton
          v-if="modelValue"
          variant="ghost"
          size="sm"
          :disabled="uploading"
          :data-testid="`${testid}-remove`"
          @click="remove"
        >Remove</BaseButton>
      </div>
    </div>
    <p class="mt-1.5 text-[11.5px] text-text-4">Shown on this source's items instead of the icon. PNG, JPEG, GIF, or WebP.</p>
    <p v-if="error" class="mt-1.5 text-[11.5px] text-severity-error" :data-testid="`${testid}-error`">{{ error }}</p>
  </div>
</template>
