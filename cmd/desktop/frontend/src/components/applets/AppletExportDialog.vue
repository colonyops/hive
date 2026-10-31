<script setup lang="ts">
// A directory can hold more than its author remembers, so the export shows what goes in
// before anything leaves.
import { Dialogs } from '@wailsio/runtime'
import { ref } from 'vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import BaseModal from '@hive/ui/components/ui/BaseModal.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { Export } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice'
import type { AppletExportPreview } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { useToasts } from '../../composables/useToasts'
import { formatBytes } from '../../lib/bytes'

const props = defineProps<{ preview: AppletExportPreview }>()
const emit = defineEmits<{ close: [] }>()

const WIDTH = 520

const { showToast } = useToasts()
const busy = ref(false)
const error = ref('')

async function save(): Promise<void> {
  busy.value = true
  error.value = ''
  try {
    const dest = await Dialogs.SaveFile({ Filename: props.preview.fileName, Title: `Export ${props.preview.title}` })
    if (!dest) return
    await Export(props.preview.id, dest)
    showToast(`Exported ${props.preview.title}`, { severity: 'success', body: dest })
    emit('close')
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Could not export the applet.'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <BaseModal
    :title="`Export ${preview.title}`"
    :width="WIDTH"
    :busy="busy"
    testid="applet-export"
    @close="emit('close')"
  >
    <div class="space-y-3 p-5 text-body">
      <p class="text-text-2">
        Check this list before you share the file. Anyone you send it to can read all of it. It holds no saved data,
        settings or anything else from this machine.
      </p>
      <div>
        <div class="text-meta text-text-3">Included</div>
        <ul
          class="mt-1 max-h-48 overflow-y-auto rounded-md border border-border px-2 py-1 font-mono text-meta"
          data-testid="applet-export-files"
        >
          <li v-for="file in preview.files" :key="file.path" class="flex justify-between gap-3">
            <span class="min-w-0 truncate">{{ file.path }}</span>
            <span class="shrink-0 text-text-3">{{ formatBytes(file.bytes) }}</span>
          </li>
        </ul>
      </div>
      <div v-if="preview.leftOut?.length">
        <div class="text-meta text-text-3">Left out: hidden files, logs and links</div>
        <ul class="mt-1 max-h-24 overflow-y-auto font-mono text-meta text-text-3" data-testid="applet-export-left-out">
          <li v-for="file in preview.leftOut" :key="file" class="truncate">{{ file }}</li>
        </ul>
      </div>
      <InlineError v-if="error" :message="error" testid="applet-export-error" />
    </div>
    <template #footer>
      <div class="flex-1" />
      <BaseButton variant="secondary" data-testid="applet-export-cancel" @click="emit('close')">Cancel</BaseButton>
      <BaseButton :busy="busy" data-testid="applet-export-save" @click="save">Save file…</BaseButton>
    </template>
  </BaseModal>
</template>
