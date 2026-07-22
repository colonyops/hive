<script setup lang="ts">
import { onUnmounted, ref } from 'vue'
import IconBell from '~icons/lucide/bell'
import { Notify as NotifyNative } from '../../bindings/github.com/colonyops/hive/desktop/notificationservice'
import { useNotificationSettings } from '../composables/useNotificationSettings'
import { notifySeverityMapping, useNotify, type NotifySeverity } from '../composables/useNotify'
import { useToasts } from '../composables/useToasts'
import BaseCard from './BaseCard.vue'
import SettingsSection from './settings/SettingsSection.vue'
import ViewHeader from './settings/ViewHeader.vue'

const emit = defineEmits<{ close: [] }>()

type NotificationTestChannel = 'auto' | 'force-toast' | 'force-system'

const severity = ref<NotifySeverity>('info')
const channel = ref<NotificationTestChannel>('auto')
const delay = ref(false)
let pendingTimeout: ReturnType<typeof setTimeout> | undefined

const { notify } = useNotify()
const { showToast } = useToasts()
const { notificationSound } = useNotificationSettings()

const testTitle = 'Test notification'
const testSubtitle = 'Hive Dev tools'

function testBody(selectedChannel: NotificationTestChannel): string {
  switch (selectedChannel) {
    case 'auto':
      return 'Dev tools auto test: uses focus and notification settings.'
    case 'force-toast':
      return 'Dev tools forced toast test: bypasses focus and Activity.'
    case 'force-system':
      return 'Dev tools forced system test: bypasses focus and Activity.'
  }
}

function reportFailure(target: string, error: unknown): void {
  console.warn(`[dev-view] ${target} notification test failed`, error)
}

function dispatchTest(selectedChannel = channel.value, selectedSeverity = severity.value): void {
  pendingTimeout = undefined
  const body = testBody(selectedChannel)

  if (selectedChannel === 'auto') {
    void notify({ title: testTitle, body, severity: selectedSeverity, category: 'system', source: 'dev-view' })
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
    severity: selectedSeverity,
    sound: notificationSound.value,
    data: { source: 'dev-view', channel: 'force-system', severity: selectedSeverity },
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
  <div class="flex h-full min-h-0 flex-1 flex-col" data-testid="dev-view">
    <ViewHeader close-testid="dev-close" @close="emit('close')">
      <template #title>
        <span class="text-[13px] font-semibold text-text">Developer tools</span>
        <span class="font-mono text-[11px] text-text-4">internal</span>
      </template>
    </ViewHeader>

    <div class="hive-scroll min-h-0 flex-1 overflow-y-auto px-6 py-6">
      <div class="mx-auto max-w-[640px]">
        <SettingsSection
          title="Notifications"
          description="Exercise notification delivery while developing Hive."
        >
          <BaseCard class="mt-4 rounded-lg border border-border bg-raised" data-testid="dev-notification-test">
            <template #icon>
              <span class="flex size-9 items-center justify-center rounded-lg bg-severity-info-tint text-severity-info"><IconBell class="size-4" /></span>
            </template>
            <div class="min-w-0 flex-1">
              <div class="text-[13.5px] font-semibold text-text">Test notification</div>
              <div class="mt-0.5 text-xs text-text-3">Choose a delivery path and severity.</div>
            </div>
            <template #actions>
              <div class="flex shrink-0 items-center gap-2">
                <select
                  v-model="severity"
                  data-testid="dev-notification-severity"
                  aria-label="Notification severity"
                  class="cursor-pointer rounded-md border border-strong bg-app px-2 py-1.5 text-[11px] font-medium text-text"
                >
                  <option value="info">info</option>
                  <option value="success">success</option>
                  <option value="warning">warning</option>
                  <option value="error">error</option>
                </select>
                <select
                  v-model="channel"
                  data-testid="dev-notification-channel"
                  aria-label="Notification channel"
                  class="cursor-pointer rounded-md border border-strong bg-app px-2 py-1.5 text-[11px] font-medium text-text"
                >
                  <option value="auto">auto</option>
                  <option value="force-toast">force-toast</option>
                  <option value="force-system">force-system</option>
                </select>
                <label class="flex cursor-pointer items-center gap-1.5 text-[11px] font-medium text-text-2">
                  <input v-model="delay" data-testid="dev-notification-delay" type="checkbox" class="size-3.5 accent-accent" />
                  delay 3s
                </label>
                <button
                  data-testid="dev-notification-send"
                  type="button"
                  class="cursor-pointer rounded-md bg-accent px-3 py-1.5 text-[11px] font-semibold text-accent-contrast hover:bg-accent-hover"
                  @click="sendTest"
                >
                  Test notification
                </button>
              </div>
            </template>
          </BaseCard>
        </SettingsSection>
      </div>
    </div>
  </div>
</template>
