import { validateCalls, validateForge, validateTree } from '../../lib/fenceData'
import CallsFence from './CallsFence.vue'
import ForgeFence from './ForgeFence.vue'
import MermaidFence from './MermaidFence.vue'
import { registerFence } from './registry'
import TreeFence from './TreeFence.vue'

// eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- typed lint cannot resolve .vue imports in .ts
registerFence({ lang: 'mermaid', builtin: true, body: 'text', component: MermaidFence })

// eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- typed lint cannot resolve .vue imports in .ts
registerFence({ lang: 'hive-tree', builtin: true, body: 'yaml', validate: validateTree, component: TreeFence })
// eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- typed lint cannot resolve .vue imports in .ts
registerFence({ lang: 'hive-calls', builtin: true, body: 'yaml', validate: validateCalls, component: CallsFence })
// eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- typed lint cannot resolve .vue imports in .ts
registerFence({ lang: 'hive-forge', builtin: true, body: 'yaml', validate: validateForge, component: ForgeFence })
