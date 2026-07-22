<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue'
import { Notify as NotifyNative } from '../../bindings/github.com/colonyops/hive/desktop/notificationservice'
import { useNotificationSettings } from '../composables/useNotificationSettings'
import { notifySeverityMapping, useNotify, type NotifySeverity } from '../composables/useNotify'
import { useToasts } from '../composables/useToasts'
import { colorForBranch, readableTextColor } from '../lib/devBarColor'

// Dev-only status strip pinned to the bottom of the window. Rendered only when
// Vite serves the app in dev mode (import.meta.env.DEV) — i.e. under
// `wails3 dev` — so it can never appear in a production `wails3 build`, the
// headless server build, or e2e. It exists to make concurrently-running dev
// instances distinguishable at a glance: the branch is injected at launch by
// the dev task, and the port is the Vite dev server this window loaded from.
//
// The bar's color is derived from the branch name (see devBarColor.ts), so each
// branch/worktree window gets a stable, distinct accent — no more guessing
// which window is which.
const branch = import.meta.env.VITE_HIVE_DEV_BRANCH || 'unknown'
const port = window.location.port

const barColor = computed(() => colorForBranch(branch))
const textColor = computed(() => readableTextColor(barColor.value))

type NotificationTestChannel = 'auto' | 'force-toast' | 'force-system'

const severity = ref<NotifySeverity>('info')
const channel = ref<NotificationTestChannel>('auto')
const delay = ref(false)
let pendingTimeout: ReturnType<typeof setTimeout> | undefined

const { notify } = useNotify()
const { showToast } = useToasts()
const { notificationSound } = useNotificationSettings()

const testTitle = 'Test notification'
const testSubtitle = 'Hive DevBar'

function testBody(selectedChannel: NotificationTestChannel): string {
  switch (selectedChannel) {
    case 'auto':
      return 'DevBar auto test: uses focus and notification settings.'
    case 'force-toast':
      return 'DevBar forced toast test: bypasses focus and Activity.'
    case 'force-system':
      return 'DevBar forced system test: bypasses focus and Activity.'
  }
}

function reportFailure(target: string, error: unknown): void {
  console.warn(`[devbar] ${target} notification test failed`, error)
}

function dispatchTest(selectedChannel = channel.value, selectedSeverity = severity.value): void {
  pendingTimeout = undefined
  const body = testBody(selectedChannel)

  if (selectedChannel === 'auto') {
    void notify({ title: testTitle, body, severity: selectedSeverity, category: 'system', source: 'devbar' })
      .catch((error: unknown) => reportFailure('auto', error))
    return
  }

  if (selectedChannel === 'force-toast') {
    showToast(testTitle, { body, severity: notifySeverityMapping[selectedSeverity].toast })
    return
  }

  void NotifyNative({
    title: testTitle,
    subtitle: testSubtitle,
    body,
    sound: notificationSound.value,
    data: { source: 'devbar', channel: 'force-system', severity: selectedSeverity },
  }).catch((error: unknown) => reportFailure('system', error))
}

function sendTest(): void {
  if (pendingTimeout !== undefined) {
    clearTimeout(pendingTimeout)
    pendingTimeout = undefined
  }

  const selectedChannel = channel.value
  const selectedSeverity = severity.value
  if (delay.value) {
    pendingTimeout = setTimeout(() => dispatchTest(selectedChannel, selectedSeverity), 3000)
    return
  }

  dispatchTest(selectedChannel, selectedSeverity)
}

onUnmounted(() => {
  if (pendingTimeout !== undefined) clearTimeout(pendingTimeout)
})
</script>

<template>
  <footer
    class="flex shrink-0 select-none items-center gap-2.5 border-t border-black/20 px-3 py-1.5 font-mono text-xs leading-none"
    :style="{ background: barColor, color: textColor }"
  >
    <span class="rounded px-2 py-1 text-[11px] font-bold uppercase tracking-wider" :style="{ background: textColor, color: barColor }">Dev</span>
    <span class="font-semibold">{{ branch }}</span>
    <span v-if="port" class="opacity-60">:{{ port }}</span>
    <span class="ml-auto opacity-70">dev instance</span>
    <div data-testid="devbar-notification-test" class="flex items-center gap-1.5">
      <select
        v-model="severity"
        data-testid="devbar-notification-severity"
        aria-label="Notification severity"
        class="cursor-pointer rounded border border-current/35 bg-black/10 px-1 py-0.5 text-[10px] font-semibold"
      >
        <option value="info">info</option>
        <option value="success">success</option>
        <option value="warning">warning</option>
        <option value="error">error</option>
      </select>
      <select
        v-model="channel"
        data-testid="devbar-notification-channel"
        aria-label="Notification channel"
        class="cursor-pointer rounded border border-current/35 bg-black/10 px-1 py-0.5 text-[10px] font-semibold"
      >
        <option value="auto">auto</option>
        <option value="force-toast">force-toast</option>
        <option value="force-system">force-system</option>
      </select>
      <label class="flex cursor-pointer items-center gap-1 text-[10px] font-semibold">
        <input v-model="delay" data-testid="devbar-notification-delay" type="checkbox" class="size-3 accent-current" />
        delay 3s
      </label>
      <button
        data-testid="devbar-notification-send"
        type="button"
        class="cursor-pointer rounded border border-current/45 bg-black/15 px-1.5 py-0.5 text-[10px] font-bold hover:bg-black/25"
        @click="sendTest"
      >
        Test notification
      </button>
    </div>
  </footer>
</template>
