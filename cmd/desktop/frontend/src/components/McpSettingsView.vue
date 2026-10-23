<script setup lang="ts">
// The MCP servers this app hosts, and how to point an agent at one. Hive wires
// a server into a Chats workspace that lists it. Every other agent, the one a
// Code session runs included, is the user's to configure, so this page is
// where the address and the setup for each agent are found
// (ADR a-code-session-s-canvases-belong-to-its-repository-and-live-in-the-hive-context-directory).
import { computed, onMounted, ref } from 'vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsRow from './settings/SettingsRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import CopyButton from '@hive/ui/components/ui/CopyButton.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import SegmentedControl from '@hive/ui/components/ui/SegmentedControl.vue'
import { mcpSetup, mcpSetupAgents, type McpSetupAgent } from '../lib/mcpSetup'
import { useAgentWorkspaces } from '../stores/useAgentWorkspaces'
import { AddDriveMCP } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/agentsservice'

// The drive first: it is the one an agent needs before it can show its work.
// hive-canvas is the same server at its older address, so it comes last.
const HOSTED_SERVERS = ['hive-drive', 'hive-desktop', 'hive-canvas']

// The shipped catalogue entry carries the live address, so this page never
// restates the port or a path.
const { checking, available, reason, mcpCatalogue, reloadMCPCatalogue } = useAgentWorkspaces()
const servers = computed(() =>
  HOSTED_SERVERS.flatMap((id) => mcpCatalogue.value.filter((entry) => entry.shipped && entry.id === id)),
)

const agent = ref<McpSetupAgent>('claude')
const addingDrive = ref(false)
const addDriveError = ref<string | null>(null)
const addDriveSuccess = ref(false)

function onAgentChange(value: string): void {
  agent.value = value as McpSetupAgent
  addDriveError.value = null
  addDriveSuccess.value = false
}

async function addDrive(): Promise<void> {
  addingDrive.value = true
  addDriveError.value = null
  addDriveSuccess.value = false
  try {
    await AddDriveMCP(agent.value)
    addDriveSuccess.value = true
  } catch (error) {
    addDriveError.value = error instanceof Error ? error.message : String(error)
  } finally {
    addingDrive.value = false
  }
}

onMounted(() => {
  void reloadMCPCatalogue()
})
</script>

<template>
  <SettingsPage testid="settings-mcp">
    <SettingsSection
      title="Connect an agent"
      description="Hive hosts these servers on this machine. A Chats workspace that enables one is set up for you. Add Hive Drive to Claude Code or Codex here for Code sessions; other agents can use the setup details below."
      boxed
    >
      <SettingsRow label="Agent" hint="The setup under each server is written for this agent.">
        <SegmentedControl
          :model-value="agent"
          :options="mcpSetupAgents"
          aria-label="Agent to set up"
          testid="settings-mcp-agent"
          @update:model-value="onAgentChange"
        />
      </SettingsRow>
    </SettingsSection>

    <SettingsSection
      v-for="server in servers"
      :key="server.id"
      :title="server.title"
      :description="server.description"
      :testid="`settings-mcp-${server.id}`"
      boxed
    >
      <div v-if="server.problem || !server.command" class="px-4 py-3.5">
        <InlineError
          variant="line"
          :message="server.problem || 'This server has no address.'"
          :testid="`settings-mcp-${server.id}-problem`"
        />
      </div>
      <template v-else>
        <SettingsRow
          label="Address"
          hint="An HTTP MCP server on this machine. The address stays the same across restarts."
        >
          <div class="flex min-w-0 items-center gap-2">
            <code
              class="min-w-0 truncate rounded-sm bg-chip px-1.5 py-0.5 font-mono text-caption text-text-2"
              :data-testid="`settings-mcp-${server.id}-url`"
              >{{ server.command }}</code
            >
            <CopyButton :text="server.command" size="xs" :data-testid="`settings-mcp-${server.id}-url-copy`" />
          </div>
        </SettingsRow>
        <div class="flex flex-col gap-2 px-4 py-3.5">
          <div class="flex items-center justify-between gap-3">
            <p class="text-small leading-relaxed text-text-3">{{ mcpSetup(agent, server.id, server.command).hint }}</p>
            <CopyButton
              :text="mcpSetup(agent, server.id, server.command).text"
              size="xs"
              :data-testid="`settings-mcp-${server.id}-setup-copy`"
            />
          </div>
          <pre
            class="hive-scroll overflow-x-auto rounded-lg border border-card bg-app p-3 font-mono text-caption leading-relaxed text-text-2"
            :data-testid="`settings-mcp-${server.id}-setup`"
            >{{ mcpSetup(agent, server.id, server.command).text }}</pre>
          <div v-if="server.id === 'hive-drive'" class="flex flex-wrap items-center gap-3">
            <BaseButton
              size="sm"
              :busy="addingDrive"
              :disabled="agent === 'json'"
              data-testid="settings-mcp-hive-drive-add"
              @click="addDrive"
            >
              Add to {{ agent === 'claude' ? 'Claude Code' : 'Codex' }}
            </BaseButton>
            <span v-if="addDriveSuccess" role="status" class="text-small text-text-3">
              Added to {{ agent === 'claude' ? 'Claude Code' : 'Codex' }}. Restart its sessions to connect.
            </span>
            <InlineError
              v-if="addDriveError"
              variant="line"
              :message="addDriveError"
              testid="settings-mcp-hive-drive-add-error"
            />
          </div>
        </div>
      </template>
    </SettingsSection>

    <EmptyState v-if="!checking && !servers.length" variant="boxed" data-testid="settings-mcp-unavailable">
      {{ available ? 'No server is available.' : reason || 'The servers are not available in this build.' }}
    </EmptyState>
  </SettingsPage>
</template>
