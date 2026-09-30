<script setup lang="ts">
// Appearance: how the app's own chrome is drawn, everywhere. A terminal draws
// itself from Settings ▸ Terminal and takes nothing from here (ADR settings-sections-name-the-surface-they-change).
import { computed, onMounted } from 'vue'
import AppSelect from './AppSelect.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsRow from './settings/SettingsRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import ThemePicker from './settings/ThemePicker.vue'
import { loadInstalledFonts, useInstalledFonts } from '../composables/useInstalledFonts'
import {
  BUNDLED_MONO,
  BUNDLED_SANS,
  setAppFontFamily,
  setAppMonoFontFamily,
  SYSTEM_MONO,
  SYSTEM_SANS,
  useAppFont,
} from '../composables/useAppFont'
import { setTheme, useTheme, type Theme } from '../composables/useTheme'

const { theme } = useTheme()
const { sans, mono } = useAppFont()
const { all: allFamilies, monospace: monoFamilies } = useInstalledFonts()

// The bundled face leads as the empty selection, the zero-payload platform
// stack follows, then whatever is installed. The bundled mono is filtered out
// of the installed list because picking it there would pin today's face by
// name; the bundled sans cannot collide, since it is declared as "Inter
// Variable" and an installed Inter is a different face worth offering.
const sansOptions = computed(() => [
  { value: '', label: `${BUNDLED_SANS} · bundled` },
  { value: SYSTEM_SANS, label: 'System' },
  ...allFamilies.value.map((family) => ({ value: family, label: family })),
])
const monoOptions = computed(() => [
  { value: '', label: `${BUNDLED_MONO} · bundled` },
  { value: SYSTEM_MONO, label: 'System' },
  ...monoFamilies.value
    .filter((family) => family !== BUNDLED_MONO)
    .map((family) => ({ value: family, label: family })),
])

function onThemeChange(value: string): void {
  setTheme(value as Theme)
}

// Scanning every font on the machine is not worth doing until this pane is the
// one on screen.
onMounted(loadInstalledFonts)
</script>

<template>
  <SettingsPage testid="settings-appearance">
    <SettingsSection
      title="Theme"
      description="Applies immediately across the whole app."
    >
      <ThemePicker :model-value="theme" @update:model-value="onThemeChange" />
    </SettingsSection>

    <SettingsSection
      title="Typography"
      description="The faces the app's chrome is drawn with. Terminals keep their own — see Settings ▸ Terminal."
      boxed
    >
      <SettingsRow
        label="Interface font"
        hint="Families installed on this machine. A name the app cannot resolve falls back to the bundled face rather than breaking the layout."
        testid="settings-appearance-font-family"
      >
        <AppSelect
          class="w-[220px]"
          :model-value="sans"
          :options="sansOptions"
          searchable
          search-placeholder="Search fonts"
          aria-label="Interface font"
          testid="settings-appearance-font-family-select"
          @update:model-value="setAppFontFamily"
        />
      </SettingsRow>
      <SettingsRow
        label="Monospace font"
        hint="Used for timestamps, ids, code, and the flow editor. Limited to fixed-pitch families so columns stay aligned."
        testid="settings-appearance-mono-font-family"
      >
        <AppSelect
          class="w-[220px]"
          :model-value="mono"
          :options="monoOptions"
          searchable
          search-placeholder="Search fonts"
          aria-label="Monospace font"
          testid="settings-appearance-mono-font-family-select"
          @update:model-value="setAppMonoFontFamily"
        />
      </SettingsRow>
      <!-- At the chrome's own sizes rather than a display-size specimen: 11-13px
           on macOS antialiasing is where a face either holds up or does not. -->
      <div class="px-4 py-3.5" data-testid="settings-appearance-font-preview">
        <div class="rounded-lg border border-card bg-app px-3.5 py-3">
          <div class="text-[13.5px] font-semibold text-text">Review requested on hive-desktop</div>
          <p class="mt-1 text-[12px] leading-relaxed text-text-3">
            Handgloves — the quick brown fox jumps over the lazy dog.
          </p>
          <div class="mt-2.5 font-mono text-[11.5px] text-text-2">
            feat/user-selectable-fonts · +148 −37 · 0Il1 O0 {}[]()
          </div>
        </div>
      </div>
    </SettingsSection>
  </SettingsPage>
</template>
