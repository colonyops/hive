<script setup lang="ts">
// The row menu of a drive entry and the dialogs it opens. One instance serves
// a whole tree or a reader header; `open` points it at an entry.
import { computed, shallowRef } from 'vue'
import IconFolderPlus from '~icons/lucide/folder-plus'
import MoveDialog from './MoveDialog.vue'
import { useDriveRowMenu, type DriveRowAction } from './useDriveRowMenu'
import AppMenu from '../ui/AppMenu.vue'
import ConfirmationHost from '../ui/ConfirmationHost.vue'
import RenameDialog from '../ui/RenameDialog.vue'
import { useClipboard } from '../../composables/useClipboard'
import { useConfirmation } from '../../composables/useConfirmation'
import { useToasts } from '../../composables/useToasts'
import { errorText } from '../../lib/appError'
import { useDrive } from '../../stores/useDrive'
import {
  Locate,
  Reveal,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/driveservice'
import type { EntryWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  space: string
  /** Rename hands the entry back through `rename` for the tree to edit in place, instead of opening a dialog. */
  inlineRename?: boolean
  /** Derives `-menu`, `-menu-<action>`, `-new-folder-*`, `-rename-*`, `-move-*`, and `-delete-confirmation`. */
  testid: string
}>()
const emit = defineEmits<{
  rename: [entry: EntryWire]
  created: [path: string]
  moved: [from: string, to: string]
  deleted: [path: string]
}>()

const drive = useDrive()
const { showToast } = useToasts()
const { copy, status: copyStatus } = useClipboard()

const tree = computed(() => drive.trees.value.get(props.space) ?? null)

const target = shallowRef<EntryWire | null>(null)
const anchor = shallowRef<HTMLElement | null>(null)
const toggle = shallowRef<HTMLElement | null>(null)
const { entries } = useDriveRowMenu(target, () => `${props.testid}-menu`)

/** Opens the menu for entry beside anchor; toggle is the button that opened it, if any. */
function open(entry: EntryWire, at: HTMLElement, from: HTMLElement | null = null): void {
  if (from && toggle.value === from) {
    close()
    return
  }
  target.value = entry
  anchor.value = at
  toggle.value = from
}

function close(): void {
  target.value = null
  anchor.value = null
  toggle.value = null
}

const newFolderIn = shallowRef<string | null>(null)
const renaming = shallowRef<EntryWire | null>(null)
const moving = shallowRef<EntryWire | null>(null)
const dialogBusy = shallowRef(false)
const dialogError = shallowRef<string | null>(null)

function joinPath(dir: string, name: string): string {
  return dir ? `${dir}/${name}` : name
}

async function createFolder(name: string): Promise<void> {
  if (newFolderIn.value === null) return
  dialogBusy.value = true
  const { result, error } = await drive.createFolder(props.space, joinPath(newFolderIn.value, name))
  dialogBusy.value = false
  dialogError.value = error
  if (result) {
    newFolderIn.value = null
    emit('created', result.path)
  }
}

async function renameTo(name: string): Promise<void> {
  const entry = renaming.value
  if (!entry) return
  dialogBusy.value = true
  const { result, error } = await drive.rename(props.space, entry.path, name)
  dialogBusy.value = false
  dialogError.value = error
  if (result) {
    renaming.value = null
    if (result.path !== entry.path) emit('moved', entry.path, result.path)
  }
}

function closeDialog(): void {
  newFolderIn.value = null
  renaming.value = null
  dialogError.value = null
}

const deletion = useConfirmation()
function requestDelete(entry: EntryWire): void {
  const folder = entry.kind === 'dir'
  deletion.request({
    title: folder ? 'Delete folder' : 'Delete document',
    description: folder
      ? `Delete the empty folder ${entry.path}? It is removed for every chat and session that shares this space.`
      : `Delete ${entry.path}? The file is removed for every chat and session that shares this space. This cannot be undone.`,
    confirmLabel: folder ? 'Delete folder' : 'Delete document',
    testid: `${props.testid}-delete-confirmation`,
    onConfirm: async () => {
      const { error } = await drive.remove(props.space, entry.path)
      if (error) throw new Error(error)
      emit('deleted', entry.path)
    },
  })
}

async function reveal(entry: EntryWire): Promise<void> {
  try {
    await Reveal(props.space, entry.path)
  } catch (err) {
    showToast(errorText(err, `Could not reveal ${entry.path || props.space}.`), { severity: 'error' })
  }
}

async function copyPath(entry: EntryWire): Promise<void> {
  let location: string
  try {
    location = await Locate(props.space, entry.path)
  } catch (err) {
    showToast(errorText(err, `Could not find ${entry.path || props.space} on disk.`), { severity: 'error' })
    return
  }
  await copy(location)
  if (copyStatus.value === 'success') showToast('Copied the path', { severity: 'success' })
  else showToast('Could not copy to the clipboard', { severity: 'error' })
}

function onSelect(id: string): void {
  const entry = target.value
  close()
  if (!entry) return
  dialogError.value = null
  switch (id as DriveRowAction) {
    case 'new-folder':
      newFolderIn.value = entry.path
      break
    case 'rename':
      if (props.inlineRename) emit('rename', entry)
      else renaming.value = entry
      break
    case 'move':
      moving.value = entry
      break
    case 'reveal':
      void reveal(entry)
      break
    case 'copy-path':
      void copyPath(entry)
      break
    case 'delete':
      requestDelete(entry)
      break
  }
}

/** Asks for a folder name and creates it under dir ("" is the space root). */
function newFolder(dir: string): void {
  dialogError.value = null
  newFolderIn.value = dir
}

defineExpose({ open, newFolder })
</script>

<template>
  <AppMenu
    v-if="target && anchor"
    :entries="entries"
    :anchor="anchor"
    :ignore="[toggle]"
    :testid="`${testid}-menu`"
    @select="onSelect"
    @close="close"
  />
  <RenameDialog
    v-if="newFolderIn !== null"
    title="New folder"
    label="Name"
    name="New folder"
    :hint="newFolderIn ? `Inside ${newFolderIn}` : `At the top of ${space}`"
    :icon="IconFolderPlus"
    confirm-label="Create"
    :busy="dialogBusy"
    :error="dialogError"
    :testid="`${testid}-new-folder`"
    @save="createFolder"
    @close="closeDialog"
  />
  <RenameDialog
    v-if="renaming"
    :title="`Rename ${renaming.name}`"
    label="Name"
    :name="renaming.name"
    :hint="renaming.kind === 'dir' ? undefined : 'A document keeps its kind: leave out the extension or repeat it.'"
    :busy="dialogBusy"
    :error="dialogError"
    :testid="`${testid}-rename`"
    @save="renameTo"
    @close="closeDialog"
  />
  <MoveDialog
    v-if="moving && tree"
    :space="space"
    :tree="tree"
    :entry="moving"
    :testid="`${testid}-move`"
    @moved="(from, to) => emit('moved', from, to)"
    @close="moving = null"
  />
  <ConfirmationHost :confirmation="deletion" />
</template>
