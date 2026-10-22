<script setup lang="ts">
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import CopyButton from '@hive/ui/components/ui/CopyButton.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import SegmentedControl from '@hive/ui/components/ui/SegmentedControl.vue'
import TextInput from '@hive/ui/components/ui/TextInput.vue'
import { useAnchoredPopover } from '@hive/ui/composables/useAnchoredPopover'
import { useEscapeToClose } from '@hive/ui/composables/useEscapeToClose'
import { errorText } from '@hive/ui/lib/errorText'
import { onClickOutside } from '@vueuse/core'
import { computed, nextTick, onMounted, ref, toRef, watch } from 'vue'
import { api } from '../api'
import { formatDate, formatTime } from '../lib/format'
import { useToast } from '../lib/toast'
import type { Share, ShareExpiry } from '../types'

const props = defineProps<{
  anchor: HTMLElement
  prefix: string
  path: string
  share: Share | null
}>()
const emit = defineEmits<{ close: []; changed: [share: Share | null] }>()

const expiryOptions: { value: ShareExpiry; label: string; title: string }[] = [
  { value: '', label: 'None', title: 'Link never expires' },
  { value: '1d', label: '1 day', title: 'Link expires in 1 day' },
  { value: '7d', label: '7 days', title: 'Link expires in 7 days' },
  { value: '30d', label: '30 days', title: 'Link expires in 30 days' },
]

const toast = useToast()
const current = ref<Share | null>(null)
const expiry = ref<ShareExpiry>('')
watch(
  () => props.share,
  (share) => {
    current.value = share
    expiry.value = share?.expires_in ?? ''
  },
  { immediate: true },
)
const busy = ref(false)
const error = ref<string | null>(null)

const panel = ref<HTMLElement | null>(null)
const open = ref(true)
const { style, measure } = useAnchoredPopover(toRef(props, 'anchor'), panel, open)
onMounted(async () => {
  await nextTick()
  measure()
  await nextTick()
  measure()
})
onClickOutside(panel, () => emit('close'), { ignore: [toRef(props, 'anchor')] })
useEscapeToClose(() => emit('close'))

const expiresAt = computed(() => current.value?.expires_at)

async function guarded(fallback: string, work: () => Promise<void>): Promise<void> {
  busy.value = true
  error.value = null
  try {
    await work()
  } catch (cause) {
    error.value = errorText(cause, fallback)
  } finally {
    busy.value = false
  }
}

function create(chosen: ShareExpiry): Promise<void> {
  return guarded('Could not share the document.', async () => {
    const saved = await api.share(props.prefix, props.path, chosen)
    current.value = saved
    expiry.value = saved.expires_in
    emit('changed', saved)
  })
}

function stop(): Promise<void> {
  return guarded('Could not stop sharing the document.', async () => {
    await api.unshare(props.prefix, props.path)
    current.value = null
    expiry.value = ''
    emit('changed', null)
  })
}

function chooseExpiry(value: ShareExpiry): void {
  expiry.value = value
  if (current.value) void create(value)
}
</script>

<template>
  <Teleport to="body">
    <div
      ref="panel"
      class="fixed z-50 flex w-[26rem] max-w-[calc(100vw-1rem)] flex-col gap-4 overflow-y-auto rounded-xl border border-strong bg-pane p-5 shadow-popover"
      role="dialog"
      aria-label="Share"
      data-testid="share-popover"
      :style="{ ...style, minWidth: '0' }"
    >
      <div>
        <h2 class="flex items-center gap-2 text-title font-semibold text-text">
          Share
          <span
            class="size-2 rounded-full"
            :class="current ? 'bg-accent' : 'bg-text-4'"
            :aria-label="current ? 'Sharing is on' : 'Sharing is off'"
            data-testid="share-state"
          />
        </h2>
        <p v-if="!current" class="mt-1 text-small text-text-3">
          Sharing is off. Only people who sign in can open this document.
        </p>
        <p v-else class="mt-1 text-small text-text-3" data-testid="share-summary">
          Anyone with the link can read it.
          <span v-if="expiresAt" :title="formatTime(expiresAt)" data-testid="share-expiry-text"
            >Expires {{ formatDate(expiresAt) }}</span
          >
          <span v-else data-testid="share-expiry-text">No expiry</span>
        </p>
      </div>

      <div v-if="current" class="flex items-center gap-2">
        <TextInput
          :model-value="current.url"
          readonly
          size="sm"
          class="min-w-0 flex-1"
          aria-label="Share link"
          data-testid="share-url"
          @focus="($event.target as HTMLInputElement).select()"
        />
        <CopyButton
          :text="current.url"
          label="Copy"
          size="sm"
          data-testid="share-copy"
          @copy="(ok: boolean) => toast.show(ok ? 'Link copied' : 'Could not copy the link')"
        />
      </div>

      <div class="flex flex-col gap-1.5">
        <span class="text-small font-medium text-text-2">Link expires</span>
        <SegmentedControl
          :model-value="expiry"
          :columns="4"
          aria-label="Share expiry"
          :options="expiryOptions"
          testid="share-expiry"
          @update:model-value="chooseExpiry"
        />
      </div>

      <InlineError v-if="error" variant="line" :message="error" testid="share-error" />

      <div class="flex items-center justify-end gap-2">
        <BaseButton
          v-if="current"
          variant="danger-outline"
          size="sm"
          :busy="busy"
          data-testid="share-stop"
          @click="stop"
        >
          Stop sharing
        </BaseButton>
        <BaseButton v-else size="sm" :busy="busy" data-testid="share-create" @click="create(expiry)">
          Create link
        </BaseButton>
      </div>
    </div>
  </Teleport>
</template>
