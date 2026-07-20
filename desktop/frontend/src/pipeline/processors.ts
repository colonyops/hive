// Worker-side registry: runtime type -> ProcessorRuntime. This module is
// imported only by the worker entry and explicit in-process test fallback;
// the app/palette registry deliberately does not import processor runtimes.

import type { ProcessorRuntime } from './engine/transport'

const modules = import.meta.glob<{ default: ProcessorRuntime }>('./nodes/*/runtime.ts', { eager: true })

export const processorRegistry: Record<string, ProcessorRuntime> = {}

for (const path in modules) {
  const runtime = modules[path]?.default
  if (!runtime || !runtime.type) {
    throw new Error(`pipeline: ${path} does not default-export a ProcessorRuntime with a "type"`)
  }
  if (processorRegistry[runtime.type]) {
    throw new Error(`pipeline: duplicate runtime type "${runtime.type}" (registered by ${path})`)
  }
  processorRegistry[runtime.type] = runtime
}
