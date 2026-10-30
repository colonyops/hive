import type { Component } from 'vue'

/**
 * Every view a pop-out window can host, by the kind popOutKinds declares in
 * wailsui/popouts.go. A surface takes `params` and emits `update:params` and
 * `title`. It re-reads Go for its data and never reads the main window's shell.
 */
export const popOutSurfaces: Record<string, () => Promise<{ default: Component }>> = {
  drive: () => import('./DriveSurface.vue'),
}
