// One variable face covers every weight the UI asks for; the subsets are split
// by unicode-range, so only the ones a glyph needs are fetched. The mono is not
// imported here — the terminal's own faces are the app's mono (ADR bundled-faces-are-jetbrains-mono-inter-and-a-symbol-font).
import '@fontsource-variable/inter'
import '@fontsource-variable/inter/wght-italic.css'
import './styles/main.css'

import { installHiveUi } from '@hive/ui/platform'
import { createApp, type App, type Component } from 'vue'
import type { Router } from 'vue-router'
import { initializeAppFont } from './composables/useAppFont'
import { initializeKeybindings } from './composables/useKeybindings'
import { initializeTheme } from './composables/useTheme'
import { hiveUiPlatform } from './lib/hiveUiPlatform'
import { captureVueErrors, installErrorCapture } from './lib/log'
import { startPerfMonitor } from './lib/perfMonitor'

installErrorCapture()
initializeTheme()
initializeAppFont()

// One window, one root, loaded on demand: a document in a pop-out must not pay
// for the feed, the flows canvas, and the shell around them.
async function createRoot(query: URLSearchParams): Promise<{ app: App; router?: Router }> {
  if (query.has('popout')) {
    const root = (await import('./windows/PopOutHost.vue')) as { default: Component }
    return { app: createApp(root.default) }
  }
  if (query.has('diagnostics')) {
    const root = (await import('./components/DiagnosticsWindow.vue')) as { default: Component }
    return { app: createApp(root.default) }
  }
  initializeKeybindings()
  const root = (await import('./App.vue')) as { default: Component }
  const { router } = await import('./router')
  return { app: createApp(root.default).use(router), router }
}

void createRoot(new URLSearchParams(window.location.search)).then(({ app, router }) => {
  installHiveUi(app, hiveUiPlatform)
  captureVueErrors(app)
  app.mount('#app')
  void startPerfMonitor(router)
})
