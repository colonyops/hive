<script setup lang="ts">
import { Browser } from '@wailsio/runtime'
import { computed, onMounted } from 'vue'
import IconBookOpen from '~icons/lucide/book-open'
import IconInfo from '~icons/lucide/info'
import BaseBadge from './BaseBadge.vue'
import RuntimeDashboard from './RuntimeDashboard.vue'
import SettingsError from './settings/SettingsError.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsSection from './settings/SettingsSection.vue'
import { useObservabilitySettings } from '../composables/useObservabilitySettings'
import type { ExportStatus } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const DOCS_URL = 'https://hivedesktop.com/configuration/settings/#telemetry'
const { current, loading, error, refresh } = useObservabilitySettings()

const restartRequired = computed(() => current.value?.otlp.restartRequired || current.value?.profiles.restartRequired)

type Status = { label: string; tone: 'success' | 'neutral' | 'danger' }

function statusFor(exporter: ExportStatus): Status {
  if (exporter.restartRequired) return { label: 'Restart needed', tone: 'neutral' }
  if (exporter.running) return { label: 'Exporting', tone: 'success' }
  if (exporter.enabled && current.value?.startError) return { label: 'Error', tone: 'danger' }
  if (exporter.enabled && !exporter.configured) return { label: 'Incomplete', tone: 'danger' }
  if (exporter.enabled) return { label: 'Enabled', tone: 'success' }
  if (exporter.configured) return { label: 'Ready', tone: 'neutral' }
  return { label: 'Disabled', tone: 'neutral' }
}

onMounted(() => {
  void refresh()
})
</script>

<template>
  <SettingsPage testid="settings-observability">
    <div
      v-if="restartRequired"
      class="flex items-center gap-3 rounded-lg border border-border bg-severity-info-tint p-3.5"
      data-testid="observability-restart-banner"
    >
      <IconInfo class="size-4 shrink-0 text-severity-info" />
      <div class="min-w-0 flex-1 text-[12.5px] text-text-2">
        Restart Hive to apply the telemetry changes in settings.yaml.
      </div>
    </div>

    <SettingsError v-if="error" :message="error" testid="observability-error" />
    <SettingsError
      v-if="current?.startError"
      :message="`Telemetry did not start: ${current.startError}`"
      testid="observability-start-error"
    />

    <RuntimeDashboard />

    <SettingsSection
      title="Grafana Cloud exports"
      description="Hive reads these destinations from settings.yaml when it starts."
      testid="observability-exports"
    >
      <template #actions>
        <button
          type="button"
          class="flex cursor-pointer items-center gap-1.5 text-[12px] font-medium text-accent hover:underline"
          data-testid="observability-docs"
          @click="Browser.OpenURL(DOCS_URL)"
        >
          <IconBookOpen class="size-3.5" />Configuration guide ↗
        </button>
      </template>

      <p v-if="loading && !current" class="font-mono text-xs text-text-4">Loading…</p>

      <div v-if="current" class="grid grid-cols-1 gap-3 @[640px]/pane:grid-cols-2">
        <article class="rounded-[11px] border border-card bg-raised p-4" data-testid="observability-otlp">
          <div class="flex items-start gap-3">
            <div class="min-w-0 flex-1">
              <div class="text-[13.5px] font-semibold text-text">Metrics, logs, and traces</div>
              <p class="mt-1 text-xs leading-relaxed text-text-3">OpenTelemetry Protocol over HTTPS.</p>
            </div>
            <BaseBadge
              :tone="statusFor(current.otlp).tone"
              variant="pill"
              class="px-2.5 py-1 text-[11px] font-semibold"
              data-testid="observability-otlp-status"
            >
              {{ statusFor(current.otlp).label }}
            </BaseBadge>
          </div>
          <div class="mt-4 flex flex-wrap gap-2">
            <BaseBadge
              :tone="current.otlp.enabled ? 'success' : 'neutral'"
              variant="pill"
              class="px-2.5 py-1 text-[11px] font-semibold"
              data-testid="observability-otlp-enabled"
            >
              {{ current.otlp.enabled ? 'Enabled' : 'Disabled' }}
            </BaseBadge>
            <BaseBadge
              :tone="current.otlp.configured ? 'success' : 'neutral'"
              variant="pill"
              class="px-2.5 py-1 text-[11px] font-semibold"
              data-testid="observability-otlp-configured"
            >
              {{ current.otlp.configured ? 'Configured' : 'Not configured' }}
            </BaseBadge>
          </div>
        </article>

        <article class="rounded-[11px] border border-card bg-raised p-4" data-testid="observability-profiles">
          <div class="flex items-start gap-3">
            <div class="min-w-0 flex-1">
              <div class="text-[13.5px] font-semibold text-text">Continuous profiles</div>
              <p class="mt-1 text-xs leading-relaxed text-text-3">CPU, allocation, and in-use heap profiles.</p>
            </div>
            <BaseBadge
              :tone="statusFor(current.profiles).tone"
              variant="pill"
              class="px-2.5 py-1 text-[11px] font-semibold"
              data-testid="observability-profiles-status"
            >
              {{ statusFor(current.profiles).label }}
            </BaseBadge>
          </div>
          <div class="mt-4 flex flex-wrap gap-2">
            <BaseBadge
              :tone="current.profiles.enabled ? 'success' : 'neutral'"
              variant="pill"
              class="px-2.5 py-1 text-[11px] font-semibold"
              data-testid="observability-profiles-enabled"
            >
              {{ current.profiles.enabled ? 'Enabled' : 'Disabled' }}
            </BaseBadge>
            <BaseBadge
              :tone="current.profiles.configured ? 'success' : 'neutral'"
              variant="pill"
              class="px-2.5 py-1 text-[11px] font-semibold"
              data-testid="observability-profiles-configured"
            >
              {{ current.profiles.configured ? 'Configured' : 'Not configured' }}
            </BaseBadge>
          </div>
        </article>
      </div>
    </SettingsSection>
  </SettingsPage>
</template>
