<script setup lang="ts">
// Drive reading settings apply to every document, in the pane and the full
// page, in Chats and in Code alike. They keep their canvas_* keys in
// settings.yaml so existing values carry over.
import CanvasTypographyPreview from './settings/CanvasTypographyPreview.vue'
import ReviewLabelsEditor from './settings/ReviewLabelsEditor.vue'
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
  <SettingsPage testid="settings-drive">
    <SettingsSection
      title="Typography"
      description="How document text is drawn. Changes apply to every open document immediately."
      boxed
    >
      <SettingsRow
        label="Font size"
        hint="Applies to headings, body text, cards, and diagrams in every markdown and canvas document."
      >
        <SegmentedControl
          :model-value="fontSize"
          :options="fontSizeOptions"
          aria-label="Document text size"
          testid="settings-drive-font-size"
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
          aria-label="Document line spacing"
          testid="settings-drive-line-spacing"
          @update:model-value="setLineSpacing($event as CanvasLineSpacing)"
        />
      </SettingsRow>
      <div class="px-4 py-3.5"><CanvasTypographyPreview /></div>
    </SettingsSection>

    <SettingsSection title="Full page" description="How the full-page view lays out a document." boxed>
      <SettingsRow
        label="Text width"
        hint="Narrow keeps lines at a comfortable reading length. Wide fits HTML laid out in columns. Full runs to the edges of the view."
      >
        <SegmentedControl
          :model-value="pageWidth"
          :options="pageWidthOptions"
          aria-label="Full-page text width"
          testid="settings-drive-page-width"
          @update:model-value="setPageWidth($event as CanvasPageWidth)"
        />
      </SettingsRow>
    </SettingsSection>

    <SettingsSection
      title="Comment labels"
      description="Preset comments the comment box applies with keys 1 to 4, in this order. Up to four."
      boxed
    >
      <ReviewLabelsEditor />
    </SettingsSection>
  </SettingsPage>
</template>
