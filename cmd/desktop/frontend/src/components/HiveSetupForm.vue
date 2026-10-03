<script setup lang="ts">
// First run is the only surface that writes the Hive config; Settings points
// at the file instead.
import { computed, ref, watch } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconFolder from '~icons/lucide/folder'
import IconFolderPlus from '~icons/lucide/folder-plus'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import IconX from '~icons/lucide/x'
import type {
  AgentOption,
  Profile,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/hiveconf/models'
import type { DraftWorkspace } from '../composables/useHiveSetup'
import AppCheckbox from './ui/AppCheckbox.vue'
import AppSelect from './ui/AppSelect.vue'

const props = defineProps<{
  agents: AgentOption[]
  selectedAgents: Set<string>
  workspaces: DraftWorkspace[]
  defaultAgent: string
  skipPermissions: boolean
  customProfiles: Profile[]
  busy?: boolean
  // The agent HIVE_DEFAULT_AGENT forces, when it is set. Shown because it
  // wins over the choice made here at load time, and a silent override is
  // how someone ends up convinced the picker is broken.
  defaultAgentOverride?: string
}>()

const emit = defineEmits<{
  toggleAgent: [agent: AgentOption, on: boolean]
  setDefaultAgent: [name: string]
  setSkipPermissions: [on: boolean]
  addWorkspace: []
  addWorkspacePath: [path: string]
  removeWorkspace: [path: string]
}>()

// Typing a path is the second way in, beside the native picker. It is not
// only for people who prefer the keyboard: the picker needs a GUI, and the
// headless build this app's e2e suite drives does not have one.
const typed = ref('')

function submitTyped(): void {
  const path = typed.value.trim()
  if (!path) return
  emit('addWorkspacePath', path)
}

// The input clears once the folder shows up in the list, not on submit: the
// parent checks the path asynchronously, and a typo should stay in the field
// beside its error rather than have to be typed again.
watch(
  () => props.workspaces,
  (workspaces) => {
    if (workspaces.some((w) => w.path === typed.value.trim())) typed.value = ''
  },
)

const defaultAgentOptions = computed(() =>
  [...props.selectedAgents].map((name) => ({
    value: name,
    label: props.agents.find((a) => a.name === name)?.label ?? name,
    hint: name,
  })),
)

function repoLabel(count: number): string {
  if (count === 1) return '1 repository'
  return `${count} repositories`
}
</script>

<template>
  <div class="flex flex-col gap-7">
    <section class="flex flex-col gap-3" data-testid="hive-setup-agents">
      <div>
        <h3 class="text-[13.5px] font-semibold text-text">Coding agent</h3>
        <p class="mt-1 text-[12.5px] leading-relaxed text-text-3">
          Pick the agents Hive can start a session with. Installed ones are marked; you can choose one you have not
          installed yet.
        </p>
      </div>

      <div class="grid grid-cols-2 gap-2 @[560px]/pane:grid-cols-3">
        <button
          v-for="agent in agents"
          :key="agent.name"
          type="button"
          class="flex items-center gap-2.5 rounded-[9px] border px-3 py-2.5 text-left transition-colors"
          :class="
            selectedAgents.has(agent.name)
              ? 'border-accent bg-accent-tint text-text'
              : 'border-strong bg-chip text-text-2 hover:border-accent/50'
          "
          :disabled="busy"
          :aria-pressed="selectedAgents.has(agent.name)"
          :data-testid="`hive-agent-${agent.name}`"
          @click="emit('toggleAgent', agent, !selectedAgents.has(agent.name))"
        >
          <span
            aria-hidden="true"
            class="flex size-4 shrink-0 items-center justify-center rounded-[5px] border"
            :class="selectedAgents.has(agent.name) ? 'border-accent bg-accent text-accent-contrast' : 'border-strong'"
            ><IconCheck v-if="selectedAgents.has(agent.name)" class="size-3"
          /></span>
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[13px] font-medium">{{ agent.label }}</span>
            <span class="block truncate font-mono text-[11px] text-text-4">{{ agent.name }}</span>
          </span>
          <span
            v-if="agent.installed"
            class="shrink-0 rounded-full bg-severity-success-tint px-1.5 py-0.5 text-[10px] font-medium text-severity-success"
            :data-testid="`hive-agent-installed-${agent.name}`"
            >installed</span
          >
        </button>
      </div>

      <p
        v-if="!agents.some((a) => a.installed)"
        class="text-[12px] leading-relaxed text-text-4"
        data-testid="hive-no-agents-installed"
      >
        None of these were found on your PATH. Pick the one you plan to use — Hive will find it once it is installed.
      </p>

      <div v-if="selectedAgents.size > 1" class="flex flex-col gap-2" data-testid="hive-default-agent">
        <span class="text-[12px] font-medium text-text-3">Start new sessions with</span>
        <AppSelect
          :model-value="defaultAgent"
          :options="defaultAgentOptions"
          :disabled="busy"
          aria-label="Start new sessions with"
          testid="hive-default-agent-select"
          @update:model-value="emit('setDefaultAgent', $event)"
        />
      </div>

      <p
        v-if="defaultAgentOverride"
        class="flex items-start gap-2 rounded-lg border border-border bg-severity-info-tint p-3 text-[12px] leading-relaxed text-text-2"
        data-testid="hive-default-agent-override"
      >
        <IconTriangleAlert class="mt-px size-3.5 shrink-0 text-severity-info" />
        <span>
          <span class="font-mono text-text">HIVE_DEFAULT_AGENT</span> is set to
          <span class="font-mono text-text">{{ defaultAgentOverride }}</span> in your shell, and it wins over the choice
          here. Unset it if you want this setting to take effect.
        </span>
      </p>

      <AppCheckbox
        :model-value="skipPermissions"
        :disabled="busy"
        label="Skip the agent's permission prompts"
        hint="Adds the flag that lets the agent act without asking — --dangerously-skip-permissions for Claude Code, and the equivalent for others that have one. Off is the safe default."
        testid="hive-skip-permissions"
        @update:model-value="emit('setSkipPermissions', $event)"
      />
    </section>

    <section class="flex flex-col gap-3" data-testid="hive-setup-workspaces">
      <div>
        <h3 class="text-[13.5px] font-semibold text-text">Where your repositories are</h3>
        <p class="mt-1 text-[12.5px] leading-relaxed text-text-3">
          Add the folders that <em>contain</em> your repositories — <span class="font-mono text-text-2">~/code</span>,
          not <span class="font-mono text-text-2">~/code/my-app</span>. Everything under them shows up in the new
          session picker. You can add more than one.
        </p>
      </div>

      <ul v-if="workspaces.length" class="flex flex-col gap-1.5" data-testid="hive-workspace-list">
        <li
          v-for="workspace in workspaces"
          :key="workspace.path"
          class="flex items-center gap-2.5 rounded-[9px] border border-strong bg-chip px-3 py-2.5"
          :data-testid="`hive-workspace-${workspace.path}`"
        >
          <IconFolder class="size-4 shrink-0 text-text-3" />
          <span class="min-w-0 flex-1">
            <span class="block truncate font-mono text-[12.5px] text-text">{{ workspace.path }}</span>
            <span class="block text-[11px]" :class="workspace.exists ? 'text-text-4' : 'text-kind-issue'">{{
              workspace.exists ? repoLabel(workspace.repos) : 'not found on this machine'
            }}</span>
          </span>
          <button
            type="button"
            class="shrink-0 cursor-pointer rounded-md p-1 text-text-4 hover:bg-raised hover:text-text"
            :disabled="busy"
            :aria-label="`Remove ${workspace.path}`"
            :data-testid="`hive-workspace-remove-${workspace.path}`"
            @click="emit('removeWorkspace', workspace.path)"
          >
            <IconX class="size-3.5" />
          </button>
        </li>
      </ul>

      <button
        type="button"
        class="flex cursor-pointer items-center justify-center gap-2 rounded-[9px] border border-dashed border-strong px-3 py-2.5 text-[12.5px] font-medium text-text-2 hover:border-accent hover:text-accent disabled:opacity-55"
        :disabled="busy"
        data-testid="hive-add-workspace"
        @click="emit('addWorkspace')"
      >
        <IconFolderPlus class="size-4" />
        {{ workspaces.length ? 'Add another folder' : 'Choose a folder' }}
      </button>

      <div class="flex items-center gap-2">
        <input
          v-model="typed"
          type="text"
          placeholder="or type a path, e.g. ~/code"
          autocomplete="off"
          spellcheck="false"
          :disabled="busy"
          aria-label="Add a repository folder by path"
          class="min-w-0 flex-1 rounded-lg border border-strong bg-app px-3 py-2 font-mono text-[12.5px] text-text outline-none placeholder:font-sans placeholder:text-text-4 focus:border-accent disabled:opacity-55"
          data-testid="hive-workspace-path"
          @keydown.enter.prevent="submitTyped"
        />
        <button
          type="button"
          class="shrink-0 cursor-pointer rounded-[7px] border border-strong px-3 py-2 text-[12.5px] font-medium text-text-2 hover:border-accent hover:text-accent disabled:opacity-55"
          :disabled="busy || !typed.trim()"
          data-testid="hive-workspace-path-add"
          @click="submitTyped"
        >
          Add
        </button>
      </div>
    </section>

    <p v-if="customProfiles.length" class="text-[12px] leading-relaxed text-text-4" data-testid="hive-custom-profiles">
      Your config also defines
      <span class="font-mono text-text-3">{{ customProfiles.map((p) => p.name).join(', ') }}</span
      >. {{ customProfiles.length === 1 ? 'It is' : 'They are' }} kept as written.
    </p>
  </div>
</template>
