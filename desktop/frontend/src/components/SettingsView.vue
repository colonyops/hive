<script setup lang="ts">
// Application-wide settings, opened from the persistent profile rail.
// Only settings backed by real behavior or explicitly marked future
// integrations belong here.
import { computed, onMounted, onUnmounted } from 'vue'
import IconArrowLeft from '~icons/lucide/arrow-left'
import IconKeyboard from '~icons/lucide/keyboard'
import IconPalette from '~icons/lucide/palette'
import IconPlug from '~icons/lucide/plug'
import IconPlay from '~icons/lucide/play'
import IconHardDrive from '~icons/lucide/hard-drive'
import ActionSettingsView from './ActionSettingsView.vue'
import KeybindingSettingsView from './KeybindingSettingsView.vue'
import SystemSettingsView from './SystemSettingsView.vue'
import githubIcon from '../assets/integrations/github.svg'
import grafanaIcon from '../assets/integrations/grafana.svg'
import posthogIcon from '../assets/integrations/posthog.svg'
import slackIcon from '../assets/integrations/slack.svg'
import SettingsSegmented from './settings/SettingsSegmented.vue'
import { setTheme, themeLabels, themes, useTheme, type Theme } from '../composables/useTheme'
import type { ApplicationSettingsSection } from '../router'

const props = withDefaults(defineProps<{
  githubConnected: boolean
  githubLogin?: string
  activeCategory: ApplicationSettingsSection
  knownFeedTypes?: string[]
}>(), { knownFeedTypes: () => [] })
const emit = defineEmits<{ close: []; 'select-category': [category: ApplicationSettingsSection] }>()
const categories = [
  { id: 'appearance' as const, label: 'Appearance', icon: IconPalette },
  { id: 'keybindings' as const, label: 'Keyboard', icon: IconKeyboard },
  { id: 'integrations' as const, label: 'Integrations', icon: IconPlug },
  { id: 'actions' as const, label: 'Actions', icon: IconPlay },
  { id: 'system' as const, label: 'System', icon: IconHardDrive },
]
const sectionTitle = computed(() => ({
  appearance: 'Appearance',
  keybindings: 'Keyboard shortcuts',
  integrations: 'Integrations',
  actions: 'Actions',
  system: 'System',
}[props.activeCategory]))

const { theme } = useTheme()
const themeOptions = themes.map((value) => ({ value, label: themeLabels[value] }))
const futureIntegrations = [
  { id: 'grafana', name: 'Grafana', description: 'Metrics, dashboards, and alerts', icon: grafanaIcon },
  { id: 'posthog', name: 'PostHog', description: 'Product analytics and events', icon: posthogIcon },
  { id: 'slack', name: 'Slack', description: 'Messages and notifications', icon: slackIcon },
]

function onThemeChange(value: string): void {
  setTheme(value as Theme)
}

function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape') emit('close')
}

