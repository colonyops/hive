import { useClipboard } from '@hive/ui/composables/useClipboard'
import { useConfirmation } from '@hive/ui/composables/useConfirmation'
import type { MenuEntry } from '@hive/ui/types/menu'
import { useRouter } from 'vue-router'
import IconCopy from '~icons/material-symbols/content-copy'
import IconExternalLink from '~icons/material-symbols/open-in-new'
import IconShare from '~icons/material-symbols/share'
import IconTrash from '~icons/material-symbols/delete'
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

/** Copy-link and delete for one document, shared by the listings and the document page. */
export function useDocumentActions() {
  const me = useMe()
  const confirmation = useConfirmation()
  const clipboard = useClipboard()
  const toast = useToast()

  async function copyLink(url: string): Promise<void> {
    toast.show((await clipboard.copy(url)) ? 'Link copied' : 'Could not copy the link')
  }

  /** `after` runs once the document is gone and the storage meter has caught up. */
  function requestDelete(apiPrefix: string, path: string, after: () => void | Promise<void>): void {
    confirmation.request({
      title: 'Delete this document?',
      description: `${path} is deleted from the server. Local copies stay. Any share link stops working.`,
      confirmLabel: 'Delete',
      testid: 'delete-dialog',
      onConfirm: async () => {
        await api.deleteDocument(apiPrefix, path)
        await me?.refresh()
        await after()
      },
    })
  }

  return { confirmation, copyLink, requestDelete }
}

/** `changed` runs after anything that alters a document. */
export function useEntryActions(changed: () => void | Promise<void>) {
  const router = useRouter()
  const { confirmation, copyLink, requestDelete } = useDocumentActions()

  async function run(id: string, entry: ListEntry): Promise<void> {
    switch (id) {
      case 'open':
        await router.push(entryRoute(entry))
        break
      case 'copy-link':
        if (entry.doc?.shared) await copyLink(entry.doc.shared.url)
        break
      case 'delete':
        requestDelete(entry.space.apiPrefix, entry.path, changed)
        break
    }
  }

  return { run, confirmation }
}
