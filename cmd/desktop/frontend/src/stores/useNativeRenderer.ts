import { computed, onScopeDispose, readonly, ref, watch } from 'vue'
import {
  AppearanceSettings as GetAppearanceSettings,
  SetTerminalNativeRenderer as PersistTerminalNativeRenderer,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import {
  RendererAvailable,
  SetOccluders,
  SetTheme,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/nativeterminalservice'
import { useTheme } from '../composables/useTheme'
import { useWailsEvent } from '../composables/useWailsEvent'
import { nativeTerminalTheme } from '../lib/terminalTheme'
import { watchOccluders } from '../lib/nativeOcclusion'
import type { CellSize } from '../lib/terminalGrid'
import { defineStore } from './defineStore'
import { usePersistedSetting } from './usePersistedSetting'

// Whether Code terminal panes are drawn by libghostty in native views instead
// of xterm.js in the page. tmux runs the sessions either way. The setting only
// takes effect in a build that can draw natively.
export const useNativeRenderer = defineStore('nativeRenderer', () => {
  const setting = usePersistedSetting({
    initial: false,
    read: async () => (await GetAppearanceSettings()).terminalNativeRenderer,
    write: (next) => PersistTerminalNativeRenderer(next),
    label: 'the native renderer setting',
  })
  const available = ref(false)
  void (async () => {
    try {
      available.value = await RendererAvailable()
    } catch {
      available.value = false
    }
  })()
  const enabled = computed(() => available.value && setting.value.value)

  // Every surface shares one font, so the last cell any of them reports is
  // the grid every native pane is sized by.
  const cell = ref<CellSize | null>(null)
  useWailsEvent('nativeterm:event', (event) => {
    const payload = (Array.isArray(event.data) ? event.data[0] : event.data) as
      { kind?: string; data?: string } | undefined
    if (payload?.kind !== 'cell' || !payload.data) return
    const [width, height] = payload.data.split(',').map(Number)
    if (width > 0 && height > 0 && (cell.value?.width !== width || cell.value.height !== height)) {
      cell.value = { width, height }
    }
  })

  // Every native surface draws with the page's terminal colors, panes and
  // herdr-core tabs alike, and follows a theme switch.
  const { theme } = useTheme()
  watch(
    [available, theme],
    ([canDraw]) => {
      if (canDraw) void SetTheme(nativeTerminalTheme()).catch(() => undefined)
    },
    { immediate: true },
  )

  // Menus, popovers and dialogs the page floats over a native surface would
  // be hidden behind it; every surface cuts them out instead.
  let stopOccluders: (() => void) | null = null
  watch(
    available,
    (canDraw) => {
      stopOccluders?.()
      stopOccluders = canDraw ? watchOccluders((rects) => void SetOccluders(rects).catch(() => undefined)) : null
    },
    { immediate: true },
  )
  onScopeDispose(() => stopOccluders?.())

  return {
    available: readonly(available),
    enabled,
    setting: setting.value,
    set: (next: boolean) => setting.set(next),
    cell: readonly(cell),
  }
})
