<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  Summary,
  SetEnabled,
  Clear,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/analyticsservice'
import type { AnalyticsSummary } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { useConfirmation } from '../composables/useConfirmation'
import { useErrorDialog } from '../composables/useErrorDialog'
import { errorText } from '../lib/appError'
import AppSwitch from './ui/AppSwitch.vue'
import BaseButton from './ui/BaseButton.vue'
import StatCard from './ui/StatCard.vue'
import ConfirmationHost from './ui/ConfirmationHost.vue'
import CopyButton from './ui/CopyButton.vue'
import InlineError from './ui/InlineError.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsSection from './settings/SettingsSection.vue'
import SettingsRow from './settings/SettingsRow.vue'

const summary = ref<AnalyticsSummary | null>(null)
const loading = ref(false)
const saving = ref(false)
const error = ref<string | null>(null)
const confirmation = useConfirmation()
const { showError } = useErrorDialog()

async function refresh(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    summary.value = await Summary()
  } catch (cause) {
    error.value = errorText(cause, 'Could not read local usage history.')
  } finally {
    loading.value = false
  }
}

async function setEnabled(enabled: boolean): Promise<void> {
  saving.value = true
  try {
    await SetEnabled(enabled)
    await refresh()
  } catch (cause) {
    showError({
      title: 'Could not save analytics collection.',
      detail: errorText(cause, 'Could not save analytics collection.'),
    })
  } finally {
    saving.value = false
  }
}

function clearHistory(): void {
  confirmation.request({
    title: 'Clear local usage history?',
    description:
      'This removes history from both Hive CLI and Desktop and resets the local installation identifier. It does not change collection settings.',
    confirmLabel: 'Clear history',
    testid: 'analytics-clear-confirmation',
    onConfirm: async () => {
      await Clear()
      await refresh()
    },
  })
}

onMounted(() => {
  void refresh()
})
</script>

<template>
  <SettingsPage testid="settings-analytics">
    <InlineError v-if="error" :message="error" testid="analytics-error" />
    <SettingsSection
      title="Local collection"
      description="Usage stays on this machine. Nothing is sent to a server."
      boxed
    >
      <SettingsRow
        label="Collect local usage"
        hint="Shared by Hive CLI and Desktop. Changes apply when each program next starts."
      >
        <AppSwitch
          :model-value="summary?.enabled ?? false"
          :disabled="!summary || saving"
          aria-label="Collect local usage"
          testid="analytics-enabled"
          @update:model-value="setEnabled"
        />
      </SettingsRow>
      <div v-if="summary" class="px-4 py-3 text-small text-text-3" data-testid="analytics-status">
        {{
          summary.active
            ? 'This process is collecting.'
            : summary.effectiveEnabled
              ? 'Local collection is unavailable in this process.'
              : 'This process is not collecting.'
        }}
        History remains readable when collection is off.
      </div>
      <div v-if="summary?.restartNeeded" class="px-4 pb-3 text-small text-text-2" data-testid="analytics-restart">
        Restart Hive Desktop to apply the saved collection setting. Already-running CLI processes keep their startup
        setting.
      </div>
      <div
        v-if="summary?.environmentOverride"
        class="px-4 pb-3 text-small text-text-2"
        data-testid="analytics-environment"
      >
        HIVE_ANALYTICS_ENABLED disables collection for this process, regardless of the saved switch.
      </div>
    </SettingsSection>
    <SettingsSection
      title="Last 90 days"
      description="UTC capture times. Counts update after a flush, which can take up to 10 seconds."
    >
      <template #actions>
        <BaseButton variant="secondary" size="xs" :busy="loading" data-testid="analytics-refresh" @click="refresh">
          Refresh counts
        </BaseButton>
      </template>
      <div class="grid grid-cols-1 gap-3 @[440px]/pane:grid-cols-2 @[720px]/pane:grid-cols-3">
        <StatCard
          label="CLI commands"
          :value="summary?.counts.cliCommands ?? 0"
          hint="Completed, including failures"
          value-testid="analytics-cli-count"
        />
        <StatCard
          label="Hive sessions created"
          :value="summary?.counts.hiveSessions ?? 0"
          hint="Successful creates from CLI and Desktop"
          value-testid="analytics-session-count"
        />
        <StatCard
          label="Desktop terminal starts"
          :value="summary?.counts.terminalStarts ?? 0"
          hint="New starts, not existing terminals"
          value-testid="analytics-terminal-count"
        />
      </div>
    </SettingsSection>
    <SettingsSection
      title="History"
      description="Only bounded event properties are stored. No repository names, paths, prompts, arguments, or terminal content."
      boxed
    >
      <SettingsRow label="Installation ID">
        <div class="flex flex-wrap items-center gap-2">
          <span class="break-all font-mono text-small text-text-2" data-testid="analytics-identity">
            {{ summary?.installationId || 'Not created yet' }}
          </span>
          <CopyButton
            v-if="summary?.installationId"
            :text="summary.installationId"
            label="Copy"
            size="xs"
            data-testid="analytics-identity-copy"
          />
        </div>
      </SettingsRow>
      <SettingsRow
        label="90-day retention"
        hint="Old rows are removed at startup and daily while collection is enabled. Disabling collection preserves history."
      >
        <BaseButton
          variant="danger-outline"
          :disabled="!summary || saving || confirmation.busy.value"
          data-testid="analytics-clear"
          @click="clearHistory"
          >Clear history</BaseButton
        >
      </SettingsRow>
    </SettingsSection>
    <ConfirmationHost :confirmation="confirmation" />
  </SettingsPage>
</template>
