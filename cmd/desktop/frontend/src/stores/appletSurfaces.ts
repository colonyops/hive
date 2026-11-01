export type AppletSurface = 'page' | 'titlebar' | 'dropdown'

// A surface the user chooses per applet, kept in settings.yaml under this surface's name.
export interface AppletPlacement {
  /** The switch in Settings. */
  label: string
  /** Finishes a sentence, as in "It offers bar.html for the title bar." */
  where: string
  /** The page an applet offers here, read from an Applet or an import preview. */
  offeredPage: (applet: { titlebar?: { entry: string } | null }) => string | undefined
}

export interface AppletSurfaceRules {
  /** The surface that opens for the applet itself, and takes what is waiting for it. */
  panel: boolean
  /** Keeps running with nothing on screen asking for it, so a command prefers it. */
  persistent: boolean
  /** Takes focus when its page loads. A page that reloads on its own must not. */
  takesFocus: boolean
  /** Escape closes it. */
  escapeCloses: boolean
  /** Its page can ask for a size with hive.resize. */
  resizable: boolean
  /** Its page can open a dropdown under it with hive.dropdown. */
  canOpenDropdown: boolean
  placement?: AppletPlacement
}

// What differs between the places a page can appear. A new surface is a row here and in
// applets.Surfaces, a list in settings.AppletsSettings, and the host that mounts AppletFrame
// (ADR applets-are-web-pages-with-full-access-to-the-machine).
export const SURFACES: Record<AppletSurface, AppletSurfaceRules> = {
  page: {
    panel: true,
    persistent: false,
    takesFocus: true,
    escapeCloses: true,
    resizable: false,
    canOpenDropdown: false,
  },
  titlebar: {
    panel: false,
    persistent: true,
    takesFocus: false,
    escapeCloses: false,
    resizable: true,
    canOpenDropdown: true,
    placement: {
      label: 'Title bar',
      where: 'the title bar',
      offeredPage: (applet) => applet.titlebar?.entry,
    },
  },
  dropdown: {
    panel: false,
    persistent: false,
    takesFocus: true,
    escapeCloses: true,
    resizable: true,
    canOpenDropdown: false,
  },
}

export const PLACEABLE_SURFACES = (Object.keys(SURFACES) as AppletSurface[]).flatMap((surface) => {
  const placement = SURFACES[surface].placement
  return placement ? [{ surface, placement }] : []
})
