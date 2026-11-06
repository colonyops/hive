<script setup lang="ts">
import { onClickOutside } from '@vueuse/core'
import { computed, ref, watch } from 'vue'
import IconActivity from '~icons/lucide/activity'
import IconArrowLeft from '~icons/lucide/arrow-left'
import IconArrowRight from '~icons/lucide/arrow-right'
import IconPanelLeftClose from '~icons/lucide/panel-left-close'
import IconPanelLeftOpen from '~icons/lucide/panel-left-open'
import IconPanelRightClose from '~icons/lucide/panel-right-close'
import IconPanelRightOpen from '~icons/lucide/panel-right-open'
import IconSearch from '~icons/lucide/search'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import IconArrowUpCircle from '~icons/lucide/arrow-up-circle'
import AppletTitlebarSlot from './applets/AppletTitlebarSlot.vue'
import JobsPopover from './JobsPopover.vue'
import TitleBarMenu from './TitleBarMenu.vue'
import TitleBarTabs from './TitleBarTabs.vue'
import type { AppTabId, GlobalTabId } from '../lib/appTabs'
import type { Job } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/jobs/models'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import Kbd from '@hive/ui/components/ui/Kbd.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import { useApplets } from '../stores/useApplets'

// One button grammar for the whole chrome, with a fixed slot per zone. The bar
// is two clusters:
//   left   — the sidebar (left-panel) toggle brackets the far edge, then
//            back/forward, then the tab strip: Inbox, Code and Chats pinned,
//            the global tabs the user added (Drive, Tasks, Settings, applets),
//            and the + that adds one. The strip scrolls when it runs out of
//            room; the empty space after it stays a drag surface.
//   right  — status chips, then the pages of the applets that list the title
//            bar, then the command-palette launcher, then app-level utilities
//            (Activity, Views) as one icon run, then the preview (right-panel)
//            toggle bracketing the far edge.
// Every control is a 28px square icon button or a 28px labeled button, both
// `rounded-lg` with a `chip` hover fill. Amber is reserved for things asking
// for attention — unread activity, an available update. Selected state is a
// neutral `chip` fill: it is the only treatment that clears contrast in all 14
// themes.
//
// profileName is empty during onboarding: the bar shows no profile controls —
// no toggle, no history, no tabs, no palette.
// errorCount (8d) is the count of the active flow's nodes whose last run
// failed. unseenActivity (6d) is the number of events since Activity was last
// opened, shown as a pulsing amber dot. canToggleSidebar disables the toggle in
// views with no left panel. previewCollapsed/canTogglePreview are the same pair
// for the right-hand pane — the detail preview in Inbox, the comments on the
// Drive page. updateAvailable renders a click-to-install chip independent of
// profileName so it can show during onboarding too.
const props = defineProps<{
  profileName?: string
  mode?: string
  activeTab?: AppTabId | null
  tabs?: readonly GlobalTabId[]
  activityActive?: boolean
  actionRunsActive?: boolean
  errorCount?: number
  unseenActivity?: number
  jobsActive?: boolean
  activeJobs?: readonly Job[]
  updateAvailable?: boolean
  updateInstalling?: boolean
  latestVersion?: string
  canGoBack?: boolean
  canGoForward?: boolean
  sidebarCollapsed?: boolean
  canToggleSidebar?: boolean
  previewCollapsed?: boolean
  canTogglePreview?: boolean
}>()
const emit = defineEmits<{
  'activate-tab': [id: AppTabId]
  'close-tab': [id: GlobalTabId]
  'move-tab': [id: GlobalTabId, before: GlobalTabId | null]
  'manage-applets': []
  back: []
  forward: []
  'open-error-node': []
  'open-activity': []
  'open-job-run': [commandId: number]
  'open-job-log': [commandId: number]
  'run-command': [commandId: string]
  'open-update': []
  'toggle-sidebar': []
  'toggle-preview': []
  'open-palette': []
  'toggle-maximise': []
}>()

