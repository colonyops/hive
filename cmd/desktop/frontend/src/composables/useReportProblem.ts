import { ref } from 'vue'
import { Preview, Save } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reportservice'
import { OpenPath } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/systemservice'
import type { ReportInput, ReportPreview, ReportResult } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

function errText(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// The bundle half. It writes a file and stops: nothing here or in the backend
// uploads it.
export function useReportProblem() {
  const preview = ref<ReportPreview | null>(null)
  const loading = ref(false)
  const saving = ref(false)
  const error = ref('')
  const saved = ref<ReportResult | null>(null)

  async function loadPreview(): Promise<void> {
    loading.value = true
    error.value = ''
    try {
      preview.value = await Preview()
    } catch (err) {
      error.value = errText(err)
    } finally {
      loading.value = false
    }
  }

  async function save(input: ReportInput): Promise<boolean> {
    saving.value = true
    error.value = ''
    try {
      saved.value = await Save(input)
      return true
    } catch (err) {
      error.value = errText(err)
      return false
    } finally {
      saving.value = false
    }
  }

  // The reports directory is one of the app's known locations, so this needs
  // no reveal binding of its own.
  async function openFolder(): Promise<void> {
    if (!saved.value) return
    error.value = ''
    try {
      await OpenPath(saved.value.dir)
    } catch (err) {
      error.value = errText(err)
    }
  }

  return { preview, loading, saving, error, saved, loadPreview, save, openFolder }
}
