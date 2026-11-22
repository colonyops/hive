<script setup lang="ts">
import { useIntersectionObserver } from '@vueuse/core'
import { computed, ref } from 'vue'
import FenceCard from '@hive/ui/components/fences/FenceCard.vue'
import type { FenceContext } from '@hive/ui/components/fences/registry'
import type { Applet } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { createLogger } from '../../lib/log'
import { useApplets } from '../../stores/useApplets'
import AppletFrame from './AppletFrame.vue'

const log = createLogger('applet-fence')

const MIN_HEIGHT = 120

// FenceProps spelled out: the SFC compiler cannot resolve a type through the package export map.
const props = defineProps<{
  data: unknown
  source: string
  ctx: FenceContext
  testid: string
  applet: Applet
  lang: string
}>()

const applets = useApplets()
const root = ref<HTMLElement | null>(null)
const visible = ref(false)
const failed = ref(false)
const height = ref(MIN_HEIGHT)

// The iframe is the expensive part, so a block far down a long document loads when it nears the screen.
const { stop } = useIntersectionObserver(
  root,
  (entries) => {
    if (entries.some((entry) => entry.isIntersecting)) {
      visible.value = true
      stop()
    }
  },
  { rootMargin: '200px' },
)

const render = computed(() => ({
  data: props.data,
  source: props.source,
  lang: props.lang,
  docPath: props.ctx.docPath,
}))

function resized(size: { height?: number }): void {
  if (size.height !== undefined && Number.isFinite(size.height)) height.value = Math.max(0, Math.ceil(size.height))
}

function failedToLoad(): void {
  failed.value = true
  log.warn('applet fence failed to load', { applet: props.applet.id, lang: props.lang })
}
</script>

<template>
  <FenceCard :kind="lang" :title="applet.title" :source="source" :lang="lang" :testid="testid">
    <pre v-if="failed" class="fence-fallback overflow-x-auto" :data-testid="`${testid}-fallback`"><code>{{
      source
    }}</code></pre>
    <div v-else ref="root" class="flex flex-col" :style="{ height: `${height}px` }" :data-testid="`${testid}-applet`">
      <AppletFrame
        v-if="visible"
        :applet="applet"
        :session-slug="applets.liveSessionSlug.value"
        surface="markdown"
        :entry="applet.surfaces.markdown?.entry"
        :render="render"
        :on-follow="ctx.follow"
        :testid="`${testid}-page`"
        @resize="resized"
        @failed="failedToLoad"
      />
    </div>
  </FenceCard>
</template>
