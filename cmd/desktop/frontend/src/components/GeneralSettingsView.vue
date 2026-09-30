<script setup lang="ts">
// General settings: values that belong to no single surface. The editor
// command is the first — the Agents area, the Code tab and the Inbox all launch
// it, so it lives here rather than with whichever of them shipped first.
import { computed, onMounted } from 'vue'
import SettingsError from './settings/SettingsError.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsRow from './settings/SettingsRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import AppSelect, { type AppSelectOption } from './AppSelect.vue'
import { useEditorSettings } from '../composables/useEditorSettings'

const { command, choices, error, refresh, setCommand } = useEditorSettings()

// "None" plus the detected commands, each labelled by whether it resolves right
// now. The control is a combobox, so a command outside the catalogue is typed
// rather than listed — it does not need an option of its own to be selectable.
const editorOptions = computed<AppSelectOption[]>(() => [
  { value: '', label: 'None' },
  ...choices.value.map((choice) => ({
    value: choice.command,
    label: choice.found ? choice.title : `${choice.title} (not found)`,
  })),
])

onMounted(() => {
  void refresh()
})
</script>

<template>
  <SettingsPage testid="settings-general">
    <SettingsError v-if="error" :message="error" testid="general-error" />

    <SettingsSection
      title="Editor"
      description="The editor 'Open in editor' actions launch — an agent workspace, a repository, a session's directory."
      boxed
      testid="general-editor"
    >
      <SettingsRow
        label="Default editor"
        hint="Pick a detected launcher or type any other single-word command on your PATH. Leave it empty for none."
      >
        <AppSelect
          :model-value="command"
          :options="editorOptions"
          editable
          placeholder="None"
          aria-label="Default editor"
          testid="general-editor-command"
          class="min-w-[200px]"
          @update:model-value="setCommand"
        />
      </SettingsRow>
    </SettingsSection>
  </SettingsPage>
</template>
