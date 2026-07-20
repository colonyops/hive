// App-registry entry (D2) — never imports runtime.ts (see runtime.ts for the
// worker-side ProcessorRuntime this pairs with, discovered by processors.ts).
import IconFilter from '~icons/lucide/filter'
import editor from './editor.vue'
import help from './help.md?raw'
import { accentToken, category, defaults, label, outputs, role, tint, type, validate } from './config'
import { defineNodeType } from '../../nodeType'

export default defineNodeType({
  type,
  label,
  category,
  role,
  glyph: IconFilter,
  accentToken,
  tint,
  defaults,
  outputs,
  validate,
  editor,
  help,
})
