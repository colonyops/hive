<script setup lang="ts">
// System settings: on-disk locations (data dir, config dir, log file,
// database) grouped into one card per section with per-row open/reveal
// actions, point-only overrides for the data and config directories that take
// effect after a restart, and an About card with the build strip and
// auto-update controls.
import { onMounted } from 'vue'
import IconInfo from '~icons/lucide/info'
import IconExternalLink from '~icons/lucide/external-link'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import SettingsPathRow from './settings/SettingsPathRow.vue'
import AppSwitch from './AppSwitch.vue'
import { useSystemSettings } from '../composables/useSystemSettings'

const {
  info,
  build,
  error,
  restartRequired,
  autoUpdate,
  update,
  checkingUpdate,
  checkedOnce,
  setAutoUpdate,
  checkForUpdates,
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
  <div class="mx-auto flex max-w-[860px] flex-col gap-6" data-testid="settings-system">
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

    <div
      v-if="error"
      class="rounded-md border border-border bg-severity-error-tint px-3 py-2 text-xs text-severity-error"
      data-testid="system-error"
    >{{ error }}</div>

    <section class="flex flex-col gap-2.5">
      <div class="flex flex-wrap items-baseline gap-x-2.5 gap-y-1">
        <h2 class="text-xs font-semibold uppercase tracking-[.1em] text-text-2">Storage locations</h2>
        <p class="text-xs text-text-3">Point Hive at a different folder. Existing data isn't moved; a new location applies after restart.</p>
      </div>
      <div v-if="info" class="divide-y divide-row overflow-hidden rounded-[11px] border border-card bg-raised">
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
      </div>
    </section>

    <section class="flex flex-col gap-2.5">
      <div class="flex flex-wrap items-baseline gap-x-2.5 gap-y-1">
        <h2 class="text-xs font-semibold uppercase tracking-[.1em] text-text-2">Diagnostics</h2>
        <p class="text-xs text-text-3">Open or locate the log file and database when troubleshooting.</p>
      </div>
      <div v-if="info" class="divide-y divide-row overflow-hidden rounded-[11px] border border-card bg-raised">
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
      </div>
    </section>

    <section class="flex flex-col gap-2.5" data-testid="system-about">
      <div class="flex flex-wrap items-baseline gap-x-2.5 gap-y-1">
        <h2 class="text-xs font-semibold uppercase tracking-[.1em] text-text-2">About</h2>
        <p class="text-xs text-text-3">The build of Hive you're running — include this when reporting an issue.</p>
      </div>
      <div v-if="build" class="overflow-hidden rounded-[11px] border border-card bg-raised">
        <div class="flex flex-wrap items-center gap-x-6 gap-y-3 px-4 py-3.5">
          <div class="flex flex-col gap-1">
            <span class="text-[11px] text-text-3">Version</span>
            <div class="flex items-center gap-2">
              <span class="font-mono text-[13px] text-text" data-testid="system-build-version">{{ build.version }}</span>
              <span
                v-if="update?.available"
                class="rounded-full border border-severity-info-border bg-severity-info-tint px-2 py-0.5 text-[11px] font-medium text-severity-info"
                data-testid="system-update-available"
              >Update available: {{ update.latestVersion }}</span>
              <span
                v-else-if="checkedOnce"
                class="text-[11px] text-text-4"
                data-testid="system-update-uptodate"
              >Up to date</span>
            </div>
          </div>
          <div class="h-[26px] w-px bg-row" />
          <div class="flex flex-col gap-1">
            <span class="text-[11px] text-text-3">Commit</span>
            <span class="font-mono text-[13px] text-text" data-testid="system-build-commit">{{ build.commit }}</span>
          </div>
          <div class="h-[26px] w-px bg-row" />
          <div class="flex flex-col gap-1">
            <span class="text-[11px] text-text-3">Built</span>
            <span class="font-mono text-[13px] text-text" data-testid="system-build-date">{{ build.date }}</span>
          </div>
          <div class="flex-1" />
          <div class="flex items-center gap-4 text-[13px]">
            <button
              type="button"
              class="flex cursor-pointer items-center gap-1.5 text-accent hover:underline"
              title="View project on GitHub"
              data-testid="system-build-repo"
              @click="openRepo"
            ><IconExternalLink class="size-3.5" />Project</button>
            <button
              v-if="build.releaseUrl"
              type="button"
              class="flex cursor-pointer items-center gap-1.5 text-accent hover:underline"
              title="View release on GitHub"
              data-testid="system-build-release"
              @click="openReleaseNotes"
            ><IconExternalLink class="size-3.5" />Release</button>
          </div>
        </div>
        <div class="flex items-center gap-3.5 border-t border-row px-4 py-3.5">
          <AppSwitch
            :model-value="autoUpdate"
            aria-label="Automatic updates"
            testid="system-auto-update"
            @update:model-value="setAutoUpdate"
          />
          <div class="min-w-0 flex-1">
            <div class="text-[13.5px] font-semibold text-text">Automatic updates</div>
            <div class="mt-0.5 text-[11.5px] text-text-3">Check for and install new versions from GitHub in the background.</div>
          </div>
          <button
            type="button"
            class="flex shrink-0 cursor-pointer items-center gap-1.5 rounded-[7px] border border-card px-3 py-1.5 text-[12.5px] font-medium text-text-2 hover:border-strong hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="checkingUpdate"
            data-testid="system-check-update"
            @click="checkForUpdates"
          >
            <IconRefreshCw class="size-3.5" :class="checkingUpdate ? 'animate-spin' : ''" />
            {{ checkingUpdate ? 'Checking…' : 'Check for updates' }}
          </button>
        </div>
      </div>
    </section>
  </div>
</template>
