<script setup lang="ts">
// Picks the folder a document or folder moves into, within its own space.
import type { FileTreeRowDecoration, FileTreeRowDecorationContext } from '@pierre/trees'
import { computed, ref, watch } from 'vue'
import IconFolderInput from '~icons/lucide/folder-input'
import IconFolderTree from '~icons/lucide/folder-tree'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import BaseModal from '@hive/ui/components/ui/BaseModal.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { seedRef } from '@hive/ui/lib/seedRef'
import PathTree from '../tree/PathTree.vue'
import { usePathTree } from '../tree/usePathTree'
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
// left out. Folders go in as "path/" so an empty one still shows.
const folderPaths = computed(() => {
  const out: string[] = []
  const own = props.entry.kind === 'dir' ? props.entry.path : null
  const walk = (parent: EntryWire) => {
    for (const child of parent.children ?? []) {
      if (child.kind !== 'dir' || child.path === own) continue
      out.push(`${child.path}/`)
      walk(child)
    }
  }
  walk(props.tree)
  return out
})

const stripSlash = (path: string) => path.replace(/\/$/, '')

function decorate({ row }: FileTreeRowDecorationContext): FileTreeRowDecoration | null {
  return stripSlash(row.path) === currentParent.value ? { text: 'current' } : null
}

const tree = usePathTree({
  paths: folderPaths,
  storageKey: 'hive.drive.move.expanded',
  storagePrefix: () => props.space,
  defaultOpen: true,
  flattenEmptyDirectories: false,
  renderRowDecoration: decorate,
  onSelectionChange(paths) {
    if (paths.length === 1) chosen.value = stripSlash(paths[0])
  },
})
watch(
  chosen,
  (path) => {
    if (path) tree.reveal(`${path}/`)
    else tree.selectOnly(null)
  },
  { immediate: true },
)

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
    <div class="flex h-80 flex-col px-3 py-3">
      <button
        type="button"
        :aria-pressed="chosen === ''"
        class="flex w-full shrink-0 cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left text-small"
        :class="chosen === '' ? 'bg-selection text-text' : 'text-text-2 hover:bg-chip'"
        :data-testid="`${testid}-root`"
        @click="chosen = ''"
        @dblclick="submit"
      >
        <IconFolderTree class="size-3.5 shrink-0 text-text-4" aria-hidden="true" />
        <span class="min-w-0 flex-1 truncate">{{ space }}</span>
        <span v-if="currentParent === ''" class="shrink-0 text-micro text-text-4">current</span>
      </button>
      <div class="flex min-h-0 flex-1 flex-col" :data-testid="`${testid}-folders`" @dblclick="submit">
        <PathTree :model="tree.model" />
      </div>
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
