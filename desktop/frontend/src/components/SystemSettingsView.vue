<script setup lang="ts">
// System settings: on-disk locations (data dir, config dir, log file,
// database) with open/reveal actions, and point-only overrides for the data
// and config directories that take effect after a restart.
import { onMounted } from 'vue'
import IconInfo from '~icons/lucide/info'
import IconExternalLink from '~icons/lucide/external-link'
import SettingsPathRow from './settings/SettingsPathRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import { useSystemSettings } from '../composables/useSystemSettings'

const {
  info,
  build,
  error,
  restartRequired,
  refresh,
  openReleaseNotes,
  openRepo,
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
  <div class="mx-auto max-w-[640px]" data-testid="settings-system">
    <div
      v-if="restartRequired"
      class="mb-5 flex items-center gap-3 rounded-lg border border-border bg-severity-info-tint p-3.5"
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

    <div
      v-if="error"
      class="mb-4 rounded-md border border-border bg-severity-error-tint px-3 py-2 text-xs text-severity-error"
      data-testid="system-error"
    >{{ error }}</div>

    <SettingsSection
      title="Storage locations"
      description="Point Hive at a different folder — for example an iCloud-synced directory to share configuration across machines. Existing data is not moved, and a new location applies after restarting."
      class="mb-6 [&>p]:mb-3"
    >
      <div v-if="info" class="flex flex-col gap-3">
        <SettingsPathRow
          label="Data directory"
          hint="Desktop state, logs, and the databases live here."
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
      </div>
    </SettingsSection>

    <SettingsSection
      title="Diagnostics"
      description="Open or locate the log file and database when troubleshooting."
      class="[&>p]:mb-3"
    >
      <div v-if="info" class="flex flex-col gap-3">
        <SettingsPathRow
          label="Log file"
          :path="info.logFile.path"
          :exists="info.logFile.exists"
          testid="system-log-file"
          @open="openPath(info.logFile.path)"
          @reveal="revealPath(info.logFile.path)"
        />
        <SettingsPathRow
          label="Database"
          :path="info.database.path"
          :exists="info.database.exists"
          testid="system-database"
          @open="openPath(info.database.path)"
          @reveal="revealPath(info.database.path)"
        />
      </div>
    </SettingsSection>

    <SettingsSection
      title="About"
      description="The build of Hive you're running. Include this when reporting an issue."
      class="mt-6 [&>p]:mb-3"
      data-testid="system-about"
    >
      <div v-if="build" class="rounded-lg border border-border">
        <div class="flex items-center justify-between gap-3 px-3.5 py-2.5">
          <span class="text-[12.5px] text-text-3">Version</span>
          <span class="font-mono text-[12.5px] text-text-2" data-testid="system-build-version">{{ build.version }}</span>
        </div>
        <div class="flex items-center justify-between gap-3 border-t border-border px-3.5 py-2.5">
          <span class="text-[12.5px] text-text-3">Commit</span>
          <span class="font-mono text-[12.5px] text-text-2" data-testid="system-build-commit">{{ build.commit }}</span>
        </div>
        <div class="flex items-center justify-between gap-3 border-t border-border px-3.5 py-2.5">
          <span class="text-[12.5px] text-text-3">Built</span>
          <span class="font-mono text-[12.5px] text-text-2" data-testid="system-build-date">{{ build.date }}</span>
        </div>
        <div class="flex flex-col gap-2 border-t border-border px-3.5 py-2.5">
          <button
            type="button"
            class="flex cursor-pointer items-center gap-1.5 text-[12.5px] font-medium text-severity-info hover:underline"
            data-testid="system-build-repo"
            @click="openRepo"
          >
            <IconExternalLink class="size-3.5" />
            View project on GitHub
          </button>
          <button
            v-if="build.releaseUrl"
            type="button"
            class="flex cursor-pointer items-center gap-1.5 text-[12.5px] font-medium text-severity-info hover:underline"
            data-testid="system-build-release"
            @click="openReleaseNotes"
          >
            <IconExternalLink class="size-3.5" />
            View release on GitHub
          </button>
        </div>
      </div>
    </SettingsSection>
  </div>
</template>
