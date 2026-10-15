import { useClipboard } from '@hive/ui/composables/useClipboard'
import { useConfirmation } from '@hive/ui/composables/useConfirmation'
import type { MenuEntry } from '@hive/ui/types/menu'
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import IconCopy from '~icons/lucide/copy'
import IconExternalLink from '~icons/lucide/external-link'
import IconLink from '~icons/lucide/link'
import IconLinkOff from '~icons/lucide/link-2-off'
import IconTrash from '~icons/lucide/trash-2'
import { api } from '../api'
import { entryRoute, type ListEntry } from './entries'
import { useMe } from './me'

export function entryMenu(entry: ListEntry): MenuEntry[] {
  const open: MenuEntry = { kind: 'action', id: 'open', label: 'Open', icon: IconExternalLink, testid: 'menu-open' }
  if (entry.kind === 'dir') return [open]
  return [
    open,
    { kind: 'separator' },
    ...(entry.shared
      ? ([
          { kind: 'action', id: 'copy-link', label: 'Copy share link', icon: IconCopy, testid: 'menu-copy-link' },
          { kind: 'action', id: 'unshare', label: 'Unshare', icon: IconLinkOff, testid: 'menu-unshare' },
        ] satisfies MenuEntry[])
      : ([
          { kind: 'action', id: 'share', label: 'Share', icon: IconLink, testid: 'menu-share' },
        ] satisfies MenuEntry[])),
    { kind: 'separator' },
    { kind: 'action', id: 'delete', label: 'Delete', icon: IconTrash, testid: 'menu-delete' },
  ]
}

/** The row-menu actions of a listing. `changed` runs after anything that alters a document. */
export function useEntryActions(changed: () => void | Promise<void>) {
  const router = useRouter()
  const me = useMe()
  const confirmation = useConfirmation()
  const clipboard = useClipboard()
  const error = ref<string | null>(null)

  async function guarded(fallback: string, work: () => Promise<void>): Promise<void> {
    error.value = null
    try {
      await work()
    } catch (cause) {
      error.value = cause instanceof Error ? cause.message : fallback
    }
  }

  async function run(id: string, entry: ListEntry): Promise<void> {
    const prefix = entry.space.apiPrefix
    switch (id) {
      case 'open':
        await router.push(entryRoute(entry))
        break
      case 'copy-link':
        if (entry.doc?.shared) await clipboard.copy(entry.doc.shared.url)
        break
      case 'share':
        await guarded('Could not share the document.', async () => {
          await api.share(prefix, entry.path, '')
          await changed()
        })
        break
      case 'unshare':
        await guarded('Could not stop sharing the document.', async () => {
          await api.unshare(prefix, entry.path)
          await changed()
        })
        break
      case 'delete':
        confirmation.request({
          title: 'Delete this document?',
          description: `${entry.path} is deleted from the server. Local copies stay. Any share link stops working.`,
          confirmLabel: 'Delete',
          testid: 'delete-dialog',
          onConfirm: async () => {
            await api.deleteDocument(prefix, entry.path)
            await me?.refresh()
            await changed()
          },
        })
        break
    }
  }

  return { run, confirmation, error }
}
