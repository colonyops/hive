<script setup lang="ts">
// The Activity view (design 6d): a filterable, day-grouped audit log of what
// the app did — refreshes, sessions, automatic and manual actions, config
// reloads, and errors. Events are recorded by backend subsystems through the
// activity.Recorder and by the frontend via ActivityService.Record; this view
// only reads and presents them. Reached from the titlebar Activity link.
import { computed, onMounted, ref, type Component } from 'vue'
import IconInfo from '~icons/lucide/info'
import IconPlay from '~icons/lucide/play'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import IconSearch from '~icons/lucide/search'
import IconSettings2 from '~icons/lucide/settings-2'
import IconSquareTerminal from '~icons/lucide/square-terminal'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import IconZap from '~icons/lucide/zap'
import { useActivity } from '../composables/useActivity'
import { useEscapeToClose } from '../composables/useEscapeToClose'
import ViewHeader from './settings/ViewHeader.vue'
import {
  ACTIVITY_FILTERS,
  eventStyleKey,
  groupEventsByDay,
  matchesFilter,
  matchesSearch,
  timeLabel,
  type ActivityFilterId,
  type ActivityStyleKey,
} from '../lib/activityPresentation'

const emit = defineEmits<{ close: [] }>()

const { events, loading, error, load, markSeen } = useActivity()

const activeFilter = ref<ActivityFilterId>('all')
const search = ref('')

// Opening the view clears the titlebar's unseen indicator.
markSeen()

const filtered = computed(() =>
  events.value.filter((e) => matchesFilter(e, activeFilter.value) && matchesSearch(e, search.value)),
)
const groups = computed(() => groupEventsByDay(filtered.value))

// style key -> lucide icon + token color classes (kept out of the pure
// presentation lib, which stays framework-free).
const STYLES: Record<ActivityStyleKey, { icon: Component; square: string; emphasis: string }> = {
  error: { icon: IconTriangleAlert, square: 'bg-severity-error-tint text-severity-error', emphasis: 'border-severity-error bg-severity-error-tint' },
  auto_action: { icon: IconZap, square: 'bg-severity-auto-tint text-accent', emphasis: 'border-accent bg-severity-auto-tint' },
  refresh: { icon: IconRefreshCw, square: 'bg-severity-info-tint text-severity-info', emphasis: '' },
  session: { icon: IconPlay, square: 'bg-severity-success-tint text-severity-success', emphasis: '' },
  action: { icon: IconSquareTerminal, square: 'bg-node-purple-tint text-node-purple', emphasis: '' },
  config: { icon: IconSettings2, square: 'bg-chip text-text-3', emphasis: '' },
  system: { icon: IconInfo, square: 'bg-severity-info-tint text-severity-info', emphasis: '' },
}

function styleFor(styleKey: ActivityStyleKey) {
  return STYLES[styleKey]
}

useEscapeToClose(() => emit('close'))

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-1 flex-col" data-testid="activity-view">
    <ViewHeader close-testid="activity-close" @close="emit('close')">
      <template #title>
        <span class="text-[13px] font-semibold text-text">Activity</span>
        <span class="font-mono text-[11px] text-text-4">audit log</span>
      </template>
    </ViewHeader>

    <!-- toolbar: filter pills + search -->
    <div class="flex shrink-0 items-center gap-2 border-b border-row bg-sidebar px-5 py-2.5">
      <button
        v-for="filter in ACTIVITY_FILTERS"
        :key="filter.id"
        type="button"
        class="flex cursor-pointer items-center gap-1.5 rounded-md border px-3 py-1.5 text-[12.5px] font-medium transition-colors"
        :class="activeFilter === filter.id
          ? 'border-accent bg-accent text-accent-contrast'
          : 'border-strong text-text-2 hover:border-text-3 hover:text-text'"
        :data-testid="`activity-filter-${filter.id}`"
        :aria-pressed="activeFilter === filter.id"
        @click="activeFilter = filter.id"
      >
        <span v-if="filter.id === 'auto_action'" class="size-1.5 rounded-full bg-accent" />
        {{ filter.label }}
      </button>
      <div class="flex-1" />
      <label class="flex w-[220px] items-center gap-2 rounded-md border border-strong bg-app px-2.5 py-1.5 focus-within:border-text-3">
        <IconSearch class="size-3.5 shrink-0 text-text-4" />
        <input
          v-model="search"
          type="text"
          placeholder="Filter activity…"
          class="min-w-0 flex-1 bg-transparent text-[12.5px] text-text placeholder:text-text-4 focus:outline-none"
          data-testid="activity-search"
        />
      </label>
    </div>

    <!-- log -->
    <div class="hive-scroll min-h-0 flex-1 overflow-y-auto pb-6" data-testid="activity-log">
      <div v-if="error" class="flex flex-col items-center gap-3 px-6 py-16 text-center font-mono text-xs text-text-4">
        <span data-testid="activity-error">Couldn't load activity — {{ error }}</span>
        <button class="cursor-pointer rounded border border-strong px-3 py-1.5 text-text-2 hover:text-text" @click="load">Retry</button>
      </div>
      <div v-else-if="!events.length && loading" class="px-6 py-16 text-center font-mono text-xs text-text-4">Loading activity…</div>
      <div v-else-if="!groups.length" class="px-6 py-16 text-center font-mono text-xs text-text-4" data-testid="activity-empty">
        {{ events.length ? 'No activity matches this filter.' : 'No activity yet. Refreshes, sessions, and actions will show up here.' }}
      </div>

      <template v-for="group in groups" v-else :key="group.key">
        <div class="px-6 pb-2 pt-5 font-mono text-[10.5px] uppercase tracking-[.12em] text-text-4">{{ group.label }}</div>
        <div
          v-for="event in group.events"
          :key="event.id"
          class="group flex gap-3.5 border-l-2 px-6 py-2.5 transition-colors hover:bg-row-hover"
          :class="styleFor(eventStyleKey(event)).emphasis || 'border-transparent'"
          data-testid="activity-row"
        >
          <span
            class="flex size-7 shrink-0 items-center justify-center rounded-lg"
            :class="styleFor(eventStyleKey(event)).square"
          ><component :is="styleFor(eventStyleKey(event)).icon" class="size-[15px]" /></span>
          <div class="min-w-0 flex-1">
            <div class="text-[13px] leading-snug text-text">{{ event.title }}</div>
            <div v-if="event.body" class="mt-0.5 text-[11.5px] leading-snug text-text-3">{{ event.body }}</div>
          </div>
          <span class="shrink-0 font-mono text-[11.5px] text-text-4">{{ timeLabel(event.createdAt) }}</span>
        </div>
      </template>
    </div>
  </div>
</template>
