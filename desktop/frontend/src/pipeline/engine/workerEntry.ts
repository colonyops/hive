// Entry point bundled by Vite for pipeline processor workers. Keep this
// separate from the app registry: importing processors.ts here ensures node
// runtime code is loaded only in a worker in production.

import { processorRegistry } from '../processors'
import type { NodeContext, NodeResult } from './transport'
import type { Msg } from '../types'

interface RunRequest {
  kind: 'run'
  id: number
  runtimeType: string
  instanceId: string
  config: unknown
  msg: Msg
  state: Record<string, any>
}

interface WorkerScope {
  onmessage: ((event: MessageEvent<RunRequest>) => void) | null
  postMessage(data: unknown): void
}

const scope = globalThis as unknown as WorkerScope
const started = new Set<string>()

scope.onmessage = (event) => {
  void run(event.data)
}

async function run(request: RunRequest): Promise<void> {
  try {
    if (request.kind !== 'run') throw new Error(`pipeline worker: unsupported request "${request.kind}"`)
    const runtime = processorRegistry[request.runtimeType]
    if (!runtime) throw new Error(`pipeline worker: no processor runtime registered for type "${request.runtimeType}"`)

    const ctx: NodeContext = { config: request.config as Record<string, any>, state: request.state }
    if (!started.has(request.instanceId)) {
      started.add(request.instanceId)
      await runtime.start?.(ctx)
    }
    const result = await runtime.onMsg(request.msg, ctx)
    scope.postMessage({ id: request.id, result, state: ctx.state } satisfies { id: number; result: NodeResult; state: Record<string, any> })
  } catch (error) {
    scope.postMessage({
      id: request.id,
      error: error instanceof Error ? error.message : String(error),
    })
  }
}
