<script setup lang="ts">
// New-chat dialog for the Chats area. A chat is a workspace plus a name —
// the command and MCP servers it launches with are the workspace's
// own declaration, so there is nothing else to ask for.
import { computed, ref, useId } from 'vue'
import IconBot from '~icons/lucide/bot'
import AppSelect, { type AppSelectOption } from './AppSelect.vue'
import BaseButton from './BaseButton.vue'
import BaseModal from './BaseModal.vue'
import { useAutofocus } from '../composables/useAutofocus'
import type { AgentWorkspace } from '../lib/agentWorkspacesClient'

const props = defineProps<{
  workspaces: AgentWorkspace[]
  /** The workspace the dialog opens on — the focused one, or the most recently used. */
  initialWorkspace: string
  root: string
}>()
const emit = defineEmits<{ close: []; submit: [input: { workspace: string; name: string }] }>()

// The footer sits outside the form, so the submit button claims it by id —
// which is also what makes Enter in the name field submit.
const formId = useId()
const workspace = ref(props.initialWorkspace)
const name = ref('')
const nameInput = ref<HTMLInputElement | null>(null)
const options = computed<AppSelectOption[]>(() =>
  props.workspaces.map((ws) => ({ value: ws.dir, label: ws.name || ws.dir })))

function submit(): void {
  if (!workspace.value) return
  emit('submit', { workspace: workspace.value, name: name.value.trim() })
}

useAutofocus(nameInput)
</script>

<template>
  <BaseModal
    title="New chat"
    :icon="IconBot"
    :width="440"
    testid="new-chat-dialog"
    @close="emit('close')"
  >
    <form :id="formId" class="flex flex-col gap-3 px-5 py-4" @submit.prevent="submit">
      <div class="flex flex-col gap-1.5 text-xs font-medium text-text-2">Workspace
        <AppSelect
          v-model="workspace"
          :options="options"
          placeholder="No workspaces yet"
          aria-label="Workspace for the new chat"
          testid="new-chat-workspace"
          :disabled="!workspaces.length"
        />
      </div>
      <label class="flex flex-col gap-1.5 text-xs font-medium text-text-2">Name <span class="font-normal text-text-4">(optional)</span>
        <input
          ref="nameInput"
          v-model="name"
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
          placeholder="New Chat"
          class="rounded-lg border border-strong bg-app px-3 py-2.5 text-[13px] text-text outline-none placeholder:text-text-4 focus:border-accent"
          data-testid="new-chat-name"
        >
      </label>
      <p v-if="!workspaces.length" class="text-xs leading-relaxed text-text-3" data-testid="new-chat-no-workspaces">
        No workspaces yet. Author one under {{ root }}.
      </p>
    </form>
    <template #footer>
      <BaseButton class="flex-1" type="submit" :form="formId" :disabled="!workspace" data-testid="new-chat-submit">Start chat</BaseButton>
      <BaseButton variant="secondary" data-testid="new-chat-cancel" @click="emit('close')">Cancel</BaseButton>
    </template>
  </BaseModal>
</template>
