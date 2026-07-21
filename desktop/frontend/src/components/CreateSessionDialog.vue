<script setup lang="ts">
import { computed, ref } from 'vue'
import IconPlay from '~icons/lucide/play'
import IconX from '~icons/lucide/x'
import type { SessionLaunchOptions } from '../../bindings/github.com/colonyops/hive/internal/desktop/pipeline/models'
import { useAutofocus } from '../composables/useAutofocus'
import { useEscapeToClose } from '../composables/useEscapeToClose'

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

function close() {
  if (!props.busy) emit('close')
}

useEscapeToClose(close)
useAutofocus(nameInput)
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-40 flex items-start justify-center bg-backdrop pt-[18vh]" @click.self="close">
      <div class="w-[460px] overflow-hidden rounded-xl border border-strong bg-pane text-text shadow-2xl" role="dialog" aria-modal="true" aria-label="Create session" data-testid="create-session-dialog">
        <header class="flex items-center gap-3 border-b border-row px-5 py-4">
          <span class="flex size-7 items-center justify-center rounded-[7px] bg-accent-tint text-accent"><IconPlay class="size-4" /></span>
          <div class="flex-1 text-[15px] font-semibold tracking-[-.01em]">{{ actionLabel }}</div>
          <button class="cursor-pointer text-text-3 hover:text-text disabled:cursor-default disabled:opacity-50" aria-label="Close" :disabled="busy" @click="close"><IconX class="size-4" /></button>
        </header>
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
        <footer class="flex gap-2.5 border-t border-row bg-raised px-5 py-3.5">
          <button class="flex-1 cursor-pointer rounded-lg bg-accent px-4 py-2.5 text-[13.5px] font-semibold text-accent-contrast hover:brightness-110 disabled:cursor-default disabled:opacity-50" :disabled="busy || !canSubmit" data-testid="create-session-submit" @click="submit">{{ busy ? 'Creating…' : 'Create session' }}</button>
          <button class="cursor-pointer rounded-lg border border-card px-4 py-2.5 text-[13.5px] text-text-2 hover:text-text disabled:cursor-default disabled:opacity-50" :disabled="busy" @click="close">Cancel</button>
        </footer>
      </div>
    </div>
  </Teleport>
</template>
