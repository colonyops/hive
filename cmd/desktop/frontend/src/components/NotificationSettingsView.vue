<script setup lang="ts">
import { computed, onMounted } from 'vue'
import AppSelect from './AppSelect.vue'
import AppSwitch from './AppSwitch.vue'
import BaseBadge from './BaseBadge.vue'
import SettingsError from './settings/SettingsError.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsRow from './settings/SettingsRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import { useNotificationSettings, type NotificationDelivery } from '../composables/useNotificationSettings'

const {
  notificationsEnabled,
  delivery,
  notificationSound,
  permission,
  requestingPermission,
  error,
  refresh,
  setNotificationsEnabled,
  setDelivery,
  setNotificationSound,
  requestPermission,
} = useNotificationSettings()

const permissionLabel = computed(() => ({
  granted: 'Granted',
  denied: 'Denied',
  'not-requested': 'Not requested',
}[permission.value]))

const permissionTone = computed(() => ({
  granted: 'success' as const,
  denied: 'danger' as const,
  'not-requested': 'neutral' as const,
}[permission.value]))

const permissionHint = computed(() => permission.value === 'granted'
  ? 'Hive may show system banners.'
  : 'Without it, a banner falls back to an in-app toast — nothing is lost, it just does not surface outside Hive.')

const deliveryOptions: Array<{ value: NotificationDelivery; label: string }> = [
  { value: 'auto', label: 'Automatic' },
  { value: 'system', label: 'Always a banner' },
  { value: 'app', label: 'Always in Hive' },
]

// Only "Automatic" is non-obvious from its label, so the row explains that one
// rather than restating all three.
const deliveryHint = computed(() => delivery.value === 'auto'
  ? 'A system banner only while you are working in another app; an in-app toast when Hive is focused.'
  : 'Automatic shows a system banner only while you are working in another app.')

onMounted(() => {
  // Permission can change in OS settings while this window is open, so every
  // visit refreshes both persisted preferences and the live authorization.
  void refresh()
})
</script>

<template>
  <!--
    Behavior matrix:
    master off: Activity only, whether Hive is focused or unfocused; delivery is disabled.
    master on + delivery "app": in-app toast only, focused or not.
    master on + delivery "auto" + Hive focused: in-app toast; no OS banner.
    master on + delivery "auto" + Hive unfocused: OS banner.
    master on + delivery "system": OS banner, focused or not.
    Sound applies only to OS banners; a flow's notify node can quiet itself but never unmute.
    OS permission denied/not requested: preferences persist, but a banner falls back to a toast.
  -->
  <SettingsPage testid="notification-settings">
    <SettingsError v-if="error" :message="error" testid="notification-settings-error" />

    <SettingsSection
      title="Notifications"
      description="How Hive tells you about activity that needs attention."
      boxed
    >
      <SettingsRow
        label="Enable notifications"
        hint="With this off, activity is still recorded and readable in Activity — you are just not interrupted for it."
      >
        <AppSwitch
          :model-value="notificationsEnabled"
          aria-label="Enable notifications"
          testid="notification-enable"
          @update:model-value="setNotificationsEnabled"
        />
      </SettingsRow>
      <SettingsRow
        label="Where they appear"
        :hint="deliveryHint"
      >
        <AppSelect
          class="w-[210px]"
          :model-value="delivery"
          :options="deliveryOptions"
          :disabled="!notificationsEnabled"
          aria-label="Where notifications appear"
          testid="notification-delivery"
          @update:model-value="(value) => setDelivery(value as NotificationDelivery)"
        />
      </SettingsRow>
      <SettingsRow
        label="Play sound"
        hint="Sound rides along with an OS banner, so it never plays for an in-app toast."
      >
        <AppSwitch
          :model-value="notificationSound"
          :disabled="!notificationsEnabled"
          aria-label="Play sound"
          testid="notification-sound"
          @update:model-value="setNotificationSound"
        />
      </SettingsRow>
    </SettingsSection>

    <SettingsSection
      title="System permission"
      description="macOS decides whether Hive may show banners at all."
      boxed
    >
      <SettingsRow
        label="OS notification permission"
        :hint="permissionHint"
      >
        <div class="flex items-center gap-2.5">
          <BaseBadge :tone="permissionTone" variant="pill" class="px-2.5 py-1 text-[11px] font-semibold">
            <span data-testid="notification-permission-status">{{ permissionLabel }}</span>
          </BaseBadge>
          <button
            v-if="permission === 'not-requested'"
            type="button"
            class="shrink-0 cursor-pointer rounded-[7px] border border-card px-3 py-1.5 text-[12.5px] font-medium text-text-2 hover:border-strong hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="requestingPermission"
            data-testid="notification-permission-request"
            @click="requestPermission"
          >{{ requestingPermission ? 'Requesting…' : 'Allow' }}</button>
        </div>
      </SettingsRow>
      <div
        v-if="permission === 'denied'"
        class="px-4 py-3.5 text-xs leading-relaxed text-text-3"
        data-testid="notification-permission-denied-guidance"
      >
        Notifications are blocked. Enable them for Hive in your operating system's notification settings — until then, anything that would have been a banner falls back to an in-app toast.
      </div>
    </SettingsSection>
  </SettingsPage>
</template>
