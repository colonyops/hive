<script setup lang="ts">
import { onClickOutside } from '@vueuse/core'
import { ref, watch } from 'vue'
import IconActivity from '~icons/lucide/activity'
import IconArrowLeft from '~icons/lucide/arrow-left'
import IconArrowRight from '~icons/lucide/arrow-right'
import IconInbox from '~icons/lucide/inbox'
import IconCode from '~icons/lucide/code'
import IconMessagesSquare from '~icons/lucide/messages-square'
import IconPanelLeftClose from '~icons/lucide/panel-left-close'
import IconPanelLeftOpen from '~icons/lucide/panel-left-open'
import IconPanelRightClose from '~icons/lucide/panel-right-close'
import IconPanelRightOpen from '~icons/lucide/panel-right-open'
import IconSearch from '~icons/lucide/search'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import IconArrowUpCircle from '~icons/lucide/arrow-up-circle'
import JobsPopover from './JobsPopover.vue'
import type { Job } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/jobs/models'
import Kbd from './ui/Kbd.vue'
import Spinner from './ui/Spinner.vue'
import SegmentedControl, { type SegmentedControlOption } from './ui/SegmentedControl.vue'

// One button grammar for the whole chrome, with a fixed slot per zone. The bar
// is two clusters, not three centered columns — a window-centered palette sat
// on top of the mode switch once three labeled modes existed:
//   left   — structure then history: the sidebar (left-panel) toggle brackets
//            the far edge, then feed-scoped back/forward, then the mode
//            switch. The cluster owns all of the bar's slack, so the empty
//            middle stays a drag surface. Below the labels breakpoint
//            (min-[860px]) the mode segments drop their labels to icons.
//   right  — status chips, then the command-palette launcher (compact, width
//            clamped to the window so it gives way before the tabs do), then
//            app-level utilities (Activity) as one icon run, then the
//            preview (right-panel) toggle bracketing the far
//            edge. Panel toggles are the only things at the extremes, so they
//            read as the frame; new utilities append to the icon run.
// Every control is one of two shapes: a 28px square icon button or a 28px
// labeled/segmented button, both 7px-radius with a `chip` hover fill. Groups
// are separated by margin, not rules — at four controls a hairline costs as
// much attention as the control beside it.
// Amber is reserved for things asking for attention — unread activity, an
// available update. Selected state is a neutral `chip` fill, so the mode
// switch (rarely used) cannot outshout them. It is also the only treatment
// that clears contrast in all 14 themes: accent-on-accent-tint falls to
// 1.8:1 in the light theme.
//
// profileName is empty during onboarding: the bar shows no profile controls —
// no toggle, no history, no palette.
// mode is the app-level Inbox|Code|Chats switch. A segment is never disabled,
// because an unavailable terminal or Chats area explains itself inside the
// mode.
// errorCount (8d) is the count of the active flow's nodes whose last run
// failed. activityActive marks the Activity icon on when the audit-log page is
// open; unseenActivity (6d) is the number of events since it was last opened,
// shown as a pulsing amber dot. sidebarCollapsed drives the panel-toggle glyph;
// canToggleSidebar disables the toggle in views with no left panel (settings,
// flows, onboarding). previewCollapsed/canTogglePreview are the same pair for
// the right-hand pane — the detail preview in Inbox, the canvas in Chats.
// previewUnseen is that pane's attention dot: in Chats an agent wrote to a
// canvas that is not on screen. updateAvailable renders a click-to-install chip in
// the right cluster, independent of profileName so it can show during
// onboarding too.
const props = defineProps<{
  profileName?: string
  mode?: 'hub' | 'terminal' | 'agents'
  activityActive?: boolean
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
  previewUnseen?: boolean
}>()
const emit = defineEmits<{
  'set-mode': [mode: 'hub' | 'terminal' | 'agents']
  back: []
  forward: []
  'open-error-node': []
  'open-activity': []
  'open-job-run': [commandId: number]
  'open-update': []
  'toggle-sidebar': []
  'toggle-preview': []
  'open-palette': []
  'toggle-maximise': []
}>()

// macOS draws its native traffic lights over the top-left of this bar
// (hidden-inset chrome configured in cmd/desktop/main.go). Pad the left cluster past
// them and keep the height in sync with InvisibleTitleBarHeight (42) in main.go
// so the controls stay vertically centered.
const isMac = navigator.userAgent.includes('Mac')

