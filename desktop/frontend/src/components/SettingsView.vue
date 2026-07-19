<script setup lang="ts">
// The settings page (hc-pppw2iww): replaces the old sidebar "Flows" pill,
// which was redundant with the footer "Edit flow" entry and the per-feed
// reveal-in-flow icon. Rendered full-screen in App.vue's main region — the
// same slot FlowsView occupies when session.flowsOpen is true — gated by a
// plain app-level `settingsOpen` ref (NOT folded into useFlowsSession:
// settings isn't a flow/profile concern, it's an app-chrome one).
//
// A left category rail + a scrollable form on the right, covering the full
// control vocabulary the design spec calls for (text, select, file, image,
// masked secret, stepper, switch, segmented). Everything is local reactive
// state — a UI shell, nothing persists to a backend — EXCEPT the Appearance
// theme control, which drives the real useTheme() composable (the one
// control here with a genuine effect), so switching it changes the app's
// actual theme immediately.
import { onMounted, onUnmounted, ref } from 'vue'
import IconArrowLeft from '~icons/lucide/arrow-left'
import IconPalette from '~icons/lucide/palette'
import IconPlug from '~icons/lucide/plug'
import IconSlidersHorizontal from '~icons/lucide/sliders-horizontal'
import IconWrench from '~icons/lucide/wrench'
import SettingsFileField from './settings/SettingsFileField.vue'
import SettingsImageField from './settings/SettingsImageField.vue'
import SettingsSecretField from './settings/SettingsSecretField.vue'
import SettingsSegmented from './settings/SettingsSegmented.vue'
import SettingsSelectField from './settings/SettingsSelectField.vue'
import SettingsStepper from './settings/SettingsStepper.vue'
import SettingsSwitch from './settings/SettingsSwitch.vue'
import SettingsTextField from './settings/SettingsTextField.vue'
import { setTheme, themeLabels, themes, useTheme, type Theme } from '../composables/useTheme'

const emit = defineEmits<{ close: [] }>()

type CategoryId = 'general' | 'appearance' | 'integrations' | 'advanced'

const categories: { id: CategoryId; label: string; icon: unknown }[] = [
  { id: 'general', label: 'General', icon: IconSlidersHorizontal },
  { id: 'appearance', label: 'Appearance', icon: IconPalette },
  { id: 'integrations', label: 'Integrations', icon: IconPlug },
  { id: 'advanced', label: 'Advanced', icon: IconWrench },
]

const activeCategory = ref<CategoryId>('general')

// ── General ──────────────────────────────────────────────────────────────
const displayName = ref('Hayden')
const defaultView = ref('all')
const autostart = ref(false)
const confirmDelete = ref(true)

// ── Appearance ───────────────────────────────────────────────────────────
// The one wired-to-a-real-composable control: reading/driving the shared
// useTheme() ref means this segmented control both reflects the app's
// current theme and changes it for real when clicked.
const { theme } = useTheme()
const themeOptions = themes.map((t) => ({ value: t, label: themeLabels[t] }))
function onThemeChange(value: string) {
  setTheme(value as Theme)
}
const fontSize = ref('medium')
const compactDensity = ref(false)
const accentImage = ref<File | null>(null)

// ── Integrations ─────────────────────────────────────────────────────────
const githubToken = ref('')
const webhookUrl = ref('')
const pollInterval = ref(60)
const desktopNotifications = ref(true)

