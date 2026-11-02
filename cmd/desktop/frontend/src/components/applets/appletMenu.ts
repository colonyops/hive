import type { MenuEntry } from '@hive/ui/types/menu'
import { launcherIconComponent } from '../../lib/launcherIcons'
import type { Applet } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

const MENU_ID_PREFIX = 'applet-menu:'

/** A "+" menu's rows for the applets the user put there. */
export function appletMenuEntries(applets: readonly Applet[]): MenuEntry[] {
  return applets.map((applet) => ({
    kind: 'action' as const,
    id: `${MENU_ID_PREFIX}${applet.id}`,
    label: applet.title,
    icon: launcherIconComponent(applet.icon),
    testid: `session-add-applet-${applet.id}`,
  }))
}

export function appletMenuChoice(id: string): string | undefined {
  return id.startsWith(MENU_ID_PREFIX) ? id.slice(MENU_ID_PREFIX.length) : undefined
}
