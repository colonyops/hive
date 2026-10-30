// One variable face covers every weight the UI asks for; the subsets are split
// by unicode-range, so only the ones a glyph needs are fetched. The mono is not
// imported here — the terminal's own faces are the app's mono (ADR bundled-faces-are-jetbrains-mono-inter-and-a-symbol-font).
import '@fontsource-variable/inter'
import '@fontsource-variable/inter/wght-italic.css'
import './styles/main.css'

import { installHiveUi } from '@hive/ui/platform'
import { createApp, type Component } from 'vue'
import App from './App.vue'
import DiagnosticsWindow from './components/DiagnosticsWindow.vue'
import { initializeAppFont } from './composables/useAppFont'
import { initializeKeybindings } from './composables/useKeybindings'
import { initializeTheme } from './composables/useTheme'
import { hiveUiPlatform } from './lib/hiveUiPlatform'
import { captureVueErrors, installErrorCapture } from './lib/log'
import { startPerfMonitor } from './lib/perfMonitor'
import { router } from './router'

installErrorCapture()
initializeTheme()
initializeAppFont()
if (new URLSearchParams(window.location.search).has('diagnostics')) {
  const app = createApp(DiagnosticsWindow as Component)
  installHiveUi(app, hiveUiPlatform)
  captureVueErrors(app)
  app.mount('#app')
  void startPerfMonitor()
} else {
  initializeKeybindings()
  const app = createApp(App as Component).use(router)
  installHiveUi(app, hiveUiPlatform)
  captureVueErrors(app)
  app.mount('#app')
  void startPerfMonitor(router)
}
