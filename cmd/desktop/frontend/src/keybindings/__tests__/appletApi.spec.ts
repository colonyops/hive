import { describe, expect, it } from 'vitest'
import { router } from '../../router'
import { commandCatalog } from '../catalog'

// Pages shared as .hiveapplet files name these through hive.command, hive.commands and
// hive.navigate, so they are part of the applet API. When this snapshot changes because an
// id or route was renamed, keep the old one as an alias: a file someone shared must keep
// working (ADR applets-are-web-pages-with-full-access-to-the-machine).
describe('what a page can name', () => {
  it('keeps every command id', () => {
    expect(commandCatalog.map((command) => command.id).sort()).toMatchSnapshot()
  })

  it('keeps every route', () => {
    const paths = router.getRoutes().map((route) => route.path)
    expect(paths.filter((path) => path !== '/dev').sort()).toMatchSnapshot()
  })
})
