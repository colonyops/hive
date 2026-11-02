import type { Component } from 'vue'
import IconAppWindow from '~icons/lucide/app-window'
import IconPanelTop from '~icons/lucide/panel-top'
import IconSquareStack from '~icons/lucide/square-stack'

export type AppletSurface = 'tab' | 'popup' | 'titlebar' | 'dropdown'

/** What applet.yaml says about where an applet shows, read from an Applet or an import preview. */
interface DeclaresSurfaces {
  entry: string
  surfaces: { tab: boolean; popup: object | null; titlebar: { entry: string } | null; titlebarPage?: string }
}

// A surface an applet's file can name under surfaces:.
interface AppletDeclaredSurface {
  /** The name in Settings. */
  label: string
  icon: Component
  /** What listing it means. */
  description: string
  /** The page the applet shows here, or nothing while this surface is off. */
  page: (applet: DeclaresSurfaces) => string | undefined
  /** Whether a switch can turn this on. Only the title bar needs a page of its own. */
  available: (applet: DeclaresSurfaces) => boolean
}

interface AppletSurfaceRules {
  /** The surface that opens for the applet itself, and takes what is waiting for it. */
  panel: boolean
  /** Keeps running with nothing on screen asking for it, so a command prefers it and an import warns of it. */
  persistent: boolean
  /** Takes focus when its page loads. A page that reloads on its own must not. */
  takesFocus: boolean
  /** Escape closes it while focus is outside the frame. Escape in a page goes through the bridge's own rule. */
  escapeCloses: boolean
  declared?: AppletDeclaredSurface
}

// What differs between the places a page can appear. A new surface is a row here, a key under
// surfaces: in applet.yaml, and the host that mounts AppletFrame
// (ADR applets-are-web-pages-with-full-access-to-the-machine).
export const SURFACES: Record<AppletSurface, AppletSurfaceRules> = {
  // Takes focus only when the layout gives its panel the keyboard, because a restored tab
  // loads with no one asking. Escape is left to the page: a tab sits beside terminals, where
  // the key means something else, and has its own close button.
  tab: {
    panel: true,
    persistent: false,
    takesFocus: false,
    escapeCloses: false,
    declared: {
      label: 'Session tab',
      icon: IconSquareStack,
      description: "In a session's + menu. Opens as a tab beside the session's terminals.",
      page: (applet) => (applet.surfaces.tab ? applet.entry : undefined),
      available: () => true,
    },
  },
  popup: {
    panel: true,
    persistent: false,
    takesFocus: true,
    escapeCloses: true,
    declared: {
      label: 'Pop-up',
      icon: IconAppWindow,
      description: 'Opens over the app from the palette or its shortcut.',
      page: (applet) => (applet.surfaces.popup ? applet.entry : undefined),
      available: () => true,
    },
  },
  titlebar: {
    panel: false,
    persistent: true,
    takesFocus: false,
    escapeCloses: false,
    declared: {
      label: 'Title bar',
      icon: IconPanelTop,
      description: 'Loads with the app and keeps running until you turn the applet off.',
      page: (applet) => applet.surfaces.titlebar?.entry,
      available: (applet) => !!applet.surfaces.titlebarPage,
    },
  },
  dropdown: {
    panel: false,
    persistent: false,
    takesFocus: true,
    escapeCloses: true,
  },
}

export const DECLARED_SURFACES = (Object.keys(SURFACES) as AppletSurface[]).flatMap((surface) => {
  const declared = SURFACES[surface].declared
  return declared ? [{ surface, declared }] : []
})
