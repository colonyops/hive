import './styles/main.css'

import { installHiveUi } from '@hive/ui/platform'
import { createApp, type Component } from 'vue'
import App from './App.vue'
import { createBrowserPlatform } from './platform'
import { router } from './router'

const app = createApp(App as Component).use(router)
installHiveUi(app, createBrowserPlatform())
app.mount('#app')
