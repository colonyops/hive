import { errorText } from '../lib/appError'
import { ref } from 'vue'
import {
  Preview,
  Save,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reportservice'
import { OpenPath } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/systemservice'
import type {
  ReportInput,
  ReportPreview,
  ReportResult,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

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
      error.value = errorText(err, 'Could not build the report preview.')
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
      error.value = errorText(err, 'Could not save the report.')
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
      error.value = errorText(err, 'Could not open the reports folder.')
    }
  }

  return { preview, loading, saving, error, saved, loadPreview, save, openFolder }
}
