// Worker-side ProcessorRuntime for the `function` node. Never imports vue or
// any DOM global (see __tests__/import-hygiene.spec.ts in ../../__tests__)
// — this module runs inside a Web Worker in production (WebWorkerTransport)
// and in-process in tests/fallback (InProcessTransport).

import type { NodeContext, ProcessorRuntime } from '../../engine/transport'
import type { Msg } from '../../types'
import { compile, type Config } from './config'

// on_start/on_stop reuse the same three-argument compiled shape as
// on_message for a single compile() implementation; they simply have no
// msg for this call, so `node` and `state` are all they get.
function runLifecycleHook(src: string | undefined, config: Config, state: Record<string, any>): void {
  if (!src) return
  const fn = compile(src)
  fn(undefined as unknown as Msg, config as unknown as Record<string, any>, state)
}

const functionRuntime: ProcessorRuntime<Config> = {
  type: 'function',

  start(ctx: NodeContext<Config>) {
    runLifecycleHook(ctx.config.on_start, ctx.config, ctx.state)
  },

  onMsg(msg, ctx: NodeContext<Config>) {
    const fn = compile(ctx.config.on_message)
    return fn(msg, ctx.config as unknown as Record<string, any>, ctx.state)
  },

  stop(ctx: NodeContext<Config>) {
    runLifecycleHook(ctx.config.on_stop, ctx.config, ctx.state)
  },
}

export default functionRuntime
