import { computed, ref } from 'vue'
import {
  SetInstall,
  Status,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/hivecliservice'
import type { HiveCLIStatus } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { Owner, Skip } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/hivecli/models'
import { errorText } from '../lib/appError'

/**
 * The `hive` command Hive Desktop can install at ~/.local/bin/hive. Each
 * caller gets its own instance: first run and Settings never share a draft.
 *
 * `loaded` is "the read finished", success or not, so a caller waiting on it
 * is never held by a status it cannot read.
 */
export function useHiveCLI() {
  const status = ref<HiveCLIStatus | null>(null)
  const loaded = ref(false)
  const saving = ref(false)
  const error = ref('')

  const command = computed(() => status.value?.command ?? null)
  const installed = computed(() => command.value?.owner === Owner.OwnerApp)
  // The app can only manage the command in a release build it can write for;
  // an unwritable directory still shows its reason, so it counts as managed.
  const managed = computed(
    () =>
      !!command.value && ![Skip.SkipDev, Skip.SkipTranslocated, Skip.SkipUnrecognizable].includes(command.value.skip),
  )
  // A `hive` the user installed another way: shown, never replaced. When one
  // shadows ours, that is the copy a shell runs, and the version is its.
  const otherCopy = computed(() => {
    const cmd = command.value
    if (!cmd) return null
    if (cmd.shadowed) return { path: cmd.resolved, version: cmd.commandVersion }
    if (cmd.owner === Owner.OwnerForeign) return { path: cmd.installPath, version: cmd.commandVersion }
    return null
  })

  async function load(): Promise<void> {
    error.value = ''
    try {
      status.value = await Status()
    } catch (err) {
      error.value = errorText(err, 'Could not read the hive command.')
    } finally {
      loaded.value = true
    }
  }

  /** Saves the choice and applies it. Returns whether the save landed. */
  async function setInstall(enabled: boolean): Promise<boolean> {
    if (saving.value) return false
    saving.value = true
    error.value = ''
    try {
      status.value = await SetInstall(enabled)
      return true
    } catch (err) {
      error.value = errorText(err, 'Could not change the hive command.')
      return false
    } finally {
      saving.value = false
    }
  }

  return { status, command, loaded, saving, error, installed, managed, otherCopy, load, setInstall }
}
