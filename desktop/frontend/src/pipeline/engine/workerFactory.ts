// Production worker construction. Vite recognizes this new URL form and
// emits workerEntry.ts as a separate module-worker bundle.

import { WebWorkerTransport, type WorkerFactory, type WorkerLike } from './webWorkerTransport'

export const createPipelineWorker: WorkerFactory = (): WorkerLike => {
  return new Worker(new URL('./workerEntry.ts', import.meta.url), { type: 'module' })
}

/** The production processor transport: function nodes receive dedicated workers. */
export function createWebWorkerTransport(): WebWorkerTransport {
  return new WebWorkerTransport(createPipelineWorker, new Set(['function']))
}
