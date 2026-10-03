<script setup lang="ts">
import InlineError from './ui/InlineError.vue'
import { computed, ref, useId } from 'vue'
import IconPlay from '~icons/lucide/play'
import ActionInputFields from './ActionInputFields.vue'
import AppSelect, { type AppSelectOption } from './ui/AppSelect.vue'
import FormField from './ui/FormField.vue'
import TextInput from './ui/TextInput.vue'
import BaseButton from './ui/BaseButton.vue'
import BaseModal from './ui/BaseModal.vue'
import RepositorySelect from './RepositorySelect.vue'
import type { InputSpec } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/actions/models'
import type { SessionLaunchOptions } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import { useAutofocus } from '../composables/useAutofocus'
import { formatCombo } from '../composables/useKeybindings'
import { useSubmitShortcut } from '../composables/useSubmitShortcut'
import { type ActionInputValues, initialActionInputs, validateActionInputs } from '../lib/actionInputs'
import { seedRef } from '../lib/seedRef'
import Kbd from './ui/Kbd.vue'

// An interactive launch-session action can also declare inputs; the two
// compose in one dialog rather than stacking two.
const props = withDefaults(
  defineProps<{
    actionLabel: string
    options: SessionLaunchOptions
    busy: boolean
    error: string | null
    inputs?: InputSpec[]
  }>(),
  { inputs: () => [] },
)
const emit = defineEmits<{
  close: []
  submit: [input: { name: string; repository?: string; workspace?: string; agent?: string; inputs: ActionInputValues }]
}>()

// The footer sits outside the form, so the submit button claims it by id —
// which is also what makes Enter in a single-line field submit.
const formId = useId()
const submitHint = formatCombo('mod+enter')
const target = ref<'repository' | 'workspace'>('repository')
const repository = seedRef(() => props.options.defaultRepository)
const workspace = seedRef(() => props.options.workspaces?.find((item) => item.supportsPrompt)?.dir || '')
const name = ref('')
const agent = seedRef(() => props.options.defaultAgent)
const inputValues = seedRef<ActionInputValues>(() => initialActionInputs(props.inputs))
const validationError = ref('')
const nameInput = ref<{ focus: () => void } | null>(null)
const canSubmit = computed(() => {
  const selectedTarget = target.value === 'repository' ? repository.value : workspace.value
  if (selectedTarget.trim() === '' || name.value.trim() === '') return false
  if (target.value === 'workspace') {
    return props.options.workspaces?.find((item) => item.dir === workspace.value)?.supportsPrompt ?? false
  }
  return true
})
// The empty value is a real choice here — it defers to whatever agent the action declares.
const agentOptions = computed(() => [
  { value: '', label: 'Use action default' },
  ...(props.options.agents ?? []).map((key) => ({ value: key, label: key })),
])
const workspaceOptions = computed<AppSelectOption[]>(() =>
  (props.options.workspaces ?? []).map((item) => ({
    value: item.dir,
    label: item.name || item.dir,
    hint: item.supportsPrompt ? item.dir : `${item.dir} · command does not accept a prompt`,
    disabled: !item.supportsPrompt,
  })),
)

function submit() {
  if (props.busy) return
  const repo = repository.value.trim()
  const selectedWorkspace = workspace.value.trim()
  const sessionName = name.value.trim()
  if (target.value === 'repository' && !repo) {
    validationError.value = 'Repository is required.'
    return
  }
  if (target.value === 'workspace' && !selectedWorkspace) {
    validationError.value = 'A prompt-capable agent workspace is required.'
    return
  }
  if (
    target.value === 'workspace' &&
    !props.options.workspaces?.find((item) => item.dir === selectedWorkspace)?.supportsPrompt
  ) {
    validationError.value = 'This agent workspace does not accept an opening prompt.'
    return
  }
  if (!sessionName) {
    validationError.value = 'Session name is required.'
    return
  }
  if (!/^[a-zA-Z0-9][a-zA-Z0-9 _.:/\-]*$/.test(sessionName)) {
    validationError.value = 'Use letters, numbers, spaces, and - _ : . /.'
    return
  }
  const inputProblem = validateActionInputs(props.inputs, inputValues.value)
  if (inputProblem) {
    validationError.value = inputProblem
    return
  }
  validationError.value = ''
  if (target.value === 'workspace') {
    emit('submit', { name: sessionName, workspace: selectedWorkspace, inputs: { ...inputValues.value } })
    return
  }
  emit('submit', {
    name: sessionName,
    repository: repo,
    ...(agent.value ? { agent: agent.value } : {}),
    inputs: { ...inputValues.value },
  })
}

