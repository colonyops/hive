<script setup lang="ts">
// Picks the folder a document or folder moves into, within its own space.
import { computed, ref } from 'vue'
import IconFolder from '~icons/lucide/folder'
import IconFolderInput from '~icons/lucide/folder-input'
import IconHardDrive from '~icons/lucide/hard-drive'
import BaseButton from '../ui/BaseButton.vue'
import BaseModal from '../ui/BaseModal.vue'
import InlineError from '../ui/InlineError.vue'
import { seedRef } from '../../lib/seedRef'
import { useDrive } from '../../stores/useDrive'
import type { EntryWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  space: string
  tree: EntryWire
  entry: EntryWire
  /** Derives `-root`, `-folder-<path>`, `-confirm`, `-cancel`, and `-error`. */
  testid: string
}>()
const emit = defineEmits<{ close: []; moved: [from: string, to: string] }>()

const { move } = useDrive()

function parentOf(path: string): string {
  const slash = path.lastIndexOf('/')
  return slash < 0 ? '' : path.slice(0, slash)
}

const currentParent = computed(() => parentOf(props.entry.path))
const chosen = seedRef(() => currentParent.value)
const busy = ref(false)
const error = ref<string | null>(null)

// A folder cannot move into itself or anything under it, so its subtree is
// left out.
const folders = computed(() => {
  const out: { path: string; name: string; depth: number }[] = []
  const own = props.entry.kind === 'dir' ? props.entry.path : null
  const walk = (parent: EntryWire, depth: number) => {
    for (const child of parent.children ?? []) {
      if (child.kind !== 'dir' || child.path === own) continue
      out.push({ path: child.path, name: child.name, depth })
      walk(child, depth + 1)
    }
  }
  walk(props.tree, 1)
  return [{ path: '', name: props.space, depth: 0 }, ...out]
})

async function submit(): Promise<void> {
  if (busy.value || chosen.value === currentParent.value) return
  busy.value = true
  const { result, error: refused } = await move(props.space, props.entry.path, chosen.value)
  busy.value = false
  error.value = refused
  if (result) {
    emit('moved', props.entry.path, result.path)
    emit('close')
  }
}
</script>

<template>
  <BaseModal
    :title="`Move ${entry.name}`"
    :icon="IconFolderInput"
    :width="440"
    :busy="busy"
    :testid="testid"
    @close="emit('close')"
  >
    <div role="listbox" :aria-label="`Folders in ${space}`" class="hive-scroll max-h-80 overflow-y-auto px-3 py-3">
      <button
        v-for="folder in folders"
        :key="folder.path"
        type="button"
        role="option"
        :aria-selected="folder.path === chosen"
        class="flex w-full cursor-pointer items-center gap-2 rounded-md py-1.5 pr-2 text-left text-small"
        :class="folder.path === chosen ? 'bg-selection text-text' : 'text-text-2 hover:bg-chip'"
        :style="{ paddingLeft: `${8 + folder.depth * 14}px` }"
        :data-testid="folder.path ? `${testid}-folder-${folder.path}` : `${testid}-root`"
        @click="chosen = folder.path"
        @dblclick="submit"
      >
        <component
          :is="folder.path ? IconFolder : IconHardDrive"
          class="size-3.5 shrink-0 text-text-4"
          aria-hidden="true"
        />
        <span class="min-w-0 flex-1 truncate">{{ folder.name }}</span>
        <span v-if="folder.path === currentParent" class="shrink-0 text-micro text-text-4">current</span>
      </button>
    </div>
    <InlineError
      v-if="error"
      variant="line"
      class="px-5 pb-3 leading-relaxed"
      :message="error"
      :testid="`${testid}-error`"
    />
    <template #footer>
      <div class="flex-1" />
      <BaseButton variant="secondary" :disabled="busy" :data-testid="`${testid}-cancel`" @click="emit('close')"
        >Cancel</BaseButton
      >
      <BaseButton :busy="busy" :disabled="chosen === currentParent" :data-testid="`${testid}-confirm`" @click="submit"
        >Move</BaseButton
      >
    </template>
  </BaseModal>
</template>
