<script setup lang="ts">
import { onClickOutside } from '@vueuse/core'
import { ref, watch } from 'vue'
import IconActivity from '~icons/lucide/activity'
import IconArrowLeft from '~icons/lucide/arrow-left'
import IconArrowRight from '~icons/lucide/arrow-right'
import IconPanelLeftClose from '~icons/lucide/panel-left-close'
import IconPanelLeftOpen from '~icons/lucide/panel-left-open'
import IconSearch from '~icons/lucide/search'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import IconLoader from '~icons/lucide/loader'
import IconArrowUpCircle from '~icons/lucide/arrow-up-circle'
import JobsPopover from './JobsPopover.vue'
import type { Job } from '../../bindings/github.com/colonyops/hive/internal/desktop/jobs/models'

// The bar is a three-column grid: a left cluster (sidebar toggle), a center
// cluster (history + command-palette launcher) that stays centered in the
// window regardless of how wide the side clusters grow, and a right cluster
// (error/activity chips). Each column is flex-1 so the center is the middle
// third of the full window width — the Slack-style centered search.
//
// profileName is empty during onboarding: the bar shows no profile controls —
// no toggle, no center/right clusters. errorCount (8d) is the count of the active flow's
// nodes whose last run failed. activityActive highlights the Activity link when
// the audit-log page is open; unseenActivity (6d) is the number of activity
// events recorded since the user last opened that page, shown as a pulsing dot.
// sidebarCollapsed drives the panel-toggle glyph; canToggleSidebar hides the
// toggle in views that have no feed sidebar (settings, flows, onboarding).
// updateAvailable renders a click-to-install chip (with the latestVersion
// label) in the right cluster, mirroring the error/activity chip pattern. It
// is independent of profileName so it can show during onboarding too.
const props = defineProps<{
  profileName?: string
  activityActive?: boolean
  errorCount?: number
  unseenActivity?: number
  jobsActive?: boolean
  activeJobs?: Job[]
  updateAvailable?: boolean
  latestVersion?: string
  canGoBack?: boolean
  canGoForward?: boolean
  sidebarCollapsed?: boolean
  canToggleSidebar?: boolean
}>()
const emit = defineEmits<{
  back: []
  forward: []
  'open-error-node': []
  'open-activity': []
  'open-job-run': [commandId: number]
  'open-update': []
  'toggle-sidebar': []
  'open-palette': []
  'toggle-maximise': []
}>()

// macOS draws its native traffic lights over the top-left of this bar
// (hidden-inset chrome configured in desktop/main.go). Pad the left cluster past
// them and keep the height in sync with InvisibleTitleBarHeight (42) in main.go
// so the controls stay vertically centered.
const isMac = navigator.userAgent.includes('Mac')
const jobsRoot = ref<HTMLElement | null>(null)
const jobsOpen = ref(false)

onClickOutside(jobsRoot, () => { jobsOpen.value = false })
watch(() => props.jobsActive, (active) => { if (!active) jobsOpen.value = false })

// Double-clicking the draggable title bar zooms the window, matching the native
// macOS title-bar gesture (we draw our own chrome, so we implement it). Clicks
// that land on an interactive control are ignored.
function onTitlebarDblclick(event: MouseEvent): void {
  if ((event.target as HTMLElement).closest('button, input, a')) return
  emit('toggle-maximise')
}
</script>

