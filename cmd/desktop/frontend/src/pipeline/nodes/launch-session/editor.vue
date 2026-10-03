<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { SessionLaunchOptions } from '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type { SessionLaunchRepository } from '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import RepositorySelect from '../../../components/RepositorySelect.vue'
import FormField from '../../../components/ui/FormField.vue'
import { SelectField, TextField, TextareaField } from '../../fields'
import { ITEM_REMOTE, repoMode, type Config, type RepoMode } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

const modeOptions: { value: RepoMode; label: string }[] = [
  { value: 'item', label: "The item's repository" },
  { value: 'configured', label: 'A fixed repository' },
  { value: 'template', label: 'A template' },
]

// Only re-derive the mode for outside changes: a template being typed has no
// `{{` yet and would flip to the fixed mode mid-keystroke.
const mode = ref<RepoMode>('configured')
let emittedRepo: string | undefined
watch(
  () => props.config.repo,
  (repo) => {
    if (repo !== emittedRepo && repo.trim()) mode.value = repoMode(repo)
  },
  { immediate: true },
)

const repositories = ref<SessionLaunchRepository[]>([])
const agents = ref<string[]>([])
onMounted(async () => {
  try {
    const options = await SessionLaunchOptions()
    repositories.value = options?.repositories ?? []
    agents.value = options?.agents ?? []
  } catch (err) {
    console.warn('Unable to load session launch options', err)
  }
})

const agentOptions = computed(() => {
  const rows = [{ value: '', label: 'Default agent' }, ...agents.value.map((key) => ({ value: key, label: key }))]
  const stored = props.config.agent
  if (stored && !agents.value.includes(stored)) rows.push({ value: stored, label: `${stored} (not found)` })
  return rows
})

function setMode(next: string) {
  const value = next as RepoMode
  mode.value = value
  if (value === 'item') set('repo', ITEM_REMOTE)
  else if (value === 'configured') set('repo', '')
}

// validate() prefixes each message with its field's key.
function fieldError(key: keyof Config): string | undefined {
  return props.errors?.find((message) => message.startsWith(`${key} `))
}

function set<K extends keyof Config>(key: K, value: Config[K]) {
  if (key === 'repo') emittedRepo = value
  emit('update:config', { ...props.config, [key]: value })
}
</script>

<template>
  <div class="flex flex-col gap-4 text-body leading-relaxed" data-testid="launch-session-node-editor">
    <p class="text-text-2">
      Each item arriving here starts a Hive coding session, once per item. The item's detail view links the session.
    </p>

    <SelectField
      label="Repository"
      :model-value="mode"
      :options="modeOptions"
      testid="launch-session-node-editor-repo-mode"
      @update:model-value="setMode"
    />

    <FormField
      v-if="mode === 'item'"
      hint="Clones the repository the item belongs to, such as the pull request's. An item that names no repository fails the launch."
      :error="fieldError('repo')"
      testid="launch-session-node-editor-repo"
    />
    <FormField
      v-else-if="mode === 'configured'"
      hint="Every item launches in this repository. Pick a known checkout or type a remote URL."
      :error="fieldError('repo')"
      testid="launch-session-node-editor-repo"
    >
      <RepositorySelect
        :model-value="config.repo"
        :repositories="repositories"
        testid="launch-session-node-editor-repo"
        @update:model-value="set('repo', $event)"
      />
    </FormField>
    <TextField
      v-else
      :model-value="config.repo"
      placeholder="https://github.com/{{ .Payload.repo }}.git"
      hint="Go template rendered over the item, producing a remote URL. {{ .ItemRemote }} is the item's own repository."
      monospace
      :error="fieldError('repo')"
      testid="launch-session-node-editor-repo"
      @update:model-value="set('repo', $event)"
    />

    <SelectField
      label="Agent"
      :model-value="config.agent ?? ''"
      :options="agentOptions"
      hint="The Hive agent profile the session runs."
      testid="launch-session-node-editor-agent"
      @update:model-value="set('agent', $event || undefined)"
    />

    <TextField
      label="Session name"
      :model-value="config.sessionName ?? ''"
      placeholder="review-{{ .Payload.num }}"
      hint="Go template, turned into a valid session name. Leave empty to derive one from the node and the item."
      monospace
      testid="launch-session-node-editor-session-name"
      @update:model-value="set('sessionName', $event || undefined)"
    />

    <TextareaField
      label="Prompt"
      :model-value="config.prompt"
      placeholder="Review pull request #{{ .Payload.num }}"
      hint="Go template rendered over the item. Fence outside content with untrustedStart and untrustedEnd."
      monospace
      :error="fieldError('prompt')"
      testid="launch-session-node-editor-prompt"
      @update:model-value="set('prompt', $event)"
    />
  </div>
</template>
