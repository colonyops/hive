// Worker-side ProcessorRuntime for github-filter. Declarative — no author
// JS, no start/stop lifecycle. Never imports vue or any DOM global (see
// __tests__/import-hygiene.spec.ts in ../../__tests__).

import type { NodeResult, ProcessorRuntime } from '../../engine/transport'
import type { Msg } from '../../types'
import { type Config, type FilterableItem, matches } from './config'

const githubFilterRuntime: ProcessorRuntime<Config> = {
  type: 'github-filter',

  onMsg(msg: Msg, ctx): NodeResult {
    const item = (msg.Payload ?? {}) as FilterableItem
    // Port 0 = pass, port 1 = fail — leaving port 1 unwired reproduces
    // today's plain "drop on fail" filter behavior via the engine's
    // unwired-port-becomes-discard rule.
    return matches(ctx.config, item) ? [msg, null] : [null, msg]
  },
}

export default githubFilterRuntime
