import { onScopeDispose, ref } from 'vue'
import {
  EditorSettings as LoadEditorSettings,
  SetEditor,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import type { EditorChoice } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

function errText(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// The typed value is persisted this long after the last keystroke. The control
// is a combobox, so every character emits; without this each one would be a
// read-modify-write of settings.yaml.
const PERSIST_DELAY_MS = 400

// useEditorSettings drives the "Open in editor" command — a shared setting with
// consumers across the Agents area, the Code tab and the Inbox, which is why it
// lives in Settings ▸ General rather than with any one of them.
export function useEditorSettings() {
  const command = ref('')
  // Resolved by Go so a button labelling the action does not reproduce the
  // command→title catalogue here.
  const title = ref('')
  const choices = ref<EditorChoice[]>([])
  const error = ref('')

  // What the backend last accepted, so a rejected write can be rolled back to
  // it rather than to whatever the input held one keystroke ago.
  let persisted = ''
  let timer: ReturnType<typeof setTimeout> | undefined

  async function refresh(): Promise<void> {
    error.value = ''
    try {
      const settings = await LoadEditorSettings()
      command.value = settings.command
      title.value = settings.title
      persisted = settings.command
      choices.value = settings.choices ?? []
    } catch (err) {
      error.value = errText(err)
    }
  }

  async function save(value: string): Promise<void> {
    error.value = ''
    try {
      await SetEditor(value)
      persisted = value
    } catch (err) {
      command.value = persisted
      error.value = errText(err)
    }
  }

  function setCommand(value: string): void {
    command.value = value
    clearTimeout(timer)
    timer = setTimeout(() => void save(value), PERSIST_DELAY_MS)
  }

  // Leaving the pane inside the delay window must not drop the edit.
  onScopeDispose(() => {
    if (timer === undefined) return
    clearTimeout(timer)
    if (command.value !== persisted) void save(command.value)
  }, true)

  return { command, title, choices, error, refresh, setCommand }
}
