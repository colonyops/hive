import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import IconClipboardCopy from '~icons/lucide/clipboard-copy'
import IconFolderInput from '~icons/lucide/folder-input'
import IconFolderOpen from '~icons/lucide/folder-open'
import IconFolderPlus from '~icons/lucide/folder-plus'
import IconPencil from '~icons/lucide/pencil'
import IconTrash2 from '~icons/lucide/trash-2'
import type { MenuActionEntry, MenuEntry } from '@hive/ui/types/menu'
import type { EntryWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

export type DriveRowAction = 'new-folder' | 'rename' | 'move' | 'reveal' | 'copy-path' | 'delete'

/**
 * The actions for one tree entry. The space root (path "") can only take a
 * new folder and be located; the drive refuses to rename, move, or delete it.
 * Entry ids are DriveRowAction values; test ids are `${testid}-<action>`.
 */
export function useDriveRowMenu(entry: MaybeRefOrGetter<EntryWire | null>, testid: MaybeRefOrGetter<string>) {
  const entries = computed<MenuEntry[]>(() => {
    const target = toValue(entry)
    if (!target) return []
    const id = toValue(testid)
    const action = (action: DriveRowAction, label: string, extra: Partial<MenuActionEntry> = {}): MenuActionEntry => ({
      kind: 'action',
      id: action,
      label,
      testid: `${id}-${action}`,
      ...extra,
    })
    const isDir = target.kind === 'dir'
    const isRoot = target.path === ''
    const out: MenuEntry[] = []
    if (isDir) out.push(action('new-folder', 'New folder here…', { icon: IconFolderPlus }))
    if (!isRoot) {
      out.push(action('rename', 'Rename', { icon: IconPencil, kbd: 'F2' }))
      out.push(action('move', 'Move to…', { icon: IconFolderInput }))
    }
    out.push({ kind: 'separator' })
    out.push(action('reveal', 'Reveal in Finder', { icon: IconFolderOpen }))
    out.push(action('copy-path', 'Copy path', { icon: IconClipboardCopy }))
    if (!isRoot) {
      const notEmpty = isDir && !!target.children?.length
      out.push({ kind: 'separator' })
      out.push(
        action('delete', isDir ? 'Delete folder' : 'Delete', {
          icon: IconTrash2,
          disabled: notEmpty,
          hint: notEmpty ? 'Only an empty folder can be deleted' : undefined,
        }),
      )
    }
    return out
  })
  return { entries }
}
