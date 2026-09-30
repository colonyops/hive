/// <reference types="vite/client" />
/// <reference types="unplugin-icons/types/vue3" />

interface ImportMetaEnv {
  /**
   * Git branch injected by the `mise run dev` launch task. Present only under
   * `wails3 dev`; drives the dev bar's branch label.
   */
  readonly VITE_HIVE_DEV_BRANCH?: string
}
