<script setup lang="ts">
// System settings: on-disk locations (data dir, config dir, log file,
// database) with open/reveal actions, and point-only overrides for the data
// and config directories that take effect after a restart.
import { onMounted } from 'vue'
import IconInfo from '~icons/lucide/info'
import SettingsPathRow from './settings/SettingsPathRow.vue'
import { useSystemSettings } from '../composables/useSystemSettings'

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

    <section class="mb-6">
      <h2 class="text-[15px] font-semibold text-text">Storage locations</h2>
      <p class="mb-3 mt-1 text-xs leading-relaxed text-text-3">
        Point Hive at a different folder — for example an iCloud-synced directory to share configuration across
        machines. Existing data is not moved, and a new location applies after restarting.
      </p>
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
    </section>

    <section>
      <h2 class="text-[15px] font-semibold text-text">Diagnostics</h2>
      <p class="mb-3 mt-1 text-xs leading-relaxed text-text-3">
        Open or locate the log file and database when troubleshooting.
      </p>
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
    </section>
  </div>
</template>
