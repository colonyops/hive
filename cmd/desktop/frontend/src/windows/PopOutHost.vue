<script setup lang="ts">
import { Window } from '@wailsio/runtime'
import { defineAsyncComponent, ref } from 'vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import { Retarget } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/popoutservice'
import ToastStack from '../components/ToastStack.vue'
import { useToasts } from '../composables/useToasts'
import { createLogger } from '../lib/log'
import { popOutSurfaces } from './surfaces'

const log = createLogger('pop-out')
const { toasts, dismissToast, clearToasts } = useToasts()

const query = new URLSearchParams(window.location.search)
const kind = query.get('popout') ?? ''
query.delete('popout')
const load = popOutSurfaces[kind]
const surface = load ? defineAsyncComponent(load) : null
const params = ref(Object.fromEntries(query))

// The URL carries the params so a reload lands on the same place.
function updateParams(next: Record<string, string>): void {
  params.value = next
  const search = new URLSearchParams({ popout: kind, ...next })
  window.history.replaceState(null, '', `?${search.toString()}`)
  Retarget({ kind, params: next }).catch((error: unknown) => {
    log.warn('recording the window scope failed', error)
  })
}

function updateTitle(title: string): void {
  document.title = title
  Window.SetTitle(title).catch((error: unknown) => {
    log.debug('window title is unavailable outside Wails', error)
  })
}
</script>

<template>
  <main class="flex h-screen w-screen flex-col overflow-hidden bg-app text-text" data-testid="popout-window">
    <component :is="surface" v-if="surface" :params="params" @update:params="updateParams" @title="updateTitle" />
    <EmptyState v-else class="m-auto" :message="`Unknown window: ${kind}`" data-testid="popout-unknown" />
    <ToastStack :toasts="toasts" @dismiss="dismissToast" @clear-all="clearToasts" />
  </main>
</template>
