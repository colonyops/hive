<script setup lang="ts">
// Canvas settings apply to every canvas, in Chats and in Code alike.
import CanvasTypographyPreview from './settings/CanvasTypographyPreview.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsRow from './settings/SettingsRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import SegmentedControl from './ui/SegmentedControl.vue'
import {
  canvasFontSizeLabels,
  canvasFontSizePx,
  canvasFontSizes,
  canvasLineSpacingLabels,
  canvasLineSpacings,
  canvasPageWidthLabels,
  canvasPageWidths,
  useCanvasSettings,
  type CanvasFontSize,
  type CanvasLineSpacing,
  type CanvasPageWidth,
} from '../stores/useCanvasSettings'

const { fontSize, lineSpacing, pageWidth, setFontSize, setLineSpacing, setPageWidth } = useCanvasSettings()

const fontSizeOptions = canvasFontSizes.map((value) => ({
  value,
  label: `${canvasFontSizePx[value]}px`,
  title: canvasFontSizeLabels[value],
}))
const lineSpacingOptions = canvasLineSpacings.map((value) => ({
  value,
  label: canvasLineSpacingLabels[value],
}))
const pageWidthOptions = canvasPageWidths.map((value) => ({
  value,
  label: canvasPageWidthLabels[value],
}))
</script>

<template>
  <SettingsPage testid="settings-canvas">
    <SettingsSection
      title="Typography"
      description="How canvas text is drawn. Changes apply to every open canvas immediately."
      boxed
    >
      <SettingsRow label="Font size" hint="Applies to headings, body text, cards, and diagrams in every canvas.">
        <SegmentedControl
          :model-value="fontSize"
          :options="fontSizeOptions"
          aria-label="Canvas text size"
          testid="settings-canvas-font-size"
          @update:model-value="setFontSize($event as CanvasFontSize)"
        />
      </SettingsRow>
      <SettingsRow
        label="Line spacing"
        hint="Changes the vertical rhythm of Markdown and HTML. Copy and save keep the original content."
      >
        <SegmentedControl
          :model-value="lineSpacing"
          :options="lineSpacingOptions"
          aria-label="Canvas line spacing"
          testid="settings-canvas-line-spacing"
          @update:model-value="setLineSpacing($event as CanvasLineSpacing)"
        />
      </SettingsRow>
      <div class="px-4 py-3.5"><CanvasTypographyPreview /></div>
    </SettingsSection>

    <SettingsSection title="Full page" description="How the full-page view lays out a canvas." boxed>
      <SettingsRow
        label="Text width"
        hint="Narrow keeps lines at a comfortable reading length. Wide fits HTML laid out in columns. Full runs to the edges of the view."
      >
        <SegmentedControl
          :model-value="pageWidth"
          :options="pageWidthOptions"
          aria-label="Canvas full-page text width"
          testid="settings-canvas-page-width"
          @update:model-value="setPageWidth($event as CanvasPageWidth)"
        />
      </SettingsRow>
    </SettingsSection>
  </SettingsPage>
</template>
