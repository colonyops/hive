<script setup lang="ts">
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import { computed, onMounted, type Component } from 'vue'
import { useRouter } from 'vue-router'
import IconBookMarked from '~icons/lucide/book-marked'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconClock from '~icons/lucide/clock'
import IconGlobe from '~icons/lucide/globe'
import IconLink from '~icons/lucide/link'
import IconMessagesSquare from '~icons/lucide/messages-square'
import { useDriveData } from '../lib/driveData'
import { formatBytes } from '../lib/format'
import { useMe } from '../lib/me'
import { useSearchQuery } from '../lib/search'
import { documentRoute, spaceFromKey, type SpaceRef } from '../lib/space'
import { buildTree, filterTree, foldersOnly } from '../lib/tree'
import type { SpaceSummary } from '../types'
import DocTree from './DocTree.vue'

const props = defineProps<{
  /** The space on screen, or null on a page outside any space. */
  space: SpaceRef | null
  /** The folder on screen, marked in the tree. */
  folder: string | null
}>()

const router = useRouter()
const data = useDriveData()
const me = useMe()
const query = useSearchQuery()

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

const filtering = computed(() => query.value.trim() !== '')
const folderTree = computed(() => {
  const docs = props.space ? data.documents[props.space.key] : undefined
  return foldersOnly(filterTree(buildTree(docs ?? []), query.value))
})

const quotaPercent = computed(() => {
  const quota = me?.me.value?.quota
  if (!quota?.limit) return 0
  return Math.min(100, Math.round((quota.used / quota.limit) * 100))
})

const rowClass = 'flex h-7 items-center gap-2 px-3 text-small hover:bg-chip'
const currentClass = 'bg-selection font-medium text-text'

function openFolder(path: string): void {
  if (props.space) void router.push(documentRoute(props.space, path))
}
</script>

<template>
  <nav
    class="flex max-h-[40vh] w-full shrink-0 flex-col border-b border-row bg-sidebar md:max-h-none md:w-64 md:border-b-0 md:border-r"
    aria-label="Drive"
    data-testid="sidebar"
  >
    <div class="min-h-0 flex-1 overflow-y-auto py-2">
      <RouterLink
        :to="{ name: 'recent' }"
        :class="rowClass"
        class="text-text-2"
        active-class="bg-selection font-medium text-text"
      >
        <IconClock class="size-3.5 shrink-0 text-text-4" aria-hidden="true" />
        Recent
      </RouterLink>
      <RouterLink
        :to="{ name: 'links' }"
        :class="rowClass"
        class="text-text-2"
        active-class="bg-selection font-medium text-text"
      >
        <IconLink class="size-3.5 shrink-0 text-text-4" aria-hidden="true" />
        Shared links
      </RouterLink>

      <InlineError v-if="data.spacesError.value" variant="line" class="px-3 py-2" :message="data.spacesError.value" />
      <Spinner v-else-if="!data.spaces.value" class="mx-3 my-3" />
      <section
        v-for="group in data.spaces.value ? groups : []"
        :key="group.label"
        class="mt-3"
        :aria-label="group.label"
      >
        <h2 class="px-3 pb-1 text-caption font-medium uppercase tracking-wide text-text-4">{{ group.label }}</h2>
        <div v-for="entry in group.spaces" :key="entry.space">
          <RouterLink
            :to="spaceFromKey(entry.space).routePrefix"
            :class="[rowClass, entry.space === space?.key && !folder ? currentClass : 'text-text-2']"
            :aria-current="entry.space === space?.key ? 'page' : undefined"
            :title="entry.space"
          >
            <IconChevronRight
              class="size-3 shrink-0 text-text-4 transition-transform"
              :class="entry.space === space?.key ? 'rotate-90' : ''"
              aria-hidden="true"
            />
            <component :is="group.icon" class="size-3.5 shrink-0 text-text-4" aria-hidden="true" />
            <span class="min-w-0 flex-1 truncate">{{ spaceFromKey(entry.space).title }}</span>
            <span class="text-caption text-text-4">{{ entry.documents }}</span>
          </RouterLink>
          <template v-if="entry.space === space?.key">
            <InlineError
              v-if="data.errors[entry.space]"
              variant="line"
              class="px-3 py-1"
              :message="data.errors[entry.space] ?? null"
            />
            <DocTree
              v-else
              :tree="folderTree"
              :space-key="entry.space"
              :selected="folder"
              :filtering="filtering"
              @open-folder="openFolder"
            />
          </template>
        </div>
      </section>
    </div>

    <div v-if="me?.me.value" class="shrink-0 border-t border-row px-3 py-2.5" data-testid="storage-meter">
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
        <p class="mt-1.5 text-caption text-text-3">
          {{ formatBytes(me.me.value.quota.used) }} of {{ formatBytes(me.me.value.quota.limit) }} used
        </p>
      </template>
      <p v-else class="text-caption text-text-3">{{ formatBytes(me.me.value.quota.used) }} used</p>
    </div>
  </nav>
</template>
