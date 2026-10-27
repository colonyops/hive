import './styles/main.css'

import { installHiveUi } from '@hive/ui/platform'
import { createApp, type Component } from 'vue'
import App from './App.vue'
import { createToast, toastKey } from './lib/toast'
import { createBrowserPlatform } from './platform'
import { router } from './router'

const toast = createToast()
const app = createApp(App as Component).use(router)
app.provide(toastKey, toast)
installHiveUi(app, createBrowserPlatform(toast))
app.mount('#app')
