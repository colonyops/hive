import '@fontsource/ibm-plex-sans/400.css'
import '@fontsource/ibm-plex-sans/500.css'
import '@fontsource/ibm-plex-sans/600.css'
import '@fontsource/ibm-plex-sans/700.css'
import '@fontsource/ibm-plex-mono/400.css'
import '@fontsource/ibm-plex-mono/500.css'
import '@fontsource/ibm-plex-mono/600.css'
import './styles/main.css'

import { createApp } from 'vue'
import App from './App.vue'
import { initializeTheme } from './composables/useTheme'
import { router } from './router'

initializeTheme()
createApp(App).use(router).mount('#app')
