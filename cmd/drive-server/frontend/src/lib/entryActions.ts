import { useClipboard } from '@hive/ui/composables/useClipboard'
import { useConfirmation } from '@hive/ui/composables/useConfirmation'
import type { MenuEntry } from '@hive/ui/types/menu'
import { useRouter } from 'vue-router'
import IconCopy from '~icons/lucide/copy'
import IconExternalLink from '~icons/lucide/external-link'
import IconShare from '~icons/lucide/share-2'
import IconTrash from '~icons/lucide/trash-2'
import { api } from '../api'
import { entryRoute, type ListEntry } from './entries'
import { useMe } from './me'
import { useToast } from './toast'

export function entryMenu(entry: ListEntry): MenuEntry[] {
  const open: MenuEntry = { kind: 'action', id: 'open', label: 'Open', icon: IconExternalLink, testid: 'menu-open' }
  if (entry.kind === 'dir') return [open]
  return [
    open,
    { kind: 'separator' },
    { kind: 'action', id: 'share', label: 'Share…', icon: IconShare, testid: 'menu-share' },
    ...(entry.shared
      ? ([
          { kind: 'action', id: 'copy-link', label: 'Copy link', icon: IconCopy, testid: 'menu-copy-link' },
        ] satisfies MenuEntry[])
      : []),
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
  const toast = useToast()

  async function run(id: string, entry: ListEntry): Promise<void> {
    const prefix = entry.space.apiPrefix
    switch (id) {
      case 'open':
        await router.push(entryRoute(entry))
        break
      case 'copy-link':
        if (entry.doc?.shared) {
          await clipboard.copy(entry.doc.shared.url)
          toast.show('Link copied')
        }
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

  return { run, confirmation }
}
