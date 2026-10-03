<script setup lang="ts">
import InlineError from './ui/InlineError.vue'
import { computed, ref, useId } from 'vue'
import IconCircleAlert from '~icons/lucide/circle-alert'
import IconCode from '~icons/lucide/code'
import IconMessagesSquare from '~icons/lucide/messages-square'
import IconPlay from '~icons/lucide/play'
import IconX from '~icons/lucide/x'
import AppSelect, { type AppSelectOption } from './ui/AppSelect.vue'
import FormField from './ui/FormField.vue'
import TextArea from './ui/TextArea.vue'
import TextInput from './ui/TextInput.vue'
import BaseButton from './ui/BaseButton.vue'
import BaseModal from './ui/BaseModal.vue'
import RepositorySelect from './RepositorySelect.vue'
import type {
  SessionCreateFailure,
  SessionLaunchOptions,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import { useAutofocus } from '../composables/useAutofocus'
import { formatCombo } from '../composables/useKeybindings'
import { useSubmitShortcut } from '../composables/useSubmitShortcut'
import { seedRef } from '../lib/seedRef'
import Kbd from './ui/Kbd.vue'

const props = defineProps<{
  options: SessionLaunchOptions
  initial: { repository: string; workspace: string; name: string; prompt: string; agent: string }
  initialTarget?: 'repository' | 'workspace'
  busy: boolean
  error: string | null
  /** The creation this form was handed back from, or null for a fresh form. */
  failure: SessionCreateFailure | null
}>()
const emit = defineEmits<{
  close: []
  submit: [input: { repository?: string; workspace?: string; name: string; prompt: string; agent?: string }]
  dismissFailure: []
}>()

// The footer sits outside the form, so the submit button claims it by id —
// which is also what makes Enter in a single-line field submit.
const formId = useId()
const submitHint = formatCombo('mod+enter')
const targetOptions = [
  { value: 'repository' as const, label: 'Code', icon: IconCode },
  { value: 'workspace' as const, label: 'Chats', icon: IconMessagesSquare },
]
const target = seedRef<'repository' | 'workspace'>(
  () => props.initialTarget ?? (props.initial.workspace ? 'workspace' : 'repository'),
)
const repository = seedRef(() => props.initial.repository || props.options.defaultRepository)
const workspace = seedRef(() => {
  const first = props.initial.prompt
    ? props.options.workspaces?.find((item) => item.supportsPrompt)
    : props.options.workspaces?.[0]
  return props.initial.workspace || first?.dir || ''
})
const name = seedRef(() => props.initial.name)
const prompt = seedRef(() => props.initial.prompt)
const agent = seedRef(() => props.initial.agent)
const validationError = ref('')
const nameInput = ref<{ focus: () => void } | null>(null)
const canSubmit = computed(() => {
  const selectedTarget = target.value === 'repository' ? repository.value : workspace.value
  if (selectedTarget.trim() === '' || name.value.trim() === '') return false
  if (target.value === 'workspace' && prompt.value.trim() !== '') {
    return props.options.workspaces?.find((item) => item.dir === workspace.value)?.supportsPrompt ?? false
  }
  return true
})
const agentOptions = computed(() => [
  { value: '', label: 'Default agent' },
  ...(props.options.agents ?? []).map((key) => ({ value: key, label: key })),
])
const workspaceOptions = computed<AppSelectOption[]>(() =>
  (props.options.workspaces ?? []).map((item) => ({
    value: item.dir,
    label: item.name || item.dir,
    hint: item.supportsPrompt ? item.dir : `${item.dir} · opening prompts are not supported`,
    disabled: prompt.value.trim() !== '' && !item.supportsPrompt,
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
    validationError.value = 'Agent workspace is required.'
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
  validationError.value = ''
  if (target.value === 'workspace') {
    emit('submit', { workspace: selectedWorkspace, name: sessionName, prompt: prompt.value.trim() })
    return
  }
  emit('submit', {
    repository: repo,
    name: sessionName,
    prompt: prompt.value.trim(),
    ...(agent.value ? { agent: agent.value } : {}),
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
    :title="target === 'workspace' ? 'New chat' : 'New session'"
    :icon="IconPlay"
    :width="520"
    :busy="busy"
    testid="new-session-dialog"
    @close="emit('close')"
  >
    <template #header-actions>
      <div
        class="grid grid-cols-2 gap-0.5 rounded-md border border-card bg-app p-0.5"
        role="group"
        aria-label="Session target"
        data-testid="new-session-target"
      >
        <button
          v-for="option in targetOptions"
          :key="option.value"
          type="button"
          class="flex items-center gap-1 rounded px-2 py-1 text-[10.5px] font-medium leading-none transition-colors"
          :class="target === option.value ? 'bg-raised text-text shadow-sm' : 'text-text-3 hover:text-text'"
          :aria-pressed="target === option.value"
          :data-testid="`new-session-target-${option.value}`"
          @click="target = option.value as 'repository' | 'workspace'"
        >
          <component :is="option.icon" class="size-3" />{{ option.label }}
        </button>
      </div>
    </template>
    <section
      v-if="failure"
      class="mx-5 mt-4 rounded-lg border border-severity-error-border bg-severity-error-tint px-3.5 py-3"
      data-testid="new-session-failure"
    >
      <div class="flex items-start gap-2">
        <IconCircleAlert class="mt-px size-[15px] shrink-0 text-severity-error" />
        <div class="min-w-0 flex-1">
          <div class="text-[12.5px] font-semibold text-severity-error">This session was not created</div>
          <p class="mt-0.5 break-words text-[12px] leading-[1.45] text-text-2" data-testid="new-session-failure-reason">
            {{ failureHeadline }}
          </p>
          <pre
            v-if="failure.output"
            class="mt-2 max-h-28 overflow-auto whitespace-pre-wrap rounded border border-strong bg-app px-2 py-1.5 font-mono text-[10.5px] leading-[1.5] text-text-3"
            data-testid="new-session-failure-output"
            >{{ failure.output }}</pre>
          <p v-if="failure.leftoverCheckout && failure.destination" class="mt-2 text-[11px] leading-[1.45] text-text-3">
            {{ failure.cloneStrategy === 'worktree' ? 'Worktree' : 'Checkout' }} left on disk, safe to delete:
            <span class="block break-all font-mono text-text-4">{{ failure.destination }}</span>
          </p>
        </div>
        <button
          type="button"
          class="shrink-0 cursor-pointer self-start leading-none text-text-3 hover:text-text"
          aria-label="Dismiss failure"
          data-testid="new-session-failure-dismiss"
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
          testid="new-session-repository"
          @update:model-value="repository = $event"
        />
      </FormField>
      <FormField
        v-else
        v-slot="{ id }"
        label="Agent workspace"
        :hint="workspaceOptions.length ? undefined : 'Create a workspace in Chats before starting one here.'"
      >
        <AppSelect
          :id="id"
          v-model="workspace"
          :options="workspaceOptions"
          searchable
          placeholder="No workspaces configured"
          aria-label="Agent workspace"
          testid="new-session-workspace"
          :disabled="!workspaceOptions.length"
        />
      </FormField>
      <FormField v-slot="{ id }" label="Session name">
        <!-- A session name slugs into a tmux name and a directory path, so the
             webview's text substitutions must not touch what was typed. -->
        <TextInput
          :id="id"
          ref="nameInput"
          v-model="name"
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
          placeholder="review-pr-123"
          data-testid="new-session-name"
        />
      </FormField>
      <FormField>
        <template #label>Prompt <span class="text-text-4">(optional)</span></template>
        <template #default="{ id }">
          <TextArea
            :id="id"
            v-model="prompt"
            :rows="6"
            placeholder="Describe the task for the agent…"
            data-testid="new-session-prompt"
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
            testid="new-session-agent"
            aria-label="Agent"
            @update:model-value="agent = $event"
          />
        </template>
      </FormField>
      <InlineError
        v-if="validationError || error"
        testid="new-session-error"
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
        data-testid="new-session-submit"
      >
        {{ busy ? 'Creating…' : failure ? 'Try again' : target === 'workspace' ? 'Start chat' : 'Create session' }}
        <Kbd v-if="!busy" variant="on-accent">{{ submitHint }}</Kbd>
      </BaseButton>
      <BaseButton variant="secondary" :disabled="busy" @click="emit('close')">Cancel</BaseButton>
    </template>
  </BaseModal>
</template>
