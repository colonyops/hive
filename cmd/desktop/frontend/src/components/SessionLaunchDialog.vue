<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import IconCircleAlert from '~icons/lucide/circle-alert'
import IconCode from '~icons/lucide/code'
import IconMessagesSquare from '~icons/lucide/messages-square'
import IconPlay from '~icons/lucide/play'
import IconX from '~icons/lucide/x'
import ActionInputFields from './ActionInputFields.vue'
import RepositorySelect from './RepositorySelect.vue'
import AppSelect, { type AppSelectOption } from './ui/AppSelect.vue'
import BaseButton from './ui/BaseButton.vue'
import BaseModal from './ui/BaseModal.vue'
import FormField from './ui/FormField.vue'
import InlineError from './ui/InlineError.vue'
import Kbd from './ui/Kbd.vue'
import SegmentedControl, { type SegmentedControlOption } from './ui/SegmentedControl.vue'
import TextArea from './ui/TextArea.vue'
import TextInput from './ui/TextInput.vue'
import type { InputSpec } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/actions/models'
import type {
  SessionCreateFailure,
  SessionLaunchOptions,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import { useAutofocus } from '../composables/useAutofocus'
import { formatCombo } from '../composables/useKeybindings'
import { useSubmitShortcut } from '../composables/useSubmitShortcut'
import { type ActionInputValues, initialActionInputs, validateActionInputs } from '../lib/actionInputs'
import { seedRef } from '../lib/seedRef'

type Target = 'repository' | 'workspace'

interface SessionLaunchInput {
  repository?: string
  workspace?: string
  name: string
  prompt?: string
  agent?: string
  inputs: ActionInputValues
}

const props = withDefaults(
  defineProps<{
    options: Partial<Omit<SessionLaunchOptions, 'workspaces'>> & {
      workspaces: readonly { dir: string; name: string; supportsPrompt?: boolean }[] | null
    }
    initial?: Partial<Record<'repository' | 'workspace' | 'name' | 'prompt' | 'agent', string>>
    initialTarget?: Target
    /** An action's launch renders its own prompt, so only prompt-capable workspaces qualify. */
    action?: { label: string; inputs?: InputSpec[] | null }
    workspaceOnly?: boolean
    withPrompt?: boolean
    /** Makes the name optional; an empty name submits as this. */
    defaultName?: string
    noWorkspacesHint?: string
    busy?: boolean
    error?: string | null
    /** The creation this form was handed back from. */
    failure?: SessionCreateFailure | null
    testid?: string
    /** Prefix for the field ids, where it differs from `testid`. */
    fieldTestid?: string
  }>(),
  {
    initial: () => ({}),
    initialTarget: undefined,
    action: undefined,
    defaultName: '',
    noWorkspacesHint: 'Create a workspace in Chats before starting one here.',
    error: null,
    failure: null,
    testid: 'new-session',
    fieldTestid: undefined,
  },
)
const emit = defineEmits<{ close: []; submit: [input: SessionLaunchInput]; dismissFailure: [] }>()

// The footer sits outside the form, so the submit button claims it by id,
// which is also what makes Enter in a single-line field submit.
const formId = useId()
const submitHint = formatCombo('mod+enter')
const fieldId = computed(() => props.fieldTestid ?? props.testid)
const targetOptions: SegmentedControlOption<Target>[] = [
  { value: 'repository', label: 'Code' },
  { value: 'workspace', label: 'Chats' },
]
const targetIcons = { repository: IconCode, workspace: IconMessagesSquare }
const inputs = computed(() => props.action?.inputs ?? [])

const target = seedRef<Target>(() =>
  props.workspaceOnly ? 'workspace' : (props.initialTarget ?? (props.initial.workspace ? 'workspace' : 'repository')),
)
const repository = seedRef(() => props.initial.repository || props.options.defaultRepository || '')
const workspace = seedRef(() => {
  const first =
    props.action || props.initial.prompt
      ? props.options.workspaces?.find((item) => item.supportsPrompt)
      : props.options.workspaces?.[0]
  return props.initial.workspace || first?.dir || ''
})
const name = seedRef(() => props.initial.name ?? '')
const prompt = seedRef(() => props.initial.prompt ?? '')
const agent = seedRef(() => props.initial.agent ?? '')
const inputValues = seedRef<ActionInputValues>(() => initialActionInputs(inputs.value))
const validationError = ref('')
const nameInput = ref<{ focus: () => void } | null>(null)

const needsPrompt = computed(() => !!props.action || prompt.value.trim() !== '')
const selectedWorkspace = computed(() => props.options.workspaces?.find((item) => item.dir === workspace.value))
const canSubmit = computed(() => {
  const selectedTarget = target.value === 'repository' ? repository.value : workspace.value
  if (selectedTarget.trim() === '') return false
  if (!props.defaultName && name.value.trim() === '') return false
  return target.value === 'repository' || !needsPrompt.value || !!selectedWorkspace.value?.supportsPrompt
})
const agentOptions = computed(() => [
  { value: '', label: props.action ? 'Use action default' : 'Default agent' },
  ...(props.options.agents ?? []).map((key) => ({ value: key, label: key })),
])
const workspaceOptions = computed<AppSelectOption[]>(() =>
  (props.options.workspaces ?? []).map((item) => ({
    value: item.dir,
    label: item.name || item.dir,
    hint: item.supportsPrompt === false ? `${item.dir} · opening prompts are not supported` : item.dir,
    disabled: needsPrompt.value && !item.supportsPrompt,
  })),
)

function validate(sessionName: string): string {
  if (target.value === 'repository' && !repository.value.trim()) return 'Repository is required.'
  if (target.value === 'workspace' && !workspace.value.trim()) {
    return props.action ? 'A prompt-capable agent workspace is required.' : 'Agent workspace is required.'
  }
  if (target.value === 'workspace' && needsPrompt.value && !selectedWorkspace.value?.supportsPrompt) {
    return 'This agent workspace does not accept an opening prompt.'
  }
  if (!props.defaultName) {
    if (!sessionName) return 'Session name is required.'
    if (!/^[a-zA-Z0-9][a-zA-Z0-9 _.:/-]*$/.test(sessionName)) return 'Use letters, numbers, spaces, and - _ : . /.'
  }
  return validateActionInputs(inputs.value, inputValues.value) ?? ''
}

function submit() {
  if (props.busy) return
  const sessionName = name.value.trim()
  validationError.value = validate(sessionName)
  if (validationError.value) return
  emit('submit', {
    ...(target.value === 'workspace'
      ? { workspace: workspace.value.trim() }
      : { repository: repository.value.trim(), ...(agent.value ? { agent: agent.value } : {}) }),
    name: sessionName || props.defaultName,
    ...(props.withPrompt ? { prompt: prompt.value.trim() } : {}),
    inputs: { ...inputValues.value },
  })
}

const failureHeadline = computed(() => {
  if (!props.failure) return ''
  return props.failure.step ? `${props.failure.step} ${props.failure.reason}` : props.failure.reason
})

useAutofocus(nameInput)
useSubmitShortcut(submit)
</script>

<template>
  <BaseModal
    :title="action?.label ?? (target === 'workspace' ? 'New chat' : 'New session')"
    :icon="IconPlay"
    :width="520"
    :busy="busy"
    :testid="`${testid}-dialog`"
    @close="emit('close')"
  >
    <template v-if="!workspaceOnly" #header-actions>
      <SegmentedControl
        v-model="target"
        variant="compact"
        size="sm"
        :columns="2"
        :options="targetOptions"
        aria-label="Session target"
        :testid="`${fieldId}-target`"
      >
        <template #option="{ option }">
          <component :is="targetIcons[option.value]" class="size-3" />{{ option.label }}
        </template>
      </SegmentedControl>
    </template>
    <section
      v-if="failure"
      class="mx-5 mt-4 rounded-lg border border-severity-error-border bg-severity-error-tint px-3.5 py-3"
      :data-testid="`${testid}-failure`"
    >
      <div class="flex items-start gap-2">
        <IconCircleAlert class="mt-px size-[15px] shrink-0 text-severity-error" />
        <div class="min-w-0 flex-1">
          <div class="text-small font-semibold text-severity-error">This session was not created</div>
          <p class="mt-0.5 break-words text-small leading-[1.45] text-text-2" :data-testid="`${testid}-failure-reason`">
            {{ failureHeadline }}
          </p>
          <pre
            v-if="failure.output"
            class="mt-2 max-h-28 overflow-auto whitespace-pre-wrap rounded border border-strong bg-app px-2 py-1.5 font-mono text-micro leading-[1.5] text-text-3"
            :data-testid="`${testid}-failure-output`"
            >{{ failure.output }}</pre>
          <p
            v-if="failure.leftoverCheckout && failure.destination"
            class="mt-2 text-caption leading-[1.45] text-text-3"
          >
            {{ failure.cloneStrategy === 'worktree' ? 'Worktree' : 'Checkout' }} left on disk, safe to delete:
            <span class="block break-all font-mono text-text-4">{{ failure.destination }}</span>
          </p>
        </div>
        <button
          type="button"
          class="shrink-0 cursor-pointer self-start leading-none text-text-3 hover:text-text"
          aria-label="Dismiss failure"
          :data-testid="`${testid}-failure-dismiss`"
          @click="emit('dismissFailure')"
        >
          <IconX class="size-[14px]" />
        </button>
      </div>
    </section>
    <form :id="formId" class="flex flex-col gap-3 px-5 py-4" @submit.prevent="submit">
      <FormField v-if="target === 'repository'" v-slot="{ id }" label="Repository">
        <RepositorySelect
          :id="id"
          :model-value="repository"
          :repositories="options.repositories"
          :testid="`${fieldId}-repository`"
          @update:model-value="repository = $event"
        />
      </FormField>
      <FormField
        v-else
        v-slot="{ id }"
        label="Agent workspace"
        :hint="workspaceOptions.length ? undefined : noWorkspacesHint"
        :testid="`${fieldId}-workspace`"
      >
        <AppSelect
          :id="id"
          v-model="workspace"
          :options="workspaceOptions"
          searchable
          :placeholder="action ? 'No prompt-capable workspaces' : 'No workspaces configured'"
          aria-label="Agent workspace"
          :testid="`${fieldId}-workspace`"
          :disabled="!workspaceOptions.length"
        />
      </FormField>
      <FormField>
        <template #label> Session name <span v-if="defaultName" class="text-text-4">(optional)</span> </template>
        <template #default="{ id }">
          <!-- A session name slugs into a tmux name and a directory path, so the
               webview's text substitutions must not touch what was typed. -->
          <TextInput
            :id="id"
            ref="nameInput"
            v-model="name"
            autocapitalize="off"
            autocorrect="off"
            spellcheck="false"
            :placeholder="defaultName || 'review-pr-123'"
            :data-testid="`${fieldId}-name`"
          />
        </template>
      </FormField>
      <FormField v-if="withPrompt">
        <template #label>Prompt <span class="text-text-4">(optional)</span></template>
        <template #default="{ id }">
          <TextArea
            :id="id"
            v-model="prompt"
            :rows="6"
            placeholder="Describe the task for the agent…"
            :data-testid="`${fieldId}-prompt`"
            class="leading-relaxed"
          />
        </template>
      </FormField>
      <FormField v-if="target === 'repository'">
        <template #label>Agent <span class="text-text-4">(optional)</span></template>
        <template #default="{ id }">
          <AppSelect
            :id="id"
            :model-value="agent"
            :options="agentOptions"
            :testid="`${fieldId}-agent`"
            aria-label="Agent"
            @update:model-value="agent = $event"
          />
        </template>
      </FormField>
      <ActionInputFields v-if="inputs.length" v-model="inputValues" :inputs="inputs" />
      <InlineError
        v-if="validationError || error"
        :testid="`${testid}-error`"
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
        :data-testid="`${testid}-submit`"
      >
        {{ busy ? 'Creating…' : failure ? 'Try again' : target === 'workspace' ? 'Start chat' : 'Create session' }}
        <Kbd v-if="!busy" variant="on-accent">{{ submitHint }}</Kbd>
      </BaseButton>
      <BaseButton variant="secondary" :disabled="busy" :data-testid="`${testid}-cancel`" @click="emit('close')">
        Cancel
      </BaseButton>
    </template>
  </BaseModal>
</template>