// macOS draws its native traffic lights over the top-left of this bar
// (hidden-inset chrome configured in cmd/desktop/main.go). Pad the left cluster past
// them and keep the height in sync with InvisibleTitleBarHeight (43) in
// wailsui/ui.go so the controls stay vertically centered.
//
// Both heights are odd on purpose: the bottom border takes 1px, and an even
// content box under it centers the 28px controls on a whole pixel. An odd one
// centers them on a half pixel, which splits their margins 7/6 on a 1x display.
const isMac = navigator.userAgent.includes('Mac')
const applets = useApplets()

const rightPane = computed(() => (props.mode === 'drive' ? 'comments' : 'preview'))
const jobsRoot = ref<HTMLElement | null>(null)
const jobsOpen = ref(false)

onClickOutside(jobsRoot, () => {
  jobsOpen.value = false
})
watch(
  () => props.jobsActive,
  (active) => {
    if (!active) jobsOpen.value = false
  },
)

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
    :class="isMac ? 'h-[43px]' : 'h-[41px]'"
    style="--wails-draggable: drag"
    data-testid="titlebar"
    @dblclick="onTitlebarDblclick"
  >
    <!-- Left: panel toggle (frame) · history · tabs -->
    <div class="flex min-w-0 flex-1 items-center gap-1 pr-2" :class="isMac ? 'pl-[84px]' : 'pl-3'">
      <IconButton
        v-if="profileName"
        :label="sidebarCollapsed ? 'Show sidebar' : 'Hide sidebar'"
        :icon="sidebarCollapsed ? IconPanelLeftOpen : IconPanelLeftClose"
        size="lg"
        style="--wails-draggable: no-drag"
        :disabled="!canToggleSidebar"
        data-testid="titlebar-toggle-sidebar"
        @click="emit('toggle-sidebar')"
      />
      <nav
        v-if="profileName"
        class="ml-1 flex shrink-0 items-center gap-0.5"
        aria-label="Page history"
        style="--wails-draggable: no-drag"
      >
        <IconButton
          label="Go back"
          :icon="IconArrowLeft"
          size="lg"
          :disabled="!canGoBack"
          data-testid="titlebar-back"
          @click="emit('back')"
        />
        <IconButton
          label="Go forward"
          :icon="IconArrowRight"
          size="lg"
          :disabled="!canGoForward"
          data-testid="titlebar-forward"
          @click="emit('forward')"
        />
      </nav>
      <TitleBarTabs
        v-if="profileName"
        class="ml-1.5"
        :active-tab="activeTab ?? null"
        :tabs="tabs ?? []"
        @activate="emit('activate-tab', $event)"
        @close="emit('close-tab', $event)"
        @move="(id, before) => emit('move-tab', id, before)"
        @manage-applets="emit('manage-applets')"
      />
    </div>

    <!-- Right: status chips · palette launcher · Activity · preview toggle (frame) -->
    <div class="flex shrink-0 items-center justify-end gap-1.5 pl-2 pr-3">
      <button
        v-if="updateAvailable"
        class="flex shrink-0 items-center gap-1.5 rounded-md border border-severity-info-border bg-severity-info-tint px-2 py-1 text-caption font-semibold text-severity-info disabled:cursor-wait enabled:cursor-pointer enabled:hover:opacity-85"
        style="--wails-draggable: no-drag"
        data-testid="titlebar-update-chip"
        :disabled="updateInstalling"
        :title="
          updateInstalling ? 'Installing update…' : latestVersion ? `Update to ${latestVersion}` : 'Update available'
        "
        @click="emit('open-update')"
      >
        <Spinner v-if="updateInstalling" />
        <IconArrowUpCircle v-else class="size-3" />
        <template v-if="updateInstalling">Installing…</template>
        <template v-else
          >Update<template v-if="latestVersion">&nbsp;{{ latestVersion }}</template></template
        >
      </button>
      <button
        v-if="errorCount && errorCount > 0"
        class="flex shrink-0 cursor-pointer items-center gap-1.5 rounded-md border border-severity-error-border bg-severity-error-tint px-2 py-1 text-caption font-semibold text-severity-error hover:opacity-85"
        style="--wails-draggable: no-drag"
        data-testid="titlebar-error-chip"
        @click="emit('open-error-node')"
      >
        <IconTriangleAlert class="size-3" />{{ errorCount }} error<template v-if="errorCount !== 1">s</template>
      </button>
      <div v-if="profileName && jobsActive" ref="jobsRoot" class="relative shrink-0" style="--wails-draggable: no-drag">
        <button
          type="button"
          class="flex cursor-pointer items-center gap-1.5 rounded-md border border-accent/50 bg-accent/10 px-2 py-1 text-caption font-medium text-accent hover:border-accent"
          data-testid="titlebar-jobs"
          aria-label="Show action jobs"
          :aria-expanded="jobsOpen"
          @click="jobsOpen = !jobsOpen"
        >
          <Spinner />
          {{ activeJobs?.length ?? 0 }} job<template v-if="(activeJobs?.length ?? 0) !== 1">s</template>
        </button>
        <JobsPopover
          v-if="jobsOpen"
          :jobs="activeJobs ?? []"
          @open-run="
            (commandId) => {
              jobsOpen = false
              emit('open-job-run', commandId)
            }
          "
          @open-log="
            (commandId) => {
              jobsOpen = false
              emit('open-job-log', commandId)
            }
          "
        />
      </div>
      <template v-if="profileName">
        <AppletTitlebarSlot v-for="applet in applets.titlebarApplets.value" :key="applet.id" :applet="applet" />
      </template>
      <button
        v-if="profileName"
        type="button"
        class="flex h-7 w-[clamp(7.5rem,16vw,14rem)] cursor-pointer items-center gap-2 rounded-md border border-border bg-app px-2.5 text-text-3 hover:border-strong hover:text-text-2"
        style="--wails-draggable: no-drag"
        aria-label="Open command palette"
        data-testid="titlebar-command-palette"
        @click="emit('open-palette')"
      >
        <IconSearch class="size-3.5 shrink-0" />
        <span class="min-w-0 flex-1 truncate text-left text-small">Search…</span>
        <Kbd variant="boxed" class="hidden shrink-0 min-[700px]:block">{{ isMac ? '⌘' : 'Ctrl ' }}K</Kbd>
      </button>
      <!-- Activity: a 28px icon between the palette and the frame. An amber dot
           flags activity recorded since the page was last opened; amber fill
           marks it on — which is why it carries its own ml: as a filled block
           it needs the same 10px on both sides that separates groups in the
           left cluster, not the cluster's 6px on one side and 10px on the
           other. -->
      <IconButton
        v-if="profileName"
        label="Open activity"
        tooltip="Activity"
        :icon="IconActivity"
        size="lg"
        :active="activityActive"
        class="relative ml-1"
        style="--wails-draggable: no-drag"
        data-testid="titlebar-activity"
        @click="emit('open-activity')"
      >
        <span
          v-if="unseenActivity && unseenActivity > 0 && !activityActive"
          class="absolute right-1 top-1 size-1.5 rounded-full bg-accent ring-2 ring-raised [animation:hivePulse_2.4s_ease-in-out_infinite]"
          data-testid="titlebar-activity-unseen"
        />
      </IconButton>
      <TitleBarMenu
        v-if="profileName"
        :active="actionRunsActive"
        @run-command="(commandId) => emit('run-command', commandId)"
      />
      <IconButton
        v-if="profileName"
        :label="`${previewCollapsed ? 'Show' : 'Hide'} ${rightPane}`"
        :icon="previewCollapsed ? IconPanelRightOpen : IconPanelRightClose"
        size="lg"
        class="ml-1"
        style="--wails-draggable: no-drag"
        :disabled="!canTogglePreview"
        data-testid="titlebar-toggle-preview"
        @click="emit('toggle-preview')"
      />
    </div>
  </header>
</template>