onMounted(() => window.addEventListener('keydown', onKeydown))
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div class="flex h-full min-h-0 flex-1" data-testid="settings-view">
    <aside class="hive-scroll w-[200px] shrink-0 overflow-y-auto border-r border-row bg-sidebar">
      <div class="border-b border-border px-4 pb-3 pt-4">
        <div class="text-[15px] font-semibold tracking-[-.01em] text-text">Application settings</div>
      </div>
      <nav class="flex flex-col gap-0.5 px-2.5 py-3">
        <button
          v-for="category in categories"
          :key="category.id"
          type="button"
          class="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-2 text-left text-[13px]"
          :class="props.activeCategory === category.id ? 'bg-hover font-medium text-accent' : 'text-text-2 hover:bg-chip hover:text-text'"
          :aria-current="props.activeCategory === category.id ? 'true' : undefined"
          :data-testid="`settings-category-${category.id}`"
          @click="emit('select-category', category.id)"
        ><component :is="category.icon" class="size-3.5 shrink-0" />{{ category.label }}</button>
      </nav>
    </aside>

    <section class="flex min-w-0 flex-1 flex-col">
      <header class="flex h-11 shrink-0 items-center gap-2.5 border-b border-row bg-canvas-toolbar px-4">
        <span class="text-[13px] font-semibold text-text">{{ sectionTitle }}</span>
        <div class="flex-1" />
        <button
          type="button"
          class="flex cursor-pointer items-center gap-1.5 rounded-md px-2.5 py-1.5 text-[12.5px] text-text-2 hover:bg-chip hover:text-text"
          data-testid="settings-close"
          @click="emit('close')"
        ><IconArrowLeft class="size-3.5" />Back to feed</button>
      </header>

      <div class="hive-scroll min-h-0 flex-1 overflow-y-auto px-6 py-6">
        <div v-if="props.activeCategory === 'appearance'" class="mx-auto max-w-[560px]">
          <SettingsSegmented
            :model-value="theme"
            label="Theme"
            :options="themeOptions"
            hint="Applies immediately across the whole app."
            testid="settings-theme-toggle"
            @update:model-value="onThemeChange"
          />
        </div>

        <KeybindingSettingsView v-else-if="props.activeCategory === 'keybindings'" />

        <ActionSettingsView v-else-if="props.activeCategory === 'actions'" :known-types="props.knownFeedTypes" />

        <SystemSettingsView v-else-if="props.activeCategory === 'system'" />

        <div v-else class="mx-auto max-w-[640px]" data-testid="settings-integrations">
          <div class="mb-5">
            <h2 class="text-[15px] font-semibold text-text">Data sources</h2>
            <p class="mt-1 text-xs leading-relaxed text-text-3">
              Connections bring external events into Hive. More providers will support guided setup here as they become available.
            </p>
          </div>

          <div class="flex flex-col gap-3">
            <article class="flex items-center gap-3 rounded-lg border border-border bg-raised p-4" data-testid="integration-github">
              <span class="flex size-10 shrink-0 items-center justify-center rounded-lg bg-white p-2">
                <img :src="githubIcon" alt="GitHub" class="size-full" />
              </span>
              <div class="min-w-0 flex-1">
                <div class="text-[13.5px] font-semibold text-text">GitHub</div>
                <div class="mt-0.5 truncate text-xs text-text-3">{{ props.githubLogin ? `Connected as ${props.githubLogin}` : 'Issues, pull requests, and notifications' }}</div>
              </div>
              <span
                class="rounded-full px-2.5 py-1 text-[11px] font-semibold"
                :class="props.githubConnected ? 'bg-severity-success-tint text-severity-success' : 'bg-chip text-text-3'"
                data-testid="integration-github-status"
              >{{ props.githubConnected ? 'Connected' : 'Not connected' }}</span>
            </article>

            <article
              v-for="integration in futureIntegrations"
              :key="integration.id"
              class="flex items-center gap-3 rounded-lg border border-border bg-raised p-4"
              :data-testid="`integration-${integration.id}`"
            >
              <span class="flex size-10 shrink-0 items-center justify-center rounded-lg bg-white p-2">
                <img :src="integration.icon" :alt="integration.name" class="size-full object-contain" />
              </span>
              <div class="min-w-0 flex-1">
                <div class="text-[13.5px] font-semibold text-text">{{ integration.name }}</div>
                <div class="mt-0.5 truncate text-xs text-text-3">{{ integration.description }}</div>
              </div>
              <div class="flex shrink-0 items-center gap-2.5">
                <span class="font-mono text-[10.5px] text-text-4">Coming soon</span>
                <button
                  type="button"
                  disabled
                  class="cursor-not-allowed rounded-md border border-border px-2.5 py-1.5 text-[11.5px] font-medium text-text-4 opacity-60"
                  :data-testid="`integration-${integration.id}-add`"
                >Add connection</button>
              </div>
            </article>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
