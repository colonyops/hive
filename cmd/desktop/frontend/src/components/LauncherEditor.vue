<script setup lang="ts">
import InlineError from './ui/InlineError.vue'
import { nextTick, onMounted, ref } from 'vue'
import IconTerminal from '~icons/lucide/terminal'
import BaseButton from './ui/BaseButton.vue'
import DrawerSheet from './ui/DrawerSheet.vue'
import { SelectField, TextField } from '../pipeline/fields'
import { launcherIconOptions } from '../lib/launcherIcons'
import { formatCombo, useKeybindings } from '../composables/useKeybindings'
import { launcherCommandID } from '../keybindings/catalog'
import type { Launcher } from '../composables/useActionsSettings'
import Kbd from './ui/Kbd.vue'

const props = defineProps<{
  isNew: boolean
  busy?: boolean
  error?: string | null
  returnFocusTo?: HTMLElement | null
}>()
const launcher = defineModel<Launcher>('launcher', { required: true })
const emit = defineEmits<{ save: []; cancel: [] }>()
const idRef = ref<{ focus: () => void } | null>(null)
const labelRef = ref<{ focus: () => void } | null>(null)
const validationError = ref<string | null>(null)

const kb = useKeybindings()
const iconOptions = [
  { value: '', label: 'Terminal (default)' },
  ...launcherIconOptions.map((o) => ({ value: o.value, label: o.label })),
]

// The chord, if there is one. A launcher is unbound until someone binds it, so
// this is the pointer to where that is done rather than a second place to do it.
function shortcut(): string {
  return formatCombo(kb.bindings.value[launcherCommandID(launcher.value.id)]?.[0] ?? '')
}

function save(): void {
  if (!launcher.value.id.trim() || !launcher.value.label.trim() || !launcher.value.command.trim()) {
    validationError.value = 'ID, label and command are required.'
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
  else labelRef.value?.focus()
})
</script>

<template>
  <DrawerSheet
    :return-focus-to="returnFocusTo"
    :title="isNew ? 'New quick terminal' : 'Edit quick terminal'"
    :subtitle="isNew ? 'Open the pop-up terminal into a program' : launcher.id"
    :icon="IconTerminal"
    header-size="lg"
    closable
    :close-disabled="busy"
    testid="launcher-editor"
    :default-size="480"
    @close="cancel"
  >
    <div class="grid gap-3">
      <TextField ref="idRef" v-model="launcher.id" label="ID" :disabled="!isNew" testid="launcher-id" />
      <TextField ref="labelRef" v-model="launcher.label" label="Label" testid="launcher-label" />
      <TextField v-model="launcher.command" label="Command" monospace testid="launcher-command" />
      <TextField
        v-model="launcher.cwd"
        label="Working directory (optional)"
        placeholder="the terminal you are looking at"
        testid="launcher-cwd"
      />
      <SelectField
        label="Icon"
        :model-value="launcher.icon ?? ''"
        :options="iconOptions"
        testid="launcher-icon"
        @update:model-value="launcher.icon = $event"
      />

      <p class="text-[11.5px] leading-relaxed text-text-3">
        Runs through a login shell, so your PATH and aliases resolve it. Leave the working directory empty to open where
        the terminal you are looking at is, wherever its prompt has been taken — it is then offered only while a
        terminal is open. Set one to reach it from anywhere.
      </p>
      <p class="text-[11.5px] leading-relaxed text-text-3" data-testid="launcher-shortcut">
        <template v-if="shortcut()"
          >Bound to <Kbd variant="boxed">{{ shortcut() }}</Kbd> — rebind it in Settings ▸ Keyboard.</template
        >
        <template v-else
          >Unbound. Give it a shortcut under <code>launcher.{{ launcher.id || 'id' }}</code> in Settings ▸
          Keyboard.</template
        >
      </p>

      <InlineError v-if="validationError || error" :message="validationError || error" testid="launcher-editor-error" />
    </div>

    <template #footer>
      <div class="flex justify-end gap-2.5">
        <BaseButton variant="secondary" size="sm" :disabled="busy" @click="cancel">Cancel</BaseButton>
        <BaseButton size="sm" :busy="busy" data-testid="launcher-save" @click="save">{{
          busy ? 'Saving…' : 'Save'
        }}</BaseButton>
      </div>
    </template>
  </DrawerSheet>
</template>
