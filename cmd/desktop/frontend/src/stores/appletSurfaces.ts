import type { Component } from 'vue'
import IconAppWindow from '~icons/lucide/app-window'
import IconPanelTop from '~icons/lucide/panel-top'
import IconSquareStack from '~icons/lucide/square-stack'

export type AppletSurface = 'tab' | 'page' | 'titlebar' | 'dropdown'

// A surface the user chooses per applet, kept in settings.yaml under this surface's name.
interface AppletPlacement {
  /** The name in Settings. */
  label: string
  icon: Component
  /** What being placed here means, under the switch in Settings. */
  description: string
  /** Finishes a sentence, as in "It offers bar.html for the title bar." */
  where: string
  /** The page an applet offers here, read from an Applet or an import preview. */
  offeredPage: (applet: { entry: string; titlebar?: { entry: string } | null }) => string | undefined
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
  placement?: AppletPlacement
}

// What differs between the places a page can appear. A new surface is a row here, a list in
// settings.AppletsSettings, and the host that mounts AppletFrame
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
    placement: {
      label: 'Session tab',
      icon: IconSquareStack,
      description: "Adds it to a session's + menu. Opens as a tab beside the session's terminals.",
      where: "a session's + menu",
      offeredPage: (applet) => applet.entry,
    },
  },
  page: {
    panel: true,
    persistent: false,
    takesFocus: true,
    escapeCloses: true,
    placement: {
      label: 'Session pop-up',
      icon: IconAppWindow,
      description: "Adds it to a session's + menu. Opens over the app, on that session.",
      where: "a session's + menu",
      offeredPage: (applet) => applet.entry,
    },
  },
  titlebar: {
    panel: false,
    persistent: true,
    takesFocus: false,
    escapeCloses: false,
    placement: {
      label: 'Title bar',
      icon: IconPanelTop,
      description: 'Loads with the app and keeps running until you turn it off.',
      where: 'the title bar',
      offeredPage: (applet) => applet.titlebar?.entry,
    },
  },
  dropdown: {
    panel: false,
    persistent: false,
    takesFocus: true,
    escapeCloses: true,
  },
}

export const PLACEABLE_SURFACES = (Object.keys(SURFACES) as AppletSurface[]).flatMap((surface) => {
  const placement = SURFACES[surface].placement
  return placement ? [{ surface, placement }] : []
})