// ── Advanced ─────────────────────────────────────────────────────────────
const logLevel = ref('info')
const logRetention = ref(14)
const importFile = ref<File | null>(null)
const experimentalFlows = ref(false)

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}
onMounted(() => window.addEventListener('keydown', onKeydown))
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div class="flex h-full min-h-0 flex-1" data-testid="settings-view">
    <aside class="hive-scroll w-[200px] shrink-0 overflow-y-auto border-r border-row bg-sidebar">
      <div class="border-b border-border px-4 pb-3 pt-4">
        <div class="text-[15px] font-semibold tracking-[-.01em] text-text">Settings</div>
      </div>
      <nav class="flex flex-col gap-0.5 px-2.5 py-3">
        <button
          v-for="cat in categories"
          :key="cat.id"
          type="button"
          class="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-2 text-left text-[13px]"
          :class="activeCategory === cat.id ? 'bg-hover font-medium text-accent' : 'text-text-2 hover:bg-chip hover:text-text'"
          :aria-current="activeCategory === cat.id ? 'true' : undefined"
          :data-testid="`settings-category-${cat.id}`"
          @click="activeCategory = cat.id"
        >
          <component :is="cat.icon" class="size-3.5 shrink-0" />
          {{ cat.label }}
        </button>
      </nav>
    </aside>

    <section class="flex min-w-0 flex-1 flex-col">
      <header class="flex h-11 shrink-0 items-center gap-2.5 border-b border-row bg-canvas-toolbar px-4">
        <span class="text-[13px] font-semibold text-text">{{ categories.find((c) => c.id === activeCategory)?.label }}</span>
        <div class="flex-1" />
        <button
          type="button"
          class="flex cursor-pointer items-center gap-1.5 rounded-md px-2.5 py-1.5 text-[12.5px] text-text-2 hover:bg-chip hover:text-text"
          data-testid="settings-close"
          @click="emit('close')"
        ><IconArrowLeft class="size-3.5" />Back to feed</button>
      </header>

      <div class="hive-scroll min-h-0 flex-1 overflow-y-auto px-6 py-6">
        <div class="mx-auto flex max-w-[560px] flex-col gap-5">
          <template v-if="activeCategory === 'general'">
            <SettingsTextField
              v-model="displayName"
              label="Display name"
              placeholder="Hayden"
              hint="Shown in the titlebar and command palette."
              testid="settings-display-name"
            />
            <SettingsSelectField
              v-model="defaultView"
              label="Default view on open"
              :options="[{ value: 'all', label: 'All items' }, { value: 'unread', label: 'Unread only' }]"
              hint="Which sidebar view loads when you switch profiles."
              testid="settings-default-view"
            />
            <SettingsSwitch
              v-model="autostart"
              label="Launch at login"
              hint="Start hive automatically when you sign in."
              testid="settings-autostart"
            />
            <SettingsSwitch
              v-model="confirmDelete"
              label="Confirm before deleting a profile"
              hint="Show the delete-profile confirmation dialog."
              testid="settings-confirm-delete"
            />
          </template>

          <template v-else-if="activeCategory === 'appearance'">
            <SettingsSegmented
              :model-value="theme"
              label="Theme"
              :options="themeOptions"
              hint="Applies immediately across the whole app."
              testid="settings-theme-toggle"
              @update:model-value="onThemeChange"
            />
            <SettingsSegmented
              v-model="fontSize"
              label="Font size"
              :options="[{ value: 'small', label: 'Small' }, { value: 'medium', label: 'Medium' }, { value: 'large', label: 'Large' }]"
              hint="Adjusts base text size across lists and detail panes."
              testid="settings-font-size"
            />
            <SettingsSwitch
              v-model="compactDensity"
              label="Compact density"
              hint="Tightens row padding in the sidebar and feed list."
              testid="settings-compact-density"
            />
            <SettingsImageField
              v-model="accentImage"
              label="Sidebar accent image"
              hint="Shown behind the profile rail. PNG or JPG, any size."
              testid="settings-accent-image"
            />
          </template>

          <template v-else-if="activeCategory === 'integrations'">
            <SettingsSecretField
              v-model="githubToken"
              label="GitHub personal access token"
              placeholder="ghp_…"
              hint="Used to poll issues, PRs, and notifications."
              testid="settings-secret-input"
            />
            <SettingsTextField
              v-model="webhookUrl"
              label="Webhook URL"
              placeholder="https://example.com/hooks/hive"
              hint="Optional — receive push notifications instead of polling."
              testid="settings-webhook-url"
            />
            <SettingsStepper
              v-model="pollInterval"
              label="Poll interval"
              :min="15"
              :max="600"
              :step="15"
              suffix="s"
              hint="How often hive checks GitHub for updates."
              testid="settings-poll-interval"
            />
            <SettingsSwitch
              v-model="desktopNotifications"
              label="Desktop notifications"
              hint="Show a system notification for new unread items."
              testid="settings-desktop-notifications"
            />
          </template>

          <template v-else-if="activeCategory === 'advanced'">
            <SettingsSegmented
              v-model="logLevel"
              label="Log level"
              :options="[{ value: 'debug', label: 'Debug' }, { value: 'info', label: 'Info' }, { value: 'warn', label: 'Warn' }, { value: 'error', label: 'Error' }]"
              hint="Verbosity written to the app log file."
              testid="settings-log-level"
            />
            <SettingsStepper
              v-model="logRetention"
              label="Log retention"
              :min="1"
              :max="90"
              suffix=" days"
              hint="How long log files are kept before rotation."
              testid="settings-log-retention"
            />
            <SettingsFileField
              v-model="importFile"
              label="Import config file"
              accept=".yaml,.yml"
              hint="Load a profiles.yaml exported from another machine."
              testid="settings-import-file"
            />
            <SettingsSwitch
              v-model="experimentalFlows"
              label="Enable experimental flow nodes"
              hint="Turns on in-development node types in the flows palette."
              testid="settings-experimental-flows"
            />
          </template>
        </div>
      </div>
    </section>
  </div>
</template>
