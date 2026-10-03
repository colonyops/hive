<script setup lang="ts">
import InlineError from './ui/InlineError.vue'
import { computed, nextTick, onMounted, ref } from 'vue'
import IconPlay from '~icons/lucide/play'
import IconX from '~icons/lucide/x'
import BaseButton from './ui/BaseButton.vue'
import AppCheckbox from './ui/AppCheckbox.vue'
import AppSelect, { type AppSelectOption } from './ui/AppSelect.vue'
import ActionInputsEditor from './ActionInputsEditor.vue'
import AppliesToField from './AppliesToField.vue'
import DrawerSheet from './ui/DrawerSheet.vue'
import { SelectField, TextareaField, TextField } from '../pipeline/fields'
import type { EditableAction } from '../composables/useActionsSettings'
import type { SessionLaunchWorkspace } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import { seedRef } from '../lib/seedRef'

const props = withDefaults(
  defineProps<{
    isNew: boolean
    busy?: boolean
    error?: string | null
    returnFocusTo?: HTMLElement | null
    knownTypes?: string[]
    workspaces?: SessionLaunchWorkspace[]
  }>(),
  { knownTypes: () => [], workspaces: () => [] },
)
const action = defineModel<EditableAction>('action', { required: true })
const emit = defineEmits<{ save: []; cancel: [] }>()
const idRef = ref<{ focus: () => void } | null>(null)
const labelRef = ref<{ focus: () => void } | null>(null)
const appliesField = ref<{ flush: () => void } | null>(null)
const closeRef = ref<HTMLButtonElement | null>(null)
const validationError = ref<string | null>(null)
const launchTarget = seedRef<'interactive' | 'repository' | 'workspace'>(() =>
  action.value.launch?.workspace ? 'workspace' : action.value.launch?.repoTemplate ? 'repository' : 'interactive',
)

const typeOptions = [
  { value: 'launch-session', label: 'Launch session' },
  { value: 'shell', label: 'Shell' },
  { value: 'publish-message', label: 'Publish message' },
  { value: 'clipboard', label: 'Copy to clipboard' },
]

// Which surfaces offer this action. A launch-session action creates a new
// session, so the terminal's row menus — which have no New Session form — are
// not among them; the backend refuses the combination outright.
const targetOptions = [
  { value: 'item', label: 'Feed item' },
  { value: 'session', label: 'Terminal session' },
  { value: 'window', label: 'Terminal window' },
]
const launchTargetOptions = [
  { value: 'interactive', label: 'Choose when run' },
  { value: 'repository', label: 'Repository' },
  { value: 'workspace', label: 'Agent workspace' },
]
const workspaceOptions = computed<AppSelectOption[]>(() =>
  props.workspaces.map((workspace) => ({
    value: workspace.dir,
    label: workspace.name || workspace.dir,
    hint: workspace.supportsPrompt ? workspace.dir : `${workspace.dir} · command does not accept a prompt`,
    disabled: !workspace.supportsPrompt,
  })),
)
const terminalTargetsAllowed = computed(() => action.value.type !== 'launch-session')
function hasTarget(value: string): boolean {
  return (action.value.targets ?? []).includes(value)
}
function setTarget(value: string, on: boolean): void {
  const next = (action.value.targets ?? []).filter((target) => target !== value)
  if (on) next.push(value)
  // An action offered nowhere is unreachable rather than merely quiet, so the
  // item surface is what an emptied set falls back to.
  action.value.targets = next.length
    ? targetOptions.map((option) => option.value).filter((option) => next.includes(option))
    : ['item']
}

function setLaunchTarget(value: string): void {
  if (!action.value.launch || !['interactive', 'repository', 'workspace'].includes(value)) return
  launchTarget.value = value as typeof launchTarget.value
  action.value.launch.repoTemplate = ''
  action.value.launch.workspace = ''
  if (value === 'workspace') {
    action.value.launch.agent = ''
    action.value.launch.postHook = ''
    action.value.launch.postHookTimeout = ''
  }
}

function setType(value: string): void {
  action.value.type = value
  action.value.launch = undefined
  action.value.shell = undefined
  action.value.message = undefined
  action.value.clipboard = undefined
  if (value === 'launch-session') {
    action.value.launch = { promptTemplate: '', repoTemplate: '', workspace: '' }
    launchTarget.value = 'interactive'
    action.value.targets = ['item']
  } else if (value === 'shell') action.value.shell = { commandTemplate: '' }
  else if (value === 'publish-message') action.value.message = { topic: '', messageTemplate: '' }
  else if (value === 'clipboard') action.value.clipboard = { textTemplate: '' }
}
function envText(): string {
  return Object.entries(action.value.shell?.env ?? {})
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => `${key}=${value ?? ''}`)
    .join('\n')
}
function setEnv(text: string): void {
  if (!action.value.shell) return
  const env: Record<string, string> = {}
  for (const line of text.split('\n')) {
    const [key, ...value] = line.split('=')
    if (key.trim()) env[key.trim()] = value.join('=')
  }
  action.value.shell.env = env
}
function save(): void {
  appliesField.value?.flush()
  if (!action.value.id.trim() || !action.value.label.trim()) {
    validationError.value = 'ID and label are required.'
    return
  }
  validationError.value = null
  emit('save')
}
function cancel(): void {
  if (!props.busy) emit('cancel')
}
onMounted(async () => {
  await nextTick()
  if (props.isNew && idRef.value) idRef.value.focus()
  else if (labelRef.value) labelRef.value.focus()
  else closeRef.value?.focus()
})
</script>

