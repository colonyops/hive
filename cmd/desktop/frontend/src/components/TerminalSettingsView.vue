<script setup lang="ts">
// Terminal settings: everything that governs a terminal wherever one is drawn
// — the Code tab, the pop-up panel, and the Agents area's chats.
import { computed, defineAsyncComponent, onMounted } from 'vue'
import AppSelect from './AppSelect.vue'
import AppSwitch from './AppSwitch.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsRow from './settings/SettingsRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import SettingsSegmented from './settings/SettingsSegmented.vue'
import SettingsStepper from './settings/SettingsStepper.vue'
import {
  maxTerminalFontSizePx,
  minTerminalFontSizePx,
  setTerminalFontFamily,
  setTerminalFontSize,
  setTerminalFontWeight,
  setTerminalFontWeightBold,
  setTerminalLetterSpacing,
  setTerminalLineHeight,
  terminalFontSizeStepPx,
  terminalFontWeightLabels,
  terminalFontWeights,
  terminalLetterSpacings,
  terminalLineHeights,
  useTerminalFont,
  type TerminalFontWeight,
  type TerminalLetterSpacing,
  type TerminalLineHeight,
} from '../composables/useTerminalFont'
import { TERMINAL_FONT } from '../lib/terminalFaces'
import { loadInstalledFonts, useInstalledFonts } from '../composables/useInstalledFonts'
import { setTerminalPoolSize, terminalPoolSizes, useTerminalPoolSize } from '../composables/useTerminalPoolSize'
import { setTerminalShowStatusBar, useTerminalStatusBar } from '../composables/useTerminalStatusBar'
import { setTerminalShowWindows, useTerminalShowWindows } from '../composables/useTerminalShowWindows'

// Async so xterm and its addons stay on the terminal chunk rather than joining
// the bundle everyone who opens any settings pane pays for.
const TerminalPreview = defineAsyncComponent(() => import('./settings/TerminalPreview.vue'))

const {
  px: fontSizePx,
  selectedFamily: fontFamily,
  weight: fontWeight,
  weightBold: fontWeightBold,
  lineHeight,
  letterSpacing,
} = useTerminalFont()
const { monospace: fontFamilies } = useInstalledFonts()
const { showWindows } = useTerminalShowWindows()
const { showStatusBar } = useTerminalStatusBar()
const { poolSize } = useTerminalPoolSize()

// The bundled face leads the list whether or not it is also installed
// system-wide, so the shipped default is always the first thing offered.
const fontFamilyOptions = computed(() => [
  { value: TERMINAL_FONT, label: `${TERMINAL_FONT} · bundled` },
  ...fontFamilies.value
    .filter((family) => family !== TERMINAL_FONT)
    .map((family) => ({ value: family, label: family })),
])
const fontWeightOptions = terminalFontWeights.map((value) => ({
  value: String(value),
  label: terminalFontWeightLabels[value],
}))
const lineHeightOptions = terminalLineHeights.map((value) => ({
  value: String(value),
  label: value.toFixed(1),
}))
const letterSpacingOptions = terminalLetterSpacings.map((value) => ({
  value: String(value),
  label: value === 0 ? 'None' : `+${value}`,
}))
const poolSizeOptions = terminalPoolSizes.map((value) => ({ value: String(value), label: String(value) }))

function onFontWeightChange(value: string): void {
  setTerminalFontWeight(Number(value) as TerminalFontWeight)
}

function onFontWeightBoldChange(value: string): void {
  setTerminalFontWeightBold(Number(value) as TerminalFontWeight)
}

function onLineHeightChange(value: string): void {
  setTerminalLineHeight(Number(value) as TerminalLineHeight)
}

function onLetterSpacingChange(value: string): void {
  setTerminalLetterSpacing(Number(value) as TerminalLetterSpacing)
}

function onPoolSizeChange(value: string): void {
  setTerminalPoolSize(Number(value))
}

// Scanning every font on the machine is not worth doing until this pane is the
// one on screen.
onMounted(() => {
  loadInstalledFonts()
})
</script>

