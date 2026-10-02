import { ref } from 'vue'
import {
  OnboardingSettings,
  SetOnboardingCompleted,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'

/**
 * A separate marker is required because profiles always exist and no other
 * state records that the closing agent hand-off happened.
 *
 * `completed` is null until the read lands. A failed read counts as completed
 * so the shell cannot be held behind a walk it cannot know it owes.
 */
export function useFirstRun() {
  const completed = ref<boolean | null>(null)

  async function load(): Promise<void> {
    try {
      completed.value = (await OnboardingSettings()).completed
    } catch (err) {
      console.warn('Unable to read the first-run state', err)
      completed.value = true
    }
  }

  // Flip locally first so a failed write cannot trap the user in first run.
  async function complete(): Promise<void> {
    completed.value = true
    try {
      await SetOnboardingCompleted()
    } catch (err) {
      console.warn('Unable to record that first run finished', err)
    }
  }

  return { completed, load, complete }
}