const modeOptions: SegmentedControlOption<'hub' | 'terminal' | 'agents'>[] = [
  { value: 'hub', label: 'Inbox', title: 'Inbox' },
  { value: 'terminal', label: 'Code', title: 'Code' },
  { value: 'agents', label: 'Chats', title: 'Chats' },
]
const modeIcons = { hub: IconInbox, terminal: IconCode, agents: IconMessagesSquare }
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
    :class="isMac ? 'h-[42px]' : 'h-10'"
    style="--wails-draggable: drag"
    @dblclick="onTitlebarDblclick"
  >
    <!-- Left: panel toggle (frame) · feed history · mode switch -->
    <div class="flex min-w-0 flex-1 items-center gap-1 pr-2" :class="isMac ? 'pl-[84px]' : 'pl-3'">
      <button
        v-if="profileName"
        type="button"
        class="flex size-7 shrink-0 items-center justify-center rounded-[7px] text-text-3 enabled:cursor-pointer enabled:hover:bg-chip enabled:hover:text-text disabled:cursor-default disabled:opacity-30"
        style="--wails-draggable: no-drag"
        :disabled="!canToggleSidebar"
        :aria-label="sidebarCollapsed ? 'Show sidebar' : 'Hide sidebar'"
        :title="sidebarCollapsed ? 'Show sidebar' : 'Hide sidebar'"
        data-testid="titlebar-toggle-sidebar"
        @click="emit('toggle-sidebar')"
      >
        <component :is="sidebarCollapsed ? IconPanelLeftOpen : IconPanelLeftClose" class="size-3.5" />
      </button>
      <nav
        v-if="profileName"
        class="ml-1 flex shrink-0 items-center gap-0.5"
        aria-label="Page history"
        style="--wails-draggable: no-drag"
      >
        <button
          type="button"
          class="flex size-7 items-center justify-center rounded-[7px] text-text-3 enabled:cursor-pointer enabled:hover:bg-chip enabled:hover:text-text disabled:opacity-30"
          :disabled="!canGoBack"
          aria-label="Go back"
          data-testid="titlebar-back"
          @click="emit('back')"
        >
          <IconArrowLeft class="size-3.5" />
        </button>
        <button
          type="button"
          class="flex size-7 items-center justify-center rounded-[7px] text-text-3 enabled:cursor-pointer enabled:hover:bg-chip enabled:hover:text-text disabled:opacity-30"
          :disabled="!canGoForward"
          aria-label="Go forward"
          data-testid="titlebar-forward"
          @click="emit('forward')"
        >
          <IconArrowRight class="size-3.5" />
        </button>
      </nav>
      <!-- Segments size to their labels. An equal-width split padded the
           shorter label out to match the longer, which read as a gap. Below
           the labels breakpoint the spans hide and the icons carry the
           segment (title keeps the name). -->
      <SegmentedControl
        v-if="profileName"
        variant="compact"
        size="sm"
        class="ml-1.5 shrink-0"
        style="--wails-draggable: no-drag"
        :model-value="mode"
        :options="modeOptions"
        aria-label="App mode"
        testid="titlebar-mode"
        @update:model-value="emit('set-mode', $event)"
      >
        <template #option="{ option }">
          <component :is="modeIcons[option.value]" class="size-3.5 shrink-0" /><span
            class="hidden min-[860px]:inline"
            >{{ option.label }}</span
          >
        </template>
      </SegmentedControl>
    </div>

    <!-- Right: status chips · palette launcher · Activity · preview toggle (frame) -->
    <div class="flex shrink-0 items-center justify-end gap-1.5 pl-2 pr-3">
      <button
        v-if="updateAvailable"
        class="flex shrink-0 items-center gap-1.5 rounded-md border border-severity-info-border bg-severity-info-tint px-2 py-1 text-[11.5px] font-semibold text-severity-info disabled:cursor-wait enabled:cursor-pointer enabled:hover:opacity-85"
        style="--wails-draggable: no-drag"
        data-testid="titlebar-update-chip"
        :disabled="updateInstalling"
        :title="
          updateInstalling ? 'Installing update…' : latestVersion ? `Update to ${latestVersion}` : 'Update available'
        "
        @click="emit('open-update')"
      >
        <Spinner v-if="updateInstalling" size="sm" />
        <IconArrowUpCircle v-else class="size-3" />
        <template v-if="updateInstalling">Installing…</template>
        <template v-else
          >Update<template v-if="latestVersion">&nbsp;{{ latestVersion }}</template></template
        >
      </button>
      <button
        v-if="errorCount && errorCount > 0"
        class="flex shrink-0 cursor-pointer items-center gap-1.5 rounded-md border border-severity-error-border bg-severity-error-tint px-2 py-1 text-[11.5px] font-semibold text-severity-error hover:opacity-85"
        style="--wails-draggable: no-drag"
        data-testid="titlebar-error-chip"
        @click="emit('open-error-node')"
      >
        <IconTriangleAlert class="size-3" />{{ errorCount }} error<template v-if="errorCount !== 1">s</template>
      </button>
      <div v-if="profileName && jobsActive" ref="jobsRoot" class="relative shrink-0" style="--wails-draggable: no-drag">
        <button
          type="button"
          class="flex cursor-pointer items-center gap-1.5 rounded-md border border-accent/50 bg-accent/10 px-2 py-1 text-[11.5px] font-medium text-accent hover:border-accent"
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
        />
      </div>
      <button
        v-if="profileName"
        type="button"
        class="flex h-7 w-[clamp(150px,22vw,260px)] cursor-pointer items-center gap-2 rounded-md border border-border bg-app px-2.5 text-text-3 hover:border-strong hover:text-text-2"
        style="--wails-draggable: no-drag"
        aria-label="Open command palette"
        data-testid="titlebar-command-palette"
        @click="emit('open-palette')"
      >
        <IconSearch class="size-3.5 shrink-0" />
        <span class="min-w-0 flex-1 truncate text-left text-[12.5px]">Search…</span>
        <Kbd class="hidden shrink-0 min-[700px]:block">{{ isMac ? '⌘' : 'Ctrl ' }}K</Kbd>
      </button>
      <!-- Activity: a 28px icon between the palette and the frame. An amber dot
           flags activity recorded since the page was last opened; amber fill
           marks it on — which is why it carries its own ml: as a filled block
           it needs the same 10px on both sides that separates groups in the
           left cluster, not the cluster's 6px on one side and 10px on the
           other. -->
      <button
        v-if="profileName"
        type="button"
        class="relative ml-1 flex size-7 shrink-0 cursor-pointer items-center justify-center rounded-[7px]"
        :class="activityActive ? 'bg-accent-tint text-accent' : 'text-text-3 hover:bg-chip hover:text-text'"
        style="--wails-draggable: no-drag"
        data-testid="titlebar-activity"
        aria-label="Open activity"
        title="Activity"
        @click="emit('open-activity')"
      >
        <IconActivity class="size-3.5" />
        <span
          v-if="unseenActivity && unseenActivity > 0 && !activityActive"
          class="absolute right-1 top-1 size-[6px] rounded-full bg-accent ring-2 ring-raised [animation:hivePulse_2.4s_ease-in-out_infinite]"
          data-testid="titlebar-activity-unseen"
        />
      </button>
      <button
        v-if="profileName"
        type="button"
        class="relative ml-1 flex size-7 shrink-0 items-center justify-center rounded-[7px] text-text-3 enabled:cursor-pointer enabled:hover:bg-chip enabled:hover:text-text disabled:cursor-default disabled:opacity-30"
        style="--wails-draggable: no-drag"
        :disabled="!canTogglePreview"
        :aria-label="previewCollapsed ? 'Show preview' : 'Hide preview'"
        :title="previewCollapsed ? 'Show preview' : 'Hide preview'"
        data-testid="titlebar-toggle-preview"
        @click="emit('toggle-preview')"
      >
        <component :is="previewCollapsed ? IconPanelRightOpen : IconPanelRightClose" class="size-3.5" />
        <span
          v-if="previewUnseen && previewCollapsed"
          class="absolute right-1 top-1 size-[6px] rounded-full bg-accent ring-2 ring-raised"
          data-testid="titlebar-preview-unseen"
        />
      </button>
    </div>
  </header>
</template>
