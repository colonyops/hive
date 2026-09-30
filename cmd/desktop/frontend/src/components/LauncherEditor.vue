<script setup lang="ts">
import SettingsError from './settings/SettingsError.vue'
import { nextTick, onMounted, ref } from 'vue'
import IconTerminal from '~icons/lucide/terminal'
import IconX from '~icons/lucide/x'
import BaseButton from './BaseButton.vue'
import DrawerSheet from './DrawerSheet.vue'
import { SelectField, TextField } from '../pipeline/fields'
import { launcherIconOptions } from '../lib/launcherIcons'
import { formatCombo, useKeybindings } from '../composables/useKeybindings'
import { useReturnFocus } from '../composables/useReturnFocus'
import { launcherCommandID } from '../keybindings/catalog'
import type { Launcher } from '../composables/useActionsSettings'

const props = defineProps<{ launcher: Launcher; isNew: boolean; busy?: boolean; error?: string | null; returnFocusTo?: HTMLElement | null }>()
const emit = defineEmits<{ save: []; cancel: [] }>()
const idRef = ref<{ focus: () => void } | null>(null)
const labelRef = ref<{ focus: () => void } | null>(null)
const closeRef = ref<HTMLButtonElement | null>(null)
const validationError = ref<string | null>(null)

const kb = useKeybindings()
const iconOptions = [{ value: '', label: 'Terminal (default)' }, ...launcherIconOptions.map((o) => ({ value: o.value, label: o.label }))]

// The chord, if there is one. A launcher is unbound until someone binds it, so
// this is the pointer to where that is done rather than a second place to do it.
function shortcut(): string {
  return formatCombo(kb.bindings.value[launcherCommandID(props.launcher.id)]?.[0] ?? '')
}

function save(): void {
  if (!props.launcher.id.trim() || !props.launcher.label.trim() || !props.launcher.command.trim()) {
    validationError.value = 'ID, label and command are required.'
    return
  }
  validationError.value = null
  emit('save')
}
function cancel(): void { if (!props.busy) emit('cancel') }

useReturnFocus(() => props.returnFocusTo)
onMounted(async () => {
  await nextTick()
  if (props.isNew && idRef.value) idRef.value.focus()
  else if (labelRef.value) labelRef.value.focus()
  else closeRef.value?.focus()
})
</script>

<template>
  <DrawerSheet
    :ariaLabel="isNew ? 'New quick terminal' : 'Edit quick terminal'"
    testid="launcher-editor"
    :default-size="480"
    @close="cancel"
  >
    <template #header>
      <div class="flex items-center gap-3">
        <span class="flex size-[38px] items-center justify-center rounded-[10px] bg-accent text-accent-contrast"><IconTerminal class="size-[18px]" /></span>
        <div class="min-w-0 flex-1">
          <div class="text-[15px] font-semibold tracking-[-.01em]">{{ isNew ? 'New quick terminal' : 'Edit quick terminal' }}</div>
          <div class="truncate font-mono text-[12px] text-text-3">{{ isNew ? 'Open the pop-up terminal into a program' : launcher.id }}</div>
        </div>
        <button ref="closeRef" class="text-text-3 hover:text-text disabled:opacity-50" aria-label="Close" :disabled="busy" @click="cancel"><IconX class="size-4" /></button>
      </div>
    </template>

    <div class="grid gap-3">
      <TextField ref="idRef" v-model="launcher.id" label="ID" :disabled="!isNew" testid="launcher-id" />
      <TextField ref="labelRef" v-model="launcher.label" label="Label" testid="launcher-label" />
      <TextField v-model="launcher.command" label="Command" monospace testid="launcher-command" />
      <TextField v-model="launcher.cwd" label="Working directory (optional)" placeholder="the terminal you are looking at" testid="launcher-cwd" />
      <SelectField label="Icon" :model-value="launcher.icon ?? ''" :options="iconOptions" testid="launcher-icon" @update:model-value="launcher.icon = $event" />

      <p class="text-[11.5px] leading-relaxed text-text-3">
        Runs through a login shell, so your PATH and aliases resolve it. Leave the
        working directory empty to open where the terminal you are looking at is,
        wherever its prompt has been taken — it is then offered only while a
        terminal is open. Set one to reach it from anywhere.
      </p>
      <p class="text-[11.5px] leading-relaxed text-text-3" data-testid="launcher-shortcut">
        <template v-if="shortcut()">Bound to <kbd class="rounded border border-card px-1 py-0.5 font-mono">{{ shortcut() }}</kbd> — rebind it in Settings ▸ Keyboard.</template>
        <template v-else>Unbound. Give it a shortcut under <code>launcher.{{ launcher.id || 'id' }}</code> in Settings ▸ Keyboard.</template>
      </p>

      <SettingsError v-if="validationError || error" :message="validationError || error" testid="launcher-editor-error" />
    </div>

    <template #footer>
      <div class="flex justify-end gap-2.5">
        <BaseButton variant="secondary" size="sm" :busy="busy" @click="cancel">Cancel</BaseButton>
        <BaseButton size="sm" :busy="busy" data-testid="launcher-save" @click="save">{{ busy ? 'Saving…' : 'Save' }}</BaseButton>
      </div>
    </template>
  </DrawerSheet>
</template>
