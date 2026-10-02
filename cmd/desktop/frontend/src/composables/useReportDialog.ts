import { ref } from 'vue'
import { Browser } from '@wailsio/runtime'
import { IssueURL } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reportservice'

// Both halves of a bug report live here so the split is visible at the call
// site: reportProblem opens a public issue, the bundle dialog writes a private
// file. Do not join them.
const bundleOpen = ref(false)

export function useReportDialog() {
  return {
    open: bundleOpen,
    openBundleDialog: () => {
      bundleOpen.value = true
    },
    close: () => {
      bundleOpen.value = false
    },
    reportProblem: async () => {
      await Browser.OpenURL(await IssueURL())
    },
  }
}
