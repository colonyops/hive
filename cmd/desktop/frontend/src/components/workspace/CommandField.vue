<script setup lang="ts">
// The launch command: a preset picker that fills the field, and Custom, which
// edits the Go template itself.
import { computed, markRaw, ref } from 'vue'
import IconPencil from '~icons/lucide/pencil'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import AppSelect, { type AppSelectOption } from '../ui/AppSelect.vue'
import SettingsSection from '../settings/SettingsSection.vue'
import { agentIcon } from '../../lib/agentIcon'
import { useAgentWorkspaces } from '../../stores/useAgentWorkspaces'

defineProps<{ busy?: boolean }>()
const command = defineModel<string>({ required: true })

const { presets } = useAgentWorkspaces()

const CUSTOM = '__custom__'

// DANGEROUS_FLAGS mirrors agentws's own list. It is duplicated rather than
// served because it only drives a warning: a flag missing here shows no
// banner, which is the same "not recognized" the Go side means, and the
// manifest's own danger flag still labels the saved row.
const DANGEROUS_FLAGS = [
  '--dangerously-skip-permissions',
  '--dangerously-bypass-approvals-and-sandbox',
  '--yolo',
  '--full-auto',
]

// Kept in step with agentws.LaunchData.
const TEMPLATE_FIELDS = [
  { field: '{{ .Dir }}', means: 'the workspace directory on disk' },
  { field: '{{ .MCPConfig }}', means: 'the MCP config Hive generates for this workspace' },
  { field: '{{ .SessionID }}', means: 'the id Hive minted for this chat' },
  { field: '{{ .Resume }}', means: 'true when reopening a chat, false on a new one' },
  {
    field: '{{ .Prompt }}',
    means: 'an optional opening message for a schedule or prompted launch; pass it through shq',
  },
  { field: 'shq', means: 'quotes a value for the shell — pipe every path through it' },
]

const dangerous = computed(() => DANGEROUS_FLAGS.some((flag) => command.value.includes(flag)))

// Without this the derived selection snaps back to a suggestion the moment the
// typed command matches one again.
const editing = ref(false)

const selection = computed<string>({
  get: () => (editing.value ? CUSTOM : (presets.value.find((p) => p.command === command.value)?.id ?? CUSTOM)),
  set(id) {
    if (id === CUSTOM) {
      editing.value = true
      return
    }
    const preset = presets.value.find((p) => p.id === id)
    if (!preset) return
    command.value = preset.command
    editing.value = false
  },
})

// A hive-seeded preset's label is its profile key, which is already the agent
// when the profile just runs that CLI.
const options = computed<AppSelectOption[]>(() => [
  ...presets.value.map((preset) => ({
    value: preset.id,
    label: preset.label === preset.agent ? preset.agent : `${preset.agent} · ${preset.label}`,
    hint: preset.command,
    icon: agentIcon(preset.agent),
  })),
  { value: CUSTOM, label: 'Custom', hint: 'Write the invocation yourself', icon: markRaw(IconPencil) },
])
</script>

<template>
  <SettingsSection
    title="Command"
    description="What a chat in this workspace launches. A suggestion fills the field; Custom edits the template itself."
    testid="agent-workspace-editor-command"
  >
    <div class="flex flex-col gap-1.5">
      <AppSelect
        v-model="selection"
        :options="options"
        searchable
        search-placeholder="Search commands…"
        aria-label="Command"
        testid="agent-workspace-editor-command-preset"
        :disabled="busy"
      />
      <p
        v-if="selection !== CUSTOM"
        class="truncate font-mono text-[11px] text-text-4"
        :title="command"
        data-testid="agent-workspace-editor-command-preview"
      >
        {{ command }}
      </p>
      <template v-else>
        <textarea
          v-model="command"
          rows="4"
          spellcheck="false"
          :disabled="busy"
          placeholder="claude --session-id {{ .SessionID }}"
          class="w-full resize-y rounded-lg border bg-app px-3 py-2.5 font-mono text-[12px] leading-relaxed text-text outline-none focus:border-accent"
          :class="dangerous ? 'border-severity-warning' : 'border-strong'"
          data-testid="agent-workspace-editor-command-input"
        />
        <div
          class="flex flex-col gap-1 rounded-lg border border-card px-3 py-2.5"
          data-testid="agent-workspace-editor-command-fields"
        >
          <p class="text-[11px] leading-relaxed text-text-3">
            Hive renders this as a Go template each time a chat starts, and fills in:
          </p>
          <dl class="flex flex-col gap-1">
            <div v-for="entry in TEMPLATE_FIELDS" :key="entry.field" class="flex flex-wrap items-baseline gap-x-2">
              <dt class="shrink-0 font-mono text-[11px] text-text-2">{{ entry.field }}</dt>
              <dd class="min-w-0 flex-1 text-[11px] leading-relaxed text-text-4">{{ entry.means }}</dd>
            </div>
          </dl>
        </div>
      </template>
      <p
        v-if="dangerous"
        class="flex items-start gap-1.5 text-[11.5px] leading-relaxed text-severity-warning"
        data-testid="agent-workspace-editor-command-danger"
      >
        <IconTriangleAlert class="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
        <span
          >This command bypasses the agent's permission prompts. Unattended, it can take any action your user account
          can, including through every enabled MCP server.</span
        >
      </p>
    </div>
  </SettingsSection>
</template>
