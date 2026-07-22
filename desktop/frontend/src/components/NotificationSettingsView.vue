<script setup lang="ts">
import { computed, onMounted } from 'vue'
import AppSwitch from './AppSwitch.vue'
import SettingsSection from './settings/SettingsSection.vue'
import { useNotificationSettings } from '../composables/useNotificationSettings'

const {
  notificationsEnabled,
  systemNotificationsEnabled,
  notificationSound,
  permission,
  requestingPermission,
  error,
  refresh,
  setNotificationsEnabled,
  setSystemNotificationsEnabled,
  setNotificationSound,
  requestPermission,
} = useNotificationSettings()

const permissionLabel = computed(() => ({
  granted: 'Granted',
  denied: 'Denied',
  'not-requested': 'Not requested',
}[permission.value]))

onMounted(() => {
  // Permission can change in OS settings while this window is open, so every
  // visit refreshes both persisted preferences and the live authorization.
  void refresh()
})
</script>

<template>
  <!--
    Behavior matrix:
    master off: Activity only, whether Hive is focused or unfocused; the system control is disabled.
    master on + Hive focused: Hive notification only; no OS banner.
    master on + Hive unfocused + system off: Activity only.
    master on + Hive unfocused + system on: OS banners are eligible; sound applies only to those banners.
    OS permission denied/not requested: preferences persist, but OS banners require permission.
  -->
  <div class="mx-auto max-w-[640px]" data-testid="notification-settings">
    <div
      v-if="error"
      class="mb-4 rounded-md border border-border bg-severity-error-tint px-3 py-2 text-xs text-severity-error"
      data-testid="notification-settings-error"
    >{{ error }}</div>

    <SettingsSection
      title="Notifications"
      description="Choose how Hive keeps you informed about new activity."
      class="[&>p]:mb-3"
    >
      <div class="rounded-lg border border-border">
        <div class="px-3.5 py-3">
          <AppSwitch
            :model-value="notificationsEnabled"
            label="Enable notifications"
            hint="Show Hive notifications for activity that needs your attention."
            testid="notification-enable"
            @update:model-value="setNotificationsEnabled"
          />
        </div>
        <div class="border-t border-border px-3.5 py-3">
          <AppSwitch
            :model-value="systemNotificationsEnabled"
            :disabled="!notificationsEnabled"
            label="System notifications when Hive isn't focused"
            hint="Show an OS banner while you are working in another app."
            testid="notification-system"
            @update:model-value="setSystemNotificationsEnabled"
          />
        </div>
        <div class="border-t border-border px-3.5 py-3">
          <AppSwitch
            :model-value="notificationSound"
            label="Play sound"
            hint="Play sound for OS banners only."
            testid="notification-sound"
            @update:model-value="setNotificationSound"
          />
        </div>
      </div>
    </SettingsSection>

    <SettingsSection
      title="System permission"
      description="Hive needs operating-system permission before it can show notification banners."
      class="mt-6 [&>p]:mb-3"
    >
      <div class="rounded-lg border border-border px-3.5 py-3">
        <div class="flex items-center justify-between gap-3">
          <span class="text-[12.5px] text-text-3">OS notification permission</span>
          <span class="text-[12.5px] font-medium text-text" data-testid="notification-permission-status">{{ permissionLabel }}</span>
        </div>
        <div v-if="permission === 'not-requested'" class="mt-3">
          <button
            type="button"
            class="cursor-pointer rounded-md border border-border px-2.5 py-1.5 text-[12px] font-medium text-text-2 hover:bg-chip hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="requestingPermission"
            data-testid="notification-permission-request"
            @click="requestPermission"
          >{{ requestingPermission ? 'Requesting…' : 'Allow notifications' }}</button>
        </div>
        <p v-else-if="permission === 'denied'" class="mt-3 text-xs leading-relaxed text-text-3" data-testid="notification-permission-denied-guidance">
          Notifications are blocked. Enable them for Hive in your operating system's notification settings.
        </p>
      </div>
    </SettingsSection>
  </div>
</template>
