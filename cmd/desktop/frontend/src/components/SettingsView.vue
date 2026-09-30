<script setup lang="ts">
// Application-wide settings, opened from the persistent profile rail.
// Only settings backed by real behavior or explicitly marked future
// integrations belong here.
import { computed, ref, watch } from 'vue'
import IconSettings from '~icons/lucide/settings'
import IconPlug from '~icons/lucide/plug'
import IconRss from '~icons/lucide/rss'
import IconWebhook from '~icons/lucide/webhook'
import BaseBadge from './BaseBadge.vue'
import BaseCard from './BaseCard.vue'
import BaseIconBadge from './BaseIconBadge.vue'
import AboutSettingsView from './AboutSettingsView.vue'
import ActionSettingsView from './ActionSettingsView.vue'
import AppearanceSettingsView from './AppearanceSettingsView.vue'
import AgentsSettingsView from './AgentsSettingsView.vue'
import GeneralSettingsView from './GeneralSettingsView.vue'
import HiveSettingsView from './HiveSettingsView.vue'
import LauncherSettingsView from './LauncherSettingsView.vue'
import KeybindingSettingsView from './KeybindingSettingsView.vue'
import SystemSettingsView from './SystemSettingsView.vue'
import TerminalSettingsView from './TerminalSettingsView.vue'
import NotificationSettingsView from './NotificationSettingsView.vue'
import MenuBarSettingsView from './MenuBarSettingsView.vue'
import ObservabilitySettingsView from './ObservabilitySettingsView.vue'
import GithubIntegrationDrawer from './settings/GithubIntegrationDrawer.vue'
import GrafanaIntegrationDrawer from './settings/GrafanaIntegrationDrawer.vue'
import PostHogIntegrationDrawer from './settings/PostHogIntegrationDrawer.vue'
import GiteaIntegrationDrawer from './settings/GiteaIntegrationDrawer.vue'
import WebhookIntegrationDrawer from './settings/WebhookIntegrationDrawer.vue'
import SettingsLayout from './settings/SettingsLayout.vue'
import SettingsNavItem from './settings/SettingsNavItem.vue'
import SettingsHeading from './settings/SettingsHeading.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsSection from './settings/SettingsSection.vue'
import GithubMark from './marks/GithubMark.vue'
import GrafanaMark from './marks/GrafanaMark.vue'
import PostHogMark from './marks/PostHogMark.vue'
import GiteaMark from './marks/GiteaMark.vue'
import { useWebhookSettings } from '../composables/useWebhookSettings'
import { isConnected, takesCredential, useIntegrations } from '../composables/useIntegrations'
import type { Integration } from '../types/integrations'
import { applicationSettingsSections, type ApplicationSettingsSection } from '../router'
import { applicationSettingsSectionMeta } from './settings/sectionMeta'

const props = withDefaults(defineProps<{
  activeCategory: ApplicationSettingsSection
  knownFeedTypes?: string[]
}>(), { knownFeedTypes: () => [] })
const emit = defineEmits<{ close: []; 'select-category': [category: ApplicationSettingsSection] }>()
// The nav mirrors the app's own mode switch — Inbox, Code, Chats — bookended
// by what the whole app answers to and by the install itself, so the rail can
// be read against the title bar rather than learned. A value one surface uses
// sits under that surface; one several use sits in General. There is
// deliberately no leftover bucket: a section that fits nowhere is a sign the
// grouping is wrong, not that it needs an "Automation" pile to fall into.
// Every section appears in exactly one group — SettingsView.spec asserts that
// against applicationSettingsSections so a new pane cannot be routable but
// absent from the nav.
const navGroups: Array<{ title: string; ids: readonly ApplicationSettingsSection[] }> = [
  { title: 'Preferences', ids: ['general', 'appearance', 'notifications', 'menubar', 'keybindings'] },
  { title: 'Inbox', ids: ['integrations', 'actions'] },
  { title: 'Code', ids: ['terminal', 'launchers', 'hive'] },
  { title: 'Chats', ids: ['agents'] },
  { title: 'Advanced', ids: ['system', 'observability', 'about'] },
]
const sectionTitle = computed(() => applicationSettingsSectionMeta[props.activeCategory].title)

const githubSettingsOpen = ref(false)
const grafanaSettingsOpen = ref(false)
const posthogSettingsOpen = ref(false)
const giteaSettingsOpen = ref(false)
const webhookSettingsOpen = ref(false)

