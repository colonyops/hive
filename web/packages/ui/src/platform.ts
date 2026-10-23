import { inject, type App, type InjectionKey, type Ref } from 'vue'
import type { Theme } from './lib/themes'

type LogMethod = (msg: string, fields?: object) => void

/**
 * Everything `@hive/ui` needs from the app that hosts it and cannot get from
 * the browser alone.
 */
export interface HiveUiPlatform {
  clipboard: { writeText(text: string): Promise<void> }
  log(name: string): { debug: LogMethod; warn: LogMethod; error: LogMethod }
  perf?: { start(name: string, attrs?: object): unknown; end(handle: unknown, attrs?: object): void }
  theme: Readonly<Ref<Theme>>
  toast?(msg: string): void
}

const platformKey: InjectionKey<HiveUiPlatform> = Symbol('HiveUiPlatform')

export function installHiveUi(app: App, platform: HiveUiPlatform): void {
  app.provide(platformKey, platform)
}

/** Throws outside an app that called `installHiveUi`. */
export function useHiveUiPlatform(): HiveUiPlatform {
  const platform = inject(platformKey, null)
  if (!platform) throw new Error('@hive/ui: call installHiveUi(app, platform) before using its components')
  return platform
}
