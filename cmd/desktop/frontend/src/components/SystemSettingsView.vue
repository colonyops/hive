<script setup lang="ts">
// System settings: this install on this machine — where its data and config
// live, the files to open when something goes wrong, and the reporter that
// bundles them. Anything a feature owns lives on that feature's pane.
import { onMounted } from 'vue'
import IconInfo from '~icons/lucide/info'
import IconBug from '~icons/lucide/bug'
import IconLifeBuoy from '~icons/lucide/life-buoy'
import IconChevronRight from '~icons/lucide/chevron-right'
import SettingsError from './settings/SettingsError.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsPathRow from './settings/SettingsPathRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import { useSystemSettings } from '../composables/useSystemSettings'
import { useReportDialog } from '../composables/useReportDialog'

const { openBundleDialog, reportProblem } = useReportDialog()

const {
  info,
  error,
  restartRequired,
  refresh,
  openPath,
  revealPath,
  changeDataDir,
  changeConfigDir,
  resetDataDir,
  resetConfigDir,
  quit,
} = useSystemSettings()

onMounted(() => {
  void refresh()
})
</script>

<template>
  <SettingsPage testid="settings-system">
    <div
      v-if="restartRequired"
      class="flex items-center gap-3 rounded-lg border border-border bg-severity-info-tint p-3.5"
      data-testid="system-restart-banner"
    >
      <IconInfo class="size-4 shrink-0 text-severity-info" />
      <div class="min-w-0 flex-1 text-[12.5px] text-text-2">Location changes take effect after restarting Hive.</div>
      <button
        type="button"
        class="shrink-0 cursor-pointer rounded-md border border-border px-2.5 py-1.5 text-[12px] font-medium text-text-2 hover:bg-chip hover:text-text"
        data-testid="system-quit"
        @click="quit"
      >Quit Hive</button>
    </div>

    <SettingsError v-if="error" :message="error" testid="system-error" />

    <button
      type="button"
      class="group flex cursor-pointer items-center gap-3.5 overflow-hidden rounded-[11px] border border-accent/40 bg-raised px-4 py-3.5 text-left transition-colors hover:border-accent hover:bg-accent-tint/20"
      data-testid="system-report-problem"
      @click="reportProblem"
    >
      <span class="flex size-9 shrink-0 items-center justify-center rounded-[9px] bg-accent-tint text-accent">
        <IconBug class="size-[18px]" />
      </span>
      <div class="min-w-0 flex-1">
        <div class="text-[13.5px] font-semibold text-text">Report a problem</div>
        <div class="mt-0.5 text-[11.5px] text-text-3">Open a GitHub issue. Nothing from this machine is attached.</div>
      </div>
      <IconChevronRight class="size-4 shrink-0 text-text-3 transition-transform group-hover:translate-x-0.5 group-hover:text-text-2" />
    </button>

    <button
      type="button"
      class="group flex cursor-pointer items-center gap-3.5 overflow-hidden rounded-[11px] border border-card bg-raised px-4 py-3.5 text-left transition-colors hover:border-strong"
      data-testid="system-save-bundle"
      @click="openBundleDialog"
    >
      <span class="flex size-9 shrink-0 items-center justify-center rounded-[9px] bg-app text-text-2">
        <IconLifeBuoy class="size-[18px]" />
      </span>
      <div class="min-w-0 flex-1">
        <div class="text-[13.5px] font-semibold text-text">Save a diagnostic bundle</div>
        <div class="mt-0.5 text-[11.5px] text-text-3">For when a maintainer asks for one. Send it privately, not on an issue.</div>
      </div>
      <IconChevronRight class="size-4 shrink-0 text-text-3 transition-transform group-hover:translate-x-0.5 group-hover:text-text-2" />
    </button>

    <SettingsSection
      v-if="info"
      title="Storage locations"
      description="Point Hive at a different folder. Existing data isn't moved; a new location applies after restart."
      boxed
    >
      <SettingsPathRow
        label="Data directory"
        hint="Desktop state, logs, and the databases live here."
        icon="folder"
        tone="accent"
        :path="info.dataDir.path"
        :exists="info.dataDir.exists"
        :overridden="info.dataDir.overridden"
        editable
        testid="system-data-dir"
        @open="openPath(info.dataDir.path)"
        @reveal="revealPath(info.dataDir.path)"
        @change="changeDataDir"
        @reset="resetDataDir"
      />
      <SettingsPathRow
        label="Config directory"
        hint="Profiles, flows, and actions.yml."
        icon="folder"
        tone="accent"
        :path="info.configDir.path"
        :exists="info.configDir.exists"
        :overridden="info.configDir.overridden"
        editable
        testid="system-config-dir"
        @open="openPath(info.configDir.path)"
        @reveal="revealPath(info.configDir.path)"
        @change="changeConfigDir"
        @reset="resetConfigDir"
      />
    </SettingsSection>

    <SettingsSection
      v-if="info"
      title="Diagnostics"
      description="Open or locate the log file and database when troubleshooting."
      boxed
    >
      <SettingsPathRow
        label="Log file"
        icon="log"
        :path="info.logFile.path"
        :exists="info.logFile.exists"
        testid="system-log-file"
        @open="openPath(info.logFile.path)"
        @reveal="revealPath(info.logFile.path)"
      />
      <SettingsPathRow
        label="Database"
        icon="database"
        :path="info.database.path"
        :exists="info.database.exists"
        testid="system-database"
        @open="openPath(info.database.path)"
        @reveal="revealPath(info.database.path)"
      />
    </SettingsSection>
  </SettingsPage>
</template>