// The webhook card's badge reflects the same state the drawer edits, so a save
// there is reflected here without a second fetch.
const { settings: webhook, refresh: refreshWebhook } = useWebhookSettings()
const webhookStatus = computed(() => {
  if (!webhook.value) return { label: 'Local', tone: 'neutral' as const }
  // A saved change the listener has not picked up yet outranks what it is
  // currently doing — otherwise disabling it would still read "Running".
  if (webhook.value.startError) return { label: 'Port in use', tone: 'danger' as const }
  if (webhook.value.restartRequired) return { label: 'Restart needed', tone: 'neutral' as const }
  if (webhook.value.running) return { label: 'Running', tone: 'success' as const }
  return { label: 'Disabled', tone: 'neutral' as const }
})
const webhookDescription = computed(() => webhook.value
  ? `Receive JSON from anything that can POST — ${webhook.value.baseUrl}`
  : 'Receive JSON from anything that can POST to a local endpoint')

watch(() => props.activeCategory, (category) => {
  if (category === 'integrations') void refreshWebhook()
}, { immediate: true })
// Cards come from the Go connector registry, so adding a connector adds a
// card. Only its presentation is here — a type the registry reports but this
// map has not met still renders, with a generic icon and no blurb, rather
// than being silently dropped.
const { integrations, loaded: integrationsLoaded } = useIntegrations()

// Keyed by the card's key: a credentialed connector's provider, or a
// provider-less connector's type (webhook).
const presentation: Record<string, { description: string }> = {
  'github': { description: 'Issues, pull requests, and notifications' },
  'grafana': { description: 'Metrics and alerts from a Grafana stack' },
  'posthog': { description: 'Error tracking issues and insight alerts from a PostHog project' },
  'gitea': { description: 'Issues, pull requests, and notifications from a Gitea or Forgejo instance' },
  'sources.webhook': { description: 'Receive JSON from anything that can POST' },
  'sources.rss': { description: 'Entries from an RSS, Atom, or JSON Feed URL' },
}

// The drawer each card's gear opens. A connector with no drawer yet gets no
// gear rather than a button that does nothing.
const drawers: Record<string, () => void> = {
  'github': () => { githubSettingsOpen.value = true },
  'grafana': () => { grafanaSettingsOpen.value = true },
  'posthog': () => { posthogSettingsOpen.value = true },
  'gitea': () => { giteaSettingsOpen.value = true },
  'sources.webhook': () => { webhookSettingsOpen.value = true },
}

function subtitleFor(integration: Integration): string {
  // The webhook listener's own state is richer than "connected" and is what
  // its card has always shown; it has no credential to describe.
  if (integration.key === 'sources.webhook') return webhookDescription.value
  if (integration.envOverride && integration.accounts.length === 0) {
    return `Connected via ${envOverrideName(integration.provider)}`
  }
  if (integration.accounts.length > 0) return `Connected as ${integration.accounts.join(', ')}`
  return presentation[integration.key]?.description ?? ''
}

// Mirrors credentials.EnvOverrideName in Go. Shown so a headless or CI run
// explains why it is connected with no account listed.
function envOverrideName(provider: string): string {
  return `HIVE_${provider.toUpperCase().replace(/[^A-Z0-9]/g, '_')}_TOKEN`
}

// Cards are keyed by the connector's bare name, not its namespaced type:
// "integration-github" reads better in a selector than
// "integration-sources.github", and the namespace is constant across every
// entry here so it carries no information.
function cardId(type: string): string {
  return type.replace(/^sources\./, '')
}

function statusFor(integration: Integration): { label: string; tone: 'success' | 'neutral' | 'danger' } {
  if (integration.key === 'sources.webhook') return webhookStatus.value
  if (!takesCredential(integration)) return { label: 'Local', tone: 'neutral' }
  return isConnected(integration)
    ? { label: 'Connected', tone: 'success' }
    : { label: 'Not connected', tone: 'neutral' }
}

</script>

