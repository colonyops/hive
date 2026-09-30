<script setup lang="ts">
import { computed } from 'vue'
import { useCanvasTypography } from '../../composables/useCanvasTypography'

const { fontSizePx, lineHeight } = useCanvasTypography()
const previewStyle = computed(() => ({
  '--hv-font-size': `${fontSizePx.value}px`,
  '--hv-line-height': String(lineHeight.value),
}))
</script>

<template>
  <div data-testid="settings-canvas-preview">
    <p class="mb-1.5 text-[12.5px] text-text-2">Preview</p>
    <div class="rounded-lg border border-border bg-app p-4">
      <div class="hv-html text-text-2" :style="previewStyle" data-testid="settings-canvas-preview-content">
        <h2>Project plan</h2>
        <p>Ship the reader controls with a clear default and enough range for a laptop or a large display.</p>
        <ul>
          <li>Keep reports comfortable to scan.</li>
          <li>Leave copied and saved content unchanged.</li>
        </ul>
        <div class="hv-row">
          <span class="hv-badge hv-success">Ready</span>
          <span class="hv-muted">Updated just now</span>
        </div>
      </div>
    </div>
  </div>
</template>