<template>
  <header
    class="relative flex shrink-0 items-center border-b border-border bg-raised"
    :class="isMac ? 'h-[42px]' : 'h-10'"
    style="--wails-draggable: drag"
    @dblclick="onTitlebarDblclick"
  >
    <!-- Left: sidebar toggle -->
    <div class="flex min-w-0 flex-1 items-center gap-2 pr-2" :class="isMac ? 'pl-[84px]' : 'pl-3'">
      <button
        v-if="canToggleSidebar"
        type="button"
        class="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded text-text-3 hover:bg-chip hover:text-text"
        style="--wails-draggable: no-drag"
        :aria-label="sidebarCollapsed ? 'Show sidebar' : 'Hide sidebar'"
        :title="sidebarCollapsed ? 'Show sidebar' : 'Hide sidebar'"
        data-testid="titlebar-toggle-sidebar"
        @click="emit('toggle-sidebar')"
      ><component :is="sidebarCollapsed ? IconPanelLeftOpen : IconPanelLeftClose" class="size-3.5" /></button>
    </div>

    <!-- Center: history controls + command-palette launcher, window-centered -->
    <div v-if="profileName" class="flex min-w-0 flex-1 items-center justify-center gap-1.5 px-2">
      <nav class="flex shrink-0 items-center gap-0.5" aria-label="Page history" style="--wails-draggable: no-drag">
        <button
          type="button"
          class="flex size-6 items-center justify-center rounded text-text-3 enabled:cursor-pointer enabled:hover:bg-chip enabled:hover:text-text disabled:opacity-30"
          :disabled="!canGoBack"
          aria-label="Go back"
          data-testid="titlebar-back"
          @click="emit('back')"
        ><IconArrowLeft class="size-3.5" /></button>
        <button
          type="button"
          class="flex size-6 items-center justify-center rounded text-text-3 enabled:cursor-pointer enabled:hover:bg-chip enabled:hover:text-text disabled:opacity-30"
          :disabled="!canGoForward"
          aria-label="Go forward"
          data-testid="titlebar-forward"
          @click="emit('forward')"
        ><IconArrowRight class="size-3.5" /></button>
      </nav>
      <button
        type="button"
        class="flex h-7 min-w-0 max-w-[460px] flex-1 cursor-pointer items-center gap-2 rounded-md border border-border bg-app px-2.5 text-text-3 hover:border-strong hover:text-text-2"
        style="--wails-draggable: no-drag"
        aria-label="Open command palette"
        data-testid="titlebar-command-palette"
        @click="emit('open-palette')"
      >
        <IconSearch class="size-3.5 shrink-0" />
        <span class="min-w-0 flex-1 truncate text-left text-[12.5px]">Search or run a command…</span>
        <kbd class="shrink-0 rounded border border-card px-1.5 py-0.5 font-mono text-[10.5px] leading-none text-text-3">{{ isMac ? '⌘' : 'Ctrl ' }}K</kbd>
      </button>
    </div>
    <div v-else class="flex-1" />

    <!-- Right: update / error / live jobs / activity chips -->
    <div class="flex min-w-0 flex-1 items-center justify-end gap-2 pl-2 pr-3">
      <button
        v-if="updateAvailable"
        class="flex shrink-0 cursor-pointer items-center gap-1.5 rounded-md border border-severity-info-border bg-severity-info-tint px-2 py-1 text-[11.5px] font-semibold text-severity-info hover:opacity-85"
        style="--wails-draggable: no-drag"
        data-testid="titlebar-update-chip"
        :title="latestVersion ? `Update to ${latestVersion}` : 'Update available'"
        @click="emit('open-update')"
      ><IconArrowUpCircle class="size-3" />Update<template v-if="latestVersion">&nbsp;{{ latestVersion }}</template></button>
      <button
        v-if="errorCount && errorCount > 0"
        class="flex shrink-0 cursor-pointer items-center gap-1.5 rounded-md border border-severity-error-border bg-severity-error-tint px-2 py-1 text-[11.5px] font-semibold text-severity-error hover:opacity-85"
        style="--wails-draggable: no-drag"
        data-testid="titlebar-error-chip"
        @click="emit('open-error-node')"
      ><IconTriangleAlert class="size-3" />{{ errorCount }} error<template v-if="errorCount !== 1">s</template></button>
      <div v-if="profileName && jobsActive" ref="jobsRoot" class="relative shrink-0" style="--wails-draggable: no-drag">
        <button
          type="button"
          class="flex cursor-pointer items-center gap-1.5 rounded-md border border-accent/50 bg-accent/10 px-2 py-1 text-[11.5px] font-medium text-accent hover:border-accent"
          data-testid="titlebar-jobs"
          aria-label="Show action jobs"
          :aria-expanded="jobsOpen"
          @click="jobsOpen = !jobsOpen"
        >
          <IconLoader class="size-3.5 animate-spin" />
          {{ activeJobs?.length ?? 0 }} job<template v-if="(activeJobs?.length ?? 0) !== 1">s</template>
        </button>
        <JobsPopover
          v-if="jobsOpen"
          :jobs="activeJobs ?? []"
          @open-run="(commandId) => { jobsOpen = false; emit('open-job-run', commandId) }"
        />
      </div>
      <!-- A link to the Activity audit log. A pulsing dot flags activity
           recorded since the page was last opened. -->
      <button
        v-if="profileName"
        class="relative flex shrink-0 cursor-pointer items-center gap-1.5 rounded-md border px-2.5 py-1 text-[11.5px] font-medium"
        :class="activityActive
          ? 'border-accent bg-accent text-accent-contrast'
          : 'border-border text-text-2 hover:border-strong hover:text-text'"
        style="--wails-draggable: no-drag"
        data-testid="titlebar-activity"
        aria-label="Open activity"
        @click="emit('open-activity')"
      >
        <IconActivity class="size-3.5" />
        Activity
        <span
          v-if="unseenActivity && unseenActivity > 0 && !activityActive"
          class="size-[7px] rounded-full bg-accent [animation:hivePulse_2.4s_ease-in-out_infinite]"
          data-testid="titlebar-activity-unseen"
        />
      </button>
    </div>
  </header>
</template>
