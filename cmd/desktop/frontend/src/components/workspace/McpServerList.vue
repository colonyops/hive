<script setup lang="ts">
// The shared library in mcps.yaml, with this workspace's own switch on each
// row. Removing a server or importing some edits the library itself, so those
// write straight away; the switches wait for the workspace's Save.
import { computed, onMounted, ref } from 'vue'
import IconPlus from '~icons/lucide/plus'
import IconTrash2 from '~icons/lucide/trash-2'
import IconButton from '../ui/IconButton.vue'
import AppSwitch from '../ui/AppSwitch.vue'
import BaseBadge from '../ui/BaseBadge.vue'
import BaseButton from '../ui/BaseButton.vue'
import EmptyState from '../ui/EmptyState.vue'
import InlineError from '../ui/InlineError.vue'
import SettingsSection from '../settings/SettingsSection.vue'
import ListFooterButton from './ListFooterButton.vue'
import { CodeField } from '../../pipeline/fields'
import { useAgentWorkspaces } from '../../stores/useAgentWorkspaces'

defineProps<{ busy?: boolean }>()
const selected = defineModel<string[]>({ required: true })

const { mcpCatalogue, reloadMCPCatalogue, importMCPServers, removeMCPServer } = useAgentWorkspaces()

onMounted(() => void reloadMCPCatalogue())

// A declared id the catalogue no longer resolves still rows, so a
// hand-authored entry that went missing is visible and removable rather than
// silently kept.
const rows = computed(() => {
  const known = new Set(mcpCatalogue.value.map((entry) => entry.id))
  return [
    ...mcpCatalogue.value.map((entry) => ({ ...entry, title: entry.title || entry.id, missing: false })),
    ...selected.value
      .filter((id) => !known.has(id))
      .map((id) => ({
        id,
        title: id,
        command: '',
        problem: '',
        shipped: false,
        stability: '',
        shadows: '',
        missing: true,
      })),
  ]
})

function enabled(id: string): boolean {
  return selected.value.includes(id)
}

function toggle(id: string): void {
  selected.value = enabled(id) ? selected.value.filter((x) => x !== id) : [...selected.value, id]
}

const error = ref('')

async function remove(id: string): Promise<void> {
  error.value = ''
  try {
    await removeMCPServer(id)
    selected.value = selected.value.filter((x) => x !== id)
  } catch (failure) {
    error.value = failure instanceof Error ? failure.message : 'The server could not be removed.'
  }
}

const importOpen = ref(false)
const importText = ref('')
const importBusy = ref(false)
const importPlaceholder = '{"mcpServers": {"my-server": {"command": "npx", "args": ["-y", "…"]}}}'

function formatImportJSON(): void {
  error.value = ''
  try {
    importText.value = JSON.stringify(JSON.parse(importText.value), null, 2)
  } catch (failure) {
    error.value = failure instanceof Error ? `Not valid JSON: ${failure.message}` : 'Not valid JSON.'
  }
}

async function submitImport(): Promise<void> {
  if (!importText.value.trim() || importBusy.value) return
  importBusy.value = true
  error.value = ''
  try {
    const added = await importMCPServers(importText.value)
    selected.value = [...new Set([...selected.value, ...added])]
    importText.value = ''
    importOpen.value = false
  } catch (failure) {
    error.value = failure instanceof Error ? failure.message : 'The pasted configuration could not be imported.'
  } finally {
    importBusy.value = false
  }
}
</script>

<template>
  <SettingsSection
    title="MCP servers"
    description="The shared library in mcps.yaml; each switch is this workspace's own."
    testid="agent-workspace-editor-mcps"
  >
    <div class="divide-y divide-row overflow-hidden rounded-xl border border-card bg-raised">
      <div v-for="row in rows" :key="row.id" class="flex items-start gap-3 px-4 py-3.5">
        <AppSwitch
          class="mt-0.5"
          :model-value="enabled(row.id)"
          :aria-label="`Enable ${row.title}`"
          :disabled="busy"
          :testid="`agent-workspace-editor-mcp-${row.id}`"
          @update:model-value="toggle(row.id)"
        />
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <span class="truncate text-body font-semibold" :class="enabled(row.id) ? 'text-text' : 'text-text-2'">{{
              row.title
            }}</span>
            <BaseBadge
              :tone="row.shipped ? 'neutral' : 'accent'"
              variant="pill"
              class="shrink-0 px-2 py-0.5 text-micro font-semibold uppercase"
              >{{ row.shipped ? row.stability : 'custom' }}</BaseBadge
            >
            <span v-if="row.shadows" class="shrink-0 text-micro text-severity-warning">replaces shipped</span>
          </div>
          <div v-if="row.command" class="mt-1 truncate font-mono text-caption text-text-4" :title="row.command">
            {{ row.command }}
          </div>
          <div v-if="row.problem" class="mt-1 text-caption text-severity-warning">{{ row.problem }}</div>
          <div v-if="row.missing" class="mt-1 text-caption text-severity-warning">
            not in the catalogue — enabled ids without an entry are skipped at launch
          </div>
        </div>
        <IconButton
          v-if="!row.shipped && !row.missing"
          :label="`Remove ${row.title}`"
          :tooltip="`Remove ${row.title} from mcps.yaml (every workspace loses it)`"
          :icon="IconTrash2"
          size="lg"
          tone="danger"
          :disabled="busy"
          :data-testid="`agent-workspace-editor-mcp-remove-${row.id}`"
          @click="remove(row.id)"
        />
      </div>
      <EmptyState v-if="!rows.length" variant="inline" class="px-4 py-3.5" message="No servers in mcps.yaml yet." />
      <div v-if="importOpen" class="flex flex-col gap-2 px-4 py-3.5">
        <CodeField
          v-model="importText"
          :rows="8"
          :placeholder="importPlaceholder"
          testid="agent-workspace-editor-mcp-import-text"
        />
        <div class="flex items-center gap-2">
          <BaseButton
            size="sm"
            :busy="importBusy"
            :disabled="!importText.trim()"
            data-testid="agent-workspace-editor-mcp-import-submit"
            @click="submitImport"
            >Add servers</BaseButton
          >
          <BaseButton
            variant="secondary"
            size="sm"
            :disabled="importBusy || !importText.trim()"
            data-testid="agent-workspace-editor-mcp-import-format"
            @click="formatImportJSON"
            >Format JSON</BaseButton
          >
          <BaseButton variant="secondary" size="sm" :disabled="importBusy" @click="importOpen = false"
            >Cancel</BaseButton
          >
        </div>
        <span class="text-xs text-text-4"
          >Pasted servers land in mcps.yaml — the library every workspace picks from — and switch on here.</span
        >
      </div>
      <ListFooterButton
        v-else
        :disabled="busy"
        data-testid="agent-workspace-editor-mcp-import"
        @click="importOpen = true"
      >
        <IconPlus class="size-3.5" />Add servers from JSON…
      </ListFooterButton>
    </div>
    <InlineError v-if="error" testid="agent-workspace-editor-mcp-error" variant="line" :message="error" />
  </SettingsSection>
</template>
