// One variable face covers every weight the UI asks for; the subsets are split
// by unicode-range, so only the ones a glyph needs are fetched. The mono is not
// imported here — the terminal's own faces are the app's mono (ADR bundled-faces-are-jetbrains-mono-inter-and-a-symbol-font).
import '@fontsource-variable/inter'
import '@fontsource-variable/inter/wght-italic.css'
import './styles/main.css'
import './styles/markdown.css'
import './styles/canvas-html.css'

import { createApp, type Component } from 'vue'
import App from './App.vue'
import DiagnosticsWindow from './components/DiagnosticsWindow.vue'
import { initializeAppFont } from './composables/useAppFont'
import { initializeKeybindings } from './composables/useKeybindings'
import { initializeTheme } from './composables/useTheme'
import { captureVueErrors, installErrorCapture } from './lib/log'
import { router } from './router'

installErrorCapture()
initializeTheme()
initializeAppFont()
if (new URLSearchParams(window.location.search).has('diagnostics')) {
  const app = createApp(DiagnosticsWindow as Component)
  captureVueErrors(app)
  app.mount('#app')
} else {
  initializeKeybindings()
  const app = createApp(App as Component).use(router)
  captureVueErrors(app)
  app.mount('#app')
}