<template>
  <DrawerSheet
    :return-focus-to="returnFocusTo"
    :ariaLabel="isNew ? 'New action' : 'Edit action'"
    testid="action-editor"
    :default-size="480"
    @close="cancel"
  >
    <template #header>
      <div class="flex items-center gap-3">
        <span class="flex size-[38px] items-center justify-center rounded-[10px] bg-accent text-accent-contrast"
          ><IconPlay class="size-[18px]"
        /></span>
        <div class="min-w-0 flex-1">
          <div class="text-[15px] font-semibold tracking-[-.01em]">{{ isNew ? 'New action' : 'Edit action' }}</div>
          <div class="truncate font-mono text-[12px] text-text-3">
            {{ isNew ? 'Create a reusable desktop action' : action.id }}
          </div>
        </div>
        <button
          ref="closeRef"
          class="text-text-3 hover:text-text disabled:opacity-50"
          aria-label="Close"
          :disabled="busy"
          @click="cancel"
        >
          <IconX class="size-4" />
        </button>
      </div>
    </template>

    <div class="grid gap-3">
      <TextField ref="idRef" v-model="action.id" label="ID" :disabled="!isNew" testid="action-id" />
      <TextField ref="labelRef" v-model="action.label" label="Label" testid="action-label" />
      <SelectField
        label="Type"
        :model-value="action.type"
        :options="typeOptions"
        testid="action-type"
        @update:model-value="setType"
      />
      <div class="grid gap-1.5" data-testid="action-targets">
        <span class="text-[12px] font-medium text-text-2">Offer on</span>
        <AppCheckbox
          v-for="option in targetOptions"
          :key="option.value"
          :model-value="hasTarget(option.value)"
          :label="option.label"
          :disabled="option.value !== 'item' && !terminalTargetsAllowed"
          :testid="`action-target-${option.value}`"
          @update:model-value="setTarget(option.value, $event)"
        />
        <p v-if="!terminalTargetsAllowed" class="text-[11.5px] text-text-3">
          A launch-session action creates a new session, so it is offered on feed items only.
        </p>
      </div>
      <template v-if="hasTarget('item')">
        <AppCheckbox
          v-model="action.showInDetail"
          label="Show manual button in detail pane"
          testid="action-show-in-detail"
        />
        <AppliesToField
          ref="appliesField"
          :model-value="action.appliesTo"
          :known-types="knownTypes"
          @update:model-value="action.appliesTo = $event"
        />
      </template>
      <template v-if="action.launch">
        <TextareaField
          v-model="action.launch.promptTemplate"
          label="Prompt template"
          :rows="4"
          monospace
          testid="action-launch-prompt"
        />
        <SelectField
          label="Session target"
          :model-value="launchTarget"
          :options="launchTargetOptions"
          testid="action-launch-target"
          @update:model-value="setLaunchTarget"
        />
        <TextField
          v-if="launchTarget === 'repository'"
          v-model="action.launch.repoTemplate"
          label="Repository template"
          testid="action-launch-repo"
        />
        <div v-if="launchTarget === 'workspace'" class="grid gap-1.5 text-[12px] font-medium text-text-2">
          <span>Agent workspace</span>
          <AppSelect
            :model-value="action.launch.workspace ?? ''"
            :options="workspaceOptions"
            searchable
            placeholder="No prompt-capable workspaces"
            aria-label="Agent workspace"
            testid="action-launch-workspace"
            :disabled="!workspaceOptions.length"
            @update:model-value="action.launch.workspace = $event"
          />
        </div>
        <template v-if="launchTarget !== 'workspace'">
          <TextField v-model="action.launch.agent" label="Agent (optional)" testid="action-launch-agent" />
          <TextareaField
            :model-value="action.launch.postHook ?? ''"
            label="Post hook"
            :rows="2"
            monospace
            testid="action-launch-post-hook"
            @update:model-value="action.launch.postHook = $event"
          />
          <TextField
            v-model="action.launch.postHookTimeout"
            label="Post hook timeout"
            placeholder="e.g. 1m"
            testid="action-launch-post-hook-timeout"
          />
        </template>
      </template>
      <template v-if="action.shell">
        <TextareaField
          v-model="action.shell.commandTemplate"
          label="Command template"
          monospace
          testid="action-shell-command"
        />
        <TextField v-model="action.shell.cwd" label="Working directory" testid="action-shell-cwd" />
        <TextField
          v-model="action.shell.timeout"
          label="Timeout"
          placeholder="e.g. 30s"
          testid="action-shell-timeout"
        />
        <TextareaField
          :model-value="envText()"
          label="Environment (KEY=value per line)"
          monospace
          testid="action-shell-env"
          @update:model-value="setEnv"
        />
      </template>
      <template v-if="action.message">
        <TextareaField
          v-model="action.message.messageTemplate"
          label="Message template"
          monospace
          testid="action-message-template"
        />
        <TextField v-model="action.message.topic" label="Topic" testid="action-message-topic" />
      </template>
      <template v-if="action.clipboard">
        <TextareaField
          v-model="action.clipboard.textTemplate"
          label="Text template"
          monospace
          testid="action-clipboard-template"
        />
      </template>
      <ActionInputsEditor :model-value="action.inputs ?? []" @update:model-value="action.inputs = $event" />
      <InlineError v-if="validationError || error" :message="validationError || error" testid="action-editor-error" />
    </div>

    <template #footer>
      <div class="flex justify-end gap-2.5">
        <BaseButton variant="secondary" size="sm" :disabled="busy" @click="cancel">Cancel</BaseButton
        ><BaseButton size="sm" :busy="busy" data-testid="action-save" @click="save">{{
          busy ? 'Saving…' : 'Save'
        }}</BaseButton>
      </div>
    </template>
  </DrawerSheet>
</template>
