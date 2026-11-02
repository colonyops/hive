import type { MenuEntry } from '@hive/ui/types/menu'
import { launcherIconComponent } from '../../lib/launcherIcons'
import type { AppletMenuRow } from '../../stores/useApplets'

const MENU_ID_PREFIX = 'applet-menu:'

/** A "+" menu's rows for the applets the user put there. Both forms of one applet need telling apart. */
export function appletMenuEntries(rows: readonly AppletMenuRow[]): MenuEntry[] {
  return rows.map(({ applet, opensAs }) => ({
    kind: 'action' as const,
    id: `${MENU_ID_PREFIX}${opensAs}:${applet.id}`,
    label: applet.title,
    icon: launcherIconComponent(applet.icon),
    hint: opensAs === 'page' ? 'Pop-up' : undefined,
    testid: `session-add-applet-${opensAs}-${applet.id}`,
  }))
}

export function appletMenuChoice(id: string): { applet: string; opensAs: AppletMenuRow['opensAs'] } | undefined {
  if (!id.startsWith(MENU_ID_PREFIX)) return undefined
  const rest = id.slice(MENU_ID_PREFIX.length)
  const colon = rest.indexOf(':')
  return { opensAs: rest.slice(0, colon) as AppletMenuRow['opensAs'], applet: rest.slice(colon + 1) }
}
