<script setup lang="ts">
// New-chat dialog for the Chats area. A chat is a workspace plus a name —
// the command and MCP servers it launches with are the workspace's
// own declaration, so there is nothing else to ask for.
import { computed, ref, useId } from 'vue'
import IconBot from '~icons/lucide/bot'
import AppSelect, { type AppSelectOption } from './ui/AppSelect.vue'
import FormField from './ui/FormField.vue'
import TextInput from './ui/TextInput.vue'
import BaseButton from './ui/BaseButton.vue'
import BaseModal from './ui/BaseModal.vue'
import { useAutofocus } from '../composables/useAutofocus'
import type { AgentWorkspace } from '../lib/agentWorkspacesClient'
import { seedRef } from '../lib/seedRef'

const props = defineProps<{
  workspaces: readonly AgentWorkspace[]
  /** The workspace the dialog opens on — the focused one, or the most recently used. */
  initialWorkspace: string
  root: string
}>()
const emit = defineEmits<{ close: []; submit: [input: { workspace: string; name: string }] }>()

// The footer sits outside the form, so the submit button claims it by id —
// which is also what makes Enter in the name field submit.
const formId = useId()
const workspace = seedRef(() => props.initialWorkspace)
const name = ref('')
const nameInput = ref<{ focus: () => void } | null>(null)
const options = computed<AppSelectOption[]>(() =>
  props.workspaces.map((ws) => ({ value: ws.dir, label: ws.name || ws.dir })),
)

function submit(): void {
  if (!workspace.value) return
  emit('submit', { workspace: workspace.value, name: name.value.trim() })
}

useAutofocus(nameInput)
</script>

<template>
  <BaseModal title="New chat" :icon="IconBot" :width="440" testid="new-chat-dialog" @close="emit('close')">
    <form :id="formId" class="flex flex-col gap-3 px-5 py-4" @submit.prevent="submit">
      <FormField v-slot="{ id }" label="Workspace">
        <AppSelect
          :id="id"
          v-model="workspace"
          :options="options"
          placeholder="No workspaces yet"
          aria-label="Workspace for the new chat"
          testid="new-chat-workspace"
          :disabled="!workspaces.length"
        />
      </FormField>
      <FormField>
        <template #label>Name <span class="text-text-4">(optional)</span></template>
        <template #default="{ id }">
          <TextInput
            :id="id"
            ref="nameInput"
            v-model="name"
            autocapitalize="off"
            autocorrect="off"
            spellcheck="false"
            placeholder="New Chat"
            data-testid="new-chat-name"
          />
        </template>
      </FormField>
      <p v-if="!workspaces.length" class="text-xs leading-relaxed text-text-3" data-testid="new-chat-no-workspaces">
        No workspaces yet. Author one under {{ root }}.
      </p>
    </form>
    <template #footer>
      <BaseButton class="flex-1" type="submit" :form="formId" :disabled="!workspace" data-testid="new-chat-submit"
        >Start chat</BaseButton
      >
      <BaseButton variant="secondary" data-testid="new-chat-cancel" @click="emit('close')">Cancel</BaseButton>
    </template>
  </BaseModal>
</template>
