<script setup lang="ts">
import { computed, ref } from 'vue'
import IconPlay from '~icons/lucide/play'
import BaseButton from './BaseButton.vue'
import BaseModal from './BaseModal.vue'
import type { SessionLaunchOptions } from '../../bindings/github.com/colonyops/hive/internal/desktop/pipeline/models'
import { useAutofocus } from '../composables/useAutofocus'

const props = defineProps<{ actionLabel: string; options: SessionLaunchOptions; busy: boolean; error: string | null }>()
const emit = defineEmits<{ close: []; submit: [input: { name: string; repository: string; agent?: string }] }>()

const repository = ref(props.options.defaultRepository)
const name = ref('')
const agent = ref(props.options.defaultAgent)
const validationError = ref('')
const nameInput = ref<HTMLInputElement | null>(null)
const canSubmit = computed(() => repository.value.trim() !== '' && name.value.trim() !== '')

function submit() {
  if (props.busy) return
  const repo = repository.value.trim()
  const sessionName = name.value.trim()
  if (!repo) {
    validationError.value = 'Repository is required.'
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
  emit('submit', { name: sessionName, repository: repo, ...(agent.value ? { agent: agent.value } : {}) })
}

useAutofocus(nameInput)
</script>

<template>
  <BaseModal
    :title="actionLabel"
    :icon="IconPlay"
    :width="460"
    pt="pt-[18vh]"
    :busy="busy"
    testid="create-session-dialog"
    @close="emit('close')"
  >
    <form class="flex flex-col gap-3 px-5 py-4" @submit.prevent="submit">
      <label class="flex flex-col gap-1.5 text-xs font-medium text-text-2">Repository
        <input v-model="repository" list="session-repositories" class="rounded-lg border border-strong bg-app px-3 py-2.5 text-[13px] text-text outline-none focus:border-accent" placeholder="https://github.com/owner/repository.git" data-testid="session-repository">
        <datalist id="session-repositories"><option v-for="repo in options.repositories" :key="repo.repository" :value="repo.repository">{{ repo.name }}</option></datalist>
      </label>
      <label class="flex flex-col gap-1.5 text-xs font-medium text-text-2">Session name
        <input ref="nameInput" v-model="name" class="rounded-lg border border-strong bg-app px-3 py-2.5 text-[13px] text-text outline-none focus:border-accent" placeholder="review-pr-123" data-testid="session-name">
      </label>
      <label class="flex flex-col gap-1.5 text-xs font-medium text-text-2">Agent <span class="font-normal text-text-4">(optional)</span>
        <select v-model="agent" class="rounded-lg border border-strong bg-app px-3 py-2.5 text-[13px] text-text outline-none focus:border-accent" data-testid="session-agent"><option value="">Use action default</option><option v-for="key in options.agents" :key="key" :value="key">{{ key }}</option></select>
      </label>
      <p v-if="validationError || error" class="text-xs text-severity-error" data-testid="create-session-error">{{ validationError || error }}</p>
    </form>
    <template #footer>
      <BaseButton class="flex-1" :busy="busy" :disabled="!canSubmit" data-testid="create-session-submit" @click="submit">{{ busy ? 'Creating…' : 'Create session' }}</BaseButton>
      <BaseButton variant="secondary" :busy="busy" @click="emit('close')">Cancel</BaseButton>
    </template>
  </BaseModal>
</template>
