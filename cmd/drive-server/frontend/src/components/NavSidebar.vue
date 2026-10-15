<script setup lang="ts">
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import { computed, onMounted, type Component } from 'vue'
import IconBookMarked from '~icons/lucide/book-marked'
import IconClock from '~icons/lucide/clock'
import IconGlobe from '~icons/lucide/globe'
import IconLink from '~icons/lucide/link'
import IconMessagesSquare from '~icons/lucide/messages-square'
import { useDriveData } from '../lib/driveData'
import { formatBytes } from '../lib/format'
import { useMe } from '../lib/me'
import { spaceFromKey, type SpaceRef } from '../lib/space'
import type { SpaceSummary } from '../types'

const props = defineProps<{
  /** The space on screen, or null on a page outside any space. */
  space: SpaceRef | null
}>()

const data = useDriveData()
const me = useMe()

onMounted(() => {
  if (!data.spaces.value) void data.loadSpaces()
})

const groups = computed(() => {
  const all = [...(data.spaces.value ?? [])]
  // A space with no documents is absent from the listing, but the route still names it.
  if (data.spaces.value && props.space && !all.some((s) => s.space === props.space?.key)) {
    all.push({ space: props.space.key, kind: 'repository', documents: 0, bytes: 0 })
  }
  const groupOf = (kind: SpaceSummary['kind']) => all.filter((s) => s.kind === kind)
  return [
    { label: 'Repositories', icon: IconBookMarked as Component, spaces: groupOf('repository') },
    { label: 'Workspaces', icon: IconMessagesSquare as Component, spaces: groupOf('workspace') },
    { label: 'Shared', icon: IconGlobe as Component, spaces: groupOf('shared') },
  ].filter((group) => group.spaces.length)
})

const quotaPercent = computed(() => {
  const quota = me?.me.value?.quota
  if (!quota?.limit) return 0
  return Math.min(100, Math.round((quota.used / quota.limit) * 100))
})

const rowClass = 'flex h-10 items-center gap-3 rounded-lg px-3 text-body hover:bg-chip'
const currentClass = 'bg-selection font-medium text-text'
</script>

<template>
  <nav
    class="flex min-h-0 w-full shrink-0 flex-col border-r border-row bg-sidebar md:static md:w-72"
    aria-label="Drive"
    data-testid="sidebar"
  >
    <div class="min-h-0 flex-1 overflow-y-auto px-3 py-3">
      <RouterLink
        :to="{ name: 'recent' }"
        :class="rowClass"
        class="text-text-2"
        active-class="bg-selection font-medium text-text"
      >
        <IconClock class="size-5 shrink-0 text-text-3" aria-hidden="true" />
        Recent
      </RouterLink>
      <RouterLink
        :to="{ name: 'links' }"
        :class="rowClass"
        class="text-text-2"
        active-class="bg-selection font-medium text-text"
      >
        <IconLink class="size-5 shrink-0 text-text-3" aria-hidden="true" />
        Shared links
      </RouterLink>

      <InlineError v-if="data.spacesError.value" variant="line" class="px-3 py-2" :message="data.spacesError.value" />
      <Spinner v-else-if="!data.spaces.value" class="mx-3 my-4" />
      <section
        v-for="group in data.spaces.value ? groups : []"
        :key="group.label"
        class="mt-5"
        :aria-label="group.label"
      >
        <h2 class="px-3 pb-1.5 text-caption font-medium uppercase tracking-wide text-text-4">{{ group.label }}</h2>
        <RouterLink
          v-for="entry in group.spaces"
          :key="entry.space"
          :to="spaceFromKey(entry.space).routePrefix"
          :class="[rowClass, entry.space === space?.key ? currentClass : 'text-text-2']"
          :aria-current="entry.space === space?.key ? 'page' : undefined"
          :title="entry.space"
          data-testid="nav-space"
        >
          <component :is="group.icon" class="size-5 shrink-0 text-text-3" aria-hidden="true" />
          <span class="min-w-0 flex-1 truncate">{{ spaceFromKey(entry.space).title }}</span>
          <span class="text-small text-text-4">{{ entry.documents }}</span>
        </RouterLink>
      </section>
    </div>

    <div v-if="me?.me.value" class="shrink-0 border-t border-row px-6 py-4" data-testid="storage-meter">
      <template v-if="me.me.value.quota.limit">
        <div
          class="h-1.5 overflow-hidden rounded-full bg-chip"
          role="progressbar"
          aria-label="Storage used"
          :aria-valuenow="quotaPercent"
          aria-valuemin="0"
          aria-valuemax="100"
        >
          <div class="h-full bg-accent" :style="{ width: `${quotaPercent}%` }" />
        </div>
        <p class="mt-2 text-small text-text-3">
          {{ formatBytes(me.me.value.quota.used) }} of {{ formatBytes(me.me.value.quota.limit) }} used
        </p>
      </template>
      <p v-else class="text-small text-text-3">{{ formatBytes(me.me.value.quota.used) }} used</p>
    </div>
  </nav>
</template>
