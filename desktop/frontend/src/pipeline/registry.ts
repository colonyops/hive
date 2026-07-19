// The worker registry: runtime type -> ProcessorRuntime, discovered via a
// Vite glob over every node type's runtime.ts. This is the *worker* half of
// D2's two-registry split (the other half — an app/palette registry over
// index.ts, `import.meta.glob('./nodes/*/index.ts')` — is Phase 6's job,
// once editor.vue/help.md/index.ts exist per node type). vitest supports
// import.meta.glob directly, so this doubles as the registry both
// InProcessTransport (via tests/the fallback) and a real worker bundle
// entry (production) would load.

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