<template>
  <SettingsLayout data-testid="settings-view" @close="emit('close')">
    <template #sidebar-title>
      <div class="text-[15px] font-semibold tracking-[-.01em] text-text">Application settings</div>
    </template>
    <template #nav>
      <div v-for="(group, index) in navGroups" :key="group.title" class="flex flex-col gap-0.5">
        <!-- Collapsed to the icon rail there is no room for a heading, so the
             groups are separated by a rule instead. -->
        <span v-if="index > 0" class="mx-1.5 my-2 h-px bg-border @[700px]/settings:hidden" />
        <SettingsHeading
          level="group"
          :rule="false"
          :title="group.title"
          class="hidden px-2.5 pb-1.5 @[700px]/settings:flex"
          :class="index > 0 ? 'pt-4' : ''"
        />
        <SettingsNavItem
          v-for="id in group.ids"
          :key="id"
          :active="props.activeCategory === id"
          :icon="applicationSettingsSectionMeta[id].icon"
          :label="applicationSettingsSectionMeta[id].label"
          :testid="`settings-category-${id}`"
          @select="emit('select-category', id)"
        />
      </div>
    </template>
    <template #header-title>
      <span class="text-[13px] font-semibold text-text">{{ sectionTitle }}</span>
    </template>

    <GeneralSettingsView v-if="props.activeCategory === 'general'" />

    <AppearanceSettingsView v-else-if="props.activeCategory === 'appearance'" />

    <KeybindingSettingsView v-else-if="props.activeCategory === 'keybindings'" />

    <TerminalSettingsView v-else-if="props.activeCategory === 'terminal'" />
    <HiveSettingsView v-else-if="props.activeCategory === 'hive'" />
    <AgentsSettingsView v-else-if="props.activeCategory === 'agents'" />

    <ActionSettingsView v-else-if="props.activeCategory === 'actions'" :known-types="props.knownFeedTypes" />
    <LauncherSettingsView v-else-if="props.activeCategory === 'launchers'" />

    <ObservabilitySettingsView v-else-if="props.activeCategory === 'observability'" />
    <SystemSettingsView v-else-if="props.activeCategory === 'system'" />
    <AboutSettingsView v-else-if="props.activeCategory === 'about'" />

    <NotificationSettingsView v-else-if="props.activeCategory === 'notifications'" />
    <MenuBarSettingsView v-else-if="props.activeCategory === 'menubar'" />

    <SettingsPage v-else testid="settings-integrations">
      <SettingsSection
        title="Data sources"
        description="Connections bring external events into Hive. Every connector the app knows about is listed here."
      >
        <div v-if="!integrationsLoaded" class="font-mono text-xs text-text-4" data-testid="integrations-loading">Loading…</div>
        <div v-else class="flex flex-col gap-3">
          <BaseCard
            v-for="integration in integrations"
            :key="integration.key"
            class="flex-wrap items-start rounded-[11px] border border-card bg-raised @[600px]/pane:flex-nowrap @[600px]/pane:items-center"
            :data-testid="`integration-${cardId(integration.key)}`"
          >
            <template #icon>
              <!-- One ground for every card, the same one the inbox source
                   badge puts these marks on. A mark carrying its own colour
                   sits on it unchanged; a currentColor one takes the badge's
                   text colour, which is what keeps GitHub's octocat legible
                   on a dark chip. -->
              <BaseIconBadge :size="40" rounded="rounded-lg" class="bg-chip p-2 text-text-2">
                <GithubMark v-if="integration.key === 'github'" class="size-full" />
                <GrafanaMark v-else-if="integration.key === 'grafana'" class="size-full" />
                <PostHogMark v-else-if="integration.key === 'posthog'" class="size-full" />
                <GiteaMark v-else-if="integration.key === 'gitea'" class="size-full" />
                <IconWebhook v-else-if="integration.key === 'sources.webhook'" class="size-full" />
                <IconRss v-else-if="integration.key === 'sources.rss'" class="size-full" />
                <IconPlug v-else class="size-full" />
              </BaseIconBadge>
            </template>
            <div class="min-w-0 flex-1">
              <div class="text-[13.5px] font-semibold text-text">{{ integration.title }}</div>
              <div class="mt-0.5 truncate text-xs text-text-3">{{ subtitleFor(integration) }}</div>
            </div>
            <template #actions>
              <div class="flex w-full items-center justify-end gap-2 @[600px]/pane:w-auto @[600px]/pane:shrink-0">
                <BaseBadge
                  v-if="integration.stability !== 'stable'"
                  tone="neutral"
                  variant="pill"
                  class="px-2 py-1 text-[10.5px] font-semibold uppercase"
                  :data-testid="`integration-${cardId(integration.key)}-stability`"
                >{{ integration.stability }}</BaseBadge>
                <BaseBadge
                  :tone="statusFor(integration).tone"
                  variant="pill"
                  class="px-2.5 py-1 text-[11px] font-semibold"
                  :data-testid="`integration-${cardId(integration.key)}-status`"
                >{{ statusFor(integration).label }}</BaseBadge>
                <button
                  v-if="drawers[integration.key]"
                  type="button"
                  class="flex size-7 cursor-pointer items-center justify-center rounded-md text-text-3 hover:bg-chip hover:text-text"
                  :aria-label="`Configure ${integration.title}`"
                  :data-testid="`integration-${cardId(integration.key)}-configure`"
                  @click="drawers[integration.key]()"
                ><IconSettings class="size-3.5" /></button>
              </div>
            </template>
          </BaseCard>
        </div>
      </SettingsSection>

      <GithubIntegrationDrawer v-if="githubSettingsOpen" @close="githubSettingsOpen = false" />
      <GrafanaIntegrationDrawer v-if="grafanaSettingsOpen" @close="grafanaSettingsOpen = false" />
      <PostHogIntegrationDrawer v-if="posthogSettingsOpen" @close="posthogSettingsOpen = false" />
      <GiteaIntegrationDrawer v-if="giteaSettingsOpen" @close="giteaSettingsOpen = false" />
      <WebhookIntegrationDrawer v-if="webhookSettingsOpen" @close="webhookSettingsOpen = false" />
    </SettingsPage>
  </SettingsLayout>
</template>