useAutofocus(nameInput)
useSubmitShortcut(submit)
</script>

<template>
  <BaseModal
    :title="actionLabel"
    :icon="IconPlay"
    :width="460"
    :busy="busy"
    testid="create-session-dialog"
    @close="emit('close')"
  >
    <form :id="formId" class="flex flex-col gap-3 px-5 py-4" @submit.prevent="submit">
      <div
        class="grid grid-cols-2 gap-1 rounded-lg border border-card bg-app p-1"
        role="group"
        aria-label="Session target"
        data-testid="session-target"
      >
        <button
          type="button"
          class="rounded-md px-3 py-1.5 text-xs font-medium"
          :class="target === 'repository' ? 'bg-raised text-text shadow-sm' : 'text-text-3 hover:text-text'"
          :aria-pressed="target === 'repository'"
          data-testid="session-target-repository"
          @click="target = 'repository'"
        >
          Repository
        </button>
        <button
          type="button"
          class="rounded-md px-3 py-1.5 text-xs font-medium"
          :class="target === 'workspace' ? 'bg-raised text-text shadow-sm' : 'text-text-3 hover:text-text'"
          :aria-pressed="target === 'workspace'"
          data-testid="session-target-workspace"
          @click="target = 'workspace'"
        >
          Agent workspace
        </button>
      </div>
      <FormField v-if="target === 'repository'" v-slot="{ id }" label="Repository">
        <RepositorySelect
          :id="id"
          :model-value="repository"
          :repositories="options.repositories"
          testid="session-repository"
          @update:model-value="repository = $event"
        />
      </FormField>
      <FormField v-else v-slot="{ id }" label="Agent workspace">
        <AppSelect
          :id="id"
          v-model="workspace"
          :options="workspaceOptions"
          searchable
          placeholder="No prompt-capable workspaces"
          aria-label="Agent workspace"
          testid="session-workspace"
          :disabled="!workspaceOptions.length"
        />
      </FormField>
      <FormField v-slot="{ id }" label="Session name">
        <TextInput
          :id="id"
          ref="nameInput"
          v-model="name"
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
          placeholder="review-pr-123"
          data-testid="session-name"
        />
      </FormField>
      <FormField v-if="target === 'repository'">
        <template #label>Agent <span class="text-text-4">(optional)</span></template>
        <template #default="{ id }">
          <AppSelect
            :id="id"
            :model-value="agent"
            :options="agentOptions"
            testid="session-agent"
            aria-label="Agent"
            @update:model-value="agent = $event"
          />
        </template>
      </FormField>
      <ActionInputFields v-if="inputs.length" v-model="inputValues" :inputs="inputs" />
      <InlineError
        v-if="validationError || error"
        testid="create-session-error"
        variant="line"
        :message="validationError || error"
      />
    </form>
    <template #footer>
      <BaseButton
        class="flex-1"
        type="submit"
        :form="formId"
        :busy="busy"
        :disabled="!canSubmit"
        data-testid="create-session-submit"
      >
        {{ busy ? 'Creating…' : target === 'workspace' ? 'Start chat' : 'Create session' }}
        <Kbd v-if="!busy" variant="on-accent">{{ submitHint }}</Kbd>
      </BaseButton>
      <BaseButton variant="secondary" :busy="busy" @click="emit('close')">Cancel</BaseButton>
    </template>
  </BaseModal>
</template>