<template>
  <SettingsPage testid="settings-terminal">
    <SettingsSection
      title="Typography"
      description="How terminal text is drawn, everywhere one appears. Changes apply to open terminals immediately."
      boxed
    >
      <SettingsRow
        label="Font"
        hint="Monospace families installed on this machine. The powerline and devicon glyphs agent TUIs draw with come from a bundled symbol face, so a family that lacks them still renders them."
        testid="settings-terminal-font-family"
      >
        <AppSelect
          class="w-[220px]"
          :model-value="fontFamily"
          :options="fontFamilyOptions"
          searchable
          search-placeholder="Search fonts"
          aria-label="Terminal font"
          testid="settings-terminal-font-family-select"
          @update:model-value="setTerminalFontFamily"
        />
      </SettingsRow>
      <SettingsRow
        label="Font size"
        hint="Applies immediately to open terminals; tmux re-fits their grid. The text size shortcuts step it by the same amount from a terminal."
      >
        <SettingsStepper
          :model-value="fontSizePx"
          :display="`${fontSizePx}px`"
          :min="minTerminalFontSizePx"
          :max="maxTerminalFontSizePx"
          :step="terminalFontSizeStepPx"
          aria-label="Font size"
          testid="settings-terminal-font-size"
          @update:model-value="setTerminalFontSize"
        />
      </SettingsRow>
      <SettingsRow
        label="Font weight"
        hint="The weight normal text draws at. A family that ships fewer weights renders the nearest one it has."
      >
        <SettingsSegmented
          :model-value="String(fontWeight)"
          :options="fontWeightOptions"
          aria-label="Font weight"
          testid="settings-terminal-font-weight"
          @update:model-value="onFontWeightChange"
        />
      </SettingsRow>
      <SettingsRow label="Bold weight" hint="The weight bold text draws at.">
        <SettingsSegmented
          :model-value="String(fontWeightBold)"
          :options="fontWeightOptions"
          aria-label="Bold weight"
          testid="settings-terminal-font-weight-bold"
          @update:model-value="onFontWeightBoldChange"
        />
      </SettingsRow>
      <SettingsRow
        label="Line height"
        hint="Multiplies the row height. Taller rows are easier to scan; each one costs a row of grid in the same pane."
      >
        <SettingsSegmented
          :model-value="String(lineHeight)"
          :options="lineHeightOptions"
          aria-label="Line height"
          testid="settings-terminal-line-height"
          @update:model-value="onLineHeightChange"
        />
      </SettingsRow>
      <SettingsRow
        label="Letter spacing"
        hint="Extra tracking in device pixels — half a point per step on a Retina display. Wider cells fit fewer columns."
      >
        <SettingsSegmented
          :model-value="String(letterSpacing)"
          :options="letterSpacingOptions"
          aria-label="Letter spacing"
          testid="settings-terminal-letter-spacing"
          @update:model-value="onLetterSpacingChange"
        />
      </SettingsRow>
      <div class="px-4 py-3.5"><TerminalPreview /></div>
    </SettingsSection>

    <SettingsSection
      title="Behaviour"
      description="What the terminal keeps on hand while you work."
      boxed
    >
      <SettingsRow
        label="Always show windows"
        hint="List every active session's windows in the session tree, not just the attached one's."
      >
        <AppSwitch
          :model-value="showWindows"
          aria-label="Always show windows"
          testid="settings-terminal-show-windows"
          @update:model-value="setTerminalShowWindows"
        />
      </SettingsRow>
      <SettingsRow
        label="Session status bar"
        hint="Show the attached session's directory, branch and pull request above the terminal, with buttons to open its checkout."
      >
        <AppSwitch
          :model-value="showStatusBar"
          aria-label="Session status bar"
          testid="settings-terminal-show-status-bar"
          @update:model-value="setTerminalShowStatusBar"
        />
      </SettingsRow>
      <SettingsRow
        label="Warm sessions"
        hint="Sessions kept attached in the background so switching back is instant. Each holds a tmux client, its stream, and its terminals."
      >
        <SettingsSegmented
          :model-value="String(poolSize)"
          :options="poolSizeOptions"
          aria-label="Warm sessions"
          testid="settings-terminal-pool-size"
          @update:model-value="onPoolSizeChange"
        />
      </SettingsRow>
    </SettingsSection>
  </SettingsPage>
</template>
