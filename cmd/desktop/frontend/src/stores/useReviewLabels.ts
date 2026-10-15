import { computed, readonly, ref } from 'vue'
import {
  ReviewLabels,
  SetReviewLabels,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import { errorText } from '../lib/appError'
import { defaultReviewLabelSettings, resolveReviewLabels, type ReviewLabelSetting } from '../lib/reviewLabels'
import { defineStore } from './defineStore'
import { useResource } from './useResource'

// The preset comments settings.yaml configures, shared by the composer that
// applies them, the cards that show their icons, and Settings ▸ Drive that
// edits them. A hand edit to the file shows up the next time the composer
// opens, which reloads.
export const useReviewLabels = defineStore('reviewLabels', () => {
  const configured = useResource<readonly ReviewLabelSetting[]>(
    async () => {
      const labels = (await ReviewLabels()) ?? []
      return labels.length ? labels : defaultReviewLabelSettings
    },
    { initial: defaultReviewLabelSettings, errorFallback: 'Could not load the comment labels.' },
  )
  const saveError = ref<string | null>(null)
  void configured.reload()

  // Saves and then re-reads, so what the composer offers is what Go kept.
  async function save(labels: readonly ReviewLabelSetting[]): Promise<boolean> {
    saveError.value = null
    try {
      await SetReviewLabels(labels.map((label) => ({ title: label.title, icon: label.icon })))
    } catch (error) {
      saveError.value = errorText(error, 'Could not save the comment labels.')
      return false
    }
    await configured.reload()
    return true
  }

  return {
    settings: readonly(configured.data),
    labels: computed(() => resolveReviewLabels(configured.data.value)),
    error: computed(() => saveError.value ?? configured.error.value),
    reload: configured.reload,
    save,
  }
})
