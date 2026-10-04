<script setup lang="ts">
// Chats settings: canvas presentation, the canvas server's address for agents
// outside a workspace, and where the area's workspaces live. A workspace's
// agent, launch command, and MCP servers belong to its manifest.
import { computed, onMounted } from 'vue'
import CanvasTypographyPreview from './settings/CanvasTypographyPreview.vue'
import InlineError from './ui/InlineError.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsPathRow from './settings/SettingsPathRow.vue'
import SettingsRow from './settings/SettingsRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import CopyButton from './ui/CopyButton.vue'
import SegmentedControl from './ui/SegmentedControl.vue'
import {
  canvasFontSizeLabels,
  canvasFontSizePx,
  canvasFontSizes,
  canvasLineSpacingLabels,
  canvasLineSpacings,
  useCanvasTypography,
  type CanvasFontSize,
  type CanvasLineSpacing,
} from '../stores/useCanvasTypography'
import { useSystemSettings } from '../composables/useSystemSettings'
import { useAgentWorkspaces } from '../stores/useAgentWorkspaces'

const { info, error: locationsError, refresh: refreshLocations, openPath, revealPath } = useSystemSettings()
const { fontSize, lineSpacing, setFontSize, setLineSpacing } = useCanvasTypography()

// The shipped catalogue entry carries the live address, so this never restates
// the port or the path.
const { mcpCatalogue, reloadMCPCatalogue } = useAgentWorkspaces()
const canvasServer = computed(() => mcpCatalogue.value.find((entry) => entry.shipped && entry.id === 'hive-canvas'))

const fontSizeOptions = canvasFontSizes.map((value) => ({
  value,
  label: `${canvasFontSizePx[value]}px`,
  title: canvasFontSizeLabels[value],
}))
const lineSpacingOptions = canvasLineSpacings.map((value) => ({
  value,
  label: canvasLineSpacingLabels[value],
}))

function onFontSizeChange(value: string): void {
  setFontSize(value as CanvasFontSize)
}

function onLineSpacingChange(value: string): void {
  setLineSpacing(value as CanvasLineSpacing)
}

onMounted(() => {
  void refreshLocations()
  void reloadMCPCatalogue()
})
</script>

<template>
  <SettingsPage testid="settings-agents">
    <InlineError v-if="locationsError" :message="locationsError" testid="agents-error" />

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
          @update:model-value="onFontSizeChange"
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
          @update:model-value="onLineSpacingChange"
        />
      </SettingsRow>
      <div class="px-4 py-3.5"><CanvasTypographyPreview /></div>
    </SettingsSection>

    <SettingsSection
      v-if="canvasServer"
      title="Canvas for every agent"
      description="A workspace that enables Hive Canvas is set up for you. Any other agent, such as the one a Code session runs, gets the canvas once you add this server to that agent's own MCP configuration. Hive does not edit it."
      boxed
    >
      <SettingsRow
        label="Server address"
        :hint="
          canvasServer.problem ||
          'Add it as an HTTP MCP server named hive-canvas. A canvas written from a Code session belongs to the session\'s repository.'
        "
        testid="settings-canvas-server"
      >
        <div v-if="canvasServer.command" class="flex min-w-0 items-center gap-2">
          <code
            class="min-w-0 truncate rounded bg-chip px-1.5 py-0.5 font-mono text-caption text-text-2"
            data-testid="settings-canvas-server-url"
            >{{ canvasServer.command }}</code
          >
          <CopyButton :text="canvasServer.command" size="xs" data-testid="settings-canvas-server-copy" />
        </div>
      </SettingsRow>
    </SettingsSection>

    <SettingsSection
      v-if="info"
      title="Workspaces"
      description="Where the Chats area keeps its workspace directories."
      boxed
    >
      <SettingsPathRow
        label="Workspace root"
        hint="Set agent_workspaces.dir in settings.yaml to move it — iCloud Drive and other synced folders are expected destinations. A new location applies after restart."
        icon="folder"
        tone="accent"
        :path="info.agentWorkspaces.path"
        :exists="info.agentWorkspaces.exists"
        testid="agents-workspace-root"
        @open="openPath(info.agentWorkspaces.path)"
        @reveal="revealPath(info.agentWorkspaces.path)"
      />
    </SettingsSection>
  </SettingsPage>
</template>
