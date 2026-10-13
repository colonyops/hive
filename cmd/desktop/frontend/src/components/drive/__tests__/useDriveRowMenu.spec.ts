import { describe, expect, it } from 'vitest'
import { useDriveRowMenu } from '../useDriveRowMenu'
import { doc, folder, root } from '../../../test-utils/drive'
import type { MenuEntry } from '../../../types/menu'
import type { EntryWire } from '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

function shape(entry: EntryWire): string[] {
  return useDriveRowMenu(entry, 'm').entries.value.map((item: MenuEntry) =>
    item.kind === 'action' ? `${item.id}${item.disabled ? ' (disabled)' : ''}` : '---',
  )
}

describe('useDriveRowMenu', () => {
  it('offers a document rename, move, reveal, copy path, and delete', () => {
    expect(shape(doc('plans/a.md'))).toEqual(['rename', 'move', '---', 'reveal', 'copy-path', '---', 'delete'])
  })

  it('adds a new folder entry for a folder, and lets an empty one be deleted', () => {
    expect(shape(folder('plans', []))).toEqual([
      'new-folder',
      'rename',
      'move',
      '---',
      'reveal',
      'copy-path',
      '---',
      'delete',
    ])
  })

  it('disables delete on a folder that is not empty and says why', () => {
    const entries = useDriveRowMenu(folder('plans', [doc('plans/a.md')]), 'm').entries.value
    const remove = entries.find((item) => item.kind === 'action' && item.id === 'delete')
    expect(remove).toMatchObject({
      disabled: true,
      hint: 'Only an empty folder can be deleted',
      label: 'Delete folder',
    })
  })

  it('offers only a new folder and the location for the space root', () => {
    expect(shape(root([doc('a.md')]))).toEqual(['new-folder', '---', 'reveal', 'copy-path'])
  })

  it('derives test ids from the menu id and gives rename its key', () => {
    const entries = useDriveRowMenu(doc('a.md'), 'drive-page-menu').entries.value
    expect(entries[0]).toMatchObject({ id: 'rename', kbd: 'F2', testid: 'drive-page-menu-rename' })
    expect(useDriveRowMenu(null, 'm').entries.value).toEqual([])
  })
})
